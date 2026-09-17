package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

// testDatadogState は login/start が返す state の固定値。本番は datadogauth が
// crypto/rand で生成する。
const testDatadogState = "test-state"

// datadogLoginStub は login/start と callback が呼ぶ外部処理を記録するスタブ。
type datadogLoginStub struct {
	mu sync.Mutex

	prepared    []datadogauth.PrepareParams
	prepareErr  error
	completed   []string
	completeErr error
	// loggedOut にはログアウトの対象が "<site>|<org>" の形で積まれる。
	loggedOut     []string
	logoutErr     error
	registered    bool
	registerCalls int
}

func (st *datadogLoginStub) deps(t *testing.T) datadogAuthDeps {
	t.Helper()
	deps := (&datadogAuthDisk{}).deps(t)
	deps.loadClient = func(_, _ string) (*datadogauth.ClientCredentials, bool, error) {
		st.mu.Lock()
		defer st.mu.Unlock()
		if !st.registered {
			return nil, false, nil
		}
		return &datadogauth.ClientCredentials{ClientID: "client-id", RedirectURIs: []string{"http://127.0.0.1:8089" + config.DatadogOAuthCallbackPath}}, true, nil
	}
	deps.prepareLogin = func(_ context.Context, p datadogauth.PrepareParams) (*datadogauth.Login, error) {
		st.mu.Lock()
		defer st.mu.Unlock()
		st.prepared = append(st.prepared, p)
		if st.prepareErr != nil {
			return nil, st.prepareErr
		}
		// PrepareLogin と同じく、未登録のときだけ Dynamic Client Registration を行う。
		if !st.registered {
			st.registerCalls++
			st.registered = true
		}
		return &datadogauth.Login{
			Site:             p.Site,
			Org:              p.Org,
			ClientID:         "client-id",
			RedirectURI:      p.RedirectURI,
			State:            testDatadogState,
			AuthorizationURL: "https://app." + p.Site + "/oauth2/v1/authorize?state=" + testDatadogState,
		}, nil
	}
	deps.completeLogin = func(_ context.Context, _ *datadogauth.Login, _, code string) (*datadogauth.TokenSet, error) {
		st.mu.Lock()
		defer st.mu.Unlock()
		st.completed = append(st.completed, code)
		if st.completeErr != nil {
			return nil, st.completeErr
		}
		return validTestToken(t, "access-token"), nil
	}
	deps.logout = func(site, org string) error {
		st.mu.Lock()
		defer st.mu.Unlock()
		st.loggedOut = append(st.loggedOut, site+"|"+org)
		return st.logoutErr
	}
	return deps
}

func (st *datadogLoginStub) completeCalls() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]string(nil), st.completed...)
}

// newDatadogLoginTestServer はログイン系エンドポイントのテスト用に、スタブを注入した
// Server をルート登録済みで組み立てる。リクエストは s.mux.ServeHTTP へ流し、
// ルートパターン (メソッドとパス) の登録も含めて検証する。
func newDatadogLoginTestServer(t *testing.T, stub *datadogLoginStub) *Server {
	t.Helper()
	s := newTestServer(t)
	s.ddAuth = stub.deps(t)
	s.ddLoginSessions = newDatadogLoginSessionStore()
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

func doDatadogRequest(t *testing.T, s *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, r)
	return w
}

func startDatadogLogin(t *testing.T, s *Server) datadogLoginStartResponse {
	t.Helper()
	return startDatadogLoginForOrg(t, s, "")
}

// startDatadogLoginForOrg は org を指定して login/start を呼ぶ。org が空なら
// クエリパラメータ自体を付けない (親組織)。
func startDatadogLoginForOrg(t *testing.T, s *Server, org string) datadogLoginStartResponse {
	t.Helper()
	target := "/api/datadog/auth/login/start"
	if org != "" {
		target += "?org=" + url.QueryEscape(org)
	}
	w := doDatadogRequest(t, s, http.MethodPost, target)
	if w.Code != http.StatusOK {
		t.Fatalf("login/start status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp datadogLoginStartResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login/start response: %v (body=%s)", err, w.Body.String())
	}
	return resp
}

