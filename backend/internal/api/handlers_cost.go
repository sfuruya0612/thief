package api

import (
	"net/http"
	"strconv"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func (s *Server) handleCost(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	q := r.URL.Query()
	opts := awsinternal.CostQueryOptions{
		IncludeToday:     q.Get("include_today") == "true",
		Granularity:      q.Get("granularity"),
		GroupByDimension: q.Get("group_by"),
		ServiceFilter:    q.Get("service"),
		AccountFilter:    q.Get("account"),
		StartDate:        q.Get("start"),
		EndDate:          q.Get("end"),
	}
	if months, err := strconv.Atoi(q.Get("months")); err == nil {
		opts.Months = months
	}
	s.serveCached(w, r, costCacheKey(profile, region, opts), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.GetCost(r.Context(), profile, region, opts)
	})
}

// costCacheKey は GetCostAndUsage の結果をキャッシュするキーを組み立てる。
// 応答内容を左右する条件 (絞り込み条件を含む) はすべてキーに含める。含めない条件があると
// その条件だけが異なるリクエスト間でキャッシュが衝突し、別の条件の結果が返る。
func costCacheKey(profile, region string, opts awsinternal.CostQueryOptions) string {
	return cacheKey(
		"cost",
		profile,
		region,
		boolStr(opts.IncludeToday),
		opts.Granularity,
		opts.GroupByDimension,
		opts.ServiceFilter,
		opts.AccountFilter,
		opts.StartDate,
		opts.EndDate,
		strconv.Itoa(opts.Months),
	)
}

func (s *Server) handleCostForecast(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("cost-forecast", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.GetForecast(r.Context(), profile, region)
	})
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
