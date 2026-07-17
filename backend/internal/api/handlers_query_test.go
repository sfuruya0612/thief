package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
	"google.golang.org/api/googleapi"
)

// decodeErrorResponse はレスポンスボディの標準エラー DTO を読み取る。
func decodeErrorResponse(t *testing.T, w *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v (body=%q)", err, w.Body.String())
	}
	return resp
}

func TestHandleBQQueryStartValidation(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "invalid json", body: "{", wantCode: http.StatusBadRequest},
		{name: "missing sql", body: `{"project_id":"p1"}`, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			r := httptest.NewRequest(http.MethodPost, "/api/bigquery/query", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			s.handleBQQueryStart(w, r)
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestHandleBQQueryStartNotConfigured(t *testing.T) {
	// project_id もサーバレベルクライアントも無い場合は 503 BQ_NOT_CONFIGURED。
	s := newTestServer(t)
	r := httptest.NewRequest(http.MethodPost, "/api/bigquery/query", strings.NewReader(`{"sql":"SELECT 1"}`))
	w := httptest.NewRecorder()
	s.handleBQQueryStart(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if resp := decodeErrorResponse(t, w); resp.Code != "BQ_NOT_CONFIGURED" {
		t.Errorf("code = %q, want BQ_NOT_CONFIGURED", resp.Code)
	}
}

func TestHandleAthenaQueryStartValidation(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "invalid json", body: "{", wantCode: http.StatusBadRequest},
		{name: "missing sql", body: `{"workgroup":"primary"}`, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			r := httptest.NewRequest(http.MethodPost, "/api/aws/profiles/p1/athena/query", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			s.handleAthenaQueryStart(w, r)
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestWriteBQError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "write not allowed",
			err:        fmt.Errorf("wrap: %w", sqlguard.ErrWriteNotAllowed),
			wantStatus: http.StatusBadRequest,
			wantCode:   "WRITE_NOT_ALLOWED",
		},
		{
			name:       "googleapi client error keeps status",
			err:        fmt.Errorf("dry run: %w", &googleapi.Error{Code: 400, Message: "syntax error"}),
			wantStatus: http.StatusBadRequest,
			wantCode:   "BIGQUERY_ERROR",
		},
		{
			name:       "googleapi not found keeps status",
			err:        &googleapi.Error{Code: 404, Message: "job not found"},
			wantStatus: http.StatusNotFound,
			wantCode:   "BIGQUERY_ERROR",
		},
		{
			name:       "googleapi server error maps to 500",
			err:        &googleapi.Error{Code: 503, Message: "backend"},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:       "generic error maps to 500",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeBQError(w, tt.err)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if resp := decodeErrorResponse(t, w); resp.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", resp.Code, tt.wantCode)
			}
		})
	}
}

func TestWriteAthenaError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "write not allowed",
			err:        sqlguard.ErrWriteNotAllowed,
			wantStatus: http.StatusBadRequest,
			wantCode:   "WRITE_NOT_ALLOWED",
		},
		{
			name:       "sso token expired maps to 401",
			err:        errors.New("operation error Athena: the SSO session has expired"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "SSO_TOKEN_EXPIRED",
		},
		{
			name:       "generic error maps to 500",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeAthenaError(w, tt.err)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if resp := decodeErrorResponse(t, w); resp.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", resp.Code, tt.wantCode)
			}
		})
	}
}

func TestHandleAthenaTablesRequiresDatabase(t *testing.T) {
	s := newTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "/api/aws/profiles/p1/athena/tables", nil)
	w := httptest.NewRecorder()
	s.handleAthenaTables(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
