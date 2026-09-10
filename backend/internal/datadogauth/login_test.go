package datadogauth

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const (
	testSite        = "datadoghq.com"
	testCLIRedirect = "http://127.0.0.1:8400/callback"
	testSrvRedirect = "http://127.0.0.1:8089/api/datadog/auth/callback"
)

// fakeAuth は Deps の差し替え先。保存はメモリ上の map で行い、Datadog への接続は
// 呼び出し内容の記録に置き換える。
type fakeAuth struct {
	clients map[string]*ClientCredentials
	tokens  map[string]*TokenSet

	// registered は RegisterClient に渡された redirect_uris の一覧 (呼び出しごとに 1 要素)。
	registered   [][]string
	registerResp *ClientCredentials
	registerErr  error

	exchanged   []exchangeArgs
	exchangeTok *TokenSet
	exchangeErr error

	refreshed  []refreshArgs
	refreshTok *TokenSet
	refreshErr error

	loadClientErr   error
	saveClientErr   error
	loadTokenErr    error
	saveTokenErr    error
	deleteTokenErr  error
	deleteClientErr error

	savedTokens   int
	deletedTokens int
	deletedClient int

	now time.Time
}

type exchangeArgs struct {
	site, clientID, code, redirectURI, verifier string
}

type refreshArgs struct {
	site, clientID, refreshToken string
}

func newFakeAuth() *fakeAuth {
	return &fakeAuth{
		clients: map[string]*ClientCredentials{},
		tokens:  map[string]*TokenSet{},
		now:     testNow,
	}
}

func (f *fakeAuth) deps() Deps {
	return Deps{
		RegisterClient: func(_ context.Context, _, clientName string, redirectURIs []string) (*ClientCredentials, error) {
			f.registered = append(f.registered, redirectURIs)
			if f.registerErr != nil {
				return nil, f.registerErr
			}
			if f.registerResp != nil {
				return f.registerResp, nil
			}
			return &ClientCredentials{ClientID: "registered-cid", ClientName: clientName, RedirectURIs: redirectURIs}, nil
		},
		ExchangeCode: func(_ context.Context, site, clientID, code, redirectURI, verifier string) (*TokenSet, error) {
			f.exchanged = append(f.exchanged, exchangeArgs{site: site, clientID: clientID, code: code, redirectURI: redirectURI, verifier: verifier})
			if f.exchangeErr != nil {
				return nil, f.exchangeErr
			}
			if f.exchangeTok != nil {
				return f.exchangeTok, nil
			}
			return &TokenSet{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600}, nil
		},
		RefreshToken: func(_ context.Context, site, clientID, refreshToken string) (*TokenSet, error) {
			f.refreshed = append(f.refreshed, refreshArgs{site: site, clientID: clientID, refreshToken: refreshToken})
			if f.refreshErr != nil {
				return nil, f.refreshErr
			}
			if f.refreshTok != nil {
				return f.refreshTok, nil
			}
			return &TokenSet{AccessToken: "new-at", RefreshToken: "new-rt", ExpiresIn: 3600}, nil
		},
		LoadClient: func(site string) (*ClientCredentials, bool, error) {
			if f.loadClientErr != nil {
				return nil, false, f.loadClientErr
			}
			creds, ok := f.clients[site]
			return creds, ok, nil
		},
		SaveClient: func(site string, creds *ClientCredentials) error {
			if f.saveClientErr != nil {
				return f.saveClientErr
			}
			f.clients[site] = creds
			return nil
		},
		LoadToken: func(site string) (*TokenSet, bool, error) {
			if f.loadTokenErr != nil {
				return nil, false, f.loadTokenErr
			}
			tok, ok := f.tokens[site]
			return tok, ok, nil
		},
		SaveToken: func(site string, tok *TokenSet) error {
			f.savedTokens++
			if f.saveTokenErr != nil {
				return f.saveTokenErr
			}
			f.tokens[site] = tok
			return nil
		},
		DeleteToken: func(site string) error {
			f.deletedTokens++
			delete(f.tokens, site)
			return f.deleteTokenErr
		},
		DeleteClient: func(site string) error {
			f.deletedClient++
			delete(f.clients, site)
			return f.deleteClientErr
		},
		Now: func() time.Time { return f.now },
	}
}

