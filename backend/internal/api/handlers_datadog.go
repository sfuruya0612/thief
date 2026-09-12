package api

import (
	"context"
	"net/http"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

func (s *Server) handleDatadogHistorical(w http.ResponseWriter, r *http.Request) {
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	s.serveCached(w, r, cacheKey("dd-historical", startMonth, endMonth, view), cacheTTL, writeDatadogCostError, func() (any, error) {
		// 認証の解決はキャッシュミス時 (実際に Datadog を呼ぶとき) だけ行う。
		// キャッシュで返せるリクエストのためにトークンを更新しても意味が無い。
		authCtx, err := s.datadogAuthContext(r.Context(), datadogParentOrg)
		if err != nil {
			return nil, err
		}
		return s.datadogCall(r.Context(), authCtx, datadogParentOrg, func(ctx context.Context) (any, error) {
			return datadog.GetHistoricalCost(ctx, s.ddV2, startMonth, endMonth, view)
		})
	})
}

func (s *Server) handleDatadogEstimated(w http.ResponseWriter, r *http.Request) {
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	s.serveCached(w, r, cacheKey("dd-estimated", startMonth, endMonth, view), cacheTTL, writeDatadogCostError, func() (any, error) {
		// 認証の解決はキャッシュミス時 (実際に Datadog を呼ぶとき) だけ行う。
		// キャッシュで返せるリクエストのためにトークンを更新しても意味が無い。
		authCtx, err := s.datadogAuthContext(r.Context(), datadogParentOrg)
		if err != nil {
			return nil, err
		}
		return s.datadogCall(r.Context(), authCtx, datadogParentOrg, func(ctx context.Context) (any, error) {
			return datadog.GetEstimatedCost(ctx, s.ddV2, startMonth, endMonth, view)
		})
	})
}
