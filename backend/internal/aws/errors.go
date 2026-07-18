package aws

import (
	"errors"
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
