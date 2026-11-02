package api

import "github.com/sfuruya0612/thief/backend/internal/accesslog"

// AccessLogSourcesResponse はアクセスログのソースの一覧を表す。
type AccessLogSourcesResponse struct {
	Sources  []accesslog.Source `json:"sources"`
	Warnings []string           `json:"warnings,omitempty"`
}

// AccessLogTableValidateRequest は Athena のテーブルの検証の入力を表す。
type AccessLogTableValidateRequest struct {
	SourceID  string `json:"source_id"`
	Catalog   string `json:"catalog"`
	Database  string `json:"database"`
	Table     string `json:"table"`
	Workgroup string `json:"workgroup,omitempty"`
}

// AccessLogTableValidateResponse は Athena のテーブルの検証の結果を表す。
type AccessLogTableValidateResponse struct {
	Validation accesslog.TableValidation `json:"validation"`
	SetupSQL   string                    `json:"setup_sql,omitempty"`
}

// AccessLogQuerySourceRequest は集計の対象にするソースを表す。
type AccessLogQuerySourceRequest struct {
	ID    string                  `json:"id"`
	Table *accesslog.TableMapping `json:"table,omitempty"`
}

// AccessLogQueryRequest はアクセスログの集計の開始の入力を表す。
type AccessLogQueryRequest struct {
	View             string                        `json:"view"`
	Sources          []AccessLogQuerySourceRequest `json:"sources"`
	From             string                        `json:"from"`
	To               string                        `json:"to"`
	Filter           accesslog.Filter              `json:"filter"`
	Limit            int                           `json:"limit"`
	ConfirmLongRange bool                          `json:"confirm_long_range"`
	Workgroup        string                        `json:"workgroup,omitempty"`
	OutputLocation   string                        `json:"output_location,omitempty"`
}

// AccessLogQueryStartResponse は集計の開始の応答を表す。
type AccessLogQueryStartResponse struct {
	JobID string `json:"job_id"`
}

// AccessLogSourceError はソースごとの失敗を表す。
type AccessLogSourceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AccessLogQuerySourceStatus はソースごとの集計の状態を表す。
type AccessLogQuerySourceStatus struct {
	SourceID       string                `json:"source_id"`
	Name           string                `json:"name"`
	Type           accesslog.SourceType  `json:"type"`
	Engine         accesslog.Engine      `json:"engine"`
	State          accesslog.QueryState  `json:"state"`
	BytesScanned   int64                 `json:"bytes_scanned"`
	RecordsScanned int64                 `json:"records_scanned"`
	RecordsMatched int64                 `json:"records_matched"`
	Error          *AccessLogSourceError `json:"error,omitempty"`
}

// AccessLogQueryResponse は集計の状態の応答を表す。
type AccessLogQueryResponse struct {
	JobID    string                       `json:"job_id"`
	State    accesslog.QueryState         `json:"state"`
	Sources  []AccessLogQuerySourceStatus `json:"sources"`
	Warnings []string                     `json:"warnings,omitempty"`
}

// AccessLogResultsResponse は集計の結果の応答を表す。
type AccessLogResultsResponse struct {
	Rows     []accesslog.PairRow          `json:"rows"`
	IPs      []accesslog.IPSummary        `json:"ips,omitempty"`
	Summary  accesslog.Summary            `json:"summary"`
	Warnings []string                     `json:"warnings,omitempty"`
	Sources  []AccessLogQuerySourceStatus `json:"sources"`
}
