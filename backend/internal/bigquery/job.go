package bigquery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
	"google.golang.org/api/iterator"
)

// defaultResultPageSize は結果取得 1 ページあたりの既定行数。
const defaultResultPageSize = 500

// defaultHistoryMax はクエリ履歴の既定取得件数。
const defaultHistoryMax = 50

// QueryJobInfo holds identity and initial state for a started query job.
type QueryJobInfo struct {
	JobID    string `json:"job_id"`
	Location string `json:"location"`
	State    string `json:"state"`
}

// QueryJobStatus holds polling state for a query job.
type QueryJobStatus struct {
	JobID               string `json:"job_id"`
	Location            string `json:"location"`
	State               string `json:"state"`
	ErrorMessage        string `json:"error_message,omitempty"`
	StartTime           string `json:"start_time,omitempty"`
	EndTime             string `json:"end_time,omitempty"`
	ElapsedMs           int64  `json:"elapsed_ms"`
	TotalBytesProcessed int64  `json:"total_bytes_processed"`
	CacheHit            bool   `json:"cache_hit"`
}

// DryRunResult holds the outcome of a dry-run cost estimation.
type DryRunResult struct {
	TotalBytesProcessed int64 `json:"total_bytes_processed"`
}

// QueryResultPage holds one page of query results.
type QueryResultPage struct {
	Columns       []string   `json:"columns"`
	Rows          [][]string `json:"rows"`
	TotalRows     uint64     `json:"total_rows"`
	NextPageToken string     `json:"next_page_token,omitempty"`
}

// QueryHistoryItem is one entry of the project's query job history.
type QueryHistoryItem struct {
	JobID               string `json:"job_id"`
	Location            string `json:"location"`
	State               string `json:"state"`
	SQL                 string `json:"sql"`
	StartTime           string `json:"start_time,omitempty"`
	EndTime             string `json:"end_time,omitempty"`
	ElapsedMs           int64  `json:"elapsed_ms"`
	TotalBytesProcessed int64  `json:"total_bytes_processed"`
	ErrorMessage        string `json:"error_message,omitempty"`
}

// StartQuery starts a read-only query as an asynchronous job and returns its
// identity. 進捗は GetQueryJob、結果は QueryJobResults で取得する。
func (c *Client) StartQuery(ctx context.Context, sql string) (*QueryJobInfo, error) {
	if err := sqlguard.ValidateReadOnly(sql); err != nil {
		return nil, err
	}
	q := c.bq.Query(sql)
	q.UseLegacySQL = false
	job, err := q.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("start bigquery query job: %w", err)
	}
	return &QueryJobInfo{
		JobID:    job.ID(),
		Location: job.Location(),
		State:    jobStateString(job.LastStatus()),
	}, nil
}

// DryRunQuery validates the SQL and returns the estimated bytes processed
// without executing the query.
func (c *Client) DryRunQuery(ctx context.Context, sql string) (*DryRunResult, error) {
	if err := sqlguard.ValidateReadOnly(sql); err != nil {
		return nil, err
	}
	q := c.bq.Query(sql)
	q.UseLegacySQL = false
	q.DryRun = true
	job, err := q.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("bigquery dry run: %w", err)
	}
	status := job.LastStatus()
	if status == nil || status.Statistics == nil {
		return &DryRunResult{}, nil
	}
	return &DryRunResult{TotalBytesProcessed: status.Statistics.TotalBytesProcessed}, nil
}

// GetQueryJob returns the current status of the given query job.
func (c *Client) GetQueryJob(ctx context.Context, jobID, location string) (*QueryJobStatus, error) {
	job, err := c.jobFromID(ctx, jobID, location)
	if err != nil {
		return nil, err
	}
	status, err := job.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("get bigquery job status for %s: %w", jobID, err)
	}
	return queryJobStatusFrom(jobID, location, status), nil
}

// QueryJobResults returns one page of results for a completed query job.
// pageToken が空のときは先頭ページを返す。
func (c *Client) QueryJobResults(ctx context.Context, jobID, location, pageToken string, pageSize int) (*QueryResultPage, error) {
	if pageSize <= 0 {
		pageSize = defaultResultPageSize
	}
	job, err := c.jobFromID(ctx, jobID, location)
	if err != nil {
		return nil, err
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("read bigquery job results for %s: %w", jobID, err)
	}
	var raw [][]bigquery.Value
	pager := iterator.NewPager(it, pageSize, pageToken)
	next, err := pager.NextPage(&raw)
	if err != nil {
		return nil, fmt.Errorf("page bigquery job results for %s: %w", jobID, err)
	}
	columns := make([]string, len(it.Schema))
	for i, f := range it.Schema {
		columns[i] = f.Name
	}
	rows := make([][]string, len(raw))
	for i, r := range raw {
		row := make([]string, len(r))
		for j, v := range r {
			row[j] = valueString(v)
		}
		rows[i] = row
	}
	return &QueryResultPage{
		Columns:       columns,
		Rows:          rows,
		TotalRows:     it.TotalRows,
		NextPageToken: next,
	}, nil
}

