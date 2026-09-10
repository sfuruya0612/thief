package datadogauth

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

func TestTokenSetIsExpired(t *testing.T) {
	tests := []struct {
		name string
		tok  *TokenSet
		want bool
	}{
		{
			name: "nil token is expired",
			tok:  nil,
			want: true,
		},
		{
			name: "fresh token",
			tok:  &TokenSet{AccessToken: "a", ExpiresIn: 3600, IssuedAt: testNow},
			want: false,
		},
		{
			name: "just inside the early buffer",
			// 期限の 301 秒前はまだ有効 (バッファは 300 秒)。
			tok:  &TokenSet{AccessToken: "a", ExpiresIn: 3600, IssuedAt: testNow.Add(-3600*time.Second + 301*time.Second)},
			want: false,
		},
		{
			name: "exactly at the early buffer",
			tok:  &TokenSet{AccessToken: "a", ExpiresIn: 3600, IssuedAt: testNow.Add(-3600*time.Second + 300*time.Second)},
			want: true,
		},
		{
			name: "past the actual expiry",
			tok:  &TokenSet{AccessToken: "a", ExpiresIn: 3600, IssuedAt: testNow.Add(-2 * time.Hour)},
			want: true,
		},
		{
			name: "zero issued_at is expired",
			tok:  &TokenSet{AccessToken: "a", ExpiresIn: 3600},
			want: true,
		},
		{
			name: "zero expires_in is expired",
			tok:  &TokenSet{AccessToken: "a", IssuedAt: testNow},
			want: true,
		},
		{
			name: "negative expires_in is expired",
			tok:  &TokenSet{AccessToken: "a", ExpiresIn: -1, IssuedAt: testNow},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tok.IsExpired(testNow); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenSetExpiresAt(t *testing.T) {
	tests := []struct {
		name string
		tok  *TokenSet
		want time.Time
	}{
		{name: "nil", tok: nil, want: time.Time{}},
		{name: "no issued_at", tok: &TokenSet{ExpiresIn: 60}, want: time.Time{}},
		{name: "no expires_in", tok: &TokenSet{IssuedAt: testNow}, want: time.Time{}},
		{name: "computed", tok: &TokenSet{ExpiresIn: 60, IssuedAt: testNow}, want: testNow.Add(60 * time.Second)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tok.ExpiresAt(); !got.Equal(tt.want) {
				t.Errorf("ExpiresAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenSetValidate(t *testing.T) {
	tests := []struct {
		name    string
		tok     *TokenSet
		wantErr error
	}{
		{name: "nil", tok: nil, wantErr: ErrTokenIncomplete},
		{name: "no access token", tok: &TokenSet{RefreshToken: "r"}, wantErr: ErrTokenIncomplete},
		{name: "ok", tok: &TokenSet{AccessToken: "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.tok.Validate(); !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientCredentialsValidate(t *testing.T) {
	tests := []struct {
		name    string
		creds   *ClientCredentials
		wantErr error
	}{
		{name: "nil", creds: nil, wantErr: ErrClientIncomplete},
		{name: "no client id", creds: &ClientCredentials{RedirectURIs: []string{"http://127.0.0.1:8400/callback"}}, wantErr: ErrClientIncomplete},
		{name: "no redirect uris", creds: &ClientCredentials{ClientID: "cid"}, wantErr: ErrClientIncomplete},
		{name: "ok", creds: &ClientCredentials{ClientID: "cid", RedirectURIs: []string{"http://127.0.0.1:8400/callback"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.creds.Validate(); !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientCredentialsHasRedirectURI(t *testing.T) {
	creds := &ClientCredentials{
		ClientID:     "cid",
		RedirectURIs: []string{"http://127.0.0.1:8400/callback", "http://127.0.0.1:8089/api/datadog/auth/callback"},
	}
	tests := []struct {
		name  string
		creds *ClientCredentials
		uri   string
		want  bool
	}{
		{name: "nil", creds: nil, uri: "http://127.0.0.1:8400/callback", want: false},
		{name: "cli uri", creds: creds, uri: "http://127.0.0.1:8400/callback", want: true},
		{name: "server uri", creds: creds, uri: "http://127.0.0.1:8089/api/datadog/auth/callback", want: true},
		{name: "different port", creds: creds, uri: "http://127.0.0.1:8401/callback", want: false},
		{name: "trailing slash is not a match", creds: creds, uri: "http://127.0.0.1:8400/callback/", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.creds.HasRedirectURI(tt.uri); got != tt.want {
				t.Errorf("HasRedirectURI(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

// TestSecretIsRedacted は、トークンの生値が文字列化と slog の経路へ出ないことを確かめる。
func TestSecretIsRedacted(t *testing.T) {
	const raw = "dd-super-secret-token"
	tok := &TokenSet{AccessToken: secret(raw), RefreshToken: secret(raw)}

	if got := tok.AccessToken.String(); got != "***" {
		t.Errorf("String() = %q, want %q", got, "***")
	}
	if got := tok.AccessTokenValue(); got != raw {
		t.Errorf("AccessTokenValue() = %q, want %q", got, raw)
	}
	if got := tok.RefreshTokenValue(); got != raw {
		t.Errorf("RefreshTokenValue() = %q, want %q", got, raw)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("token", "access_token", tok.AccessToken, "refresh_token", tok.RefreshToken)
	if strings.Contains(buf.String(), raw) {
		t.Errorf("log output leaks the raw token: %s", buf.String())
	}
}

// TestNilTokenValueAccessors は nil レシーバでも panic しないことを確かめる。
// EnsureFreshToken は未ログイン時に nil を返すため、呼び出し側が誤って触っても
// 落ちないようにしておく。
func TestNilTokenValueAccessors(t *testing.T) {
	var tok *TokenSet
	if got := tok.AccessTokenValue(); got != "" {
		t.Errorf("AccessTokenValue() = %q, want empty", got)
	}
	if got := tok.RefreshTokenValue(); got != "" {
		t.Errorf("RefreshTokenValue() = %q, want empty", got)
	}
}
