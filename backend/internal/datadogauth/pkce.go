package datadogauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const (
	// codeVerifierBytes は code_verifier の元にする乱数のバイト数。
	// パディング無しの base64url は 3 バイトを 4 文字へ写すため、96 バイトはちょうど
	// 128 文字になる。RFC 7636 §4.1 が定める上限 (128 文字) いっぱいを使う。
	codeVerifierBytes = 96
	// CodeVerifierLength は生成する code_verifier の文字数。
	CodeVerifierLength = codeVerifierBytes / 3 * 4
	// codeChallengeMethodS256 は RFC 7636 §4.2 の S256 変換。plain は使わない。
	codeChallengeMethodS256 = "S256"
	// stateBytes は state パラメータの元にする乱数のバイト数 (256 ビット)。
	stateBytes = 32
)

// PKCE は RFC 7636 の code_verifier と、そこから導出した code_challenge の組。
// Verifier は認可コードの引き換えでしか送らない秘密であり、ログへ出さないよう
// redact する型で持つ。
type PKCE struct {
	Verifier  secret
	Challenge string
	Method    string
}

// NewPKCE は crypto/rand の乱数から code_verifier (128 文字の base64url、パディング無し)
// を生成し、その SHA-256 を base64url (パディング無し) で符号化した code_challenge を
// 添えて返す (RFC 7636 §4.1 / §4.2)。
func NewPKCE() (PKCE, error) {
	buf := make([]byte, codeVerifierBytes)
	if _, err := rand.Read(buf); err != nil {
		return PKCE{}, fmt.Errorf("generate pkce code verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	return PKCE{
		Verifier:  secret(verifier),
		Challenge: codeChallengeS256(verifier),
		Method:    codeChallengeMethodS256,
	}, nil
}

// codeChallengeS256 は RFC 7636 §4.2 の S256 変換
// (BASE64URL-ENCODE(SHA256(ASCII(code_verifier)))) を行う。
func codeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// newState は CSRF 対策の state パラメータ (RFC 6749 §10.12) を生成する。
func newState() (string, error) {
	buf := make([]byte, stateBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
