package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
)

// handleDatadogMetricsQuery は 1 つのメトリクスクエリを実行して時系列を返す。
//
// ダッシュボードのウィジェットは定義 (クエリ文字列) しか持たないため、実際の値を描くには
// クエリを実行する必要がある。描画側はウィジェットのクエリごとにこのエンドポイントを呼ぶ。
func (s *Server) handleDatadogMetricsQuery(w http.ResponseWriter, r *http.Request) {
	org, ok := datadogOrgFromQuery(w, r)
	if !ok {
		return
	}
	query := r.URL.Query().Get("query")
	if query == "" {
		writeBadRequest(w, "the query parameter is required")
		return
	}
	from, to, ok := metricsWindowFromQuery(w, r)
	if !ok {
		return
	}

	// 時間窓もキャッシュキーに含める。窓が変われば別の結果になるため、同じクエリでも
	// 使い回してはならない。呼び出し側が現在時刻を丸めて渡すことで、TTL の間は同じ
	// キーになりキャッシュが効く。
	key := cacheKey("dd-metrics", org, query, strconv.FormatInt(from.Unix(), 10), strconv.FormatInt(to.Unix(), 10))
	s.serveCached(w, r, key, cacheTTL, writeDatadogError, func() (any, error) {
		// 認証の解決はキャッシュミス時 (実際に Datadog を呼ぶとき) だけ行う。
		authCtx, err := s.datadogAuthContext(r.Context(), org)
		if err != nil {
			return nil, err
		}
		return s.datadogCall(r.Context(), authCtx, org, func(ctx context.Context) (any, error) {
			return ddclient.QueryMetrics(ctx, s.ddMetricsV1, query, from, to)
		})
	})
}

// metricsWindowFromQuery は from と to (どちらも Unix 秒) を取り出す。
//
// 既定値を持たせず両方を必須にしている。時間窓を省略できると、クエリ結果が
// リクエストのたびに変わる一方でキャッシュキーは同じになり、いつの区間のデータを
// 見ているのかが呼び出し側から決められなくなるため。
func metricsWindowFromQuery(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	fromSec, ok := unixSecondsFromQuery(w, r, "from")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	toSec, ok := unixSecondsFromQuery(w, r, "to")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	if fromSec >= toSec {
		writeBadRequest(w, "the from parameter must be earlier than the to parameter")
		return time.Time{}, time.Time{}, false
	}
	return time.Unix(fromSec, 0), time.Unix(toSec, 0), true
}

// unixSecondsFromQuery は Unix 秒のクエリパラメータを取り出す。
func unixSecondsFromQuery(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		writeBadRequest(w, "the "+name+" parameter is required")
		return 0, false
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeBadRequest(w, "the "+name+" parameter must be a unix timestamp in seconds")
		return 0, false
	}
	return sec, true
}