func testPrepareParams() PrepareParams {
	return PrepareParams{
		Site:                 testSite,
		RedirectURI:          testCLIRedirect,
		RegisterRedirectURIs: []string{testCLIRedirect, testSrvRedirect},
	}
}

func TestPrepareLogin(t *testing.T) {
	storedOK := &ClientCredentials{ClientID: "stored-cid", ClientName: ClientName, RedirectURIs: []string{testCLIRedirect, testSrvRedirect}}
	storedOther := &ClientCredentials{ClientID: "stored-cid", ClientName: ClientName, RedirectURIs: []string{testSrvRedirect}}

	tests := []struct {
		name         string
		params       PrepareParams
		setup        func(*fakeAuth)
		wantClientID string
		wantRegister bool
		wantSaved    bool
		wantErr      error
	}{
		{
			name:         "registers a client when none is stored",
			params:       testPrepareParams(),
			wantClientID: "registered-cid",
			wantRegister: true,
			wantSaved:    true,
		},
		{
			name:   "reuses the stored client",
			params: testPrepareParams(),
			setup: func(f *fakeAuth) {
				f.clients[testSite] = storedOK
			},
			wantClientID: "stored-cid",
		},
		{
			name:   "does not re-register when the stored client lacks the redirect URI",
			params: testPrepareParams(),
			setup: func(f *fakeAuth) {
				f.clients[testSite] = storedOther
			},
			wantErr: ErrRedirectURINotRegistered,
		},
		{
			name:   "registration returning no client id is rejected",
			params: testPrepareParams(),
			setup: func(f *fakeAuth) {
				f.registerResp = &ClientCredentials{RedirectURIs: []string{testCLIRedirect}}
			},
			wantRegister: true,
			wantErr:      ErrClientIncomplete,
		},
		{
			name:   "load error is propagated",
			params: testPrepareParams(),
			setup: func(f *fakeAuth) {
				f.loadClientErr = errAny
			},
			wantErr: errAny,
		},
		{
			name:   "registration error is propagated",
			params: testPrepareParams(),
			setup: func(f *fakeAuth) {
				f.registerErr = errAny
			},
			wantRegister: true,
			wantErr:      errAny,
		},
		{
			name:   "save error is propagated",
			params: testPrepareParams(),
			setup: func(f *fakeAuth) {
				f.saveClientErr = errAny
			},
			wantRegister: true,
			wantErr:      errAny,
		},
		{
			name:    "invalid site",
			params:  PrepareParams{Site: "../evil", RedirectURI: testCLIRedirect, RegisterRedirectURIs: []string{testCLIRedirect}},
			wantErr: ErrInvalidSite,
		},
		{
			name:    "empty redirect uri",
			params:  PrepareParams{Site: testSite, RegisterRedirectURIs: []string{testCLIRedirect}},
			wantErr: errAny,
		},
		{
			name:    "redirect uri is not among the URIs to register",
			params:  PrepareParams{Site: testSite, RedirectURI: testCLIRedirect, RegisterRedirectURIs: []string{testSrvRedirect}},
			wantErr: errAny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeAuth()
			if tt.setup != nil {
				tt.setup(f)
			}
			got, err := PrepareLogin(context.Background(), tt.params, f.deps())

			if wantCalls := boolToInt(tt.wantRegister); len(f.registered) != wantCalls {
				t.Errorf("RegisterClient calls = %d, want %d", len(f.registered), wantCalls)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("PrepareLogin() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("PrepareLogin() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareLogin() err = %v", err)
			}
			if got.ClientID != tt.wantClientID {
				t.Errorf("client id = %q, want %q", got.ClientID, tt.wantClientID)
			}
			if got.Site != tt.params.Site {
				t.Errorf("site = %q, want %q", got.Site, tt.params.Site)
			}
			if got.RedirectURI != tt.params.RedirectURI {
				t.Errorf("redirect URI = %q, want %q", got.RedirectURI, tt.params.RedirectURI)
			}
			if got.State == "" {
				t.Error("state is empty")
			}
			if got.verifier.value() == "" {
				t.Error("code verifier is empty")
			}
			if tt.wantSaved {
				if _, ok := f.clients[tt.params.Site]; !ok {
					t.Error("the registered client was not saved")
				}
				if diff := cmp.Diff([][]string{tt.params.RegisterRedirectURIs}, f.registered); diff != "" {
					t.Errorf("registered redirect URIs mismatch (-want +got):\n%s", diff)
				}
			}

			// 認可 URL が Login の値と PKCE の対応を保っていることを確かめる。
			u, perr := url.Parse(got.AuthorizationURL)
			if perr != nil {
				t.Fatalf("parse authorization URL: %v", perr)
			}
			q := u.Query()
			for _, c := range []struct{ key, want string }{
				{"client_id", got.ClientID},
				{"state", got.State},
				{"redirect_uri", got.RedirectURI},
				{"code_challenge", codeChallengeS256(got.verifier.value())},
				{"code_challenge_method", "S256"},
			} {
				if v := q.Get(c.key); v != c.want {
					t.Errorf("authorization URL %s = %q, want %q", c.key, v, c.want)
				}
			}
		})
	}
}

