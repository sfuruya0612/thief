package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/sfuruya0612/thief/backend/internal/bigquery"
	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
	"google.golang.org/api/googleapi"
)

func (s *Server) handleBQDatasets(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	client, cleanup, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	defer cleanup()
	s.serveCached(w, r, cacheKey("bq-datasets", projectID), cacheTTL, writeInternalFromError, func() (any, error) {
		return client.ListDatasets(r.Context())
	})
}

func (s *Server) handleBQTables(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	dataset := r.PathValue("dataset")
	client, cleanup, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	defer cleanup()
	s.serveCached(w, r, cacheKey("bq-tables", projectID, dataset), cacheTTL, writeInternalFromError, func() (any, error) {
		return client.ListTables(r.Context(), dataset)
	})
}

func (s *Server) handleBQSchema(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	dataset := r.PathValue("dataset")
	table := r.PathValue("table")
	client, cleanup, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	defer cleanup()
	s.serveCached(w, r, cacheKey("bq-schema", projectID, dataset, table), cacheTTL, writeInternalFromError, func() (any, error) {
		return client.GetTableSchema(r.Context(), dataset, table)
	})
}

// handleBQQueryStart はクエリを非同期ジョブとして開始しジョブ ID を返す。
// クエリ実行系のレスポンスは決してキャッシュしない。
func (s *Server) handleBQQueryStart(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBQQueryRequest(w, r)
	if !ok {
		return
	}
	client, cleanup, ok := s.bqClientFromQuery(w, r, req.ProjectID)
	if !ok {
		return
	}
	defer cleanup()
	info, err := client.StartQuery(r.Context(), req.SQL)
	if err != nil {
		writeBQError(w, err)
		return
	}
	writeJSON(w, info)
}

// handleBQQueryDryRun はドライランで処理予定バイト数を見積もる。
func (s *Server) handleBQQueryDryRun(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBQQueryRequest(w, r)
	if !ok {
		return
	}
	client, cleanup, ok := s.bqClientFromQuery(w, r, req.ProjectID)
	if !ok {
		return
	}
	defer cleanup()
	result, err := client.DryRunQuery(r.Context(), req.SQL)
	if err != nil {
		writeBQError(w, err)
		return
	}
	writeJSON(w, result)
}

// handleBQQueryJob はジョブの現在状態 (ポーリング用) を返す。
func (s *Server) handleBQQueryJob(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	jobID := r.PathValue("job")
	location := r.URL.Query().Get("location")
	client, cleanup, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	defer cleanup()
	status, err := client.GetQueryJob(r.Context(), jobID, location)
	if err != nil {
		writeBQError(w, err)
		return
	}
	writeJSON(w, status)
}

// handleBQQueryJobResults は完了したジョブの結果を 1 ページ返す。
func (s *Server) handleBQQueryJobResults(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	jobID := r.PathValue("job")
	location := r.URL.Query().Get("location")
	pageToken := r.URL.Query().Get("page_token")
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	client, cleanup, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	defer cleanup()
	page, err := client.QueryJobResults(r.Context(), jobID, location, pageToken, pageSize)
	if err != nil {
		writeBQError(w, err)
		return
	}
	writeJSON(w, page)
}

// handleBQQueryJobCancel はジョブのキャンセルを要求する。
func (s *Server) handleBQQueryJobCancel(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	jobID := r.PathValue("job")
	location := r.URL.Query().Get("location")
	client, cleanup, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	defer cleanup()
	if err := client.CancelQueryJob(r.Context(), jobID, location); err != nil {
		writeBQError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBQQueryHistory は直近のクエリジョブ履歴を返す。鮮度が必要なためキャッシュしない。
func (s *Server) handleBQQueryHistory(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	maxItems, _ := strconv.Atoi(r.URL.Query().Get("max"))
	client, cleanup, ok := s.bqClientFromQuery(w, r, projectID)
	if !ok {
		return
	}
	defer cleanup()
	items, err := client.ListQueryHistory(r.Context(), maxItems)
	if err != nil {
		writeBQError(w, err)
		return
	}
	writeJSON(w, items)
}

// decodeBQQueryRequest はクエリ実行系リクエストのボディを読み取り検証する。
func decodeBQQueryRequest(w http.ResponseWriter, r *http.Request) (BigQueryQueryRequest, bool) {
	var req BigQueryQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body: "+err.Error())
		return BigQueryQueryRequest{}, false
	}
	if req.SQL == "" {
		writeBadRequest(w, "sql is required")
		return BigQueryQueryRequest{}, false
	}
	return req, true
}

// writeBQError は BigQuery 系エラーを HTTP ステータスへマップする。
// 読み取り専用違反と Google API のクライアントエラー (4xx) は 4xx、それ以外は 500。
func writeBQError(w http.ResponseWriter, err error) {
	if errors.Is(err, sqlguard.ErrWriteNotAllowed) {
		writeError(w, http.StatusBadRequest, "WRITE_NOT_ALLOWED", err.Error())
		return
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code >= 400 && gerr.Code < 500 {
		writeError(w, gerr.Code, "BIGQUERY_ERROR", err.Error())
		return
	}
	writeInternalError(w, err.Error())
}

// bqClientFromQuery resolves a BigQuery client from the query-param project_id
// or falls back to the server-level client. cleanup はリクエスト単位で生成した
// クライアントのみを閉じる (サーバ共有クライアントには何もしない)。
func (s *Server) bqClientFromQuery(w http.ResponseWriter, r *http.Request, projectID string) (*bigquery.Client, func(), bool) {
	if projectID != "" {
		bq, err := bigquery.NewClient(r.Context(), projectID)
		if err != nil {
			writeInternalError(w, "bigquery client: "+err.Error())
			return nil, nil, false
		}
		return bq, func() { _ = bq.Close() }, true
	}
	if s.bq != nil {
		return s.bq, func() {}, true
	}
	writeError(w, http.StatusServiceUnavailable, "BQ_NOT_CONFIGURED",
		"BigQuery is not configured; provide ?project_id= or set GOOGLE_CLOUD_PROJECT")
	return nil, nil, false
}
