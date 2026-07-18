package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestWriteGCPError(t *testing.T) {
	const disabledMsg = "Cloud Resource Manager API has not been used in project gumi-green-1222 before or it is disabled."

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string // 空でなければメッセージの完全一致を検証する
	}{
		{
			name: "SERVICE_DISABLED を Details から検出して 403 GCP_API_DISABLED",
			err: &googleapi.Error{
				Code:    http.StatusForbidden,
				Message: disabledMsg,
				Details: []any{
					map[string]any{
						"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
						"reason": "SERVICE_DISABLED",
					},
				},
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "GCP_API_DISABLED",
			wantMsg:    disabledMsg,
		},
		{
			name: "accessNotConfigured を Errors から検出して 403 GCP_API_DISABLED",
			err: &googleapi.Error{
				Code:    http.StatusForbidden,
				Message: disabledMsg,
				Errors:  []googleapi.ErrorItem{{Reason: "accessNotConfigured", Message: disabledMsg}},
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "GCP_API_DISABLED",
			wantMsg:    disabledMsg,
		},
		{
			name: "%w でラップされていても errors.As で未有効化を検出する",
			err: fmt.Errorf("get iam policy for gumi-green-1222: %w", &googleapi.Error{
				Code:    http.StatusForbidden,
				Message: disabledMsg,
				Errors:  []googleapi.ErrorItem{{Reason: "accessNotConfigured"}},
			}),
			wantStatus: http.StatusForbidden,
			wantCode:   "GCP_API_DISABLED",
			wantMsg:    disabledMsg,
		},
		{
			name:       "未有効化以外の 403 は当該ステータスで GCP_ERROR",
			err:        &googleapi.Error{Code: http.StatusForbidden, Message: "permission denied", Errors: []googleapi.ErrorItem{{Reason: "forbidden"}}},
			wantStatus: http.StatusForbidden,
			wantCode:   "GCP_ERROR",
		},
		{
			name:       "一般的な 4xx は当該ステータスで GCP_ERROR",
			err:        &googleapi.Error{Code: http.StatusNotFound, Message: "not found"},
			wantStatus: http.StatusNotFound,
			wantCode:   "GCP_ERROR",
		},
		{
			name:       "5xx の googleapi エラーは 500 INTERNAL_ERROR",
			err:        &googleapi.Error{Code: http.StatusServiceUnavailable, Message: "backend error"},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:       "googleapi 以外の error は 500 INTERNAL_ERROR",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeGCPError(rec, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var body ErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if body.Error == "" {
				t.Errorf("error message is empty; want non-empty")
			}
			if tt.wantMsg != "" && body.Error != tt.wantMsg {
				t.Errorf("error message = %q, want %q", body.Error, tt.wantMsg)
			}
		})
	}
}
