package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	bqclient "github.com/sfuruya0612/thief/backend/internal/bigquery"
	"github.com/sfuruya0612/thief/backend/internal/cache"
	"github.com/sfuruya0612/thief/backend/internal/config"
	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/snippet"
	tidbclient "github.com/sfuruya0612/thief/backend/internal/tidb"
	"golang.org/x/sync/singleflight"
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
	ddOrgV2       *ddclient.OrganizationsV2API
	ddDashV1      *ddclient.DashboardsV1API
	ddMetricsV1   *ddclient.MetricsV1API
	tidb          *tidbclient.Client
	snippets      *snippet.Store
	resourceCache *cache.Cache[any]
	mux           *http.ServeMux

	// ec2Resources は EC2 の一覧を取得する関数。テストで差し替えられるよう、関数として持つ。
	ec2Resources func(ctx context.Context, profile, region string) ([]awsinternal.EC2Resource, error)
	// ec2InstanceCountSeries は Auto Scaling グループごとの InService 台数の時系列を
	// 取得する関数。テストで実 AWS へ接続せずに差し替えられるよう、関数として持つ。
	ec2InstanceCountSeries func(ctx context.Context, profile, region string, r awsinternal.TimeseriesRange, w awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error)
	// ecsTaskCountSeries は ECS のタスク数の時系列を取得する関数。テストで実 AWS へ
	// 接続せずに差し替えられるよう、関数として持つ。
	ecsTaskCountSeries func(ctx context.Context, profile, region string, r awsinternal.TimeseriesRange, w awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error)

	// SSO デバイス認可 (start / complete エンドポイント) の進行中セッションと外部依存。
	ssoLoginSessions *ssoLoginSessionStore
	ssoLogin         ssoLoginDeps

	// Datadog の OAuth (login/start・callback・status・logout エンドポイント) の
	// 進行中セッションと外部依存。認証コンテキストはリクエストのたびに
	// datadogAuthContext がトークンファイルから組み立てるため、Server では保持しない。
	ddLoginSessions *datadogLoginSessionStore
	ddAuth          datadogAuthDeps
	// ddRefresh は同一プロセス内で重なったトークンのリフレッシュを 1 回に集約する。
	ddRefresh singleflight.Group
}

// NewServer initialises the API server. The BigQuery client is optional:
// if projectID is empty or ADC fails, BigQuery endpoints return 503.
func NewServer(ctx context.Context, cfg *config.Config) (*Server, error) {
	s := &Server{
		cfg:                    cfg,
		resourceCache:          cache.New[any](5 * time.Minute),
		ssoLoginSessions:       newSSOLoginSessionStore(),
		ssoLogin:               defaultSSOLoginDeps(),
		ddLoginSessions:        newDatadogLoginSessionStore(),
		ddAuth:                 defaultDatadogAuthDeps(),
		ec2Resources:           awsinternal.ListEC2Resources,
		ec2InstanceCountSeries: awsinternal.ListEC2InstanceCountSeries,
		ecsTaskCountSeries:     awsinternal.ListECSTaskCountSeries,
	}

	// BigQuery: try to initialise but don't fail server startup.
	if cfg.BigQuery.ProjectID != "" {
		bq, err := bqclient.NewClient(ctx, cfg.BigQuery.ProjectID)
		if err == nil {
			s.bq = bq
		}
		// non-fatal: BQ endpoints will return 503 if s.bq == nil
	}

	// Datadog. 認証は起動時に固定せず、リクエストごとに datadogAuthContext が解決する
	// (OAuth トークンの更新と CLI 側のログイン / ログアウトを稼働中に反映するため)。
	ddCfg := ddclient.NewConfiguration(cfg.Datadog.Site)
	s.ddV2 = ddclient.NewUsageMeteringV2API(ddCfg)
	s.ddOrgV2 = ddclient.NewOrganizationsV2API(ddCfg)
	s.ddDashV1 = ddclient.NewDashboardsV1API(ddCfg)
	s.ddMetricsV1 = ddclient.NewMetricsV1API(ddCfg)

	// TiDB
	s.tidb = tidbclient.NewClient(cfg.TiDB.PublicKey, cfg.TiDBPrivateKey())

	// クエリスニペット (ローカルファイル保存)
	s.snippets = snippet.NewStore(cfg.SnippetsDir)

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
// (デフォルト 127.0.0.1:8089、環境変数 THIEF_LISTEN_ADDR で上書き可能) に従う。
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
// 各要素は url.QueryEscape でエスケープしてから ":" で連結する。エスケープしないと要素の値に
// 含まれる ":" が区切りと区別できず、異なる引数の組み合わせが同一のキーへ衝突する
// (例: ("a:b", "c") と ("a", "b:c") はどちらも "a:b:c" になり、一方の結果が他方に返る)。
// 要素にはクエリパラメータやリクエストボディ由来の自由入力 (Cost Explorer の絞り込み条件、
// DynamoDB の属性値、S3/GCS のオブジェクトプレフィックス等) が入るため、エスケープは必須。
//
// QueryEscape は ":" を "%3A" に、"%" を "%25" に変換するため要素ごとに単射になり、
// 連結結果も引数列に対して単射になる。空文字はエスケープしても空文字のままなので、
// InvalidatePrefix 向けに末尾要素を空文字にして前方一致プレフィックスを組む用法は維持される。
func cacheKey(parts ...string) string {
	escaped := make([]string, len(parts))
	for i, p := range parts {
		escaped[i] = url.QueryEscape(p)
	}
	return strings.Join(escaped, ":")
}

// serveCached は resourceCache.Load の結果をキャッシュヘッダ付き JSON で書き出す。
// キャッシュ応答を返すハンドラ共通のボイラープレート (Load → エラー → ヘッダ → JSON) を集約する。
// エラー応答は onErr に委ねる。AWS リソース系と cost は writeAWSError (SSO 期限切れで 401)、
// gcp は writeGCPError、datadog のデータ取得は writeDatadogError (資格情報無しで 401
// DATADOG_NO_CREDENTIALS、それ以外は 500)、tidb / bq は writeInternalFromError を渡し、
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
