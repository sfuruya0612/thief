package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/config"
)

// recordedLog は recordingHandler が捕捉した 1 レコード。
type recordedLog struct {
	level slog.Level
	msg   string
	attrs map[string]string
	keys  []string
}

// recordingHandler は slog のレコードを構造のまま蓄積するテスト用ハンドラ。
type recordingHandler struct {
	mu      sync.Mutex
	records []recordedLog
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	rec := recordedLog{level: r.Level, msg: r.Message, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.String()
		rec.keys = append(rec.keys, a.Key)
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, rec)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

// captureLogs は既定の slog ロガーを記録ハンドラへ差し替える。
// 既定ロガーはプロセス全体で共有されるため、これを使うテストは t.Parallel を呼ばない。
func captureLogs(t *testing.T) *recordingHandler {
	t.Helper()
	h := &recordingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func TestLoggingMiddlewareLevels(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.Handler
		wantStatus int
		wantLevel  slog.Level
		// wantBody はログの body 属性の期待値。wantBodyAttr が false なら body 属性が
		// 無いことを検証する。
		wantBody     string
		wantBodyAttr bool
	}{
		{
			// http.ServeMux 自体が返す 404 (未登録パス) も writeError を通らず捕捉される
			name:         "mux 404 logs warn",
			handler:      http.NewServeMux(),
			wantStatus:   http.StatusNotFound,
			wantLevel:    slog.LevelWarn,
			wantBody:     "404 page not found\n",
			wantBodyAttr: true,
		},
		{
			// メソッド不一致で http.ServeMux 自体が返す 405 も捕捉される
			name: "mux 405 logs warn",
			handler: func() http.Handler {
				mux := http.NewServeMux()
				mux.HandleFunc("POST /api/nonexistent", func(http.ResponseWriter, *http.Request) {})
				return mux
			}(),
			wantStatus:   http.StatusMethodNotAllowed,
			wantLevel:    slog.LevelWarn,
			wantBody:     "Method Not Allowed\n",
			wantBodyAttr: true,
		},
		{
			name: "writeError 400 logs warn with json body",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeBadRequest(w, "name must not be empty")
			}),
			wantStatus:   http.StatusBadRequest,
			wantLevel:    slog.LevelWarn,
			wantBody:     `{"error":"name must not be empty","code":"BAD_REQUEST"}` + "\n",
			wantBodyAttr: true,
		},
		{
			// websocket.Accept のハイジャック前失敗と同じ http.Error (text/plain) 経路
			name: "http.Error 400 logs warn with plain body",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "upgrade failed", http.StatusBadRequest)
			}),
			wantStatus:   http.StatusBadRequest,
			wantLevel:    slog.LevelWarn,
			wantBody:     "upgrade failed\n",
			wantBodyAttr: true,
		},
		{
			name: "sso token expired 401 logs warn",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeUnauthorized(w, "sso token expired")
			}),
			wantStatus:   http.StatusUnauthorized,
			wantLevel:    slog.LevelWarn,
			wantBody:     `{"error":"sso token expired","code":"SSO_TOKEN_EXPIRED"}` + "\n",
			wantBodyAttr: true,
		},
		{
			name: "writeError 500 logs error",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeInternalError(w, "boom")
			}),
			wantStatus:   http.StatusInternalServerError,
			wantLevel:    slog.LevelError,
			wantBody:     `{"error":"boom","code":"INTERNAL_ERROR"}` + "\n",
			wantBodyAttr: true,
		},
		{
			name: "2xx logs info without body attr",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte("ok")); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}),
			wantStatus:   http.StatusOK,
			wantLevel:    slog.LevelInfo,
			wantBodyAttr: false,
		},
		{
			name: "3xx logs info without body attr",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "/api/elsewhere")
				w.WriteHeader(http.StatusFound)
			}),
			wantStatus:   http.StatusFound,
			wantLevel:    slog.LevelInfo,
			wantBodyAttr: false,
		},
		{
			// WriteHeader を呼ばずにボディを書く応答は 200 として扱われ捕捉対象にならない
			name: "implicit 200 logs info without body attr",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := w.Write([]byte("implicit")); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}),
			wantStatus:   http.StatusOK,
			wantLevel:    slog.LevelInfo,
			wantBodyAttr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogs(t)
			h := loggingMiddleware(tt.handler)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			// 同一リクエストに対する loggingMiddleware 由来のログは 1 行だけ
			if len(logs.records) != 1 {
				t.Fatalf("records = %d, want 1 (%+v)", len(logs.records), logs.records)
			}
			got := logs.records[0]
			if got.level != tt.wantLevel {
				t.Errorf("level = %v, want %v", got.level, tt.wantLevel)
			}
			if got.msg != "http" {
				t.Errorf("msg = %q, want %q", got.msg, "http")
			}
			for _, key := range []string{"method", "path", "status", "duration_ms"} {
				if _, ok := got.attrs[key]; !ok {
					t.Errorf("attr %q missing", key)
				}
			}
			body, hasBody := got.attrs["body"]
			if hasBody != tt.wantBodyAttr {
				t.Fatalf("body attr present = %v, want %v", hasBody, tt.wantBodyAttr)
			}
			if tt.wantBodyAttr {
				if body != tt.wantBody {
					t.Errorf("body = %q, want %q", body, tt.wantBody)
				}
				// ログのボディはクライアントへ返した応答ボディの先頭からの写しと一致する
				if !strings.HasPrefix(rec.Body.String(), body) {
					t.Errorf("logged body %q is not a prefix of response body %q", body, rec.Body.String())
				}
			}
			if !tt.wantBodyAttr {
				// 2xx / 3xx は従来どおり 4 属性のままで構成が変わらない
				want := []string{"method", "path", "status", "duration_ms"}
				if len(got.keys) != len(want) {
					t.Errorf("attr keys = %v, want %v", got.keys, want)
				}
			}
		})
	}
}

