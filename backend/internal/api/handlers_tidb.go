package api

import (
	"net/http"
)

func (s *Server) handleTiDBProjects(w http.ResponseWriter, r *http.Request) {
	key := cacheKey("tidb-projects")
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return s.tidb.ListProjects()
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleTiDBClusters(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	key := cacheKey("tidb-clusters", projectID)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return s.tidb.ListClusters(projectID)
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
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

	key := cacheKey("tidb-cost", start, end)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return s.tidb.GetCostRange(start, end)
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}
