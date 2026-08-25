package api

import (
	"log/slog"
	"net/http"
	"time"
)

// maxErrorBodyLogBytes は 4xx / 5xx 応答のログに載せるボディの最大バイト数。
// writeError の JSON はエラーコードとメッセージのみで通常は数百バイトに収まるため、
// 2048 バイトあれば切り詰めなしで写せる。上限はリクエストごとのバッファの上限でも
// あり、エラー応答が並行して多数発生してもメモリ使用量を上限 × 同時応答数に抑える。
const maxErrorBodyLogBytes = 2048

// loggingMiddleware logs each request's method, path, status, and duration.
// 4xx / 5xx の応答では Info のアクセスログの代わりに、捕捉した応答ボディを
// 添えて 4xx は Warn、5xx は Error で 1 行出す。
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		switch {
		case rw.status >= http.StatusInternalServerError:
			slog.Error("http", append(attrs, "body", string(rw.body))...)
		case rw.status >= http.StatusBadRequest:
			slog.Warn("http", append(attrs, "body", string(rw.body))...)
		default:
			slog.Info("http", attrs...)
		}
	})
}

// corsMiddleware adds CORS headers for localhost web clients.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
	// body は 4xx / 5xx 応答のボディの写し (先頭 maxErrorBodyLogBytes バイトまで)。
	// WriteHeader を経ずにボディが書かれた応答は status が既定の 200 のままなので
	// 捕捉対象にならない。
	body []byte
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// Write はボディをそのまま透過しつつ、4xx / 5xx の場合だけ上限まで写しを蓄積する。
// 複数回の呼び出しをまたいで累積し、上限に達した以降は捕捉だけを打ち切る。
func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status >= http.StatusBadRequest && len(rw.body) < maxErrorBodyLogBytes {
		n := maxErrorBodyLogBytes - len(rw.body)
		if n > len(b) {
			n = len(b)
		}
		rw.body = append(rw.body, b[:n]...)
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying http.ResponseWriter so that http.ResponseController
// (and libraries following the same convention, e.g. websocket.Accept's hijack lookup)
// can reach interfaces like http.Hijacker through this wrapper.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
