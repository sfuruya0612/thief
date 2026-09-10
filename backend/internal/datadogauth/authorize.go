package datadogauth

import (
	"errors"
	"net/url"
	"strings"
)

// authorizePath は認可エンドポイントのパス。ホストは DCR やトークンの api. ではなく
// app. サブドメインである。
const authorizePath = "/oauth2/v1/authorize"

// defaultScopes は認可要求で既定として求めるスコープ。usage_read は Usage Metering
// (historical_cost / estimated_cost) の取得に対応する。
var defaultScopes = []string{"usage_read"}

// DefaultScopes は既定のスコープの複製を返す。呼び出し側による書き換えが
// パッケージの状態へ波及しないよう、内部のスライスは共有しない。
func DefaultScopes() []string {
	return append([]string(nil), defaultScopes...)
}

// BuildAuthorizationURL は https://app.{site}/oauth2/v1/authorize の認可 URL を
// 組み立てる (RFC 6749 §4.1.1、RFC 7636 §4.3)。
//
// scopes が空の場合は DefaultScopes() を使う。スコープの区切りは RFC 6749 §3.3 の
// 空白 1 文字である。
func BuildAuthorizationURL(site, clientID, redirectURI, state string, pkce PKCE, scopes []string) (string, error) {
	if err := ValidateSite(site); err != nil {
		return "", err
	}
	if clientID == "" {
		return "", errors.New("build datadog authorization URL: client ID is empty")
	}
	if redirectURI == "" {
		return "", errors.New("build datadog authorization URL: redirect URI is empty")
	}
	if state == "" {
		return "", errors.New("build datadog authorization URL: state is empty")
	}
	if pkce.Challenge == "" || pkce.Method == "" {
		return "", errors.New("build datadog authorization URL: pkce code challenge is empty")
	}
	if len(scopes) == 0 {
		scopes = DefaultScopes()
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", pkce.Method)

	u := url.URL{
		Scheme:   "https",
		Host:     "app." + site,
		Path:     authorizePath,
		RawQuery: q.Encode(),
	}
	return u.String(), nil
}
