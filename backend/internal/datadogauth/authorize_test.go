package datadogauth

import (
	"errors"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuildAuthorizationURL(t *testing.T) {
	pkce := PKCE{Verifier: "verifier", Challenge: "challenge", Method: codeChallengeMethodS256}

	tests := []struct {
		name        string
		site        string
		clientID    string
		redirectURI string
		state       string
		pkce        PKCE
		scopes      []string
		wantHost    string
		wantQuery   map[string]string
		wantErr     error
	}{
		{
			name:        "default scopes",
			site:        "datadoghq.com",
			clientID:    "cid",
			redirectURI: "http://127.0.0.1:8400/callback",
			state:       "st",
			pkce:        pkce,
			wantHost:    "app.datadoghq.com",
			wantQuery: map[string]string{
				"response_type":         "code",
				"client_id":             "cid",
				"redirect_uri":          "http://127.0.0.1:8400/callback",
				"state":                 "st",
				"scope":                 "usage_read",
				"code_challenge":        "challenge",
				"code_challenge_method": "S256",
			},
		},
		{
			name:        "explicit scopes are joined with a space",
			site:        "us3.datadoghq.com",
			clientID:    "cid",
			redirectURI: "http://127.0.0.1:8400/callback",
			state:       "st",
			pkce:        pkce,
			scopes:      []string{"usage_read", "metrics_read"},
			wantHost:    "app.us3.datadoghq.com",
			wantQuery: map[string]string{
				"scope": "usage_read metrics_read",
			},
		},
		{name: "invalid site", site: "../etc", clientID: "cid", redirectURI: "u", state: "st", pkce: pkce, wantErr: ErrInvalidSite},
		{name: "empty site", site: "", clientID: "cid", redirectURI: "u", state: "st", pkce: pkce, wantErr: ErrInvalidSite},
		{name: "empty client id", site: "datadoghq.com", redirectURI: "u", state: "st", pkce: pkce, wantErr: errAny},
		{name: "empty redirect uri", site: "datadoghq.com", clientID: "cid", state: "st", pkce: pkce, wantErr: errAny},
		{name: "empty state", site: "datadoghq.com", clientID: "cid", redirectURI: "u", pkce: pkce, wantErr: errAny},
		{name: "empty challenge", site: "datadoghq.com", clientID: "cid", redirectURI: "u", state: "st", wantErr: errAny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildAuthorizationURL(tt.site, tt.clientID, tt.redirectURI, tt.state, tt.pkce, tt.scopes)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("BuildAuthorizationURL() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("BuildAuthorizationURL() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildAuthorizationURL() err = %v", err)
			}

			u, perr := url.Parse(got)
			if perr != nil {
				t.Fatalf("parse built URL: %v", perr)
			}
			if u.Scheme != "https" {
				t.Errorf("scheme = %q, want https", u.Scheme)
			}
			if u.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tt.wantHost)
			}
			if u.Path != authorizePath {
				t.Errorf("path = %q, want %q", u.Path, authorizePath)
			}
			q := u.Query()
			for k, want := range tt.wantQuery {
				if got := q.Get(k); got != want {
					t.Errorf("query %q = %q, want %q", k, got, want)
				}
			}
		})
	}
}

// TestDefaultScopesIsACopy は、返したスライスへの書き込みがパッケージの既定値へ
// 波及しないことを確かめる。
func TestDefaultScopesIsACopy(t *testing.T) {
	got := DefaultScopes()
	if diff := cmp.Diff([]string{"usage_read"}, got); diff != "" {
		t.Fatalf("DefaultScopes() mismatch (-want +got):\n%s", diff)
	}
	got[0] = "tampered"
	if diff := cmp.Diff([]string{"usage_read"}, DefaultScopes()); diff != "" {
		t.Errorf("DefaultScopes() was mutated (-want +got):\n%s", diff)
	}
}

// errAny は「エラーになること自体」を期待するテストケースの目印。
var errAny = errors.New("any error")
