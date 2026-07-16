package api

import (
	"encoding/json"
	"net/http"

	"github.com/sfuruya0612/thief/backend/internal/bigquery"
)

func (s *Server) handleBQDatasets(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	client, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("bq-datasets", projectID), cacheTTL, writeInternalFromError, func() (any, error) {
		return client.ListDatasets(r.Context())
	})
}

func (s *Server) handleBQTables(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	dataset := r.PathValue("dataset")
	client, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("bq-tables", projectID, dataset), cacheTTL, writeInternalFromError, func() (any, error) {
		return client.ListTables(r.Context(), dataset)
	})
}

func (s *Server) handleBQSchema(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	dataset := r.PathValue("dataset")
	table := r.PathValue("table")
	client, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("bq-schema", projectID, dataset, table), cacheTTL, writeInternalFromError, func() (any, error) {
		return client.GetTableSchema(r.Context(), dataset, table)
	})
}

func (s *Server) handleBQQuery(w http.ResponseWriter, r *http.Request) {
	var req BigQueryQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body: "+err.Error())
		return
	}
	client, ok := s.bqClientFromQuery(w, r, req.ProjectID)
	if !ok {
		return
	}
	// Queries are never cached.
	result, err := client.ExecuteQuery(r.Context(), req.SQL)
	if err != nil {
		if err == bigquery.ErrWriteNotAllowed {
			writeBadRequest(w, err.Error())
			return
		}
		writeInternalError(w, err.Error())
		return
	}
	writeJSON(w, result)
}

// bqClientFromQuery resolves a BigQuery client from the query-param project_id
// or falls back to the server-level client.
func (s *Server) bqClientFromQuery(w http.ResponseWriter, r *http.Request, projectID string) (*bigquery.Client, bool) {
	if projectID != "" {
		bq, err := bigquery.NewClient(r.Context(), projectID)
		if err != nil {
			writeInternalError(w, "bigquery client: "+err.Error())
			return nil, false
		}
		return bq, true
	}
	if s.bq != nil {
		return s.bq, true
	}
	writeError(w, http.StatusServiceUnavailable, "BQ_NOT_CONFIGURED",
		"BigQuery is not configured; provide ?project_id= or set GOOGLE_CLOUD_PROJECT")
	return nil, false
}
