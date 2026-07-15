package api

import (
	"net/http"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

func (s *Server) handleDatadogHistorical(w http.ResponseWriter, r *http.Request) {
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	key := cacheKey("dd-historical", startMonth, endMonth, view)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return datadog.GetHistoricalCost(s.ddCtx, s.ddV2, startMonth, endMonth, view)
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleDatadogEstimated(w http.ResponseWriter, r *http.Request) {
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	key := cacheKey("dd-estimated", startMonth, endMonth, view)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return datadog.GetEstimatedCost(s.ddCtx, s.ddV2, startMonth, endMonth, view)
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}
