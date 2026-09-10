package datadogauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRegisterClient(t *testing.T) {
	redirectURIs := []string{
		"http://127.0.0.1:8400/callback",
		"http://127.0.0.1:8089/api/datadog/auth/callback",
	}

	tests := []struct {
		name         string
		site         string
		clientName   string
		redirectURIs []string
		status       int
		body         string
		want         *ClientCredentials
		wantURL      string
		wantErr      error
	}{
		{
			name:         "registered",
			site:         "datadoghq.com",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			status:       http.StatusCreated,
			body:         `{"client_id":"cid-1","client_name":"thief","redirect_uris":["http://127.0.0.1:8400/callback","http://127.0.0.1:8089/api/datadog/auth/callback"]}`,
			want: &ClientCredentials{
				ClientID:     "cid-1",
				ClientName:   "thief",
				RedirectURIs: redirectURIs,
			},
			wantURL: "https://api.datadoghq.com/api/v2/oauth2/register",
		},
		{
			name:         "client secret is kept when returned",
			site:         "datadoghq.eu",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			status:       http.StatusCreated,
			body:         `{"client_id":"cid-2","client_secret":"sec","redirect_uris":["http://127.0.0.1:8400/callback"]}`,
			want: &ClientCredentials{
				ClientID:     "cid-2",
				ClientName:   ClientName,
				ClientSecret: "sec",
				RedirectURIs: []string{"http://127.0.0.1:8400/callback"},
			},
			wantURL: "https://api.datadoghq.eu/api/v2/oauth2/register",
		},
		{
			name:         "requested redirect uris fill in an omitted response field",
			site:         "datadoghq.com",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			status:       http.StatusCreated,
			body:         `{"client_id":"cid-3"}`,
			want: &ClientCredentials{
				ClientID:     "cid-3",
				ClientName:   ClientName,
				RedirectURIs: redirectURIs,
			},
			wantURL: "https://api.datadoghq.com/api/v2/oauth2/register",
		},
		{
			name:         "200 is not accepted",
			site:         "datadoghq.com",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			status:       http.StatusOK,
			body:         `{"client_id":"cid"}`,
			wantErr:      errAny,
		},
		{
			name:         "403 from the authorization server",
			site:         "datadoghq.com",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			status:       http.StatusForbidden,
			body:         `{"errors":["OAuth apps are not enabled"]}`,
			wantErr:      errAny,
		},
		{
			name:         "broken json",
			site:         "datadoghq.com",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			status:       http.StatusCreated,
			body:         `{"client_id":`,
			wantErr:      errAny,
		},
		{
			name:         "response without client id",
			site:         "datadoghq.com",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			status:       http.StatusCreated,
			body:         `{"redirect_uris":["http://127.0.0.1:8400/callback"]}`,
			wantErr:      ErrClientIncomplete,
		},
		{
			name:         "invalid site",
			site:         "../../etc/passwd",
			clientName:   ClientName,
			redirectURIs: redirectURIs,
			wantErr:      ErrInvalidSite,
		},
		{
			name:         "empty client name",
			site:         "datadoghq.com",
			redirectURIs: redirectURIs,
			wantErr:      errAny,
		},
		{
			name:       "empty redirect uris",
			site:       "datadoghq.com",
			clientName: ClientName,
			wantErr:    errAny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, observed := newStubServer(t, tt.status, tt.body)
			client, tr := newTestClient(t, srv)
			got, err := client.RegisterClient(context.Background(), tt.site, tt.clientName, tt.redirectURIs)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("RegisterClient() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("RegisterClient() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RegisterClient() err = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("credentials mismatch (-want +got):\n%s", diff)
			}

			o := receiveObservation(t, observed)
			if o.path != registerPath {
				t.Errorf("request path = %q, want %q", o.path, registerPath)
			}
			if !strings.HasPrefix(o.contentType, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", o.contentType)
			}
			var gotBody registrationRequest
			if err := json.Unmarshal(o.body, &gotBody); err != nil {
				t.Fatalf("unmarshal request body: %v", err)
			}
			wantBody := registrationRequest{
				ClientName:   tt.clientName,
				RedirectURIs: tt.redirectURIs,
				GrantTypes:   []string{"authorization_code", "refresh_token"},
			}
			if diff := cmp.Diff(wantBody, gotBody); diff != "" {
				t.Errorf("request body mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{tt.wantURL}, tr.requested()); diff != "" {
				t.Errorf("requested URL mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRegisterClientUsesDefaultClient は、パッケージ関数がゼロ値の Client へ
// 委譲していること (site の検証がそこでも効くこと) を確かめる。実ホストへは
// 接続しないよう、検証で弾かれる site を使う。
func TestRegisterClientUsesDefaultClient(t *testing.T) {
	if _, err := RegisterClient(context.Background(), "INVALID SITE", ClientName, []string{"http://127.0.0.1:8400/callback"}); !errors.Is(err, ErrInvalidSite) {
		t.Errorf("RegisterClient() err = %v, want %v", err, ErrInvalidSite)
	}
}