func getDatadogLoginStatus(t *testing.T, s *Server, state string) datadogLoginStatusResponse {
	t.Helper()
	w := doDatadogRequest(t, s, http.MethodGet, "/api/datadog/auth/login/status?state="+url.QueryEscape(state))
	if w.Code != http.StatusOK {
		t.Fatalf("login/status status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp datadogLoginStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login/status response: %v (body=%s)", err, w.Body.String())
	}
	return resp
}

// TestDatadogLoginStartRegistersClientWithoutCLI は、CLI を一度も実行していない状態
// (クライアント登録ファイル無し) でも login/start が Dynamic Client Registration を
// 行い、CLI 用とサーバ用の両方の redirect_uri を登録することを確認する。
// 片方しか登録しないと、後からもう一方でログインするときに再登録が必要になり、
// 既存のトークンが無効になる。
func TestDatadogLoginStartRegistersClientWithoutCLI(t *testing.T) {
	stub := &datadogLoginStub{}
	s := newDatadogLoginTestServer(t, stub)

	resp := startDatadogLogin(t, s)
	if resp.State != testDatadogState {
		t.Errorf("state = %q, want %q", resp.State, testDatadogState)
	}
	if !strings.HasPrefix(resp.AuthorizationURL, "https://app."+testDatadogSite+"/oauth2/v1/authorize") {
		t.Errorf("authorization_url = %q, want the Datadog authorize endpoint", resp.AuthorizationURL)
	}

	stub.mu.Lock()
	registerCalls := stub.registerCalls
	p := stub.prepared[0]
	stub.mu.Unlock()

	if registerCalls != 1 {
		t.Fatalf("client registrations = %d, want 1", registerCalls)
	}
	wantRedirect := config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath
	if p.RedirectURI != wantRedirect {
		t.Errorf("redirect URI = %q, want %q", p.RedirectURI, wantRedirect)
	}
	for _, want := range []string{config.DefaultDatadogOAuthCLIRedirectURI, wantRedirect} {
		if !slices.Contains(p.RegisterRedirectURIs, want) {
			t.Errorf("registered redirect URIs = %v, want it to contain %q", p.RegisterRedirectURIs, want)
		}
	}

	// 2 回目のログインは登録済みのクライアントを再利用する。
	startDatadogLogin(t, s)
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.registerCalls != 1 {
		t.Errorf("client registrations after a second login = %d, want 1", stub.registerCalls)
	}
}

// TestDatadogLoginStartErrors は認可準備の失敗が HTTP へどう写るかを確認する。
// 登録済みクライアントに今回の redirect_uri が無い場合は、利用者がログアウトして
// やり直せるよう他の失敗と区別する。
func TestDatadogLoginStartErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "redirect URI is not registered",
			err:        datadogauth.ErrRedirectURINotRegistered,
			wantStatus: http.StatusConflict,
			wantCode:   "DATADOG_REDIRECT_URI_NOT_REGISTERED",
		},
		{
			name:       "registration failed",
			err:        errors.New("register datadog oauth client: unexpected status 500"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "DATADOG_LOGIN_FAILED",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newDatadogLoginTestServer(t, &datadogLoginStub{prepareErr: tt.err})
			w := doDatadogRequest(t, s, http.MethodPost, "/api/datadog/auth/login/start")
			assertErrorCode(t, w, tt.wantStatus, tt.wantCode)
		})
	}
}

