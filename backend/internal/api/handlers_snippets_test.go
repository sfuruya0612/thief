package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/snippet"
)

// snippetRequest は service / name をパス値に持つスニペット API リクエストを組み立てる。
func snippetRequest(t *testing.T, method, service, name string, body string) *http.Request {
	t.Helper()
	url := "/api/snippets/" + service
	if name != "" {
		url += "/" + name
	}
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	r.SetPathValue("service", service)
	if name != "" {
		r.SetPathValue("name", name)
	}
	return r
}

func TestHandleSnippetSaveValidation(t *testing.T) {
	tests := []struct {
		name     string
		service  string
		body     string
		wantCode int
	}{
		{name: "invalid json", service: "athena", body: "{", wantCode: http.StatusBadRequest},
		{name: "missing name", service: "athena", body: `{"sql":"SELECT 1"}`, wantCode: http.StatusBadRequest},
		{name: "missing sql", service: "athena", body: `{"name":"q"}`, wantCode: http.StatusBadRequest},
		{name: "blank sql", service: "athena", body: `{"name":"q","sql":"  "}`, wantCode: http.StatusBadRequest},
		{name: "traversal name", service: "athena", body: `{"name":"../evil","sql":"SELECT 1"}`, wantCode: http.StatusBadRequest},
		{name: "unknown service", service: "redshift", body: `{"name":"q","sql":"SELECT 1"}`, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			w := httptest.NewRecorder()
			s.handleSnippetSave(w, snippetRequest(t, http.MethodPost, tt.service, "", tt.body))
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body=%q)", w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

func TestHandleSnippetsRoundTrip(t *testing.T) {
	s := newTestServer(t)

	// 保存 (bigquery 側にも保存し、サービス間で混ざらないことを確認する)
	w := httptest.NewRecorder()
	s.handleSnippetSave(w, snippetRequest(t, http.MethodPost, "athena", "", `{"name":"q1","sql":"SELECT 1"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.handleSnippetSave(w, snippetRequest(t, http.MethodPost, "bigquery", "", `{"name":"q2","sql":"SELECT 2"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}

	// 一覧 (athena には q1 のみ)
	list := func(service string) []snippet.Snippet {
		t.Helper()
		w := httptest.NewRecorder()
		s.handleSnippetsList(w, snippetRequest(t, http.MethodGet, service, "", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("list status = %d, want %d", w.Code, http.StatusOK)
		}
		var items []snippet.Snippet
		if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
			t.Fatalf("unmarshal list: %v (body=%q)", err, w.Body.String())
		}
		return items
	}
	athena := list("athena")
	if len(athena) != 1 || athena[0].Name != "q1" || athena[0].SQL != "SELECT 1" {
		t.Fatalf("list(athena) = %+v, want single q1", athena)
	}
	if bigquery := list("bigquery"); len(bigquery) != 1 || bigquery[0].Name != "q2" {
		t.Fatalf("list(bigquery) = %+v, want single q2", bigquery)
	}

	// 削除
	w = httptest.NewRecorder()
	s.handleSnippetDelete(w, snippetRequest(t, http.MethodDelete, "athena", "q1", ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if items := list("athena"); len(items) != 0 {
		t.Errorf("list(athena) after delete = %+v, want empty", items)
	}
	if items := list("bigquery"); len(items) != 1 {
		t.Errorf("list(bigquery) after delete = %+v, want 1 item", items)
	}
}

func TestHandleSnippetDeleteErrors(t *testing.T) {
	tests := []struct {
		name     string
		service  string
		snippet  string
		wantCode int
		wantErr  string
	}{
		{name: "not found", service: "athena", snippet: "nope", wantCode: http.StatusNotFound, wantErr: "SNIPPET_NOT_FOUND"},
		{name: "invalid name", service: "athena", snippet: "..", wantCode: http.StatusBadRequest, wantErr: "BAD_REQUEST"},
		{name: "unknown service", service: "redshift", snippet: "q", wantCode: http.StatusBadRequest, wantErr: "BAD_REQUEST"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			w := httptest.NewRecorder()
			s.handleSnippetDelete(w, snippetRequest(t, http.MethodDelete, tt.service, tt.snippet, ""))
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if resp := decodeErrorResponse(t, w); resp.Code != tt.wantErr {
				t.Errorf("code = %q, want %q", resp.Code, tt.wantErr)
			}
		})
	}
}

func TestHandleSnippetsListUnknownService(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.handleSnippetsList(w, snippetRequest(t, http.MethodGet, "redshift", "", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