// TestPrepareLoginRegistersOnlyOnce は、2 回目以降のログインで Dynamic Client
// Registration を繰り返さないこと (既存トークンを無効化しないこと) を確かめる。
func TestPrepareLoginRegistersOnlyOnce(t *testing.T) {
	f := newFakeAuth()
	deps := f.deps()

	first, err := PrepareLogin(context.Background(), testPrepareParams(), deps)
	if err != nil {
		t.Fatalf("PrepareLogin() 1st err = %v", err)
	}
	second, err := PrepareLogin(context.Background(), testPrepareParams(), deps)
	if err != nil {
		t.Fatalf("PrepareLogin() 2nd err = %v", err)
	}

	if len(f.registered) != 1 {
		t.Errorf("RegisterClient calls = %d, want 1", len(f.registered))
	}
	if first.ClientID != second.ClientID {
		t.Errorf("client id changed: %q -> %q", first.ClientID, second.ClientID)
	}
	if first.State == second.State {
		t.Error("state was reused between logins, want a fresh value")
	}
	if first.verifier.value() == second.verifier.value() {
		t.Error("code verifier was reused between logins, want a fresh value")
	}
	// CLI とサーバの両方の redirect_uri が 1 回の登録でまとめて登録される。
	if diff := cmp.Diff([]string{testCLIRedirect, testSrvRedirect}, f.registered[0]); diff != "" {
		t.Errorf("registered redirect URIs mismatch (-want +got):\n%s", diff)
	}
}

