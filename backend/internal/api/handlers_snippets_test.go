package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/snippet"
)

// snippetRequest は service / name をパス値に持つスニペット API リクエストを組み立てる。
// URL 上の name は frontend の encodeURIComponent と同様にパーセントエンコードし、
// パス値には http.ServeMux がデコード済みの値を渡す挙動に合わせて生の名前を設定する。
func snippetRequest(t *testing.T, method, service, name string, body string) *http.Request {
	t.Helper()
	target := "/api/snippets/" + service
	if name != "" {
		target += "/" + url.PathEscape(name)
	}
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, nil)
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
		// エンコード後のファイル名がバイト長上限を超える名前は 400 (文字種の制限は撤廃済み)。
		// 43 は snippet.maxNameLength (128、unexported のためここでは参照できない) / 3 + 1 で、
		// `/` 1 文字が %2F の 3 バイトに膨らみ 129 バイトになる最小の個数
		{name: "encoded name too long", service: "athena", body: fmt.Sprintf(`{"name":%q,"sql":"SELECT 1"}`, strings.Repeat("/", 43)), wantCode: http.StatusBadRequest},
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

// TestHandleSnippetSaveNameWithSlash はスラッシュを含む名前 (issue 0150 の再現名) の
// POST が 200 を返し、一覧に元の名前で載り、その名前の DELETE が 204 を返すことを検証する。
func TestHandleSnippetSaveNameWithSlash(t *testing.T) {
	const name = "virtual_money_issue_refund / virtual_money_use_refund（取消・返金）"
	s := newTestServer(t)

	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"name":%q,"sql":"SELECT 1"}`, name)
	s.handleSnippetSave(w, snippetRequest(t, http.MethodPost, "bigquery", "", body))
	if w.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}
	var saved snippet.Snippet
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatalf("unmarshal save response: %v (body=%q)", err, w.Body.String())
	}
	if saved.Name != name {
		t.Fatalf("saved name = %q, want %q", saved.Name, name)
	}

	// 一覧にはエンコードが露出しない元の名前で載る
	w = httptest.NewRecorder()
	s.handleSnippetsList(w, snippetRequest(t, http.MethodGet, "bigquery", "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", w.Code, http.StatusOK)
	}
	var items []snippet.Snippet
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal list: %v (body=%q)", err, w.Body.String())
	}
	if len(items) != 1 || items[0].Name != name {
		t.Fatalf("list = %+v, want single %q", items, name)
	}

	w = httptest.NewRecorder()
	s.handleSnippetDelete(w, snippetRequest(t, http.MethodDelete, "bigquery", name, ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d (body=%q)", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestHandleSnippetSaveTraversalNameAccepted は文字種の制限の撤廃により、旧仕様で
// 400 だったトラバーサル風の名前が handler 層でも保存できることを検証する
// (エンコードによりファイル名としては安全になる。Store 層の検証は
// TestStoreRoundTripUnsafeNames が担う)。
func TestHandleSnippetSaveTraversalNameAccepted(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.handleSnippetSave(w, snippetRequest(t, http.MethodPost, "athena", "", `{"name":"../evil","sql":"SELECT 1"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}
}

// TestSnippetRoutesSlashNameViaMux はスラッシュを含む名前の保存と削除が実際の
// http.ServeMux ルーティングを経由して成立することを検証する。issue 0150 の修正方針は
// 「ServeMux が %2F を含むパスセグメントを 1 セグメントとして解決し、PathValue が
// デコード済みの名前を返す」ことに依存するため、ハンドラ直呼びではなく mux を通し、
// ルートパターン (routes.go の DELETE /api/snippets/{service}/{name}) ごと検証する。
func TestSnippetRoutesSlashNameViaMux(t *testing.T) {
	const name = "virtual_money_issue_refund / virtual_money_use_refund（取消・返金）"
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.registerRoutes()

	body := fmt.Sprintf(`{"name":%q,"sql":"SELECT 1"}`, name)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/snippets/bigquery", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}

	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/snippets/bigquery/"+url.PathEscape(name), nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d (body=%q)", w.Code, http.StatusNoContent, w.Body.String())
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
		// 先頭ドットの名前は不正ではなくなり、保存されていなければ 404 になる
		{name: "dotdot not found", service: "athena", snippet: "..", wantCode: http.StatusNotFound, wantErr: "SNIPPET_NOT_FOUND"},
		{name: "empty name", service: "athena", snippet: "", wantCode: http.StatusBadRequest, wantErr: "BAD_REQUEST"},
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
