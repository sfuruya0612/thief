package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	smithy "github.com/aws/smithy-go"
	"github.com/sfuruya0612/thief/backend/internal/accesslog"
)

func TestWriteAccessLogError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid source",
			err:        accesslog.ErrInvalidSource,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ACCESS_LOGS_INVALID_SOURCE",
		},
		{
			name:       "source not ready",
			err:        accesslog.ErrSourceNotReady,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ACCESS_LOGS_SOURCE_NOT_READY",
		},
		{
			name:       "invalid table",
			err:        accesslog.ErrInvalidTable,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ACCESS_LOGS_TABLE_INVALID",
		},
		{
			name:       "attribution",
			err:        accesslog.ErrAttributionRequired,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ACCESS_LOGS_ATTRIBUTION_REQUIRED",
		},
		{
			name:       "range too long",
			err:        accesslog.ErrRangeTooLong,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ACCESS_LOGS_RANGE_TOO_LONG",
		},
		{
			name:       "access denied",
			err:        &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not authorized"},
			wantStatus: http.StatusForbidden,
			wantCode:   "ACCESS_DENIED",
		},
		{
			name:       "sso expired",
			err:        errors.New("the SSO session has expired or is invalid"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "SSO_TOKEN_EXPIRED",
		},
		{
			name:       "query failed",
			err:        errors.New("athena failed"),
			wantStatus: http.StatusBadGateway,
			wantCode:   "ACCESS_LOGS_QUERY_FAILED",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeAccessLogError(rec, tt.err)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if body := rec.Body.String(); !strings.Contains(body, tt.wantCode) {
				t.Errorf("body = %s, want code %s", body, tt.wantCode)
			}
		})
	}
}