// TestDatadogAuthCallback は callback の分岐を網羅する。応答の本文はどの分岐でも
// クエリの値を含まない固定文言であること (反射型 XSS 対策) を毎回検証する。
func TestDatadogAuthCallback(t *testing.T) {
	// xssPayload はクエリ経由で応答へ混入しうる値。callback はブラウザが直接表示する
	// ため、1 文字でも書き戻すと反射型 XSS になる。
	const xssPayload = `<script>alert("xss")</script>`

	tests := []struct {
		name string
		// query は state を除いたクエリ。state は useValidState に従って足す。
		query         url.Values
		useValidState bool
		completeErr   error
		// repeat が真なら同じ callback を 2 回送り、2 回目の応答を検証する。
		repeat bool

		wantStatus        int
		wantBody          string
		wantCompleteCalls int
		wantLoginStatus   datadogLoginStatus
		wantStatusErrPart string
	}{
		{
			name:              "authorization code is exchanged",
			query:             url.Values{"code": {"auth-code"}},
			useValidState:     true,
			wantStatus:        http.StatusOK,
			wantBody:          datadogCallbackSuccessBody,
			wantCompleteCalls: 1,
			wantLoginStatus:   datadogLoginSucceeded,
		},
		{
			name:            "state does not match",
			query:           url.Values{"code": {"auth-code"}, "state": {"other-state" + xssPayload}},
			wantStatus:      http.StatusBadRequest,
			wantBody:        datadogCallbackUnknownBody,
			wantLoginStatus: datadogLoginPending,
		},
		{
			name:              "second callback with the same state is rejected",
			query:             url.Values{"code": {"auth-code"}},
			useValidState:     true,
			repeat:            true,
			wantStatus:        http.StatusBadRequest,
			wantBody:          datadogCallbackUnknownBody,
			wantCompleteCalls: 1,
			wantLoginStatus:   datadogLoginSucceeded,
		},
		{
			name:              "authorization was denied",
			query:             url.Values{"error": {"access_denied"}, "error_description": {"user denied " + xssPayload}},
			useValidState:     true,
			wantStatus:        http.StatusBadRequest,
			wantBody:          datadogCallbackFailedBody,
			wantLoginStatus:   datadogLoginFailed,
			wantStatusErrPart: "access_denied",
		},
		{
			name:            "callback has no authorization code",
			query:           url.Values{},
			useValidState:   true,
			wantStatus:      http.StatusBadRequest,
			wantBody:        datadogCallbackFailedBody,
			wantLoginStatus: datadogLoginFailed,
		},
		{
			name:              "token exchange fails",
			query:             url.Values{"code": {"auth-code" + xssPayload}},
			useValidState:     true,
			completeErr:       errors.New("exchange datadog authorization code: unexpected status 400"),
			wantStatus:        http.StatusBadGateway,
			wantBody:          datadogCallbackFailedBody,
			wantCompleteCalls: 1,
			wantLoginStatus:   datadogLoginFailed,
			wantStatusErrPart: "exchange datadog authorization code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &datadogLoginStub{completeErr: tt.completeErr}
			s := newDatadogLoginTestServer(t, stub)
			started := startDatadogLogin(t, s)

			q := url.Values{}
			for k, vs := range tt.query {
				q[k] = vs
			}
			if tt.useValidState {
				q.Set("state", started.State)
			}
			target := config.DatadogOAuthCallbackPath + "?" + q.Encode()

			w := doDatadogRequest(t, s, http.MethodGet, target)
			if tt.repeat {
				w = doDatadogRequest(t, s, http.MethodGet, target)
			}

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body=%s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if got := strings.TrimSpace(w.Body.String()); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
			// 応答にクエリ由来の値が 1 つも混ざっていないことを確認する。
			for _, vs := range q {
				for _, v := range vs {
					if v != "" && strings.Contains(w.Body.String(), v) {
						t.Errorf("body %q reflects the query value %q", w.Body.String(), v)
					}
				}
			}
			if got := len(stub.completeCalls()); got != tt.wantCompleteCalls {
				t.Errorf("token exchanges = %d, want %d", got, tt.wantCompleteCalls)
			}

			status := getDatadogLoginStatus(t, s, started.State)
			if status.Status != string(tt.wantLoginStatus) {
				t.Errorf("login status = %q, want %q", status.Status, tt.wantLoginStatus)
			}
			if tt.wantStatusErrPart != "" && !strings.Contains(status.ErrorMessage, tt.wantStatusErrPart) {
				t.Errorf("login status error = %q, want it to contain %q", status.ErrorMessage, tt.wantStatusErrPart)
			}
			if tt.wantStatusErrPart == "" && tt.wantLoginStatus != datadogLoginFailed && status.ErrorMessage != "" {
				t.Errorf("login status error = %q, want empty", status.ErrorMessage)
			}
		})
	}
}

