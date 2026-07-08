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
	month := r.URL.Query().Get("month")
	key := cacheKey("tidb-cost", month)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return s.tidb.GetCost(month)
	})
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}
