package api

import (
	"net/http"
)

func (s *Server) handleTiDBProjects(w http.ResponseWriter, r *http.Request) {
	s.serveCached(w, r, cacheKey("tidb-projects"), cacheTTL, writeInternalFromError, func() (any, error) {
		return s.tidb.ListProjects()
	})
}

func (s *Server) handleTiDBClusters(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	s.serveCached(w, r, cacheKey("tidb-clusters", projectID), cacheTTL, writeInternalFromError, func() (any, error) {
		return s.tidb.ListClusters(projectID)
	})
}

func (s *Server) handleTiDBCost(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start := q.Get("start")
	end := q.Get("end")
	if start == "" && end == "" {
		// 後方互換: month のみ指定された単月クエリ。
		if month := q.Get("month"); month != "" {
			start, end = month, month
		}
	}

	s.serveCached(w, r, cacheKey("tidb-cost", start, end), cacheTTL, writeInternalFromError, func() (any, error) {
		return s.tidb.GetCostRange(start, end)
	})
}
