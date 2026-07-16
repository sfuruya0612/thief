package api

import (
	"net/http"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

func (s *Server) handleDatadogHistorical(w http.ResponseWriter, r *http.Request) {
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	s.serveCached(w, r, cacheKey("dd-historical", startMonth, endMonth, view), cacheTTL, writeInternalFromError, func() (any, error) {
		return datadog.GetHistoricalCost(s.ddCtx, s.ddV2, startMonth, endMonth, view)
	})
}

func (s *Server) handleDatadogEstimated(w http.ResponseWriter, r *http.Request) {
	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	view := r.URL.Query().Get("view")
	s.serveCached(w, r, cacheKey("dd-estimated", startMonth, endMonth, view), cacheTTL, writeInternalFromError, func() (any, error) {
		return datadog.GetEstimatedCost(s.ddCtx, s.ddV2, startMonth, endMonth, view)
	})
}
