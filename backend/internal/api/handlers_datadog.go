package api

import (
	"context"
	"net/http"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

// datadogOrgFromQuery は org クエリパラメータ (Sub Organization の識別子) を取り出す。
// 省略時は親組織 (空文字) を意味する。
//
// 値はキャッシュキーと、認証情報の保存先のファイル名の組み立てに渡るため、ここで
// datadogauth.ValidateOrg を通す。検証を後段任せにすると、不正な値は「資格情報が無い」
// (401) として返り、入力の誤りだと分からない。
func datadogOrgFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	org := r.URL.Query().Get("org")
	if err := datadogauth.ValidateOrg(org); err != nil {
		writeBadRequest(w, err.Error())
		return "", false
	}
	return org, true
}

func (s *Server) handleDatadogHistorical(w http.ResponseWriter, r *http.Request) {
	org, ok := datadogOrgFromQuery(w, r)
	if !ok {
		return
	}
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	s.serveCached(w, r, cacheKey("dd-historical", org, startMonth, endMonth, view), cacheTTL, writeDatadogError, func() (any, error) {
		// 認証の解決はキャッシュミス時 (実際に Datadog を呼ぶとき) だけ行う。
		// キャッシュで返せるリクエストのためにトークンを更新しても意味が無い。
		authCtx, err := s.datadogAuthContext(r.Context(), org)
		if err != nil {
			return nil, err
		}
		return s.datadogCall(r.Context(), authCtx, org, func(ctx context.Context) (any, error) {
			return datadog.GetHistoricalCost(ctx, s.ddV2, startMonth, endMonth, view)
		})
	})
}

func (s *Server) handleDatadogEstimated(w http.ResponseWriter, r *http.Request) {
	org, ok := datadogOrgFromQuery(w, r)
	if !ok {
		return
	}
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	s.serveCached(w, r, cacheKey("dd-estimated", org, startMonth, endMonth, view), cacheTTL, writeDatadogError, func() (any, error) {
		// 認証の解決はキャッシュミス時 (実際に Datadog を呼ぶとき) だけ行う。
		// キャッシュで返せるリクエストのためにトークンを更新しても意味が無い。
		authCtx, err := s.datadogAuthContext(r.Context(), org)
		if err != nil {
			return nil, err
		}
		return s.datadogCall(r.Context(), authCtx, org, func(ctx context.Context) (any, error) {
			return datadog.GetEstimatedCost(ctx, s.ddV2, startMonth, endMonth, view)
		})
	})
}
