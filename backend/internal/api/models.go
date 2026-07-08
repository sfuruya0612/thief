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
}

// SSMValueResponse is returned by GET /api/aws/profiles/{profile}/ssm/parameters/{name}.
type SSMValueResponse struct {
	Value string `json:"value"`
}

// BigQueryQueryRequest is the body for POST /api/bigquery/query.
type BigQueryQueryRequest struct {
	ProjectID string `json:"project_id"`
	SQL       string `json:"sql"`
}
