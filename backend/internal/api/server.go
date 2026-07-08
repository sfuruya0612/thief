package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	bqclient "github.com/sfuruya0612/thief/backend/internal/bigquery"
	"github.com/sfuruya0612/thief/backend/internal/cache"
	"github.com/sfuruya0612/thief/backend/internal/config"
	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
	tidbclient "github.com/sfuruya0612/thief/backend/internal/tidb"
)

const cacheTTL = time.Hour

// Server holds all shared state for the HTTP API server.
type Server struct {
	cfg           *config.Config
	bq            *bqclient.Client
	ddV1          *ddclient.UsageMeteringV1API
	ddV2          *ddclient.UsageMeteringV2API
	ddCtx         context.Context
	tidb          *tidbclient.Client
	resourceCache *cache.Cache[any]
	mux           *http.ServeMux
}

// NewServer initialises the API server. The BigQuery client is optional:
// if projectID is empty or ADC fails, BigQuery endpoints return 503.
func NewServer(ctx context.Context, cfg *config.Config) (*Server, error) {
	s := &Server{
		cfg:           cfg,
		resourceCache: cache.New[any](5 * time.Minute),
	}

	// BigQuery: try to initialise but don't fail server startup.
	if cfg.BigQuery.ProjectID != "" {
		bq, err := bqclient.NewClient(ctx, cfg.BigQuery.ProjectID)
		if err == nil {
			s.bq = bq
		}
		// non-fatal: BQ endpoints will return 503 if s.bq == nil
	}

	// Datadog
	ddCfg := ddclient.NewConfiguration(cfg.Datadog.Site)
	s.ddV1 = ddclient.NewUsageMeteringV1API(ddCfg)
	s.ddV2 = ddclient.NewUsageMeteringV2API(ddCfg)
	s.ddCtx = ddclient.NewContext(ctx, cfg.DatadogAPIKey(), cfg.DatadogAppKey())

	// TiDB
	s.tidb = tidbclient.NewClient(cfg.TiDB.PublicKey, cfg.TiDBPrivateKey())

	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s, nil
}

// Handler returns the HTTP handler wrapped in middleware.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = corsMiddleware(h)
	h = loggingMiddleware(h)
	return h
}

// Close releases resources held by the server.
func (s *Server) Close() {
	s.resourceCache.Close()
	if s.bq != nil {
		s.bq.Close()
	}
}

// readHeaderTimeout はリクエストヘッダ読み取りのタイムアウト。Slowloris 対策として必須。
const readHeaderTimeout = 10 * time.Second

// HTTPServer builds a ready-to-start http.Server bound to 127.0.0.1:8080.
//
// ReadTimeout/WriteTimeout はセッションブリッジ (EC2 Start Session / ECS Exec Command) の
// WebSocket 接続がハンドラ内で長時間ブロックすることと衝突するため設定しない。アイドル接続の
// 切断はアプリ層 (ブラウザ切断検知によるブリッジ終了) に委ねる。ReadHeaderTimeout のみ設定する。
func (s *Server) HTTPServer(ctx context.Context) *http.Server {
	return &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           s.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
}

// cacheKey builds a namespaced cache key to avoid collisions between services/profiles.
func cacheKey(parts ...string) string {
	key := ""
	for i, p := range parts {
		if i > 0 {
			key += ":"
		}
		key += p
	}
	return key
}

func writeCacheHeaders(w http.ResponseWriter, headers CacheHeaders) {
	w.Header().Set("X-Cache-Status", headers.Status)
	w.Header().Set("X-Cached-At", headers.CachedAt.UTC().Format(time.RFC3339))
	w.Header().Set("X-Cache-Expires-At", headers.ExpiresAt.UTC().Format(time.RFC3339))
	w.Header().Set("X-Cache-TTL-Seconds", fmt.Sprintf("%d", headers.TTL))
}
