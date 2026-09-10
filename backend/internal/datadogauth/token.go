package datadogauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// tokenPath はトークンエンドポイントのパス。
const tokenPath = "/oauth2/v1/token"

// tokenResponse はトークンエンドポイントの応答 (RFC 6749 §5.1)。
// expires_in は秒数、refresh_token と scope は省略されうる。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// ExchangeCode は POST https://api.{site}/oauth2/v1/token に
// grant_type=authorization_code を送り、認可コードをトークンへ引き換える
// (RFC 6749 §4.1.3、RFC 7636 §4.5)。
func ExchangeCode(ctx context.Context, site, clientID, code, redirectURI, codeVerifier string) (*TokenSet, error) {
	return Client{}.ExchangeCode(ctx, site, clientID, code, redirectURI, codeVerifier)
}

// ExchangeCode は ExchangeCode の本体。
func (c Client) ExchangeCode(ctx context.Context, site, clientID, code, redirectURI, codeVerifier string) (*TokenSet, error) {
	base, err := apiBaseURL(site)
	if err != nil {
		return nil, err
	}
	switch {
	case clientID == "":
		return nil, errors.New("exchange datadog authorization code: client ID is empty")
	case code == "":
		return nil, errors.New("exchange datadog authorization code: authorization code is empty")
	case redirectURI == "":
		return nil, errors.New("exchange datadog authorization code: redirect URI is empty")
	case codeVerifier == "":
		return nil, errors.New("exchange datadog authorization code: pkce code verifier is empty")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	tok, err := c.requestToken(ctx, base, form)
	if err != nil {
		return nil, fmt.Errorf("exchange datadog authorization code: %w", err)
	}
	tok.ClientID = clientID
	return tok, nil
}

// Refresh は POST https://api.{site}/oauth2/v1/token に grant_type=refresh_token を
// 送り、アクセストークンを更新する (RFC 6749 §6)。
//
// 応答が refresh_token を省いた場合 (§6 で OPTIONAL) は、渡された refresh_token を
// 引き継ぐ。省略は「同じリフレッシュトークンを使い続けてよい」の意であり、ここで
// 捨てると次回の更新ができなくなる。
func Refresh(ctx context.Context, site, clientID, refreshToken string) (*TokenSet, error) {
	return Client{}.Refresh(ctx, site, clientID, refreshToken)
}

// Refresh は Refresh の本体。
func (c Client) Refresh(ctx context.Context, site, clientID, refreshToken string) (*TokenSet, error) {
	base, err := apiBaseURL(site)
	if err != nil {
		return nil, err
	}
	switch {
	case clientID == "":
		return nil, errors.New("refresh datadog oauth token: client ID is empty")
	case refreshToken == "":
		return nil, errors.New("refresh datadog oauth token: refresh token is empty")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("refresh_token", refreshToken)

	tok, err := c.requestToken(ctx, base, form)
	if err != nil {
		return nil, fmt.Errorf("refresh datadog oauth token: %w", err)
	}
	if tok.RefreshToken.value() == "" {
		tok.RefreshToken = secret(refreshToken)
	}
	tok.ClientID = clientID
	return tok, nil
}

// requestToken はトークンエンドポイントを 1 回呼び、応答を TokenSet へ写す。
//
// IssuedAt には応答を受け取った時刻を入れる。RFC 6749 §5.1 の expires_in は応答の
// 生成時点からの秒数であり、期限の起点はここでしか分からない。
func (c Client) requestToken(ctx context.Context, base string, form url.Values) (*TokenSet, error) {
	var res tokenResponse
	if err := c.postForm(ctx, base+tokenPath, form.Encode(), http.StatusOK, &res); err != nil {
		return nil, err
	}
	tok := &TokenSet{
		AccessToken:  secret(res.AccessToken),
		RefreshToken: secret(res.RefreshToken),
		ExpiresIn:    res.ExpiresIn,
		IssuedAt:     time.Now().UTC(),
		Scope:        res.Scope,
	}
	if err := tok.Validate(); err != nil {
		return nil, err
	}
	return tok, nil
}