func TestLoggingMiddlewareBodyCapture(t *testing.T) {
	t.Run("truncates body over limit", func(t *testing.T) {
		logs := captureLogs(t)
		full := strings.Repeat("a", maxErrorBodyLogBytes+100)
		h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte(full)); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/big", nil))

		// クライアントへの書き込みは捕捉の上限に関わらず全量透過する
		if rec.Body.Len() != len(full) {
			t.Fatalf("response body = %d bytes, want %d", rec.Body.Len(), len(full))
		}
		if len(logs.records) != 1 {
			t.Fatalf("records = %d, want 1", len(logs.records))
		}
		if got, want := logs.records[0].attrs["body"], full[:maxErrorBodyLogBytes]; got != want {
			t.Errorf("body = %d bytes (%q...), want first %d bytes", len(got), got[:16], maxErrorBodyLogBytes)
		}
	})
	t.Run("keeps body exactly at limit without truncation", func(t *testing.T) {
		logs := captureLogs(t)
		full := strings.Repeat("b", maxErrorBodyLogBytes)
		h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte(full)); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/exact", nil))

		if len(logs.records) != 1 {
			t.Fatalf("records = %d, want 1", len(logs.records))
		}
		if got := logs.records[0].attrs["body"]; got != full {
			t.Errorf("body = %d bytes, want full %d bytes without truncation", len(got), len(full))
		}
	})
	t.Run("truncates accumulation across writes at limit", func(t *testing.T) {
		// 複数回の Write が上限を跨ぐ場合に、残り容量の再計算が呼び出しごとに
		// 正しく行われ、累積が上限で止まることを検証する
		logs := captureLogs(t)
		first := strings.Repeat("c", 1500)
		second := strings.Repeat("d", 1000)
		h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			for _, part := range []string{first, second} {
				if _, err := w.Write([]byte(part)); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/straddle", nil))

		// クライアントへの書き込みは全量透過する
		if rec.Body.Len() != len(first)+len(second) {
			t.Fatalf("response body = %d bytes, want %d", rec.Body.Len(), len(first)+len(second))
		}
		if len(logs.records) != 1 {
			t.Fatalf("records = %d, want 1", len(logs.records))
		}
		want := (first + second)[:maxErrorBodyLogBytes]
		if got := logs.records[0].attrs["body"]; got != want {
			t.Errorf("body = %d bytes, want first %d bytes of concatenation", len(got), maxErrorBodyLogBytes)
		}
	})
	t.Run("does not buffer 2xx body internally", func(t *testing.T) {
		// loggingMiddleware のログ属性からは観測できない捕捉条件 (status >= 400) を
		// responseWriter 単体で検証し、2xx でボディがバッファされない (メモリを
		// 消費しない) ことを固定する
		rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
		rw.WriteHeader(http.StatusOK)
		if _, err := rw.Write([]byte(strings.Repeat("e", 512))); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if rw.body != nil {
			t.Errorf("body = %d bytes captured, want none for 2xx", len(rw.body))
		}
	})
	t.Run("accumulates across multiple writes", func(t *testing.T) {
		logs := captureLogs(t)
		h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			for _, part := range []string{"part1 ", "part2 ", "part3"} {
				if _, err := w.Write([]byte(part)); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chunked", nil))

		if len(logs.records) != 1 {
			t.Fatalf("records = %d, want 1", len(logs.records))
		}
		got := logs.records[0]
		if got.level != slog.LevelError {
			t.Errorf("level = %v, want %v", got.level, slog.LevelError)
		}
		if want := "part1 part2 part3"; got.attrs["body"] != want {
			t.Errorf("body = %q, want %q", got.attrs["body"], want)
		}
		// 累積した捕捉もクライアントへ返した応答ボディの写しと一致する
		if got.attrs["body"] != rec.Body.String() {
			t.Errorf("logged body %q != response body %q", got.attrs["body"], rec.Body.String())
		}
	})
}

// TestCORSMiddleware は、許可リスト (THIEF_WEB_ORIGINS 相当) に無い Origin に
// CORS のヘッダを返さず、状態を変えるメソッドを 403 で拒否することを確認する。
func TestCORSMiddleware(t *testing.T) {
	t.Parallel()

	// frontend dev server (mise run frontend:run) の既定の許可オリジン。
	patterns := []string{"localhost:8088", "127.0.0.1:8088"}

	tests := []struct {
		name           string
		method         string
		origin         string
		host           string
		wantStatus     int
		wantCORS       bool
		wantNextCalled bool
		wantErrCode    string
	}{
		{
			name:           "preflight from web client",
			method:         http.MethodOptions,
			origin:         "http://localhost:8088",
			wantStatus:     http.StatusNoContent,
			wantCORS:       true,
			wantNextCalled: false,
		},
		{
			name:           "cross-origin GET passes through with CORS headers",
			method:         http.MethodGet,
			origin:         "http://localhost:8088",
			wantStatus:     http.StatusOK,
			wantCORS:       true,
			wantNextCalled: true,
		},
		{
			name:           "cross-origin DELETE passes through with CORS headers",
			method:         http.MethodDelete,
			origin:         "http://localhost:8088",
			wantStatus:     http.StatusOK,
			wantCORS:       true,
			wantNextCalled: true,
		},
		{
			name:           "cross-origin POST passes through with CORS headers",
			method:         http.MethodPost,
			origin:         "http://localhost:8088",
			wantStatus:     http.StatusOK,
			wantCORS:       true,
			wantNextCalled: true,
		},
		{
			name:           "allowed origin is matched case-insensitively",
			method:         http.MethodGet,
			origin:         "http://LOCALHOST:8088",
			wantStatus:     http.StatusOK,
			wantCORS:       true,
			wantNextCalled: true,
		},
		{
			name:           "same-host origin passes through with CORS headers",
			method:         http.MethodGet,
			origin:         "http://127.0.0.1:8089",
			host:           "127.0.0.1:8089",
			wantStatus:     http.StatusOK,
			wantCORS:       true,
			wantNextCalled: true,
		},
		{
			name:           "request without Origin gets no CORS headers",
			method:         http.MethodGet,
			wantStatus:     http.StatusOK,
			wantCORS:       false,
			wantNextCalled: true,
		},
		{
			name:           "request without Origin can POST",
			method:         http.MethodPost,
			wantStatus:     http.StatusOK,
			wantCORS:       false,
			wantNextCalled: true,
		},
		{
			name:           "disallowed origin GET passes through without CORS headers",
			method:         http.MethodGet,
			origin:         "https://evil.example",
			wantStatus:     http.StatusOK,
			wantCORS:       false,
			wantNextCalled: true,
		},
		{
			name:           "disallowed origin HEAD passes through without CORS headers",
			method:         http.MethodHead,
			origin:         "https://evil.example",
			wantStatus:     http.StatusOK,
			wantCORS:       false,
			wantNextCalled: true,
		},
		{
			name:           "disallowed origin preflight gets no CORS headers",
			method:         http.MethodOptions,
			origin:         "https://evil.example",
			wantStatus:     http.StatusNoContent,
			wantCORS:       false,
			wantNextCalled: false,
		},
		{
			name:           "disallowed origin POST is rejected",
			method:         http.MethodPost,
			origin:         "https://evil.example",
			wantStatus:     http.StatusForbidden,
			wantCORS:       false,
			wantNextCalled: false,
			wantErrCode:    "FORBIDDEN_ORIGIN",
		},
		{
			name:           "disallowed origin DELETE is rejected",
			method:         http.MethodDelete,
			origin:         "https://evil.example",
			wantStatus:     http.StatusForbidden,
			wantCORS:       false,
			wantNextCalled: false,
			wantErrCode:    "FORBIDDEN_ORIGIN",
		},
		{
			name:           "disallowed origin PUT is rejected",
			method:         http.MethodPut,
			origin:         "https://evil.example",
			wantStatus:     http.StatusForbidden,
			wantCORS:       false,
			wantNextCalled: false,
			wantErrCode:    "FORBIDDEN_ORIGIN",
		},
		{
			name:           "null origin POST is rejected",
			method:         http.MethodPost,
			origin:         "null",
			wantStatus:     http.StatusForbidden,
			wantCORS:       false,
			wantNextCalled: false,
			wantErrCode:    "FORBIDDEN_ORIGIN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nextCalled := false
			h := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			}), patterns)

			req := httptest.NewRequest(tt.method, "/api/snippets/athena/example", nil)
			if tt.host != "" {
				req.Host = tt.host
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			// Origin を持つリクエストの応答には、許可の有無にかかわらず Vary: Origin を付ける。
			wantVary := ""
			if tt.origin != "" {
				wantVary = "Origin"
			}
			if got := rec.Header().Get("Vary"); got != wantVary {
				t.Errorf("Vary = %q, want %q", got, wantVary)
			}
			gotOrigin := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.wantCORS {
				if gotOrigin != tt.origin {
					t.Errorf("Access-Control-Allow-Origin = %q, want %q", gotOrigin, tt.origin)
				}
				if got := rec.Header().Get("Access-Control-Allow-Methods"); got != corsAllowMethods {
					t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, corsAllowMethods)
				}
				if got := rec.Header().Get("Access-Control-Allow-Headers"); got != corsAllowHeaders {
					t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, corsAllowHeaders)
				}
			} else {
				if gotOrigin != "" {
					t.Errorf("Access-Control-Allow-Origin = %q, want none", gotOrigin)
				}
				if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
					t.Errorf("Access-Control-Allow-Methods = %q, want none", got)
				}
				if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "" {
					t.Errorf("Access-Control-Allow-Headers = %q, want none", got)
				}
			}
			if nextCalled != tt.wantNextCalled {
				t.Errorf("next handler called = %v, want %v", nextCalled, tt.wantNextCalled)
			}
			if tt.wantErrCode != "" {
				var resp ErrorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if resp.Code != tt.wantErrCode {
					t.Errorf("error code = %q, want %q", resp.Code, tt.wantErrCode)
				}
			}
		})
	}
}

