package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
)

func (s *Server) handleAthenaCatalogs(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("athena-catalogs", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListAthenaCatalogs(r.Context(), profile, region)
	})
}

func (s *Server) handleAthenaDatabases(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	catalog := r.URL.Query().Get("catalog")
	s.serveCached(w, r, cacheKey("athena-databases", profile, region, catalog), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListAthenaDatabases(r.Context(), profile, region, catalog)
	})
}

func (s *Server) handleAthenaWorkgroups(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("athena-workgroups", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListAthenaWorkgroups(r.Context(), profile, region)
	})
}

func (s *Server) handleAthenaTables(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	catalog := r.URL.Query().Get("catalog")
	database := r.URL.Query().Get("database")
	if database == "" {
		writeBadRequest(w, "database query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("athena-tables", profile, region, catalog, database), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListAthenaTables(r.Context(), profile, region, catalog, database)
	})
}

// handleAthenaQueryStart はクエリ実行を開始し実行情報を返す。
// クエリ実行系のレスポンスは決してキャッシュしない。
func (s *Server) handleAthenaQueryStart(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	var req AthenaQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body: "+err.Error())
		return
	}
	if req.SQL == "" {
		writeBadRequest(w, "sql is required")
		return
	}
	exec, err := awsinternal.StartAthenaQuery(r.Context(), profile, region, awsinternal.StartAthenaQueryInput{
		SQL:            req.SQL,
		Catalog:        req.Catalog,
		Database:       req.Database,
		Workgroup:      req.Workgroup,
		OutputLocation: req.OutputLocation,
	})
	if err != nil {
		writeAthenaError(w, err)
		return
	}
	writeJSON(w, exec)
}

// handleAthenaQueryGet は実行状態 (ポーリング用) を返す。
func (s *Server) handleAthenaQueryGet(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	id := r.PathValue("id")
	exec, err := awsinternal.GetAthenaQuery(r.Context(), profile, region, id)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, exec)
}

// handleAthenaQueryResults は完了した実行の結果を 1 ページ返す。
func (s *Server) handleAthenaQueryResults(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	id := r.PathValue("id")
	nextToken := r.URL.Query().Get("next_token")
	maxResults, _ := strconv.Atoi(r.URL.Query().Get("max"))
	page, err := awsinternal.GetAthenaQueryResults(r.Context(), profile, region, id, nextToken, maxResults)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, page)
}

// handleAthenaQueryStop は実行のキャンセルを要求する。
func (s *Server) handleAthenaQueryStop(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	id := r.PathValue("id")
	if err := awsinternal.StopAthenaQuery(r.Context(), profile, region, id); err != nil {
		writeAWSError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAthenaQueryHistory は直近の実行履歴を返す。鮮度が必要なためキャッシュしない。
func (s *Server) handleAthenaQueryHistory(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	workgroup := r.URL.Query().Get("workgroup")
	maxItems, _ := strconv.Atoi(r.URL.Query().Get("max"))
	items, err := awsinternal.ListAthenaQueryHistory(r.Context(), profile, region, workgroup, maxItems)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, items)
}

// writeAthenaError は読み取り専用違反を 400 に、それ以外を writeAWSError に委ねる。
func writeAthenaError(w http.ResponseWriter, err error) {
	if errors.Is(err, sqlguard.ErrWriteNotAllowed) {
		writeError(w, http.StatusBadRequest, "WRITE_NOT_ALLOWED", err.Error())
		return
	}
	writeAWSError(w, err)
}
