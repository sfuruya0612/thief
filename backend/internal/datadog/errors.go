package datadog

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

// IsForbidden reports whether err is a Datadog API error carrying HTTP 403.
//
// datadog.GenericOpenAPIError does not keep the status code itself: for a
// non-2xx response the SDK puts http.Response.Status ("403 Forbidden") into
// ErrorMessage and the raw body into ErrorBody. The leading token of
// ErrorMessage is therefore the only place the status code survives.
func IsForbidden(err error) bool {
	var apiErr datadog.GenericOpenAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code, _, _ := strings.Cut(apiErr.ErrorMessage, " ")
	return code == strconv.Itoa(http.StatusForbidden)
}

// ErrorBody returns the raw response body of a Datadog API error, truncated to
// maxLen bytes. It returns an empty string when err is not a Datadog API error
// or carries no body. Callers use it to log why a call was rejected, since
// GenericOpenAPIError.Error() only yields the status line.
func ErrorBody(err error, maxLen int) string {
	var apiErr datadog.GenericOpenAPIError
	if !errors.As(err, &apiErr) {
		return ""
	}
	body := strings.TrimSpace(string(apiErr.ErrorBody))
	if len(body) > maxLen {
		return body[:maxLen]
	}
	return body
}