func TestCompleteLogin(t *testing.T) {
	newLogin := func() *Login {
		return &Login{
			Site:        testSite,
			ClientID:    "cid",
			RedirectURI: testCLIRedirect,
			State:       "state-value",
			verifier:    "verifier-value",
		}
	}

	tests := []struct {
		name         string
		login        *Login
		state        string
		code         string
		setup        func(*fakeAuth)
		wantExchange bool
		wantSaved    bool
		wantErr      error
	}{
		{
			name:         "exchanged and saved",
			login:        newLogin(),
			state:        "state-value",
			code:         "auth-code",
			wantExchange: true,
			wantSaved:    true,
		},
		{
			name:    "state mismatch",
			login:   newLogin(),
			state:   "other-state",
			code:    "auth-code",
			wantErr: ErrStateMismatch,
		},
		{
			name:    "empty state",
			login:   newLogin(),
			code:    "auth-code",
			wantErr: ErrStateMismatch,
		},
		{
			name:    "empty code",
			login:   newLogin(),
			state:   "state-value",
			wantErr: errAny,
		},
		{
			name:    "nil login",
			state:   "state-value",
			code:    "auth-code",
			wantErr: ErrLoginIncomplete,
		},
		{
			name:    "login without a verifier",
			login:   &Login{Site: testSite, ClientID: "cid", RedirectURI: testCLIRedirect, State: "state-value"},
			state:   "state-value",
			code:    "auth-code",
			wantErr: ErrLoginIncomplete,
		},
		{
			name:         "exchange error",
			login:        newLogin(),
			state:        "state-value",
			code:         "auth-code",
			setup:        func(f *fakeAuth) { f.exchangeErr = errAny },
			wantExchange: true,
			wantErr:      errAny,
		},
		{
			name:         "token without an access token",
			login:        newLogin(),
			state:        "state-value",
			code:         "auth-code",
			setup:        func(f *fakeAuth) { f.exchangeTok = &TokenSet{ExpiresIn: 3600} },
			wantExchange: true,
			wantErr:      ErrTokenIncomplete,
		},
		{
			name:         "save error",
			login:        newLogin(),
			state:        "state-value",
			code:         "auth-code",
			setup:        func(f *fakeAuth) { f.saveTokenErr = errAny },
			wantExchange: true,
			wantErr:      errAny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeAuth()
			if tt.setup != nil {
				tt.setup(f)
			}
			got, err := CompleteLogin(context.Background(), tt.login, tt.state, tt.code, f.deps())

			if wantCalls := boolToInt(tt.wantExchange); len(f.exchanged) != wantCalls {
				t.Errorf("ExchangeCode calls = %d, want %d", len(f.exchanged), wantCalls)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("CompleteLogin() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("CompleteLogin() err = %v, want %v", err, tt.wantErr)
				}
				if _, ok := f.tokens[testSite]; ok {
					t.Error("a token was stored although the login failed")
				}
				return
			}
			if err != nil {
				t.Fatalf("CompleteLogin() err = %v", err)
			}

			want := exchangeArgs{site: testSite, clientID: "cid", code: tt.code, redirectURI: testCLIRedirect, verifier: "verifier-value"}
			if diff := cmp.Diff(want, f.exchanged[0], cmp.AllowUnexported(exchangeArgs{})); diff != "" {
				t.Errorf("ExchangeCode args mismatch (-want +got):\n%s", diff)
			}
			if tt.wantSaved {
				stored, ok := f.tokens[testSite]
				if !ok {
					t.Fatal("the token was not saved")
				}
				if stored != got {
					t.Error("the returned token is not the saved one")
				}
			}
			// IssuedAt と ClientID は保存の前に補われる。
			if !got.IssuedAt.Equal(testNow) {
				t.Errorf("issued_at = %v, want %v", got.IssuedAt, testNow)
			}
			if got.ClientID != "cid" {
				t.Errorf("client id = %q, want %q", got.ClientID, "cid")
			}
		})
	}
}

// TestCompleteLoginKeepsIssuedAtFromTheTokenEndpoint は、トークンエンドポイントの応答が
// 持つ発行時刻を Now() で上書きしないことを確かめる。
func TestCompleteLoginKeepsIssuedAtFromTheTokenEndpoint(t *testing.T) {
	issued := testNow.Add(-time.Minute)
	f := newFakeAuth()
	f.exchangeTok = &TokenSet{AccessToken: "at", ExpiresIn: 3600, IssuedAt: issued, ClientID: "response-cid"}

	got, err := CompleteLogin(context.Background(), &Login{Site: testSite, ClientID: "cid", RedirectURI: testCLIRedirect, State: "st", verifier: "v"}, "st", "code", f.deps())
	if err != nil {
		t.Fatalf("CompleteLogin() err = %v", err)
	}
	if !got.IssuedAt.Equal(issued) {
		t.Errorf("issued_at = %v, want %v", got.IssuedAt, issued)
	}
	if got.ClientID != "response-cid" {
		t.Errorf("client id = %q, want %q", got.ClientID, "response-cid")
	}
}

