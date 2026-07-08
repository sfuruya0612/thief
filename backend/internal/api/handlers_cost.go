package api

import (
	"net/http"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func (s *Server) handleCost(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	includeToday := r.URL.Query().Get("include_today") == "true"
	key := cacheKey("cost", profile, region, boolStr(includeToday))
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.GetCost(r.Context(), profile, region, includeToday)
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
