package api

import "time"

// ErrorResponse is the standard error DTO returned by all endpoints.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details any    `json:"details,omitempty"`
}

// CacheHeaders holds the values written to X-Cache-* response headers.
type CacheHeaders struct {
	Status    string // HIT or MISS
	CachedAt  time.Time
	ExpiresAt time.Time
	TTL       int
}

// ProfileInfo is returned by GET /api/aws/profiles.
type ProfileInfo struct {
	Name string `json:"name"`
	// AccountID / SSORoleName are parsed statically from ~/.aws/config
	// (sso_account_id / sso_role_name) and are empty for non-SSO profiles.
	AccountID   string `json:"account_id,omitempty"`
	SSORoleName string `json:"sso_role_name,omitempty"`
	// Region はプロファイルの region キー (未設定なら欠落)。
	Region string `json:"region,omitempty"`
	// AuthType は "sso" | "access_key" | "assume_role" | "credential_process" |
	// "unknown" のいずれか。全プロファイルで必ず返す。
	AuthType string `json:"auth_type"`
	// SSOStatus は "valid" | "expired" | "not_logged_in"。SSO 以外のプロファイル
	// および判定不能時は欠落する。
	SSOStatus string `json:"sso_status,omitempty"`
	// SSOExpiresAt はトークンキャッシュの expiresAt (RFC3339, UTC)。不明時は欠落。
	SSOExpiresAt string `json:"sso_expires_at,omitempty"`
}

// CallerIdentityInfo is returned by GET /api/aws/profiles/{profile}/identity.
type CallerIdentityInfo struct {
	AccountID string `json:"account_id"`
	Arn       string `json:"arn"`
	UserID    string `json:"user_id"`
}

// SSMValueResponse is returned by GET /api/aws/profiles/{profile}/ssm/parameters/{name}.
type SSMValueResponse struct {
	Value string `json:"value"`
}

// BigQueryQueryRequest is the body for POST /api/bigquery/query and
// POST /api/bigquery/query/dryrun.
type BigQueryQueryRequest struct {
	ProjectID string `json:"project_id"`
	SQL       string `json:"sql"`
}

// AthenaQueryRequest is the body for POST /api/aws/profiles/{profile}/athena/query.
type AthenaQueryRequest struct {
	SQL            string `json:"sql"`
	Catalog        string `json:"catalog"`
	Database       string `json:"database"`
	Workgroup      string `json:"workgroup"`
	OutputLocation string `json:"output_location"`
}

// SnippetRequest is the body for POST /api/snippets.
type SnippetRequest struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}
