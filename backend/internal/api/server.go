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

// regionsCacheTTL はリージョン一覧の長期キャッシュ TTL。
// 有効化済みリージョンは頻繁に変わらないため 24 時間保持する。
const regionsCacheTTL = 24 * time.Hour

// Server holds all shared state for the HTTP API server.
type Server struct {
	cfg           *config.Config
	bq            *bqclient.Client
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

// HTTPServer builds a ready-to-start http.Server. Listen アドレスは cfg.ListenAddr
// (デフォルト 127.0.0.1:8080、環境変数 THIEF_LISTEN_ADDR で上書き可能) に従う。
//
// ReadTimeout/WriteTimeout はセッションブリッジ (EC2 Start Session / ECS Exec Command) の
// WebSocket 接続がハンドラ内で長時間ブロックすることと衝突するため設定しない。アイドル接続の
// 切断はアプリ層 (ブラウザ切断検知によるブリッジ終了) に委ねる。ReadHeaderTimeout のみ設定する。
func (s *Server) HTTPServer(ctx context.Context) *http.Server {
	return &http.Server{
		Addr:              s.cfg.ListenAddr,
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

// serveCached は resourceCache.Load の結果をキャッシュヘッダ付き JSON で書き出す。
// キャッシュ応答を返すハンドラ共通のボイラープレート (Load → エラー → ヘッダ → JSON) を集約する。
// エラー応答は onErr に委ねる。AWS リソース系は writeAWSError (SSO 期限切れで 401)、
// それ以外 (cost / gcp / datadog / tidb / bq) は writeInternalError を渡し、
// 既存のエラーレスポンス形状を変えないこと。
func (s *Server) serveCached(
	w http.ResponseWriter,
	r *http.Request,
	key string,
	ttl time.Duration,
	onErr func(http.ResponseWriter, error),
	load func() (any, error),
) {
	entry, hit, err := s.resourceCache.Load(key, ttl, s.refresh(r), load)
	if err != nil {
		onErr(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func writeCacheHeaders(w http.ResponseWriter, headers CacheHeaders) {
	w.Header().Set("X-Cache-Status", headers.Status)
	w.Header().Set("X-Cached-At", headers.CachedAt.UTC().Format(time.RFC3339))
	w.Header().Set("X-Cache-Expires-At", headers.ExpiresAt.UTC().Format(time.RFC3339))
	w.Header().Set("X-Cache-TTL-Seconds", fmt.Sprintf("%d", headers.TTL))
}
