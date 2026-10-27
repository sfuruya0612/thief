package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	sdkdatadog "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

var testDatadogNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// testDatadogDeps は I/O を行わない Datadog の認証依存を返す。使わない関数は
// 呼ばれたらテストを失敗させる。
func testDatadogDeps() datadogauth.Deps {
	return datadogauth.Deps{
		LoadToken: func(string, string) (*datadogauth.TokenSet, bool, error) {
			return nil, false, nil
		},
		LoadClient: func(string, string) (*datadogauth.ClientCredentials, bool, error) {
			return nil, false, nil
		},
		RefreshToken: func(context.Context, string, string, string) (*datadogauth.TokenSet, error) {
			return nil, errors.New("refresh must not be called")
		},
		SaveToken: func(string, string, *datadogauth.TokenSet) error {
			return nil
		},
		Now: func() time.Time { return testDatadogNow },
	}
}

func TestDatadogAuth_UsesValidOAuthToken(t *testing.T) {
	deps := testDatadogDeps()
	deps.LoadToken = func(site, org string) (*datadogauth.TokenSet, bool, error) {
		if site != "datadoghq.com" || org != "sub" {
			t.Errorf("LoadToken(%q, %q), want datadoghq.com/sub", site, org)
		}
		return &datadogauth.TokenSet{AccessToken: "at", ExpiresIn: 3600, IssuedAt: testDatadogNow}, true, nil
	}
	auth := newDatadogAuth("datadoghq.com", "api", "app", deps)
	ctx, err := auth.Context(context.Background(), "sub")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if !ddclient.HasOAuthToken(ctx) {
		t.Error("the context should carry the OAuth access token")
	}
}

func TestDatadogAuth_RefreshesExpiredTokenAndSavesIt(t *testing.T) {
	var saved *datadogauth.TokenSet
	deps := testDatadogDeps()
	deps.LoadToken = func(string, string) (*datadogauth.TokenSet, bool, error) {
		return &datadogauth.TokenSet{
			AccessToken: "old", RefreshToken: "rt", ExpiresIn: 3600,
			IssuedAt: testDatadogNow.Add(-2 * time.Hour), ClientID: "cid",
		}, true, nil
	}
	deps.RefreshToken = func(_ context.Context, site, clientID, refreshToken string) (*datadogauth.TokenSet, error) {
		if site != "datadoghq.com" || clientID != "cid" || refreshToken != "rt" {
			t.Errorf("RefreshToken args = %q/%q/%q", site, clientID, refreshToken)
		}
		return &datadogauth.TokenSet{AccessToken: "new", ExpiresIn: 3600}, nil
	}
	deps.SaveToken = func(site, org string, tok *datadogauth.TokenSet) error {
		if site != "datadoghq.com" || org != "" {
			t.Errorf("SaveToken(%q, %q)", site, org)
		}
		saved = tok
		return nil
	}
	auth := newDatadogAuth("datadoghq.com", "", "", deps)
	ctx, err := auth.Context(context.Background(), "")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if !ddclient.HasOAuthToken(ctx) {
		t.Error("the context should carry the refreshed OAuth access token")
	}
	if saved == nil || saved.AccessTokenValue() != "new" {
		t.Errorf("saved token = %#v, want the refreshed token", saved)
	}
	if saved.RefreshTokenValue() != "rt" || saved.ClientID != "cid" {
		t.Errorf("saved token should inherit refresh_token and client_id: %#v", saved)
	}
}

func TestDatadogAuth_FallsBackToStaticKeysForParentOnly(t *testing.T) {
	auth := newDatadogAuth("datadoghq.com", "api", "app", testDatadogDeps())
	if _, err := auth.Context(context.Background(), ""); err != nil {
		t.Errorf("parent context: %v", err)
	}
	if _, err := auth.Context(context.Background(), "sub"); !errors.Is(err, ErrDatadogNoCredentials) {
		t.Errorf("sub organization error = %v, want ErrDatadogNoCredentials", err)
	}
}

func TestDatadogAuth_NoCredentials(t *testing.T) {
	auth := newDatadogAuth("datadoghq.com", "", "", testDatadogDeps())
	_, err := auth.Context(context.Background(), "")
	if !errors.Is(err, ErrDatadogNoCredentials) {
		t.Fatalf("err = %v, want ErrDatadogNoCredentials", err)
	}
	if !errors.Is(err, ErrDatadogNoCredentials) || err.Error() == "" {
		t.Errorf("the error should explain the missing credentials")
	}
}