// TestOriginAllowed は Origin の照合が websocket.Accept と同じ規則で行われることを確認する。
func TestOriginAllowed(t *testing.T) {
	t.Parallel()

	patterns := []string{"localhost:8088", "127.0.0.1:8088"}

	tests := []struct {
		name     string
		host     string
		origin   string
		patterns []string
		want     bool
	}{
		{
			name:     "pattern matches host",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088",
			patterns: patterns,
			want:     true,
		},
		{
			name:     "pattern match is case-insensitive",
			host:     "127.0.0.1:8089",
			origin:   "http://LOCALHOST:8088",
			patterns: patterns,
			want:     true,
		},
		{
			name:     "same host is allowed without a pattern",
			host:     "localhost:8088",
			origin:   "http://localhost:8088",
			patterns: patterns,
			want:     true,
		},
		{
			name:     "same host comparison is case-insensitive",
			host:     "LOCALHOST:8088",
			origin:   "http://localhost:8088",
			patterns: patterns,
			want:     true,
		},
		{
			name:     "different port does not match",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088",
			patterns: []string{"localhost:8089"},
			want:     false,
		},
		{
			name:     "wildcard pattern matches",
			host:     "127.0.0.1:8089",
			origin:   "http://192.168.1.10:8088",
			patterns: []string{"192.168.*:8088"},
			want:     true,
		},
		{
			name:     "scheme pattern matches scheme and host",
			host:     "127.0.0.1:8089",
			origin:   "https://app.example.com",
			patterns: []string{"https://app.example.com"},
			want:     true,
		},
		{
			name:     "scheme pattern does not match another scheme",
			host:     "127.0.0.1:8089",
			origin:   "http://app.example.com",
			patterns: []string{"https://app.example.com"},
			want:     false,
		},
		{
			name:     "host-only pattern matches any scheme",
			host:     "127.0.0.1:8089",
			origin:   "https://app.example.com",
			patterns: []string{"app.example.com"},
			want:     true,
		},
		{
			name:     "null origin does not match",
			host:     "127.0.0.1:8089",
			origin:   "null",
			patterns: patterns,
			want:     false,
		},
		{
			name:     "unparsable origin does not match",
			host:     "127.0.0.1:8089",
			origin:   "http://[::1",
			patterns: patterns,
			want:     false,
		},
		{
			name:     "bad pattern does not match",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088",
			patterns: []string{"["},
			want:     false,
		},
		{
			// websocket.Accept も不正なパターンに当たると 403 で拒否する。
			name:     "bad pattern denies even when another pattern matches",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088",
			patterns: []string{"[", "localhost:8088"},
			want:     false,
		},
		{
			// websocket.Accept と同じく先頭から照合するため、不正なパターンより前に一致すれば許可する。
			name:     "pattern before a bad pattern still allows",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088",
			patterns: []string{"localhost:8088", "["},
			want:     true,
		},
		{
			name:     "suffix-extended host does not match",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088.evil.example",
			patterns: patterns,
			want:     false,
		},
		{
			name:     "userinfo does not make another host match",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088@evil.example",
			patterns: patterns,
			want:     false,
		},
		{
			name:     "no patterns allow only the same host",
			host:     "127.0.0.1:8089",
			origin:   "http://localhost:8088",
			patterns: nil,
			want:     false,
		},
		{
			name:     "default web origins allow the frontend",
			host:     "127.0.0.1:8089",
			origin:   "http://127.0.0.1:8088",
			patterns: config.Defaults().WebOrigins,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			req.Host = tt.host
			req.Header.Set("Origin", tt.origin)
			if got := originAllowed(req, tt.patterns); got != tt.want {
				t.Errorf("originAllowed(host=%q, origin=%q, patterns=%q) = %v, want %v",
					tt.host, tt.origin, tt.patterns, got, tt.want)
			}
		})
	}
}