// TestDatadogAuthCallbackResponseIsPlainText は callback の応答が HTML として解釈
// されないことを確認する。値を埋め込まない設計と合わせた二重の防御。
func TestDatadogAuthCallbackResponseIsPlainText(t *testing.T) {
	s := newDatadogLoginTestServer(t, &datadogLoginStub{})
	started := startDatadogLogin(t, s)

	w := doDatadogRequest(t, s, http.MethodGet, config.DatadogOAuthCallbackPath+"?code=auth-code&state="+url.QueryEscape(started.State))
	if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

// TestDatadogLoginStatusErrors は status エンドポイントの入力検証を確認する。
func TestDatadogLoginStatusErrors(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "state is missing",
			target:     "/api/datadog/auth/login/status",
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "state is unknown",
			target:     "/api/datadog/auth/login/status?state=unknown",
			wantStatus: http.StatusNotFound,
			wantCode:   "DATADOG_LOGIN_SESSION_NOT_FOUND",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newDatadogLoginTestServer(t, &datadogLoginStub{})
			w := doDatadogRequest(t, s, http.MethodGet, tt.target)
			assertErrorCode(t, w, tt.wantStatus, tt.wantCode)
		})
	}
}

// TestDatadogLoginStatusIsPendingBeforeCallback は、callback が来るまで status が
// pending を返すことを確認する (frontend はこれをポーリングする)。
func TestDatadogLoginStatusIsPendingBeforeCallback(t *testing.T) {
	s := newDatadogLoginTestServer(t, &datadogLoginStub{})
	started := startDatadogLogin(t, s)

	if got := getDatadogLoginStatus(t, s, started.State); got.Status != string(datadogLoginPending) {
		t.Errorf("status = %q, want %q", got.Status, datadogLoginPending)
	}
}

// TestDatadogAuthLogout はログアウトがローカルの認証情報を削除し、失敗を 500 で
// 返すことを確認する。
func TestDatadogAuthLogout(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		stub := &datadogLoginStub{}
		s := newDatadogLoginTestServer(t, stub)

		w := doDatadogRequest(t, s, http.MethodPost, "/api/datadog/auth/logout")
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
		}
		stub.mu.Lock()
		defer stub.mu.Unlock()
		if want := testDatadogSite + "|"; len(stub.loggedOut) != 1 || stub.loggedOut[0] != want {
			t.Errorf("logged out targets = %v, want [%s]", stub.loggedOut, want)
		}
	})

	t.Run("fails", func(t *testing.T) {
		s := newDatadogLoginTestServer(t, &datadogLoginStub{logoutErr: errors.New("remove token file: permission denied")})
		w := doDatadogRequest(t, s, http.MethodPost, "/api/datadog/auth/logout")
		assertErrorCode(t, w, http.StatusInternalServerError, "DATADOG_LOGOUT_FAILED")
	})
}

// TestDatadogRedirectBaseReregistrationWarning は、redirect base を既定値から変えた
// 状態での初回ログインが、これから起きる再登録の影響を警告することを確認する。
// 再登録は新しい client_id を発行し、それ以前に発行されたトークン (CLI 側を含む) を
// 使えなくするため、黙って進めてはいけない。
func TestDatadogRedirectBaseReregistrationWarning(t *testing.T) {
	const customBase = "https://thief.example.com"

	tests := []struct {
		name         string
		redirectBase string
		registered   bool
		wantWarn     bool
		wantRedirect string
	}{
		{
			name:         "default redirect base does not warn",
			redirectBase: config.DefaultDatadogOAuthRedirectBase,
			wantRedirect: config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath,
		},
		{
			name:         "changed redirect base warns on the first login",
			redirectBase: customBase,
			wantWarn:     true,
			wantRedirect: customBase + config.DatadogOAuthCallbackPath,
		},
		{
			name:         "changed redirect base does not warn once a client is registered",
			redirectBase: customBase,
			registered:   true,
			wantRedirect: customBase + config.DatadogOAuthCallbackPath,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogs(t)
			stub := &datadogLoginStub{registered: tt.registered}
			s := newDatadogLoginTestServer(t, stub)
			s.cfg.Datadog.OAuthRedirectBase = tt.redirectBase

			startDatadogLogin(t, s)

			stub.mu.Lock()
			gotRedirect := stub.prepared[0].RedirectURI
			stub.mu.Unlock()
			if gotRedirect != tt.wantRedirect {
				t.Errorf("redirect URI = %q, want %q", gotRedirect, tt.wantRedirect)
			}

			want := ""
			if tt.wantWarn {
				want = "registering a new datadog oauth client for a non-default redirect base"
			}
			assertDatadogWarn(t, logs, want)
		})
	}
}

