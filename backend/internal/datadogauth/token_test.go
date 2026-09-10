package datadogauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestExchangeCode(t *testing.T) {
	tests := []struct {
		name         string
		site         string
		clientID     string
		code         string
		redirectURI  string
		codeVerifier string
		status       int
		body         string
		wantToken    *TokenSet
		wantForm     url.Values
		wantURL      string
		wantErr      error
	}{
		{
			name:         "exchanged",
			site:         "datadoghq.com",
			clientID:     "cid",
			code:         "auth-code",
			redirectURI:  "http://127.0.0.1:8400/callback",
			codeVerifier: "verifier",
			status:       http.StatusOK,
			body:         `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600,"scope":"usage_read"}`,
			wantToken: &TokenSet{
				AccessToken:  "at",
				RefreshToken: "rt",
				ExpiresIn:    3600,
				Scope:        "usage_read",
				ClientID:     "cid",
			},
			wantForm: url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {"cid"},
				"code":          {"auth-code"},
				"redirect_uri":  {"http://127.0.0.1:8400/callback"},
				"code_verifier": {"verifier"},
			},
			wantURL: "https://api.datadoghq.com/oauth2/v1/token",
		},
		{
			name:         "400 invalid_grant",
			site:         "datadoghq.com",
			clientID:     "cid",
			code:         "auth-code",
			redirectURI:  "http://127.0.0.1:8400/callback",
			codeVerifier: "verifier",
			status:       http.StatusBadRequest,
			body:         `{"error":"invalid_grant","error_description":"code is expired"}`,
			wantErr:      errAny,
		},
		{
			name:         "response without access token",
			site:         "datadoghq.com",
			clientID:     "cid",
			code:         "auth-code",
			redirectURI:  "http://127.0.0.1:8400/callback",
			codeVerifier: "verifier",
			status:       http.StatusOK,
			body:         `{"token_type":"Bearer","expires_in":3600}`,
			wantErr:      ErrTokenIncomplete,
		},
		{
			name:         "broken json",
			site:         "datadoghq.com",
			clientID:     "cid",
			code:         "auth-code",
			redirectURI:  "http://127.0.0.1:8400/callback",
			codeVerifier: "verifier",
			status:       http.StatusOK,
			body:         `not json`,
			wantErr:      errAny,
		},
		{name: "invalid site", site: "bad site", clientID: "cid", code: "c", redirectURI: "u", codeVerifier: "v", wantErr: ErrInvalidSite},
		{name: "empty client id", site: "datadoghq.com", code: "c", redirectURI: "u", codeVerifier: "v", wantErr: errAny},
		{name: "empty code", site: "datadoghq.com", clientID: "cid", redirectURI: "u", codeVerifier: "v", wantErr: errAny},
		{name: "empty redirect uri", site: "datadoghq.com", clientID: "cid", code: "c", codeVerifier: "v", wantErr: errAny},
		{name: "empty code verifier", site: "datadoghq.com", clientID: "cid", code: "c", redirectURI: "u", wantErr: errAny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, observed := newStubServer(t, tt.status, tt.body)
			client, tr := newTestClient(t, srv)
			before := time.Now().UTC()
			got, err := client.ExchangeCode(context.Background(), tt.site, tt.clientID, tt.code, tt.redirectURI, tt.codeVerifier)
			after := time.Now().UTC()

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ExchangeCode() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("ExchangeCode() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExchangeCode() err = %v", err)
			}
			assertIssuedAtBetween(t, got, before, after)

			want := *tt.wantToken
			want.IssuedAt = got.IssuedAt
			if diff := cmp.Diff(&want, got); diff != "" {
				t.Errorf("token mismatch (-want +got):\n%s", diff)
			}
			o := receiveObservation(t, observed)
			if diff := cmp.Diff(tt.wantForm, parseObservedForm(t, o)); diff != "" {
				t.Errorf("request form mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{tt.wantURL}, tr.requested()); diff != "" {
				t.Errorf("requested URL mismatch (-want +got):\n%s", diff)
			}
			if !strings.HasPrefix(o.contentType, "application/x-www-form-urlencoded") {
				t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", o.contentType)
			}
		})
	}
}

