package api

import (
	"encoding/json"
	"errors"
	"net/http"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"google.golang.org/api/googleapi"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func writeError(w http.ResponseWriter, code int, errCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: msg,
		Code:  errCode,
	})
}

func writeBadRequest(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusBadRequest, "BAD_REQUEST", msg)
}

func writeInternalError(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", msg)
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusUnauthorized, "SSO_TOKEN_EXPIRED", msg)
}

// writeGCPNotConfigured は GCP project ID が解決できない場合に 503 を返す。
func writeGCPNotConfigured(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "GCP_NOT_CONFIGURED",
		"GCP is not configured; provide ?project_id= or set GOOGLE_CLOUD_PROJECT")
}

// writeInternalFromError は err を 500 INTERNAL_ERROR として書き込む。
// serveCached のエラー writer として writeAWSError と対で使う。
func writeInternalFromError(w http.ResponseWriter, err error) {
	writeInternalError(w, err.Error())
}

// writeAWSError writes the appropriate HTTP error based on whether err is an
// SSO token expiry (401 SSO_TOKEN_EXPIRED) or a generic AWS error (500).
func writeAWSError(w http.ResponseWriter, err error) {
	if awsinternal.IsSSOTokenExpired(err) {
		writeUnauthorized(w, err.Error())
		return
	}
	writeInternalError(w, err.Error())
}

// writePricingError maps pricing/savings plans errors to HTTP responses.
// Unlike writeAWSError, it distinguishes an IAM permission error (403) from
// SSO token expiry (401): pricing:GetProducts and
// savingsplans:DescribeSavingsPlansOfferingRates are commonly missing from a
// role's policy even when the SSO session itself is valid, and
// IsSSOTokenExpired's substring match ("not authorized") would otherwise
// misclassify an AccessDenied error as an expired session, sending the user
// to re-login when re-login cannot fix a missing IAM permission. Order
// matters: IsAccessDenied (a precise smithy error-code check) must run
// before IsSSOTokenExpired (a loose substring check) so the precise
// classification wins.
func writePricingError(w http.ResponseWriter, err error) {
	switch {
	case awsinternal.IsAccessDenied(err):
		writeError(w, http.StatusForbidden, "PRICING_ACCESS_DENIED",
			"missing IAM permission: pricing:GetProducts and savingsplans:DescribeSavingsPlansOfferingRates are required")
	case awsinternal.IsThrottled(err):
		writeError(w, http.StatusTooManyRequests, "PRICING_THROTTLED", err.Error())
	case awsinternal.IsSSOTokenExpired(err):
		writeUnauthorized(w, err.Error())
	default:
		writeInternalError(w, err.Error())
	}
}

// writeGCPError は GCP 系エラーを HTTP ステータスへマップする。
// API 未有効化 (SERVICE_DISABLED / accessNotConfigured) は 403 GCP_API_DISABLED として
// 有効化を促すメッセージを返し、その他の Google API クライアントエラー (4xx) は当該
// ステータスで GCP_ERROR を返す。いずれにも当たらなければ 500 INTERNAL_ERROR とする。
// REST 系クライアント (*googleapi.Error) と gRPC 系クライアント (google.golang.org/grpc/status)
// の双方を検査する。serveCached のエラー writer として writeInternalFromError の代わりに使う。
func writeGCPError(w http.ResponseWriter, err error) {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gcpAPIDisabled(gerr) {
			msg := gerr.Message
			if msg == "" {
				msg = err.Error()
			}
			writeError(w, http.StatusForbidden, "GCP_API_DISABLED", msg)
			return
		}
		if gerr.Code >= 400 && gerr.Code < 500 {
			writeError(w, gerr.Code, "GCP_ERROR", err.Error())
			return
		}
	}
	// logadmin / Cloud Run Admin など gRPC トランスポートのクライアントは
	// *googleapi.Error ではなく gRPC status のエラーを返す。REST 系と同じく API 未有効化
	// (SERVICE_DISABLED) を 403 GCP_API_DISABLED へ分類し、その他のクライアント起因コード
	// (4xx 相当) は当該ステータスで GCP_ERROR を返す。status.FromError は ok=false のとき
	// codes.Unknown のダミーステータスを返すため、ok が true の場合のみ検査する。
	if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
		if gcpGRPCAPIDisabled(st) {
			msg := st.Message()
			if msg == "" {
				msg = err.Error()
			}
			writeError(w, http.StatusForbidden, "GCP_API_DISABLED", msg)
			return
		}
		if hs := grpcCodeToHTTP(st.Code()); hs >= 400 && hs < 500 {
			writeError(w, hs, "GCP_ERROR", err.Error())
			return
		}
	}
	writeInternalError(w, err.Error())
}

// gcpAPIDisabled は googleapi.Error が「API 未有効化」を示すかを判定する。
// 旧形式の Errors[].Reason ("accessNotConfigured") と新形式の ErrorInfo detail
// ("reason": "SERVICE_DISABLED") の双方を検査する。
func gcpAPIDisabled(gerr *googleapi.Error) bool {
	for _, item := range gerr.Errors {
		if item.Reason == "accessNotConfigured" {
			return true
		}
	}
	for _, d := range gerr.Details {
		m, ok := d.(map[string]any)
		if !ok {
			continue
		}
		if reason, _ := m["reason"].(string); reason == "SERVICE_DISABLED" {
			return true
		}
	}
	return false
}

// gcpGRPCAPIDisabled は gRPC status が「API 未有効化」を示すかを判定する。
// gRPC トランスポートでは未有効化は google.rpc.ErrorInfo detail の
// Reason == "SERVICE_DISABLED" として埋め込まれる (REST 系の gcpAPIDisabled と対になる)。
func gcpGRPCAPIDisabled(st *status.Status) bool {
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			if info.GetReason() == "SERVICE_DISABLED" {
				return true
			}
		}
	}
	return false
}

// grpcCodeToHTTP は gRPC ステータスコードを対応する HTTP ステータスへ変換する。
// クライアント起因のコード (4xx 相当) のみを明示的にマップし、サーバ起因・不明なコードは
// 500 に倒す。writeGCPError では 4xx にマップされた場合のみ当該ステータスで GCP_ERROR を返し、
// それ以外は INTERNAL_ERROR (500) とする (REST 系の gerr.Code >= 400 && < 500 と同じ方針)。
func grpcCodeToHTTP(code codes.Code) int {
	switch code {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}