// TestDatadogServerRedirectURITrimsTrailingSlash は、末尾スラッシュ付きの redirect base を
// 設定しても redirect_uri のパスが二重スラッシュにならないことを確認する。
// DatadogOAuthCallbackPath は先頭にスラッシュを持つため、ベース側の末尾スラッシュが残ると
// 連結結果が "//api/datadog/auth/callback" になり、Datadog に登録済みの redirect_uri と
// 文字列一致しない (RFC 6749 3.1.2.3)。正規化は config.Load が担うため、Server には
// Load 経由で組み立てた Config を渡す。
func TestDatadogServerRedirectURITrimsTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("THIEF_DATADOG_OAUTH_REDIRECT_BASE", "http://127.0.0.1:8089/")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	s := &Server{cfg: cfg}

	want := "http://127.0.0.1:8089/api/datadog/auth/callback"
	if got := s.datadogServerRedirectURI(); got != want {
		t.Errorf("datadogServerRedirectURI() = %q, want %q", got, want)
	}
}

// TestDatadogAuthOrgQueryParam は org クエリパラメータが login/start と logout に届き、
// 不正な値が 400 になることを確認する。org を取り違えると、別の Sub Organization の
// 認証情報を上書きしたり消したりすることになる。
//
// callback と login/status は org を取らない。callback の redirect_uri は Dynamic Client
// Registration で登録した文字列と完全一致していなければならずクエリを足せないため、
// org は login/start が作った中間状態を state 経由で引き継ぐ。login/status も同じく
// state で引く。
func TestDatadogAuthOrgQueryParam(t *testing.T) {
	t.Run("login start passes the org through", func(t *testing.T) {
		tests := []struct {
			name string
			org  string
		}{
			{name: "parent organization", org: ""},
			{name: "sub organization", org: "suborg1"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stub := &datadogLoginStub{}
				s := newDatadogLoginTestServer(t, stub)

				started := startDatadogLoginForOrg(t, s, tt.org)

				stub.mu.Lock()
				gotOrg := stub.prepared[0].Org
				stub.mu.Unlock()
				if gotOrg != tt.org {
					t.Errorf("PrepareParams.Org = %q, want %q", gotOrg, tt.org)
				}

				// callback は org をクエリで受け取らず、state から引いた中間状態を使う。
				w := doDatadogRequest(t, s, http.MethodGet, datadogCallbackTarget(started.State, "auth-code"))
				if w.Code != http.StatusOK {
					t.Fatalf("callback status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
				}
			})
		}
	})

	t.Run("logout passes the org through", func(t *testing.T) {
		stub := &datadogLoginStub{}
		s := newDatadogLoginTestServer(t, stub)

		if w := doDatadogRequest(t, s, http.MethodPost, "/api/datadog/auth/logout?org=suborg1"); w.Code != http.StatusNoContent {
			t.Fatalf("logout status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
		}
		stub.mu.Lock()
		defer stub.mu.Unlock()
		if want := testDatadogSite + "|suborg1"; len(stub.loggedOut) != 1 || stub.loggedOut[0] != want {
			t.Errorf("logged out targets = %v, want [%s]", stub.loggedOut, want)
		}
	})

	t.Run("an invalid org is rejected", func(t *testing.T) {
		// org は保存先のファイル名の一部になるため、パスを遡れる値は受け付けない。
		const badOrg = "../../etc/passwd"

		tests := []struct {
			name   string
			method string
			path   string
			// prepareErr はスタブが返すエラー。login/start は datadogauth 側の検証を
			// 通るため、その失敗が HTTP へどう写るかを見る。
			prepareErr error
			logoutErr  error
		}{
			{
				name:       "login start",
				method:     http.MethodPost,
				path:       "/api/datadog/auth/login/start",
				prepareErr: fmt.Errorf("%w: %q", datadogauth.ErrInvalidOrg, badOrg),
			},
			{
				name:      "logout",
				method:    http.MethodPost,
				path:      "/api/datadog/auth/logout",
				logoutErr: fmt.Errorf("%w: %q", datadogauth.ErrInvalidOrg, badOrg),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := newDatadogLoginTestServer(t, &datadogLoginStub{prepareErr: tt.prepareErr, logoutErr: tt.logoutErr})
				w := doDatadogRequest(t, s, tt.method, tt.path+"?org="+url.QueryEscape(badOrg))
				assertErrorCode(t, w, http.StatusBadRequest, "BAD_REQUEST")
			})
		}
	})
}
