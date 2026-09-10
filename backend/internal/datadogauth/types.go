// Package datadogauth は Datadog の OAuth 2.0 ログイン (RFC 6749 Authorization Code Grant、
// RFC 7636 PKCE、RFC 7591 Dynamic Client Registration) を提供する。
//
// internal/cli と将来の internal/api の両方から使う共有ロジックであり、フローを
// 「認可の準備」(PrepareLogin)、「認可コードの引き換え」(CompleteLogin)、「トークンの
// 有効性確保」(EnsureFreshToken)、「ローカル認証情報の削除」(Logout) の 4 つの公開関数に
// 分割している。認可 URL の提示、ブラウザの起動、コールバックの受信といった利用者への
// 提示手段と待ち受けは呼び出し側の責務であり、本パッケージは扱わない。
//
// 本パッケージが依存する internal パッケージは config だけである (保存先の解決に
// config.Dir() と Datadog の OAuth コールバックの既定値の定数を使う)。逆向きの依存を
// 作ると循環 import になるため、config 以外の internal パッケージを import しない。
package datadogauth

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
)

// expiryBuffer はアクセストークンを期限切れとみなす早期バッファ。実際の期限の
// 300 秒 (5 分) 前から期限切れ扱いにして、API 呼び出しの途中で期限が切れることを避ける。
// DataDog 公式 CLI pup の is_expired() と同じ値にする。
const expiryBuffer = 300 * time.Second

var (
	// ErrTokenIncomplete は access_token を持たないトークンを表す。
	ErrTokenIncomplete = errors.New("datadog oauth token is incomplete")
	// ErrClientIncomplete は client_id を持たないクライアント登録を表す。
	ErrClientIncomplete = errors.New("datadog oauth client registration is incomplete")
)

// secret は認証情報の文字列を、ログや文字列化の経路へ生値のまま出さないための型。
// internal/config の redacted と同じ考え方である。encoding/json は String() /
// LogValue() を見ないため、ファイルへの保存と API レスポンスの解釈では生値が使われる。
type secret string

func (s secret) String() string       { return "***" }
func (s secret) LogValue() slog.Value { return slog.StringValue("***") }
func (s secret) value() string        { return string(s) }

// TokenSet はトークンエンドポイントが返したトークンと、その有効期限の判定に必要な値。
// 保存先のファイル (token_<site>.json) の JSON 形状でもある。
//
// IssuedAt はトークンエンドポイントの応答を受け取った時刻である。RFC 6749 §5.1 の
// expires_in は応答の生成時点からの秒数なので、期限は IssuedAt + ExpiresIn で求める。
type TokenSet struct {
	AccessToken  secret    `json:"access_token"`
	RefreshToken secret    `json:"refresh_token,omitempty"`
	ExpiresIn    int64     `json:"expires_in"`
	IssuedAt     time.Time `json:"issued_at"`
	Scope        string    `json:"scope,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
}

// AccessTokenValue はアクセストークンの生値を返す。
func (t *TokenSet) AccessTokenValue() string {
	if t == nil {
		return ""
	}
	return t.AccessToken.value()
}

// RefreshTokenValue はリフレッシュトークンの生値を返す。
func (t *TokenSet) RefreshTokenValue() string {
	if t == nil {
		return ""
	}
	return t.RefreshToken.value()
}

// ExpiresAt はアクセストークンの期限 (バッファを引く前の値) を返す。
// 期限を判定できない場合 (IssuedAt が零値、または ExpiresIn が正でない場合) は零値を返す。
func (t *TokenSet) ExpiresAt() time.Time {
	if t == nil || t.IssuedAt.IsZero() || t.ExpiresIn <= 0 {
		return time.Time{}
	}
	return t.IssuedAt.Add(time.Duration(t.ExpiresIn) * time.Second)
}

// IsExpired は now の時点でアクセストークンが期限切れ (expiryBuffer の分だけ早める) か
// どうかを返す。期限を判定できないトークン (nil、IssuedAt が零値、ExpiresIn が正でない)
// は期限切れとして扱う。判定できないものを有効側に倒すと、期限切れのトークンで API を
// 呼んで失敗するまで気付けないためである。
func (t *TokenSet) IsExpired(now time.Time) bool {
	expiresAt := t.ExpiresAt()
	if expiresAt.IsZero() {
		return true
	}
	return !now.Before(expiresAt.Add(-expiryBuffer))
}

// Validate は保存または利用に足るトークンかどうかを検証する。
func (t *TokenSet) Validate() error {
	if t == nil {
		return fmt.Errorf("%w: token is nil", ErrTokenIncomplete)
	}
	if t.AccessToken.value() == "" {
		return fmt.Errorf("%w: access_token is empty", ErrTokenIncomplete)
	}
	return nil
}

// ClientCredentials は Dynamic Client Registration で発行されたクライアント登録。
// 保存先のファイル (client_<site>.json) の JSON 形状でもある。
//
// ClientSecret は public client + PKCE 前提の設計では返らない (pup の RegistrationResponse
// にもフィールドが無い) が、返った場合に生値がログへ乗らないよう redact する型で受ける。
type ClientCredentials struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name,omitempty"`
	ClientSecret secret   `json:"client_secret,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

// Validate は利用に足るクライアント登録かどうかを検証する。
func (c *ClientCredentials) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: client registration is nil", ErrClientIncomplete)
	}
	if c.ClientID == "" {
		return fmt.Errorf("%w: client_id is empty", ErrClientIncomplete)
	}
	if len(c.RedirectURIs) == 0 {
		return fmt.Errorf("%w: redirect_uris is empty", ErrClientIncomplete)
	}
	return nil
}

// HasRedirectURI は uri が登録済みの redirect_uris に含まれるかどうかを返す。
// 認可サーバは登録済みの値と完全一致しない redirect_uri を拒否する (RFC 6749 §3.1.2.3)
// ため、認可 URL を組み立てる前にこちらで検査する。
func (c *ClientCredentials) HasRedirectURI(uri string) bool {
	if c == nil {
		return false
	}
	return slices.Contains(c.RedirectURIs, uri)
}
