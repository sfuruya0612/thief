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
