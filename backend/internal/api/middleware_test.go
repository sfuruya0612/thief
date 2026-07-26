package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
