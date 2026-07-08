package aws

import (
	"errors"
	"strings"
)

var (
	ErrProfileNotFound = errors.New("aws profile not found")
	ErrInvalidProfile  = errors.New("invalid profile name")
	ErrSSOTokenExpired = errors.New("SSO token expired")
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