// CancelQueryJob requests cancellation of the given query job.
func (c *Client) CancelQueryJob(ctx context.Context, jobID, location string) error {
	job, err := c.jobFromID(ctx, jobID, location)
	if err != nil {
		return err
	}
	if err := job.Cancel(ctx); err != nil {
		return fmt.Errorf("cancel bigquery job %s: %w", jobID, err)
	}
	return nil
}

// ListQueryHistory returns up to maxItems recent query jobs (newest first).
// クエリ以外のジョブ (load / copy / extract) は除外する。
func (c *Client) ListQueryHistory(ctx context.Context, maxItems int) ([]QueryHistoryItem, error) {
	if maxItems <= 0 {
		maxItems = defaultHistoryMax
	}
	it := c.bq.Jobs(ctx)
	var items []QueryHistoryItem
	for len(items) < maxItems {
		job, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate bigquery jobs: %w", err)
		}
		if item, ok := queryHistoryItemFrom(job); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

// jobFromID resolves a job reference, honouring its location when known.
func (c *Client) jobFromID(ctx context.Context, jobID, location string) (*bigquery.Job, error) {
	var job *bigquery.Job
	var err error
	if location != "" {
		job, err = c.bq.JobFromIDLocation(ctx, jobID, location)
	} else {
		job, err = c.bq.JobFromID(ctx, jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve bigquery job %s: %w", jobID, err)
	}
	return job, nil
}

// queryHistoryItemFrom converts a listed job into a history item.
// クエリジョブでない場合や設定が取得できない場合は ok=false を返す。
func queryHistoryItemFrom(job *bigquery.Job) (QueryHistoryItem, bool) {
	cfg, err := job.Config()
	if err != nil {
		return QueryHistoryItem{}, false
	}
	qc, ok := cfg.(*bigquery.QueryConfig)
	if !ok {
		return QueryHistoryItem{}, false
	}
	st := queryJobStatusFrom(job.ID(), job.Location(), job.LastStatus())
	return QueryHistoryItem{
		JobID:               st.JobID,
		Location:            st.Location,
		State:               st.State,
		SQL:                 qc.Q,
		StartTime:           st.StartTime,
		EndTime:             st.EndTime,
		ElapsedMs:           st.ElapsedMs,
		TotalBytesProcessed: st.TotalBytesProcessed,
		ErrorMessage:        st.ErrorMessage,
	}, true
}

// queryJobStatusFrom は SDK の JobStatus を API 応答用の形へ変換する。
func queryJobStatusFrom(jobID, location string, status *bigquery.JobStatus) *QueryJobStatus {
	s := &QueryJobStatus{JobID: jobID, Location: location, State: jobStateString(status)}
	if status == nil {
		return s
	}
	if err := status.Err(); err != nil {
		s.ErrorMessage = err.Error()
	} else if len(status.Errors) > 0 {
		s.ErrorMessage = status.Errors[0].Error()
	}
	if st := status.Statistics; st != nil {
		s.TotalBytesProcessed = st.TotalBytesProcessed
		if !st.StartTime.IsZero() {
			s.StartTime = st.StartTime.UTC().Format(time.RFC3339)
			if st.EndTime.IsZero() {
				s.ElapsedMs = time.Since(st.StartTime).Milliseconds()
			} else {
				s.EndTime = st.EndTime.UTC().Format(time.RFC3339)
				s.ElapsedMs = st.EndTime.Sub(st.StartTime).Milliseconds()
			}
		}
		if qs, ok := st.Details.(*bigquery.QueryStatistics); ok {
			s.CacheHit = qs.CacheHit
		}
	}
	return s
}

// jobStateString maps the SDK job state to the API string representation.
func jobStateString(status *bigquery.JobStatus) string {
	if status == nil {
		return "PENDING"
	}
	switch status.State {
	case bigquery.Pending:
		return "PENDING"
	case bigquery.Running:
		return "RUNNING"
	case bigquery.Done:
		return "DONE"
	default:
		return "PENDING"
	}
}

// valueString は BigQuery の値セルを表示用文字列へ変換する。nil は空文字列。
func valueString(v bigquery.Value) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