func TestRefresh(t *testing.T) {
	tests := []struct {
		name         string
		site         string
		clientID     string
		refreshToken string
		status       int
		body         string
		wantToken    *TokenSet
		wantForm     url.Values
		wantErr      error
	}{
		{
			name:         "refreshed with a new refresh token",
			site:         "datadoghq.com",
			clientID:     "cid",
			refreshToken: "old-rt",
			status:       http.StatusOK,
			body:         `{"access_token":"new-at","refresh_token":"new-rt","expires_in":3600,"scope":"usage_read"}`,
			wantToken: &TokenSet{
				AccessToken:  "new-at",
				RefreshToken: "new-rt",
				ExpiresIn:    3600,
				Scope:        "usage_read",
				ClientID:     "cid",
			},
			wantForm: url.Values{
				"grant_type":    {"refresh_token"},
				"client_id":     {"cid"},
				"refresh_token": {"old-rt"},
			},
		},
		{
			name:         "omitted refresh token is carried over",
			site:         "datadoghq.com",
			clientID:     "cid",
			refreshToken: "old-rt",
			status:       http.StatusOK,
			body:         `{"access_token":"new-at","expires_in":3600}`,
			wantToken: &TokenSet{
				AccessToken:  "new-at",
				RefreshToken: "old-rt",
				ExpiresIn:    3600,
				ClientID:     "cid",
			},
			wantForm: url.Values{
				"grant_type":    {"refresh_token"},
				"client_id":     {"cid"},
				"refresh_token": {"old-rt"},
			},
		},
		{
			name:         "401 invalid_grant",
			site:         "datadoghq.com",
			clientID:     "cid",
			refreshToken: "old-rt",
			status:       http.StatusUnauthorized,
			body:         `{"error":"invalid_grant"}`,
			wantErr:      errAny,
		},
		{name: "invalid site", site: "bad site", clientID: "cid", refreshToken: "rt", wantErr: ErrInvalidSite},
		{name: "empty client id", site: "datadoghq.com", refreshToken: "rt", wantErr: errAny},
		{name: "empty refresh token", site: "datadoghq.com", clientID: "cid", wantErr: errAny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, observed := newStubServer(t, tt.status, tt.body)
			client, _ := newTestClient(t, srv)
			before := time.Now().UTC()
			got, err := client.Refresh(context.Background(), tt.site, tt.clientID, tt.refreshToken)
			after := time.Now().UTC()

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Refresh() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("Refresh() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Refresh() err = %v", err)
			}
			assertIssuedAtBetween(t, got, before, after)

			want := *tt.wantToken
			want.IssuedAt = got.IssuedAt
			if diff := cmp.Diff(&want, got); diff != "" {
				t.Errorf("token mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantForm, parseObservedForm(t, receiveObservation(t, observed))); diff != "" {
				t.Errorf("request form mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRefreshUsesDefaultClient は、パッケージ関数がゼロ値の Client へ委譲している
// ことを確かめる。実ホストへは接続しないよう、検証で弾かれる site を使う。
func TestRefreshUsesDefaultClient(t *testing.T) {
	if _, err := Refresh(context.Background(), "bad site", "cid", "rt"); !errors.Is(err, ErrInvalidSite) {
		t.Errorf("Refresh() err = %v, want %v", err, ErrInvalidSite)
	}
	if _, err := ExchangeCode(context.Background(), "bad site", "cid", "code", "uri", "verifier"); !errors.Is(err, ErrInvalidSite) {
		t.Errorf("ExchangeCode() err = %v, want %v", err, ErrInvalidSite)
	}
}

// parseObservedForm は観測したリクエスト本文を form として解釈する。
func parseObservedForm(t *testing.T, o httpObservation) url.Values {
	t.Helper()
	if o.path != tokenPath {
		t.Errorf("request path = %q, want %q", o.path, tokenPath)
	}
	values, err := url.ParseQuery(string(o.body))
	if err != nil {
		t.Fatalf("parse request form: %v", err)
	}
	return values
}

func assertIssuedAtBetween(t *testing.T, tok *TokenSet, before, after time.Time) {
	t.Helper()
	if tok.IssuedAt.Before(before) || tok.IssuedAt.After(after) {
		t.Errorf("issued_at = %v, want between %v and %v", tok.IssuedAt, before, after)
	}
}