// fakeDatadogAuth は datadogCall の OAuth と静的キーの切り替えを確かめるための認証。
type fakeDatadogAuth struct {
	staticKeys bool
	staticCall int
}

func (a *fakeDatadogAuth) Context(ctx context.Context, _ string) (context.Context, error) {
	return ddclient.NewOAuthContext(ctx, "oauth-token"), nil
}

func (a *fakeDatadogAuth) StaticContext(ctx context.Context) (context.Context, bool) {
	a.staticCall++
	if !a.staticKeys {
		return nil, false
	}
	return ddclient.NewContext(ctx, "api-key", "app-key"), true
}

func TestDatadogCall_FallsBackToStaticKeysForParentOnForbidden(t *testing.T) {
	auth := &fakeDatadogAuth{staticKeys: true}
	var calls int
	got, err := datadogCall(context.Background(), auth, "", func(ctx context.Context) (string, error) {
		calls++
		if ddclient.HasOAuthToken(ctx) {
			return "", sdkdatadog.GenericOpenAPIError{ErrorMessage: "403 Forbidden"}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("datadogCall: %v", err)
	}
	if got != "ok" || calls != 2 {
		t.Errorf("got=%q calls=%d, want ok/2", got, calls)
	}
	if auth.staticCall != 1 {
		t.Errorf("StaticContext calls = %d, want 1", auth.staticCall)
	}
}

func TestDatadogCall_DoesNotFallBackForSubOrganization(t *testing.T) {
	auth := &fakeDatadogAuth{staticKeys: true}
	_, err := datadogCall(context.Background(), auth, "sub", func(context.Context) (string, error) {
		return "", sdkdatadog.GenericOpenAPIError{ErrorMessage: "403 Forbidden"}
	})
	if err == nil {
		t.Fatal("a sub organization must not fall back to the static keys")
	}
	if auth.staticCall != 0 {
		t.Errorf("StaticContext calls = %d, want 0", auth.staticCall)
	}
}

func TestDatadogCall_DoesNotFallBackWithoutStaticKeys(t *testing.T) {
	auth := &fakeDatadogAuth{}
	_, err := datadogCall(context.Background(), auth, "", func(context.Context) (string, error) {
		return "", sdkdatadog.GenericOpenAPIError{ErrorMessage: "403 Forbidden"}
	})
	if err == nil {
		t.Fatal("the 403 error should be returned when no static keys exist")
	}
	if auth.staticCall != 1 {
		t.Errorf("StaticContext calls = %d, want 1", auth.staticCall)
	}
}

func TestDatadogAuth_BrokenTokenAndRefreshFailureFallBack(t *testing.T) {
	tests := []struct {
		name string
		deps func() datadogauth.Deps
	}{{
		name: "broken token",
		deps: func() datadogauth.Deps {
			d := testDatadogDeps()
			d.LoadToken = func(string, string) (*datadogauth.TokenSet, bool, error) {
				return nil, false, errors.New("invalid json")
			}
			return d
		},
	}, {
		name: "refresh failure",
		deps: func() datadogauth.Deps {
			d := testDatadogDeps()
			d.LoadToken = func(string, string) (*datadogauth.TokenSet, bool, error) {
				return &datadogauth.TokenSet{
					AccessToken: "old", RefreshToken: "rt", ExpiresIn: 3600,
					IssuedAt: testDatadogNow.Add(-2 * time.Hour), ClientID: "cid",
				}, true, nil
			}
			d.RefreshToken = func(context.Context, string, string, string) (*datadogauth.TokenSet, error) {
				return nil, errors.New("invalid_grant")
			}
			return d
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := newDatadogAuth("datadoghq.com", "api", "app", tt.deps())
			if _, err := auth.Context(context.Background(), ""); err != nil {
				t.Errorf("parent should fall back to the static keys: %v", err)
			}
			if _, err := auth.Context(context.Background(), "sub"); !errors.Is(err, ErrDatadogNoCredentials) {
				t.Errorf("sub organization error = %v, want ErrDatadogNoCredentials", err)
			}
		})
	}
}
