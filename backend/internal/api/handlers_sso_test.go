package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteSSOLoginResult は aws sso login の実行結果 (成功 / タイムアウト / 一般エラー)
// が正しい HTTP ステータスとエラーコードへ変換されることを検証する。
func TestWriteSSOLoginResult(t *testing.T) {
	tests := []struct {
		name       string
		runErr     error
		ctxErr     error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "成功時は204",
			runErr:     nil,
			ctxErr:     nil,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "コンテキストタイムアウト時は504",
			runErr:     errors.New("signal: killed"),
			ctxErr:     context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "SSO_LOGIN_TIMEOUT",
		},
		{
			name:       "aws cli の一般エラー時は500",
			runErr:     errors.New("exit status 1"),
			ctxErr:     nil,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "SSO_LOGIN_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeSSOLoginResult(w, tt.runErr, tt.ctxErr)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantCode == "" {
				if w.Body.Len() != 0 {
					t.Fatalf("body = %q, want empty", w.Body.String())
				}
				return
			}

			var body ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal body: %v (body=%s)", err, w.Body.String())
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
		})
	}
}
