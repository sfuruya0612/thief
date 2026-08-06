package aws

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	smithy "github.com/aws/smithy-go"
)

var (
	ErrProfileNotFound       = errors.New("aws profile not found")
	ErrInvalidProfile        = errors.New("invalid profile name")
	ErrSSOTokenExpired       = errors.New("SSO token expired")
	ErrInvalidPricingService = errors.New("invalid pricing service")
)

// IsSSOTokenExpired returns true when err indicates the AWS SSO token has
// expired or is missing, covering both local cache misses and SDK auth errors.
func IsSSOTokenExpired(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSSOTokenExpired) {
		return true
	}
	msg := err.Error()
	for _, fragment := range []string{
		"token has expired",
		"token is expired",
		"ExpiredTokenException",
		"expired_token",
		"UnauthorizedException",
		"no valid SSO token",
		"sso_token_expired",
		"ForbiddenException",
		"not authorized",
		"sso session has expired",
		"failed to refresh cached credentials",
	} {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

// IsAccessDenied returns true when err is an IAM authorization failure
// (missing IAM policy), detected via the smithy API error code rather than a
// substring match. Callers that also check IsSSOTokenExpired must check
// IsAccessDenied first: an AccessDenied message commonly contains the phrase
// "is not authorized to perform", which IsSSOTokenExpired's substring match
// would otherwise misclassify as an expired SSO token, sending the user to
// re-login when re-login cannot fix a missing IAM permission.
func IsAccessDenied(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "AccessDeniedException", "AccessDenied", "UnauthorizedOperation":
		return true
	default:
		return false
	}
}

// shouldWarnIgnoredErr は「失敗を無視する」詳細呼び出しのエラーに slog.Warn を出すべきかを返す。
// context.Canceled / context.DeadlineExceeded は errgroup のキャンセル連鎖で 1 リクエストに
// 大量の Warn が出るため対象外とする (キャンセル起因の欠損は handleIgnoredErr が全体エラーとして
// 伝播させる)。
func shouldWarnIgnoredErr(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// handleIgnoredErr は「失敗を無視する」詳細呼び出しのエラーを処理する。
// キャンセル起因のエラーはそのまま返して全体エラーとして伝播させる (詳細が欠けた結果を
// serveCached のキャッシュに書き込ませない)。それ以外の失敗は slog.Warn を出して nil を
// 返し、欠損データ (タグ空など) として続行する。スロットリング等の失敗を観測可能にする。
func handleIgnoredErr(err error, msg string, attrs ...any) error {
	if err == nil {
		return nil
	}
	if !shouldWarnIgnoredErr(err) {
		return err
	}
	slog.Warn(msg, append(attrs, "err", err)...)
	return nil
}

// handleIgnoredErrFlag は handleIgnoredErr を呼び出し、その結果から呼び出し側が
// 縮退の発生を機械的に判定できる bool を併せて返す。degraded が true になるのは
// err が非 nil で、かつ handleIgnoredErr が (Warn ログを出して) nil を返した場合
// (縮退して続行する場合) に限る。err が nil の場合、または err がキャンセル起因で
// handleIgnoredErr がそのまま返す場合は false になる (後者は呼び出し側が
// propagated を見て全体エラーとして return するため、フラグは意味を持たない)。
func handleIgnoredErrFlag(err error, msg string, attrs ...any) (degraded bool, propagated error) {
	propagated = handleIgnoredErr(err, msg, attrs...)
	degraded = err != nil && propagated == nil
	return degraded, propagated
}

// IsThrottled returns true when err indicates the AWS API throttled the request.
func IsThrottled(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "ThrottlingException", "Throttling", "TooManyRequestsException", "RequestLimitExceeded":
		return true
	default:
		return false
	}
}
