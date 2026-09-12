package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	smithy "github.com/aws/smithy-go"
	"google.golang.org/api/googleapi"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
)

func TestWriteAWSError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "AccessDeniedException は 403 ACCESS_DENIED",
			err:        &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "User: arn:aws:sts::123456789012:assumed-role/x is not authorized to perform: wafv2:ListWebACLs"},
			wantStatus: http.StatusForbidden,
			wantCode:   "ACCESS_DENIED",
		},
		{
			name:       "AccessDenied は 403 ACCESS_DENIED",
			err:        &smithy.GenericAPIError{Code: "AccessDenied", Message: "Access Denied"},
			wantStatus: http.StatusForbidden,
			wantCode:   "ACCESS_DENIED",
		},
		{
			name:       "UnauthorizedOperation は 403 ACCESS_DENIED",
			err:        &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: "You are not authorized to perform this operation."},
			wantStatus: http.StatusForbidden,
			wantCode:   "ACCESS_DENIED",
		},
		{
			name:       "%w でラップされた AccessDenied も 403 ACCESS_DENIED",
			err:        fmt.Errorf("list waf acls: %w", &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not authorized"}),
			wantStatus: http.StatusForbidden,
			wantCode:   "ACCESS_DENIED",
		},
		{
			name:       "SSO 期限切れは引き続き 401 SSO_TOKEN_EXPIRED",
			err:        errors.New("the SSO session has expired or is invalid"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "SSO_TOKEN_EXPIRED",
		},
		{
			name:       "ExpiredTokenException は 401 SSO_TOKEN_EXPIRED",
			err:        &smithy.GenericAPIError{Code: "ExpiredTokenException", Message: "ExpiredTokenException: token has expired"},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "SSO_TOKEN_EXPIRED",
		},
		{
			name:       "その他のエラーは 500 INTERNAL_ERROR",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeAWSError(rec, tt.err)

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
		})
	}
}

// TestWriteDatadogCostError は、資格情報が 1 つも無い場合だけを再ログイン可能な 401 に
// 分類し、それ以外 (スコープ不足の 403 や静的キーの拒否を含む) を 500 のままにすることを
// 確認する。401 の範囲が広がると、再ログインでは解決しない失敗でバナーを出すことになる。
func TestWriteDatadogCostError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "資格情報が無い場合は 401 DATADOG_NO_CREDENTIALS",
			err:        ErrDatadogNoCredentials,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "DATADOG_NO_CREDENTIALS",
		},
		{
			name:       "%w でラップされていても 401 DATADOG_NO_CREDENTIALS",
			err:        fmt.Errorf("%w: no Datadog OAuth token is stored; run 'thief datadog auth login', and DATADOG_API_KEY / DATADOG_APP_KEY are not both set", ErrDatadogNoCredentials),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "DATADOG_NO_CREDENTIALS",
		},
		{
			// datadogCall が静的キーへ倒せずそのまま返す 403 (OAuth のスコープ不足)。
			// 再ログインしても同じスコープのトークンが出るだけなので 500 のままとする。
			name:       "OAuth スコープ不足の 403 は 500 INTERNAL_ERROR",
			err:        forbiddenDatadogError(),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:       "その他のエラーは 500 INTERNAL_ERROR",
			err:        errors.New("get datadog historical cost: boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeDatadogCostError(rec, tt.err)

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
		})
	}
}

func TestWriteGCPError(t *testing.T) {
	const disabledMsg = "Cloud Resource Manager API has not been used in project gumi-green-1222 before or it is disabled."

	tests := []struct {
		name            string
		err             error
		wantStatus      int
		wantCode        string
		wantMsg         string // 空でなければメッセージの完全一致を検証する
		wantMsgContains string // 空でなければメッセージがこの文字列を含むことを検証する
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
		{
			// issue 0046: logadmin (gRPC トランスポート) の ListLogEntries が返す実際のエラー形状の再現。
			name: "gRPC PermissionDenied + ErrorInfo(SERVICE_DISABLED) を検出して 403 GCP_API_DISABLED",
			err: grpcErrorWithDetails(codes.PermissionDenied, disabledMsg,
				&errdetails.ErrorInfo{Reason: "SERVICE_DISABLED"}),
			wantStatus: http.StatusForbidden,
			wantCode:   "GCP_API_DISABLED",
			wantMsg:    disabledMsg,
		},
		{
			// status.FromError は %w でラップされたエラーを検出した場合、Message() を
			// err.Error() 全体 (ラップの prefix 込み) へ書き換える仕様 (grpc-go の
			// status.FromError の godoc に明記)。detail による SERVICE_DISABLED 判定は
			// ラップの有無に関わらず行えることと、書き換え後のメッセージも disabledMsg を
			// 含む (情報が失われない) ことの両方を確認する。
			name: "%w でラップされた gRPC エラーでも errors.As 相当で未有効化を検出する",
			err: fmt.Errorf("list gcp log entries: %w", grpcErrorWithDetails(
				codes.PermissionDenied, disabledMsg, &errdetails.ErrorInfo{Reason: "SERVICE_DISABLED"})),
			wantStatus:      http.StatusForbidden,
			wantCode:        "GCP_API_DISABLED",
			wantMsgContains: disabledMsg,
		},
		{
			name:       "SERVICE_DISABLED を伴わない gRPC PermissionDenied は 403 GCP_ERROR",
			err:        status.New(codes.PermissionDenied, "permission denied").Err(),
			wantStatus: http.StatusForbidden,
			wantCode:   "GCP_ERROR",
		},
		{
			name:       "gRPC NotFound は 404 GCP_ERROR",
			err:        status.New(codes.NotFound, "not found").Err(),
			wantStatus: http.StatusNotFound,
			wantCode:   "GCP_ERROR",
		},
		{
			name:       "サーバ起因の gRPC Internal は 500 INTERNAL_ERROR",
			err:        status.New(codes.Internal, "backend error").Err(),
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
			if tt.wantMsgContains != "" && !strings.Contains(body.Error, tt.wantMsgContains) {
				t.Errorf("error message = %q, want substring %q", body.Error, tt.wantMsgContains)
			}
		})
	}
}

// grpcErrorWithDetails は google.rpc.ErrorInfo 等の detail 付き gRPC status エラーを組み立てる。
// logadmin / Cloud Run Admin など gRPC トランスポートのクライアントが返す実際のエラー形状
// (detail に ErrorInfo を含む status) を模す。
func grpcErrorWithDetails(code codes.Code, msg string, details ...*errdetails.ErrorInfo) error {
	st := status.New(code, msg)
	msgs := make([]protoadapt.MessageV1, len(details))
	for i, d := range details {
		msgs[i] = d
	}
	withDetails, err := st.WithDetails(msgs...)
	if err != nil {
		panic(fmt.Sprintf("grpcErrorWithDetails: WithDetails failed: %v", err))
	}
	return withDetails.Err()
}
