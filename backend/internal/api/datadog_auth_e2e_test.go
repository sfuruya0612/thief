package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

const (
	datadogTestServerRedirectURI = "http://127.0.0.1:8089/api/datadog/auth/callback"
	datadogTestCLIRedirectURI    = "http://127.0.0.1:8400/callback"
)

// datadogRedirectTransport は本番コードが組み立てた https://api.{site}/... 宛の
// リクエストを httptest のサーバへ向け直す。URL ではなく HTTP クライアントを差し替える
// ことで、URL の組み立て自体をテストの対象に残す (internal/datadogauth のテストと同じ形)。
type datadogRedirectTransport struct {
	base *url.URL
}

func (t *datadogRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = t.base.Scheme
	r.URL.Host = t.base.Host
	r.Host = ""
	return http.DefaultTransport.RoundTrip(r)
}

// datadogFakeOrg は Datadog の OAuth エンドポイント (Dynamic Client Registration と
// トークン) および Usage Metering を模したサーバ群。
type datadogFakeOrg struct {
	mu sync.Mutex

	registrations int
	exchanges     int
	// usageAuth には Usage Metering が受け取った資格情報が呼び出しごとに積まれる。
	usageAuth []string
}

const datadogFakeCostBody = `{
  "data": [
    {
      "id": "1",
      "type": "cost_by_org",
      "attributes": {
        "date": "2026-09-01T00:00:00+00:00",
        "org_name": "example-org",
        "charges": [{"product_name": "infrastructure", "charge_type": "on_demand", "cost": 42.5}]
      }
    }
  ]
}`

// start は OAuth 用と Usage Metering 用の httptest サーバを立て、それらに接続する
// Server をルート登録済みで返す。
func (org *datadogFakeOrg) start(t *testing.T, dir string) *Server {
	t.Helper()

	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/oauth2/register":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read registration body: %v", err)
			}
			org.mu.Lock()
			org.registrations++
			clientID := fmt.Sprintf("generated-client-id-%d", org.registrations)
			org.mu.Unlock()
			// CLI とサーバの両方の redirect_uri を 1 回でまとめて登録すること。
			for _, want := range []string{datadogTestServerRedirectURI, datadogTestCLIRedirectURI} {
				if !strings.Contains(string(body), want) {
					t.Errorf("registration body = %s, want it to contain %q", body, want)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"client_id":"`+clientID+`","redirect_uris":["`+datadogTestServerRedirectURI+`","`+datadogTestCLIRedirectURI+`"]}`)
		case "/oauth2/v1/token":
			// アクセストークンは引き換えごとに変える。org ごとに別のトークンが
			// 保存されることを、値の違いで見分けられるようにするため。
			org.mu.Lock()
			org.exchanges++
			accessToken := fmt.Sprintf("oauth-access-token-%d", org.exchanges)
			org.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"access_token":"`+accessToken+`","refresh_token":"oauth-refresh-token","token_type":"Bearer","expires_in":3600,"scope":"usage_read"}`)
		default:
			t.Errorf("unexpected request to the datadog oauth server: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(oauthSrv.Close)

	usageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org.mu.Lock()
		org.usageAuth = append(org.usageAuth, datadogRequestCredentials(r))
		org.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, datadogFakeCostBody)
	}))
	t.Cleanup(usageSrv.Close)

	oauthURL := mustParseURL(t, oauthSrv.URL)
	usageURL := mustParseURL(t, usageSrv.URL)

	client := datadogauth.Client{HTTP: &http.Client{Transport: &datadogRedirectTransport{base: oauthURL}}}
	authDeps := datadogauth.Deps{
		RegisterClient: client.RegisterClient,
		ExchangeCode:   client.ExchangeCode,
		RefreshToken:   client.Refresh,
		LoadClient: func(site, o string) (*datadogauth.ClientCredentials, bool, error) {
			return datadogauth.LoadClient(dir, site, o)
		},
		SaveClient: func(site, o string, c *datadogauth.ClientCredentials) error {
			return datadogauth.SaveClient(dir, site, o, c)
		},
		LoadToken: func(site, o string) (*datadogauth.TokenSet, bool, error) {
			return datadogauth.LoadToken(dir, site, o)
		},
		SaveToken: func(site, o string, tok *datadogauth.TokenSet) error {
			return datadogauth.SaveToken(dir, site, o, tok)
		},
		DeleteToken:  func(site, o string) error { return datadogauth.DeleteToken(dir, site, o) },
		DeleteClient: func(site, o string) error { return datadogauth.DeleteClient(dir, site, o) },
		Now:          time.Now,
	}

	s := newTestServer(t)
	s.ddAuth = datadogAuthDepsFrom(authDeps)
	s.ddLoginSessions = newDatadogLoginSessionStore()
	ddCfg := ddclient.NewConfiguration(testDatadogSite)
	ddCfg.Host = usageURL.Host
	ddCfg.Scheme = usageURL.Scheme
	s.ddV2 = ddclient.NewUsageMeteringV2API(ddCfg)
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