func TestEnsureFreshToken(t *testing.T) {
	valid := &TokenSet{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600, IssuedAt: testNow, ClientID: "cid"}
	expired := &TokenSet{AccessToken: "old-at", RefreshToken: "old-rt", ExpiresIn: 3600, IssuedAt: testNow.Add(-2 * time.Hour), ClientID: "cid"}

	tests := []struct {
		name            string
		site            string
		setup           func(*fakeAuth)
		wantOK          bool
		wantAccessToken string
		wantRefresh     []refreshArgs
		wantSaved       int
		wantErr         error
	}{
		{
			name:   "not logged in",
			site:   testSite,
			wantOK: false,
		},
		{
			name:            "a valid token is returned as is",
			site:            testSite,
			setup:           func(f *fakeAuth) { f.tokens[testSite] = valid },
			wantOK:          true,
			wantAccessToken: "at",
		},
		{
			name:            "an expired token is refreshed",
			site:            testSite,
			setup:           func(f *fakeAuth) { f.tokens[testSite] = expired },
			wantOK:          true,
			wantAccessToken: "new-at",
			wantRefresh:     []refreshArgs{{site: testSite, clientID: "cid", refreshToken: "old-rt"}},
			wantSaved:       1,
		},
		{
			name: "a token inside the early expiry buffer is refreshed",
			site: testSite,
			setup: func(f *fakeAuth) {
				// 期限まで 299 秒 (バッファは 300 秒)。
				f.tokens[testSite] = &TokenSet{AccessToken: "at", RefreshToken: "old-rt", ExpiresIn: 3600, IssuedAt: testNow.Add(-3601*time.Second + 300*time.Second), ClientID: "cid"}
			},
			wantOK:          true,
			wantAccessToken: "new-at",
			wantRefresh:     []refreshArgs{{site: testSite, clientID: "cid", refreshToken: "old-rt"}},
			wantSaved:       1,
		},
		{
			name: "the client id falls back to the stored client registration",
			site: testSite,
			setup: func(f *fakeAuth) {
				f.tokens[testSite] = &TokenSet{AccessToken: "old-at", RefreshToken: "old-rt", ExpiresIn: 3600, IssuedAt: testNow.Add(-2 * time.Hour)}
				f.clients[testSite] = &ClientCredentials{ClientID: "stored-cid", RedirectURIs: []string{testCLIRedirect}}
			},
			wantOK:          true,
			wantAccessToken: "new-at",
			wantRefresh:     []refreshArgs{{site: testSite, clientID: "stored-cid", refreshToken: "old-rt"}},
			wantSaved:       1,
		},
		{
			name: "no client id anywhere",
			site: testSite,
			setup: func(f *fakeAuth) {
				f.tokens[testSite] = &TokenSet{AccessToken: "old-at", RefreshToken: "old-rt", ExpiresIn: 3600, IssuedAt: testNow.Add(-2 * time.Hour)}
			},
			wantErr: ErrClientIncomplete,
		},
		{
			name: "an expired token without a refresh token",
			site: testSite,
			setup: func(f *fakeAuth) {
				f.tokens[testSite] = &TokenSet{AccessToken: "old-at", ExpiresIn: 3600, IssuedAt: testNow.Add(-2 * time.Hour), ClientID: "cid"}
			},
			wantErr: ErrRefreshTokenMissing,
		},
		{
			name: "refresh failure is an error, not a silent logout",
			site: testSite,
			setup: func(f *fakeAuth) {
				f.tokens[testSite] = expired
				f.refreshErr = errAny
			},
			wantRefresh: []refreshArgs{{site: testSite, clientID: "cid", refreshToken: "old-rt"}},
			wantErr:     errAny,
		},
		{
			name: "a refreshed token without an access token",
			site: testSite,
			setup: func(f *fakeAuth) {
				f.tokens[testSite] = expired
				f.refreshTok = &TokenSet{ExpiresIn: 3600}
			},
			wantRefresh: []refreshArgs{{site: testSite, clientID: "cid", refreshToken: "old-rt"}},
			wantErr:     ErrTokenIncomplete,
		},
		{
			name: "save failure after a refresh",
			site: testSite,
			setup: func(f *fakeAuth) {
				f.tokens[testSite] = expired
				f.saveTokenErr = errAny
			},
			wantRefresh: []refreshArgs{{site: testSite, clientID: "cid", refreshToken: "old-rt"}},
			wantSaved:   1,
			wantErr:     errAny,
		},
		{
			name:    "a broken stored token is an error",
			site:    testSite,
			setup:   func(f *fakeAuth) { f.loadTokenErr = errAny },
			wantErr: errAny,
		},
		{
			name:    "invalid site",
			site:    "../evil",
			wantErr: ErrInvalidSite,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeAuth()
			if tt.setup != nil {
				tt.setup(f)
			}
			got, ok, err := EnsureFreshToken(context.Background(), tt.site, f.deps())

			if diff := cmp.Diff(tt.wantRefresh, f.refreshed, cmp.AllowUnexported(refreshArgs{})); diff != "" {
				t.Errorf("RefreshToken calls mismatch (-want +got):\n%s", diff)
			}
			if f.savedTokens != tt.wantSaved {
				t.Errorf("SaveToken calls = %d, want %d", f.savedTokens, tt.wantSaved)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("EnsureFreshToken() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("EnsureFreshToken() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("EnsureFreshToken() err = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("EnsureFreshToken() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				if got != nil {
					t.Errorf("EnsureFreshToken() token = %v, want nil", got)
				}
				return
			}
			if got.AccessTokenValue() != tt.wantAccessToken {
				t.Errorf("access token = %q, want %q", got.AccessTokenValue(), tt.wantAccessToken)
			}
		})
	}
}

// TestEnsureFreshTokenCarriesOverTheRefreshToken は、更新の応答が refresh_token を
// 省いた場合 (RFC 6749 §6 で OPTIONAL) に元の値を引き継ぐことを確かめる。
func TestEnsureFreshTokenCarriesOverTheRefreshToken(t *testing.T) {
	f := newFakeAuth()
	f.tokens[testSite] = &TokenSet{AccessToken: "old-at", RefreshToken: "old-rt", ExpiresIn: 3600, IssuedAt: testNow.Add(-2 * time.Hour), ClientID: "cid"}
	f.refreshTok = &TokenSet{AccessToken: "new-at", ExpiresIn: 3600}

	got, ok, err := EnsureFreshToken(context.Background(), testSite, f.deps())
	if err != nil || !ok {
		t.Fatalf("EnsureFreshToken() = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if got.RefreshTokenValue() != "old-rt" {
		t.Errorf("refresh token = %q, want %q", got.RefreshTokenValue(), "old-rt")
	}
	if got.ClientID != "cid" {
		t.Errorf("client id = %q, want %q", got.ClientID, "cid")
	}
	if !got.IssuedAt.Equal(testNow) {
		t.Errorf("issued_at = %v, want %v", got.IssuedAt, testNow)
	}
	if stored := f.tokens[testSite]; stored != got {
		t.Error("the refreshed token was not saved")
	}
}

func TestLogout(t *testing.T) {
	tests := []struct {
		name        string
		site        string
		setup       func(*fakeAuth)
		wantDeletes int
		wantErr     error
	}{
		{
			name: "both files are removed",
			site: testSite,
			setup: func(f *fakeAuth) {
				f.tokens[testSite] = &TokenSet{AccessToken: "at"}
				f.clients[testSite] = &ClientCredentials{ClientID: "cid", RedirectURIs: []string{testCLIRedirect}}
			},
			wantDeletes: 1,
		},
		{
			name:        "removing a missing credential is not an error",
			site:        testSite,
			wantDeletes: 1,
		},
		{
			// 片方が失敗しても、もう片方の削除は試みる。
			name:        "the client is still removed when removing the token fails",
			site:        testSite,
			setup:       func(f *fakeAuth) { f.deleteTokenErr = errAny },
			wantDeletes: 1,
			wantErr:     errAny,
		},
		{
			name:        "the token is still removed when removing the client fails",
			site:        testSite,
			setup:       func(f *fakeAuth) { f.deleteClientErr = errAny },
			wantDeletes: 1,
			wantErr:     errAny,
		},
		{
			name:    "invalid site",
			site:    "../evil",
			wantErr: ErrInvalidSite,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeAuth()
			if tt.setup != nil {
				tt.setup(f)
			}
			err := Logout(tt.site, f.deps())

			if f.deletedTokens != tt.wantDeletes {
				t.Errorf("DeleteToken calls = %d, want %d", f.deletedTokens, tt.wantDeletes)
			}
			if f.deletedClient != tt.wantDeletes {
				t.Errorf("DeleteClient calls = %d, want %d", f.deletedClient, tt.wantDeletes)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Logout() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("Logout() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Logout() err = %v", err)
			}
			if len(f.tokens) != 0 || len(f.clients) != 0 {
				t.Errorf("credentials are left after logout: tokens=%d clients=%d", len(f.tokens), len(f.clients))
			}
		})
	}
}

// TestDefaultDepsIsFilled は本番用の Deps に未設定のフィールドが無いことを確かめる。
// nil の関数値は呼び出し時の panic になるため、構築の時点で検出する。
func TestDefaultDepsIsFilled(t *testing.T) {
	d := DefaultDeps()
	checks := map[string]bool{
		"RegisterClient": d.RegisterClient == nil,
		"ExchangeCode":   d.ExchangeCode == nil,
		"RefreshToken":   d.RefreshToken == nil,
		"LoadClient":     d.LoadClient == nil,
		"SaveClient":     d.SaveClient == nil,
		"LoadToken":      d.LoadToken == nil,
		"SaveToken":      d.SaveToken == nil,
		"DeleteToken":    d.DeleteToken == nil,
		"DeleteClient":   d.DeleteClient == nil,
		"Now":            d.Now == nil,
	}
	for name, isNil := range checks {
		if isNil {
			t.Errorf("DefaultDeps().%s is nil", name)
		}
	}
}

// TestDefaultDepsUsesTheConfigDir は、本番用の Deps が config.Dir()/datadog 配下へ
// 読み書きすることを確かめる。Datadog への接続を伴わない保存系だけを対象にする。
func TestDefaultDepsUsesTheConfigDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	d := DefaultDeps()

	tok := &TokenSet{AccessToken: "at", ExpiresIn: 3600, IssuedAt: testNow}
	if err := d.SaveToken(testSite, tok); err != nil {
		t.Fatalf("SaveToken() err = %v", err)
	}
	got, ok, err := d.LoadToken(testSite)
	if err != nil || !ok {
		t.Fatalf("LoadToken() = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if got.AccessTokenValue() != "at" {
		t.Errorf("access token = %q, want %q", got.AccessTokenValue(), "at")
	}

	creds := &ClientCredentials{ClientID: "cid", RedirectURIs: []string{testCLIRedirect}}
	if err := d.SaveClient(testSite, creds); err != nil {
		t.Fatalf("SaveClient() err = %v", err)
	}
	if _, ok, err := d.LoadClient(testSite); err != nil || !ok {
		t.Fatalf("LoadClient() = (_, %v, %v), want (_, true, nil)", ok, err)
	}

	if err := d.DeleteToken(testSite); err != nil {
		t.Fatalf("DeleteToken() err = %v", err)
	}
	if err := d.DeleteClient(testSite); err != nil {
		t.Fatalf("DeleteClient() err = %v", err)
	}
	if _, ok, _ := d.LoadToken(testSite); ok {
		t.Error("the token is still readable after DeleteToken")
	}
	if _, ok, _ := d.LoadClient(testSite); ok {
		t.Error("the client is still readable after DeleteClient")
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
