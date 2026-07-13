package api

import (
	"encoding/json"
	"net/http"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
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

// writeAWSError writes the appropriate HTTP error based on whether err is an
// SSO token expiry (401 SSO_TOKEN_EXPIRED) or a generic AWS error (500).
func writeAWSError(w http.ResponseWriter, err error) {
	if awsinternal.IsSSOTokenExpired(err) {
		writeUnauthorized(w, err.Error())
		return
	}
	writeInternalError(w, err.Error())
}
