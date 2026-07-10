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
		StartDate:        q.Get("start"),
		EndDate:          q.Get("end"),
	}
	if months, err := strconv.Atoi(q.Get("months")); err == nil {
		opts.Months = months
	}
	key := cacheKey("cost", profile, region, boolStr(opts.IncludeToday), opts.Granularity, opts.GroupByDimension, opts.ServiceFilter, opts.StartDate, opts.EndDate, strconv.Itoa(opts.Months))
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.GetCost(r.Context(), profile, region, opts)
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleCostForecast(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("cost-forecast", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.GetForecast(r.Context(), profile, region)
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