// TestHostCheckMiddleware は、許可するホストの一覧に無い Host を 421 で拒否することを確認する。
func TestHostCheckMiddleware(t *testing.T) {
	t.Parallel()

	allowedHosts := []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089"}

	tests := []struct {
		name        string
		host        string
		wantStatus  int
		wantNext    bool
		wantErrCode string
	}{
		{
			name:       "allowed localhost",
			host:       "localhost:8089",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "allowed ipv4 loopback",
			host:       "127.0.0.1:8089",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "allowed ipv6 loopback",
			host:       "[::1]:8089",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "host comparison is case-insensitive",
			host:       "LocalHost:8089",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			// DNS リバインディング: 攻撃者のドメインが Host になる。
			name:        "dns rebinding host is rejected",
			host:        "evil.example",
			wantStatus:  http.StatusMisdirectedRequest,
			wantNext:    false,
			wantErrCode: "MISDIRECTED_REQUEST",
		},
		{
			name:        "wrong port is rejected",
			host:        "localhost:9999",
			wantStatus:  http.StatusMisdirectedRequest,
			wantNext:    false,
			wantErrCode: "MISDIRECTED_REQUEST",
		},
		{
			name:        "empty host is rejected",
			host:        "",
			wantStatus:  http.StatusMisdirectedRequest,
			wantNext:    false,
			wantErrCode: "MISDIRECTED_REQUEST",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nextCalled := false
			h := hostCheckMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			}), allowedHosts)

			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			req.Host = tt.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if nextCalled != tt.wantNext {
				t.Errorf("next handler called = %v, want %v", nextCalled, tt.wantNext)
			}
			if tt.wantErrCode != "" {
				var resp ErrorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if resp.Code != tt.wantErrCode {
					t.Errorf("error code = %q, want %q", resp.Code, tt.wantErrCode)
				}
			}
		})
	}

	t.Run("empty allow list rejects every host", func(t *testing.T) {
		t.Parallel()
		h := hostCheckMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler must not be called")
		}), nil)
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		req.Host = "localhost:8089"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMisdirectedRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusMisdirectedRequest)
		}
	})
}

