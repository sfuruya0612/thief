package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
)

// datadogRequiredOrgFromQuery は org クエリパラメータを必須として取り出す。
//
// ダッシュボードは組織ごとに完全に分離しており、組織を跨いだ一覧取得は行わない。
// 対象組織が指定されていないリクエストは、どの組織のダッシュボードを返すべきか決まらない
// ため、Datadog を呼ぶ前に 400 で弾く。コスト取得 (datadogOrgFromQuery) が org の省略を
// 親組織として受け付けるのとはここが異なる。
func datadogRequiredOrgFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	org, ok := datadogOrgFromQuery(w, r)
	if !ok {
		return "", false
	}
	if org == datadogParentOrg {
		writeBadRequest(w, "the org query parameter is required")
		return "", false
	}
	return org, true
}

// handleDatadogDashboards は選択中の組織のダッシュボード一覧を返す。
func (s *Server) handleDatadogDashboards(w http.ResponseWriter, r *http.Request) {
	org, ok := datadogRequiredOrgFromQuery(w, r)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("dd-dashboards", org), cacheTTL, writeDatadogError, func() (any, error) {
		// 認証の解決はキャッシュミス時 (実際に Datadog を呼ぶとき) だけ行う。
		authCtx, err := s.datadogAuthContext(r.Context(), org)
		if err != nil {
			return nil, err
		}
		v, err := s.datadogCall(r.Context(), authCtx, org, func(ctx context.Context) (any, error) {
			return ddclient.ListDashboards(ctx, s.ddDashV1)
		})
		if err != nil {
			return nil, err
		}
		dashboards := v.([]ddclient.DashboardInfo)
		for i := range dashboards {
			dashboards[i].URL = datadogDashboardURL(s.cfg.Datadog.Site, dashboards[i].URL)
		}
		return dashboards, nil
	})
}

// handleDatadogDashboard は 1 つのダッシュボードと、thief が描けるウィジェットを返す。
func (s *Server) handleDatadogDashboard(w http.ResponseWriter, r *http.Request) {
	org, ok := datadogRequiredOrgFromQuery(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeBadRequest(w, "the dashboard id is required")
		return
	}
	s.serveCached(w, r, cacheKey("dd-dashboard", org, id), cacheTTL, writeDatadogError, func() (any, error) {
		authCtx, err := s.datadogAuthContext(r.Context(), org)
		if err != nil {
			return nil, err
		}
		v, err := s.datadogCall(r.Context(), authCtx, org, func(ctx context.Context) (any, error) {
			return ddclient.GetDashboard(ctx, s.ddDashV1, id)
		})
		if err != nil {
			return nil, err
		}
		detail := v.(ddclient.DashboardDetail)
		detail.URL = datadogDashboardURL(s.cfg.Datadog.Site, detail.URL)
		return detail, nil
	})
}

// datadogDashboardURL は Datadog が返すダッシュボードの url を、ブラウザで開ける絶対
// URL にする。Datadog は "/dashboard/<id>/<slug>" の相対パスで返すため、未対応ウィジェットの
// 「Datadog で開く」リンクに使うにはサイト (app.<site>) を補う必要がある。
//
// site は起動時の設定で固定なので、組み立てた URL はキャッシュに載せてよい。
func datadogDashboardURL(site, raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		// 解析できない url はリンクにできない。空にして描画側の出し分けに委ねる。
		slog.Warn("datadog returned an unparsable dashboard url", "url", raw, "err", err)
		return ""
	}
	if u.IsAbs() {
		return raw
	}
	return (&url.URL{Scheme: "https", Host: "app." + site, Path: u.Path, RawQuery: u.RawQuery}).String()
}
