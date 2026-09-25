package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleClientConfig は GET /api/config が Config の ObjectQueryMaxBytes を
// object_query_max_bytes として返すことを確認する。リクエストは s.mux.ServeHTTP へ流し、
// ルートパターン (メソッドとパス) の登録も含めて検証する。
func TestHandleClientConfig(t *testing.T) {
	tests := []struct {
		name     string
		maxBytes int64
	}{
		{name: "default", maxBytes: 1 << 30},
		{name: "custom", maxBytes: 8 << 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			s.cfg.ObjectQueryMaxBytes = tt.maxBytes
			s.mux = http.NewServeMux()
			s.registerRoutes()

			rec := httptest.NewRecorder()
			s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			var resp ClientConfigResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.ObjectQueryMaxBytes != tt.maxBytes {
				t.Errorf("object_query_max_bytes = %d, want %d", resp.ObjectQueryMaxBytes, tt.maxBytes)
			}
		})
	}
}

// TestHandleClientConfigMethodNotAllowed は GET 以外のメソッドが /api/config の
// ルートパターンに一致しないことを確認する。
func TestHandleClientConfigMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.registerRoutes()

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/config", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