func (org *datadogFakeOrg) registrationCount() int {
	org.mu.Lock()
	defer org.mu.Unlock()
	return org.registrations
}

func (org *datadogFakeOrg) usageCredentials() []string {
	org.mu.Lock()
	defer org.mu.Unlock()
	return append([]string(nil), org.usageAuth...)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

// datadogRequestCredentials は Usage Metering へのリクエストが提示した資格情報を 1 行に
// まとめる。OAuth と静的キーのどちらで呼んだか、両方が載っていないかを検証するために使う。
func datadogRequestCredentials(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	apiKey := r.Header.Get("DD-API-KEY")
	switch {
	case auth != "" && apiKey != "":
		return "both:" + auth + "+" + apiKey
	case auth != "":
		return auth
	case apiKey != "":
		return "keys:" + apiKey + "/" + r.Header.Get("DD-APPLICATION-KEY")
	default:
		return "none"
	}
}

// TestDatadogOAuthEndToEnd は、ログイン (Dynamic Client Registration からトークンの
// 引き換えまで) とコスト取得、ログアウト後のフォールバックを通しで検証する。
//
// issue 0165 の完了条件は本来、実際に OAuth でログインしてコストが取得できることの
// 手動確認を求めている。本実装環境には Datadog の認証情報も Organization の管理者権限も
// 無く実 Datadog に対するライブ検証ができないため、完了条件が定める代替条項に従い、
// トークンエンドポイントと Usage Metering をモックした本テストで代替する。
func TestDatadogOAuthEndToEnd(t *testing.T) {
	dir := t.TempDir()
	org := &datadogFakeOrg{}
	s := org.start(t, dir)

	// 1. CLI を一度も実行していない状態からのログイン。login/start が登録を行う。
	started := startDatadogLogin(t, s)
	if got := org.registrationCount(); got != 1 {
		t.Fatalf("client registrations = %d, want 1", got)
	}
	if _, ok, err := datadogauth.LoadClient(dir, testDatadogSite, ""); err != nil || !ok {
		t.Fatalf("the client registration was not stored: ok=%v err=%v", ok, err)
	}

	// 2. 認可サーバがブラウザをコールバックへ遷移させ、認可コードがトークンになる。
	w := doDatadogRequest(t, s, http.MethodGet, datadogCallbackTarget(started.State, "auth-code"))
	if w.Code != http.StatusOK {
		t.Fatalf("callback status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if got := getDatadogLoginStatus(t, s, started.State); got.Status != string(datadogLoginSucceeded) {
		t.Fatalf("login status = %q (error=%q), want %q", got.Status, got.ErrorMessage, datadogLoginSucceeded)
	}
	tok, ok, err := datadogauth.LoadToken(dir, testDatadogSite, "")
	if err != nil || !ok {
		t.Fatalf("the token was not stored: ok=%v err=%v", ok, err)
	}
	if tok.AccessTokenValue() != "oauth-access-token-1" {
		t.Errorf("stored access token = %q, want %q", tok.AccessTokenValue(), "oauth-access-token-1")
	}

	// 3. コスト取得が OAuth トークンで通る。
	costs := getDatadogHistoricalCost(t, s, "2026-09")
	if len(costs) != 1 || costs[0].Cost != 42.5 {
		t.Fatalf("costs = %+v, want a single 42.5 charge", costs)
	}
	if got := org.usageCredentials(); len(got) != 1 || got[0] != "Bearer oauth-access-token-1" {
		t.Fatalf("usage metering credentials = %v, want [Bearer oauth-access-token-1]", got)
	}

	// 4. ログアウトするとトークンが消え、静的キーが無いので明確なエラーになる。
	if w := doDatadogRequest(t, s, http.MethodPost, "/api/datadog/auth/logout"); w.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}
	if _, ok, _ := datadogauth.LoadToken(dir, testDatadogSite, ""); ok {
		t.Error("the token file still exists after logout")
	}
	w = doDatadogRequest(t, s, http.MethodGet, "/api/datadog/cost/historical?start_month=2026-10")
	assertErrorCode(t, w, http.StatusUnauthorized, "DATADOG_NO_CREDENTIALS")
	if !strings.Contains(w.Body.String(), "no usable Datadog credentials") {
		t.Errorf("error body = %s, want it to state that no credentials are usable", w.Body.String())
	}

	// 5. 静的キーだけの構成は OAuth を導入する前と同じように動く (非破壊性)。
	s.cfg.SetDatadogAPIKey("static-api-key")
	s.cfg.SetDatadogAppKey("static-app-key")
	if costs := getDatadogHistoricalCost(t, s, "2026-11"); len(costs) != 1 {
		t.Fatalf("costs after falling back to the static keys = %+v, want a single charge", costs)
	}
	got := org.usageCredentials()
	if len(got) != 2 || got[1] != "keys:static-api-key/static-app-key" {
		t.Fatalf("usage metering credentials = %v, want the second call to use the static keys", got)
	}
}

func datadogCallbackTarget(state, code string) string {
	q := url.Values{"state": {state}, "code": {code}}
	return "/api/datadog/auth/callback?" + q.Encode()
}

// getDatadogHistoricalCost は historical cost を取得して応答を復号する。
// start_month はキャッシュキーの一部なので、呼び出しごとに変えてキャッシュを避ける。
func getDatadogHistoricalCost(t *testing.T, s *Server, startMonth string) []ddclient.CostInfo {
	t.Helper()
	w := doDatadogRequest(t, s, http.MethodGet, "/api/datadog/cost/historical?start_month="+startMonth)
	if w.Code != http.StatusOK {
		t.Fatalf("cost status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var costs []ddclient.CostInfo
	if err := json.Unmarshal(w.Body.Bytes(), &costs); err != nil {
		t.Fatalf("unmarshal cost response: %v (body=%s)", err, w.Body.String())
	}
	return costs
}

// TestDatadogCorruptTokenFileFallsBackWithWarning は、トークンファイルを壊した状態で
// コストを取得すると、未ログインと区別できる警告を残したうえで静的キーへ倒れること、
// 静的キーが無ければエラーになることを確認する。
func TestDatadogCorruptTokenFileFallsBackWithWarning(t *testing.T) {
	dir := t.TempDir()
	org := &datadogFakeOrg{}
	s := org.start(t, dir)

	path, err := datadogauth.TokenPath(dir, testDatadogSite, "")
	if err != nil {
		t.Fatalf("token path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create the token directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write the corrupt token file: %v", err)
	}

	t.Run("with static keys it falls back", func(t *testing.T) {
		logs := captureLogs(t)
		s.cfg.SetDatadogAPIKey("static-api-key")
		s.cfg.SetDatadogAppKey("static-app-key")

		if costs := getDatadogHistoricalCost(t, s, "2026-09"); len(costs) != 1 {
			t.Fatalf("costs = %+v, want a single charge", costs)
		}
		if got := org.usageCredentials(); len(got) != 1 || got[0] != "keys:static-api-key/static-app-key" {
			t.Errorf("usage metering credentials = %v, want the static keys", got)
		}
		assertDatadogWarn(t, logs, "stored datadog oauth token is unusable")
	})

	t.Run("without static keys it errors", func(t *testing.T) {
		logs := captureLogs(t)
		s.cfg.SetDatadogAPIKey("")
		s.cfg.SetDatadogAppKey("")

		w := doDatadogRequest(t, s, http.MethodGet, "/api/datadog/cost/historical?start_month=2026-10")
		assertErrorCode(t, w, http.StatusUnauthorized, "DATADOG_NO_CREDENTIALS")
		assertDatadogWarn(t, logs, "stored datadog oauth token is unusable")
	})
}

// TestDatadogOAuthPerOrgSessions は、org ごとに別のクライアント登録とトークンが
// 保存されることを、Dynamic Client Registration とトークンエンドポイントを模した
// サーバ相手に通しで確認する。
//
// Datadog の client_id は登録を行った組織に紐づくため、Sub Organization ごとに
// 登録をやり直す必要がある。1 つの client_id を使い回すと、別の組織のトークンを
// 取得することになる。
func TestDatadogOAuthPerOrgSessions(t *testing.T) {
	dir := t.TempDir()
	fake := &datadogFakeOrg{}
	s := fake.start(t, dir)

	orgs := []string{"", "suborg1", "suborg2"}
	for i, org := range orgs {
		started := startDatadogLoginForOrg(t, s, org)
		if got := fake.registrationCount(); got != i+1 {
			t.Fatalf("client registrations after logging in to org %q = %d, want %d", org, got, i+1)
		}
		w := doDatadogRequest(t, s, http.MethodGet, datadogCallbackTarget(started.State, "auth-code"))
		if w.Code != http.StatusOK {
			t.Fatalf("callback status for org %q = %d, want %d (body=%s)", org, w.Code, http.StatusOK, w.Body.String())
		}
	}

	for i, org := range orgs {
		wantClientID := fmt.Sprintf("generated-client-id-%d", i+1)
		wantToken := fmt.Sprintf("oauth-access-token-%d", i+1)

		creds, ok, err := datadogauth.LoadClient(dir, testDatadogSite, org)
		if err != nil || !ok {
			t.Fatalf("LoadClient(org=%q) = (_, %v, %v), want (_, true, nil)", org, ok, err)
		}
		if creds.ClientID != wantClientID {
			t.Errorf("stored client id for org %q = %q, want %q", org, creds.ClientID, wantClientID)
		}
		tok, ok, err := datadogauth.LoadToken(dir, testDatadogSite, org)
		if err != nil || !ok {
			t.Fatalf("LoadToken(org=%q) = (_, %v, %v), want (_, true, nil)", org, ok, err)
		}
		if got := tok.AccessTokenValue(); got != wantToken {
			t.Errorf("stored access token for org %q = %q, want %q", org, got, wantToken)
		}
	}

	// 保存先のファイル名が org ごとに分かれていること。親組織のファイル名は org を
	// 導入する前と同じままであること。
	for _, tt := range []struct {
		org      string
		wantName string
	}{
		{org: "", wantName: "token_" + testDatadogSite + ".json"},
		{org: "suborg1", wantName: "token_" + testDatadogSite + "_suborg1.json"},
	} {
		path, err := datadogauth.TokenPath(dir, testDatadogSite, tt.org)
		if err != nil {
			t.Fatalf("TokenPath(org=%q) err = %v", tt.org, err)
		}
		if got := filepath.Base(path); got != tt.wantName {
			t.Errorf("token file name for org %q = %q, want %q", tt.org, got, tt.wantName)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("stat %s: %v", path, err)
		}
	}

	// 1 つの org のログアウトは他の org の認証情報を消さない。
	if w := doDatadogRequest(t, s, http.MethodPost, "/api/datadog/auth/logout?org=suborg1"); w.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}
	if _, ok, _ := datadogauth.LoadToken(dir, testDatadogSite, "suborg1"); ok {
		t.Error("the token of suborg1 still exists after logging out of suborg1")
	}
	for _, org := range []string{"", "suborg2"} {
		if _, ok, err := datadogauth.LoadToken(dir, testDatadogSite, org); !ok || err != nil {
			t.Errorf("LoadToken(org=%q) after logging out of suborg1 = (_, %v, %v), want (_, true, nil)", org, ok, err)
		}
	}
}
