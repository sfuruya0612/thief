package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/accesslog"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func jsonDecode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func newAccessLogJobID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func accessLogTableError(err error) error {
	if awsinternal.IsAccessDenied(err) || awsinternal.IsSSOTokenExpired(err) {
		return err
	}
	return fmt.Errorf("%w: %v", accesslog.ErrInvalidTable, err)
}

func writeAccessLogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, accesslog.ErrInvalidSource):
		writeError(w, http.StatusBadRequest, "ACCESS_LOGS_INVALID_SOURCE", err.Error())
	case errors.Is(err, accesslog.ErrSourceNotReady):
		writeError(w, http.StatusBadRequest, "ACCESS_LOGS_SOURCE_NOT_READY", err.Error())
	case errors.Is(err, accesslog.ErrInvalidTable):
		writeError(w, http.StatusBadRequest, "ACCESS_LOGS_TABLE_INVALID", err.Error())
	case errors.Is(err, accesslog.ErrAttributionRequired):
		writeError(w, http.StatusBadRequest, "ACCESS_LOGS_ATTRIBUTION_REQUIRED", err.Error())
	case errors.Is(err, accesslog.ErrInvalidTimeRange):
		writeError(w, http.StatusBadRequest, "ACCESS_LOGS_INVALID_RANGE", err.Error())
	case errors.Is(err, accesslog.ErrRangeTooLong):
		writeError(w, http.StatusBadRequest, "ACCESS_LOGS_RANGE_TOO_LONG", err.Error())
	case awsinternal.IsAccessDenied(err):
		writeError(w, http.StatusForbidden, "ACCESS_DENIED", err.Error())
	case awsinternal.IsSSOTokenExpired(err):
		writeUnauthorized(w, err.Error())
	default:
		writeError(w, http.StatusBadGateway, "ACCESS_LOGS_QUERY_FAILED", err.Error())
	}
}