// TestAllowedHostsFromConfig は、Host の検査で許可するホストの一覧が cfg から
// 正しく組み立てられることを確認する。
func TestAllowedHostsFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		listenAddr   string
		redirectBase string
		want         []string
	}{
		{
			name:         "default listen addr",
			listenAddr:   "127.0.0.1:8089",
			redirectBase: "http://127.0.0.1:8089",
			want:         []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089"},
		},
		{
			name:         "custom port and oauth host",
			listenAddr:   "127.0.0.1:9000",
			redirectBase: "https://thief.example.com",
			want:         []string{"localhost:9000", "127.0.0.1:9000", "[::1]:9000", "thief.example.com"},
		},
		{
			name:         "specific bind host is kept",
			listenAddr:   "192.168.1.5:8089",
			redirectBase: "http://127.0.0.1:8089",
			want:         []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089", "192.168.1.5:8089"},
		},
		{
			name:         "unspecified ipv4 bind is not allowed as a host",
			listenAddr:   "0.0.0.0:8089",
			redirectBase: "http://127.0.0.1:8089",
			want:         []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089"},
		},
		{
			name:         "unspecified ipv6 bind is not allowed as a host",
			listenAddr:   "[::]:8089",
			redirectBase: "http://127.0.0.1:8089",
			want:         []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089"},
		},
		{
			name:         "empty bind host is not allowed as a host",
			listenAddr:   ":8089",
			redirectBase: "http://127.0.0.1:8089",
			want:         []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089"},
		},
		{
			// net/http は空の Addr を ":http" (ポート 80) として待ち受ける。ブラウザは既定ポートを
			// Host から省くため、ポートを省いた形も許可する。
			name:         "empty listen addr uses the default port and its port-less forms",
			listenAddr:   "",
			redirectBase: "http://127.0.0.1:8089",
			want:         []string{"localhost:80", "localhost", "127.0.0.1:80", "127.0.0.1", "[::1]:80", "[::1]", "127.0.0.1:8089"},
		},
		{
			// ポートの無い空でない値は net.Listen が拒否してサーバが起動しない。空と同じく扱い、
			// 待ち受けアドレス自体は加えない。
			name:         "listen addr without port is treated like the empty listen addr",
			listenAddr:   "127.0.0.1",
			redirectBase: "",
			want:         []string{"localhost:80", "localhost", "127.0.0.1:80", "127.0.0.1", "[::1]:80", "[::1]"},
		},
		{
			name:         "port 80 bind host is allowed with and without the port",
			listenAddr:   "192.168.1.5:80",
			redirectBase: "",
			want:         []string{"localhost:80", "localhost", "127.0.0.1:80", "127.0.0.1", "[::1]:80", "[::1]", "192.168.1.5:80", "192.168.1.5"},
		},
		{
			name:         "port 80 ipv6 bind host keeps the brackets without the port",
			listenAddr:   "[fe80::1]:80",
			redirectBase: "",
			want:         []string{"localhost:80", "localhost", "127.0.0.1:80", "127.0.0.1", "[::1]:80", "[::1]", "[fe80::1]:80", "[fe80::1]"},
		},
		{
			name:         "ipv6 bind host on a non-default port",
			listenAddr:   "[fe80::1]:8089",
			redirectBase: "",
			want:         []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089", "[fe80::1]:8089"},
		},
		{
			name:         "empty oauth redirect base adds nothing",
			listenAddr:   "127.0.0.1:8089",
			redirectBase: "",
			want:         []string{"localhost:8089", "127.0.0.1:8089", "[::1]:8089"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Defaults()
			cfg.ListenAddr = tt.listenAddr
			cfg.Datadog.OAuthRedirectBase = tt.redirectBase
			if got := allowedHostsFromConfig(cfg); !slices.Equal(got, tt.want) {
				t.Errorf("allowedHostsFromConfig() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHandlerAppliesOriginAndHostChecks は Handler のミドルウェアの適用順を確認する。
// Host の検査が CORS より外側にあるため、Host が許可されないリクエストは Origin が
// 同一ホストでも 421 になる (DNS リバインディングはこの経路で止まる)。
func TestHandlerAppliesOriginAndHostChecks(t *testing.T) {
	captureLogs(t)

	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.registerRoutes()
	h := s.Handler()

	t.Run("allowed origin and host reaches the mux with CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		req.Host = "127.0.0.1:8089"
		req.Header.Set("Origin", "http://localhost:8088")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8088" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:8088")
		}
	})

	t.Run("dns rebinding host is rejected even with a same-host origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		req.Host = "evil.example"
		req.Header.Set("Origin", "http://evil.example")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusMisdirectedRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMisdirectedRequest)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want none", got)
		}
	})

	t.Run("disallowed origin cannot POST", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/cache/invalidate", nil)
		req.Host = "127.0.0.1:8089"
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}

// TestNewServerHandlerAllowsLoopbackHost は、NewServer が許可するホストの一覧を組み立て、
// Handler が既定の待ち受けアドレス宛てのリクエストを通すことを確認する。newTestServer は
// allowedHosts を自前で初期化するため、NewServer の初期化はこのテストだけが検査する。
func TestNewServerHandlerAllowsLoopbackHost(t *testing.T) {
	captureLogs(t)

	cfg := config.Defaults()
	cfg.BigQuery.ProjectID = ""
	cfg.SnippetsDir = t.TempDir()
	cfg.PriceCacheDir = t.TempDir()
	s, err := NewServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	h := s.Handler()

	for _, host := range []string{"127.0.0.1:8089", "localhost:8089", "[::1]:8089"} {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Host %q: status = %d, want %d", host, rec.Code, http.StatusOK)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "evil.example:8089"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMisdirectedRequest {
		t.Errorf("Host %q: status = %d, want %d", req.Host, rec.Code, http.StatusMisdirectedRequest)
	}
}
