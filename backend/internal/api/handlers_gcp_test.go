package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

// TestHandleGCPGCSObjects は /api/gcp/gcs/{bucket}/objects の delimiter / prefix の扱いと
// 応答形状を検証する。一覧の取得は Server のフィールド (gcsObjects) を差し替えて行い、
// 実 GCP へは接続しない。
func TestHandleGCPGCSObjects(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.registerRoutes()

	var gotPrefix, gotDelimiter string
	called := false
	s.gcsObjects = func(_ context.Context, projectID, bucket, prefix, delimiter string) ([]gcp.ObjectInfo, []string, bool, error) {
		if projectID != "test-project" || bucket != "my-bucket" {
			t.Errorf("unexpected args: projectID=%q bucket=%q", projectID, bucket)
		}
		called = true
		gotPrefix, gotDelimiter = prefix, delimiter
		if prefix == "empty/" {
			// フォルダだけの階層 (objects が nil)
			return nil, []string{"empty/sub/"}, false, nil
		}
		if delimiter == "/" {
			return []gcp.ObjectInfo{{Name: "logs/a.txt", Bucket: "my-bucket", Size: 1}}, []string{"logs/", "notes/"}, false, nil
		}
		return []gcp.ObjectInfo{{Name: "logs/a.txt", Bucket: "my-bucket", Size: 1}}, nil, false, nil
	}

	do := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, r)
		return w
	}

	t.Run("階層モードは prefixes 付きで返す", func(t *testing.T) {
		called, gotPrefix, gotDelimiter = false, "", ""
		w := do("/api/gcp/gcs/my-bucket/objects?project_id=test-project&prefix=logs/&delimiter=%2F")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
		}
		if gotPrefix != "logs/" || gotDelimiter != "/" {
			t.Errorf("called with prefix=%q delimiter=%q, want prefix=%q delimiter=%q",
				gotPrefix, gotDelimiter, "logs/", "/")
		}
		var body GCSObjectsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		want := GCSObjectsResponse{
			Objects:  []gcp.ObjectInfo{{Name: "logs/a.txt", Bucket: "my-bucket", Size: 1}},
			Prefixes: []string{"logs/", "notes/"},
		}
		if diff := cmp.Diff(want, body); diff != "" {
			t.Errorf("body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("フラットモードは prefixes を [] で返す", func(t *testing.T) {
		called, gotPrefix, gotDelimiter = false, "", ""
		w := do("/api/gcp/gcs/my-bucket/objects?project_id=test-project&prefix=logs/")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
		}
		if gotDelimiter != "" {
			t.Errorf("delimiter = %q, want empty", gotDelimiter)
		}
		var body GCSObjectsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if body.Prefixes == nil {
			t.Error("prefixes = null, want []")
		}
		if len(body.Prefixes) != 0 {
			t.Errorf("len(prefixes) = %d, want 0", len(body.Prefixes))
		}
		// JSON 上も null ではなく [] であること (frontend の Raw 型が非 null のため)
		if !strings.Contains(w.Body.String(), `"prefixes":[]`) {
			t.Errorf("body = %s, want \"prefixes\":[]", w.Body.String())
		}
	})

	t.Run("フォルダだけの階層は objects を [] で返す", func(t *testing.T) {
		called, gotPrefix, gotDelimiter = false, "", ""
		w := do("/api/gcp/gcs/my-bucket/objects?project_id=test-project&prefix=empty/&delimiter=%2F")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
		}
		// JSON 上で objects が null ではなく [] であること (階層モードではフォルダだけの階層が常態)
		if !strings.Contains(w.Body.String(), `"objects":[]`) {
			t.Errorf("body = %s, want \"objects\":[]", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"prefixes":["empty/sub/"]`) {
			t.Errorf("body = %s, want \"prefixes\":[\"empty/sub/\"]", w.Body.String())
		}
	})

	t.Run("delimiter の不正値は 400", func(t *testing.T) {
		called, gotPrefix, gotDelimiter = false, "", ""
		w := do("/api/gcp/gcs/my-bucket/objects?project_id=test-project&delimiter=%5C")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
		}
		var body ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if body.Code != "BAD_REQUEST" {
			t.Errorf("code = %q, want BAD_REQUEST", body.Code)
		}
		if !strings.Contains(body.Error, "delimiter") {
			t.Errorf("error = %q, want a message about delimiter", body.Error)
		}
		if called {
			t.Errorf("the list function was called with prefix=%q delimiter=%q, want no call", gotPrefix, gotDelimiter)
		}
	})
}

// TestHandleGCPGCSObjectsCacheKeyIncludesDelimiter は同じ prefix でも delimiter が違えば
// 別のキャッシュエントリになることを検証する。
func TestHandleGCPGCSObjectsCacheKeyIncludesDelimiter(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.registerRoutes()

	calls := 0
	s.gcsObjects = func(context.Context, string, string, string, string) ([]gcp.ObjectInfo, []string, bool, error) {
		calls++
		return nil, nil, false, nil
	}

	do := func(query string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet,
			"/api/gcp/gcs/my-bucket/objects?project_id=test-project"+query, nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, r)
		return w
	}

	if w := do(""); w.Code != http.StatusOK {
		t.Fatalf("flat status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if w := do("&delimiter=%2F"); w.Code != http.StatusOK {
		t.Fatalf("hierarchy status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if calls != 2 {
		t.Fatalf("loader calls = %d, want 2 (the flat and hierarchy caches must be separate)", calls)
	}
}
