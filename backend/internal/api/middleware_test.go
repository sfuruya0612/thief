package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
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

func TestCORSMiddleware(t *testing.T) {
	t.Parallel()

	// DELETE ルート (Athena クエリ停止、BigQuery ジョブキャンセル、スニペット削除) が
	// クロスオリジンの preflight を通過できるよう、許可メソッドの値を固定する。
	const wantAllowMethods = "GET, POST, DELETE, OPTIONS"

	tests := []struct {
		name             string
		method           string
		origin           string
		wantStatus       int
		wantAllowMethods string
		wantNextCalled   bool
	}{
		{
			name:             "preflight from web client",
			method:           http.MethodOptions,
			origin:           "http://localhost:8088",
			wantStatus:       http.StatusNoContent,
			wantAllowMethods: wantAllowMethods,
			wantNextCalled:   false,
		},
		{
			name:             "cross-origin GET passes through with CORS headers",
			method:           http.MethodGet,
			origin:           "http://localhost:8088",
			wantStatus:       http.StatusOK,
			wantAllowMethods: wantAllowMethods,
			wantNextCalled:   true,
		},
		{
			name:             "cross-origin DELETE passes through with CORS headers",
			method:           http.MethodDelete,
			origin:           "http://localhost:8088",
			wantStatus:       http.StatusOK,
			wantAllowMethods: wantAllowMethods,
			wantNextCalled:   true,
		},
		{
			name:             "same-origin request gets no CORS headers",
			method:           http.MethodGet,
			origin:           "",
			wantStatus:       http.StatusOK,
			wantAllowMethods: "",
			wantNextCalled:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nextCalled := false
			h := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.method, "/api/snippets/athena/example", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Access-Control-Allow-Methods"); got != tt.wantAllowMethods {
				t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, tt.wantAllowMethods)
			}
			if tt.origin != "" {
				if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.origin {
					t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.origin)
				}
			}
			if nextCalled != tt.wantNextCalled {
				t.Errorf("next handler called = %v, want %v", nextCalled, tt.wantNextCalled)
			}
		})
	}
}
