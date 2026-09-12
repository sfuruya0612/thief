package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
	"github.com/spf13/cobra"
)

// errDatadogAuthTest は「エラーになること自体」を期待するテストケースの目印。
var errDatadogAuthTest = errors.New("datadog auth test error")

// fakeDatadogAuth は datadogAuthDeps の差し替え先。ログインは複数の goroutine
// (コールバックの待ち受けと標準入力の読み取り) から呼ばれるため、記録は mutex で守る。
type fakeDatadogAuth struct {
	mu sync.Mutex

	prepareCalls  []datadogauth.PrepareParams
	prepareLogin  *datadogauth.Login
	prepareErr    error
	completeCalls []completeArgs
	completeErr   error
	ensureTok     *datadogauth.TokenSet
	ensureOrgs    []string
	ensureOK      bool
	ensureErr     error
	// logoutCalls にはログアウトの対象が "<site>|<org>" の形で積まれる。
	logoutCalls   []string
	logoutErr     error
	openedURLs    []string
	openErr       error
	listenErr     error
	callbackState string
	callbackCode  string
	callbackErr   error
	pasted        string
	pasteErr      error

	// blockCallback が真のとき、コールバックの待ち受けは ctx のキャンセルまで戻らない。
	// 標準入力へのフォールバックが先に採用されることを確かめるために使う。
	blockCallback bool
}

type completeArgs struct {
	state string
	code  string
}

func newFakeDatadogAuth() *fakeDatadogAuth {
	return &fakeDatadogAuth{
		prepareLogin: &datadogauth.Login{
			Site:             "datadoghq.com",
			ClientID:         "cid",
			RedirectURI:      datadogCLIRedirectURI,
			State:            "state-value",
			AuthorizationURL: "https://app.datadoghq.com/oauth2/v1/authorize?state=state-value",
		},
	}
}

func (f *fakeDatadogAuth) deps(t *testing.T) datadogAuthDeps {
	t.Helper()
	return datadogAuthDeps{
		prepareLogin: func(_ context.Context, p datadogauth.PrepareParams) (*datadogauth.Login, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.prepareCalls = append(f.prepareCalls, p)
			if f.prepareErr != nil {
				return nil, f.prepareErr
			}
			return f.prepareLogin, nil
		},
		completeLogin: func(_ context.Context, _ *datadogauth.Login, state, code string) (*datadogauth.TokenSet, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.completeCalls = append(f.completeCalls, completeArgs{state: state, code: code})
			if f.completeErr != nil {
				return nil, f.completeErr
			}
			return &datadogauth.TokenSet{AccessToken: "at", ExpiresIn: 3600, IssuedAt: time.Now()}, nil
		},
		ensureFreshToken: func(_ context.Context, _, org string) (*datadogauth.TokenSet, bool, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.ensureOrgs = append(f.ensureOrgs, org)
			return f.ensureTok, f.ensureOK, f.ensureErr
		},
		logout: func(site, org string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.logoutCalls = append(f.logoutCalls, site+"|"+org)
			return f.logoutErr
		},
		openBrowser: func(rawURL string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.openedURLs = append(f.openedURLs, rawURL)
			return f.openErr
		},
		listen: func(_ string) (net.Listener, error) {
			if f.listenErr != nil {
				return nil, f.listenErr
			}
			// 実際の待ち受けは serveCallback を差し替えているため使わないが、
			// 本体が defer で Close するので実物のリスナを渡す。
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen on a random port: %v", err)
			}
			return ln, nil
		},
		serveCallback: func(ctx context.Context, _ net.Listener) (string, string, error) {
			if f.blockCallback {
				<-ctx.Done()
				return "", "", ctx.Err()
			}
			return f.callbackState, f.callbackCode, f.callbackErr
		},
		readPastedCode: func(ctx context.Context) (string, error) {
			if f.pasted == "" && f.pasteErr == nil {
				<-ctx.Done()
				return "", ctx.Err()
			}
			return f.pasted, f.pasteErr
		},
	}
}

func (f *fakeDatadogAuth) prepared() []datadogauth.PrepareParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]datadogauth.PrepareParams(nil), f.prepareCalls...)
}

func (f *fakeDatadogAuth) completed() []completeArgs {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]completeArgs(nil), f.completeCalls...)
}

// newDatadogAuthTestCmd は site フラグと出力先を持つコマンドを返す。site を明示して
// 利用者の設定ファイルに依存しないようにする。
func newDatadogAuthTestCmd(t *testing.T, site string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	return newDatadogAuthOrgTestCmd(t, site, "")
}

// newDatadogAuthOrgTestCmd は site に加えて org フラグも持つコマンドを返す。
func newDatadogAuthOrgTestCmd(t *testing.T, site, org string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{Use: "login"}
	cmd.Flags().String("site", "", "")
	if err := cmd.Flags().Set("site", site); err != nil {
		t.Fatalf("set site flag: %v", err)
	}
	cmd.Flags().String("org", "", datadogOrgFlagUsage)
	if err := cmd.Flags().Set("org", org); err != nil {
		t.Fatalf("set org flag: %v", err)
	}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetIn(strings.NewReader(""))
	return cmd, out, errOut
}

func TestDatadogAuthLoginWith(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(*fakeDatadogAuth)
		wantPrepared    bool
		wantOpened      bool
		wantPastePrompt bool
		wantComplete    []completeArgs
		wantOut         []string
		wantErrOut      []string
		wantErr         error
	}{
		{
			name: "the loopback callback completes the login",
			setup: func(f *fakeDatadogAuth) {
				f.callbackState, f.callbackCode = "state-value", "auth-code"
			},
			wantPrepared: true,
			wantOpened:   true,
			wantComplete: []completeArgs{{state: "state-value", code: "auth-code"}},
			wantOut: []string{
				"https://app.datadoghq.com/oauth2/v1/authorize?state=state-value",
				"Successfully logged into Datadog site: datadoghq.com",
			},
		},
		{
			// ブラウザを開けなくてもフローは止めない。URL は既に提示済みである。
			name: "a browser failure is not fatal and enables pasting",
			setup: func(f *fakeDatadogAuth) {
				f.openErr = errDatadogAuthTest
				f.blockCallback = true
				f.pasted = "pasted-code"
			},
			wantPrepared:    true,
			wantOpened:      true,
			wantPastePrompt: true,
			wantComplete:    []completeArgs{{state: "state-value", code: "pasted-code"}},
			wantOut: []string{
				"paste the authorization code",
				"Successfully logged into Datadog site: datadoghq.com",
			},
			wantErrOut: []string{"warning: open browser"},
		},
		{
			name: "a pasted redirect URL carries its own state",
			setup: func(f *fakeDatadogAuth) {
				f.openErr = errDatadogAuthTest
				f.blockCallback = true
				f.pasted = "http://127.0.0.1:8400/callback?code=url-code&state=url-state"
			},
			wantPrepared:    true,
			wantOpened:      true,
			wantPastePrompt: true,
			wantComplete:    []completeArgs{{state: "url-state", code: "url-code"}},
		},
		{
			name: "a paste error is returned",
			setup: func(f *fakeDatadogAuth) {
				f.openErr = errDatadogAuthTest
				f.blockCallback = true
				f.pasteErr = errDatadogAuthTest
			},
			wantPrepared:    true,
			wantOpened:      true,
			wantPastePrompt: true,
			wantErr:         errDatadogAuthTest,
		},
		{
			// ブラウザを開けた場合は貼り付けを促さない。
			name: "pasting is not offered when the browser opens",
			setup: func(f *fakeDatadogAuth) {
				f.callbackState, f.callbackCode = "state-value", "auth-code"
				f.pasted = "must-not-be-used"
			},
			wantPrepared: true,
			wantOpened:   true,
			wantComplete: []completeArgs{{state: "state-value", code: "auth-code"}},
		},
		{
			name: "a listen failure fails before any client registration",
			setup: func(f *fakeDatadogAuth) {
				f.listenErr = errDatadogAuthTest
			},
			wantErr:    errDatadogAuthTest,
			wantErrOut: nil,
		},
		{
			name: "a prepare failure is returned before opening a browser",
			setup: func(f *fakeDatadogAuth) {
				f.prepareErr = errDatadogAuthTest
			},
			wantPrepared: true,
			wantErr:      errDatadogAuthTest,
		},
		{
			name: "a callback failure is returned",
			setup: func(f *fakeDatadogAuth) {
				f.callbackErr = errDatadogAuthTest
			},
			wantPrepared: true,
			wantOpened:   true,
			wantErr:      errDatadogAuthTest,
		},
		{
			name: "an exchange failure is returned",
			setup: func(f *fakeDatadogAuth) {
				f.callbackState, f.callbackCode = "state-value", "auth-code"
				f.completeErr = errDatadogAuthTest
			},
			wantPrepared: true,
			wantOpened:   true,
			wantComplete: []completeArgs{{state: "state-value", code: "auth-code"}},
			wantErr:      errDatadogAuthTest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeDatadogAuth()
			tt.setup(f)
			cmd, out, errOut := newDatadogAuthTestCmd(t, "datadoghq.com")

			err := datadogAuthLoginWith(cmd, f.deps(t))

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("datadogAuthLoginWith() err = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("datadogAuthLoginWith() err = %v", err)
			}

			if got := len(f.prepared()); got != boolToCount(tt.wantPrepared) {
				t.Errorf("prepareLogin calls = %d, want %d", got, boolToCount(tt.wantPrepared))
			}
			if tt.wantPrepared {
				want := datadogauth.PrepareParams{
					Site:                 "datadoghq.com",
					RedirectURI:          datadogCLIRedirectURI,
					RegisterRedirectURIs: []string{datadogCLIRedirectURI, datadogServerRedirectURI()},
				}
				if diff := cmp.Diff(want, f.prepared()[0]); diff != "" {
					t.Errorf("prepareLogin params mismatch (-want +got):\n%s", diff)
				}
			}
			if got := len(f.openedURLs); got != boolToCount(tt.wantOpened) {
				t.Errorf("openBrowser calls = %d, want %d", got, boolToCount(tt.wantOpened))
			}
			if diff := cmp.Diff(tt.wantComplete, f.completed(), cmp.AllowUnexported(completeArgs{})); diff != "" {
				t.Errorf("completeLogin calls mismatch (-want +got):\n%s", diff)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout does not contain %q:\n%s", want, out.String())
				}
			}
			for _, want := range tt.wantErrOut {
				if !strings.Contains(errOut.String(), want) {
					t.Errorf("stderr does not contain %q:\n%s", want, errOut.String())
				}
			}
			// ブラウザを開けたときは貼り付けを促さない。促すのはブラウザの起動に失敗し、
			// 標準入力への退路を残す場合だけである。
			const pastePrompt = "paste the authorization code"
			if got := strings.Contains(out.String(), pastePrompt); got != tt.wantPastePrompt {
				t.Errorf("stdout contains %q = %v, want %v:\n%s", pastePrompt, got, tt.wantPastePrompt, out.String())
			}
		})
	}
}

// TestDatadogAuthLoginWithFixedPortInUse は、固定ポート 8400 が使用中のときに別ポートへ
// 黙ってフォールバックせず、明確なエラーで失敗することを確かめる。ポートが変わると
// 登録済みの redirect_uri と食い違い、認可サーバがコールバックを拒否するためである。
func TestDatadogAuthLoginWithFixedPortInUse(t *testing.T) {
	blocker, err := net.Listen("tcp", datadogAuthCallbackAddr)
	if err != nil {
		t.Fatalf("occupy %s for the test: %v", datadogAuthCallbackAddr, err)
	}
	t.Cleanup(func() { _ = blocker.Close() })

	f := newFakeDatadogAuth()
	deps := f.deps(t)
	// listen だけは本番の実装 (net.Listen) に戻し、固定ポートの衝突を実際に起こす。
	deps.listen = func(addr string) (net.Listener, error) { return net.Listen("tcp", addr) }

	cmd, _, _ := newDatadogAuthTestCmd(t, "datadoghq.com")
	err = datadogAuthLoginWith(cmd, deps)
	if err == nil {
		t.Fatal("datadogAuthLoginWith() err = nil, want an error")
	}
	if !strings.Contains(err.Error(), datadogAuthCallbackAddr) {
		t.Errorf("error %q does not mention %q", err.Error(), datadogAuthCallbackAddr)
	}
	if got := len(f.prepared()); got != 0 {
		t.Errorf("prepareLogin calls = %d, want 0 (no client registration when the port is unusable)", got)
	}
	if got := len(f.openedURLs); got != 0 {
		t.Errorf("openBrowser calls = %d, want 0", got)
	}
}

// TestDatadogCLIRedirectURI は redirect_uri が固定ポートの形を保っていることを、
// TestDatadogServerRedirectURI は API サーバ側の redirect_uri が config の既定値から
// 組み立てられることを確かめる。どちらも Dynamic Client Registration で 1 回だけ登録
// されるため、変わると既存の登録が使えなくなる。
func TestDatadogCLIRedirectURI(t *testing.T) {
	if want := "http://127.0.0.1:8400/callback"; datadogCLIRedirectURI != want {
		t.Errorf("datadogCLIRedirectURI = %q, want %q", datadogCLIRedirectURI, want)
	}
}

func TestDatadogServerRedirectURI(t *testing.T) {
	want := config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath
	if got := datadogServerRedirectURI(); got != want {
		t.Errorf("datadogServerRedirectURI() = %q, want %q", got, want)
	}
	if want := "http://127.0.0.1:8089/api/datadog/auth/callback"; datadogServerRedirectURI() != want {
		t.Errorf("datadogServerRedirectURI() = %q, want %q", datadogServerRedirectURI(), want)
	}
}

func TestParseDatadogAuthCodeInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCode  string
		wantState string
		wantErr   bool
	}{
		{name: "bare code", input: "auth-code", wantCode: "auth-code"},
		{name: "surrounding spaces are trimmed", input: "  auth-code \n", wantCode: "auth-code"},
		{name: "redirect URL", input: "http://127.0.0.1:8400/callback?code=c1&state=s1", wantCode: "c1", wantState: "s1"},
		{name: "https redirect URL", input: "https://example.com/callback?code=c2", wantCode: "c2"},
		{name: "empty", input: "   ", wantErr: true},
		{name: "redirect URL with an error parameter", input: "http://127.0.0.1:8400/callback?error=access_denied&error_description=denied", wantErr: true},
		{name: "redirect URL without a code", input: "http://127.0.0.1:8400/callback?state=s1", wantErr: true},
		{name: "unparsable redirect URL", input: "http://127.0.0.1:8400/callback?code=%zz", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, state, err := parseDatadogAuthCodeInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDatadogAuthCodeInput(%q) err = nil, want an error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDatadogAuthCodeInput(%q) err = %v", tt.input, err)
			}
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
			if state != tt.wantState {
				t.Errorf("state = %q, want %q", state, tt.wantState)
			}
		})
	}
}

func TestReadDatadogAuthCode(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("auth-code\n"))
	got, err := readDatadogAuthCode(context.Background(), cmd)
	if err != nil {
		t.Fatalf("readDatadogAuthCode() err = %v", err)
	}
	if got != "auth-code" {
		t.Errorf("readDatadogAuthCode() = %q, want %q", got, "auth-code")
	}

	// 改行の無い入力 (EOF) も、読めた分を入力として扱う。
	cmd.SetIn(strings.NewReader("no-newline"))
	got, err = readDatadogAuthCode(context.Background(), cmd)
	if err != nil {
		t.Fatalf("readDatadogAuthCode() err = %v", err)
	}
	if got != "no-newline" {
		t.Errorf("readDatadogAuthCode() = %q, want %q", got, "no-newline")
	}
}

// TestReadDatadogAuthCodeCanceled は、入力が来ないまま ctx がキャンセルされた場合に
// 待ち続けないことを確かめる。
func TestReadDatadogAuthCodeCanceled(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })

	cmd := &cobra.Command{}
	cmd.SetIn(pr)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := readDatadogAuthCode(ctx, cmd); !errors.Is(err, context.Canceled) {
		t.Errorf("readDatadogAuthCode() err = %v, want %v", err, context.Canceled)
	}
}

func TestServeDatadogAuthCallback(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantState string
		wantCode  string
		wantErr   string
	}{
		{name: "code and state", query: "?code=auth-code&state=state-value", wantState: "state-value", wantCode: "auth-code"},
		{name: "authorization error", query: "?error=access_denied&error_description=user+denied", wantErr: "access_denied: user denied"},
		{name: "authorization error without a description", query: "?error=access_denied", wantErr: "access_denied"},
		{name: "no code", query: "?state=state-value", wantErr: "no authorization code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			t.Cleanup(func() { _ = ln.Close() })

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)

			type result struct {
				state, code string
				err         error
			}
			done := make(chan result, 1)
			go func() {
				state, code, err := serveDatadogAuthCallback(ctx, ln)
				done <- result{state: state, code: code, err: err}
			}()

			resp, err := http.Get("http://" + ln.Addr().String() + datadogAuthCallbackPath + tt.query) //nolint:noctx // テスト内の単発リクエスト
			if err != nil {
				t.Fatalf("call the callback: %v", err)
			}
			_ = resp.Body.Close()

			r := <-done
			if tt.wantErr != "" {
				if r.err == nil {
					t.Fatalf("serveDatadogAuthCallback() err = nil, want an error containing %q", tt.wantErr)
				}
				if !strings.Contains(r.err.Error(), tt.wantErr) {
					t.Errorf("err = %q, want it to contain %q", r.err.Error(), tt.wantErr)
				}
				if resp.StatusCode != http.StatusBadRequest {
					t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
				}
				return
			}
			if r.err != nil {
				t.Fatalf("serveDatadogAuthCallback() err = %v", r.err)
			}
			if r.state != tt.wantState || r.code != tt.wantCode {
				t.Errorf("(state, code) = (%q, %q), want (%q, %q)", r.state, r.code, tt.wantState, tt.wantCode)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestDatadogAuthLogoutWith(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*fakeDatadogAuth)
		wantOut string
		wantErr error
	}{
		{
			name:    "removed",
			setup:   func(*fakeDatadogAuth) {},
			wantOut: "Removed the local Datadog OAuth credentials for site: datadoghq.com",
		},
		{
			name:    "removal failure is returned",
			setup:   func(f *fakeDatadogAuth) { f.logoutErr = errDatadogAuthTest },
			wantErr: errDatadogAuthTest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeDatadogAuth()
			tt.setup(f)
			cmd, out, _ := newDatadogAuthTestCmd(t, "datadoghq.com")

			err := datadogAuthLogoutWith(cmd, f.deps(t))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("datadogAuthLogoutWith() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("datadogAuthLogoutWith() err = %v", err)
			}
			if diff := cmp.Diff([]string{"datadoghq.com|"}, f.logoutCalls); diff != "" {
				t.Errorf("logout calls mismatch (-want +got):\n%s", diff)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout does not contain %q:\n%s", tt.wantOut, out.String())
			}
		})
	}
}

func TestDatadogAuthRefreshWith(t *testing.T) {
	issued := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(*fakeDatadogAuth)
		wantOut string
		wantErr string
	}{
		{
			name: "the expiry is shown",
			setup: func(f *fakeDatadogAuth) {
				f.ensureTok = &datadogauth.TokenSet{AccessToken: "at", ExpiresIn: 3600, IssuedAt: issued}
				f.ensureOK = true
			},
			wantOut: "valid until 2026-09-11T13:00:00Z",
		},
		{
			name: "an unknown expiry is reported as such",
			setup: func(f *fakeDatadogAuth) {
				f.ensureTok = &datadogauth.TokenSet{AccessToken: "at"}
				f.ensureOK = true
			},
			wantOut: "(expiry unknown)",
		},
		{
			name:    "not logged in",
			setup:   func(f *fakeDatadogAuth) { f.ensureOK = false },
			wantErr: "Run 'thief datadog auth login' first",
		},
		{
			name:    "a refresh failure is returned",
			setup:   func(f *fakeDatadogAuth) { f.ensureErr = errDatadogAuthTest },
			wantErr: errDatadogAuthTest.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeDatadogAuth()
			tt.setup(f)
			cmd, out, _ := newDatadogAuthTestCmd(t, "datadoghq.com")

			err := datadogAuthRefreshWith(cmd, f.deps(t))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("datadogAuthRefreshWith() err = nil, want an error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("datadogAuthRefreshWith() err = %v", err)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout does not contain %q:\n%s", tt.wantOut, out.String())
			}
		})
	}
}

// TestNewDatadogAuthCmd はコマンド木の形を確かめる。
func TestNewDatadogAuthCmd(t *testing.T) {
	authCmd := newDatadogAuthCmd()
	if authCmd.Name() != "auth" {
		t.Errorf("command name = %q, want %q", authCmd.Name(), "auth")
	}
	var names []string
	for _, c := range authCmd.Commands() {
		names = append(names, c.Name())
	}
	if diff := cmp.Diff([]string{"login", "logout", "refresh"}, names); diff != "" {
		t.Errorf("subcommands mismatch (-want +got):\n%s", diff)
	}
}

// TestDatadogCmdHasAuth は `thief datadog` に auth が生えていることを確かめる。
func TestDatadogCmdHasAuth(t *testing.T) {
	var found bool
	for _, c := range newDatadogCmd().Commands() {
		if c.Name() == "auth" {
			found = true
		}
	}
	if !found {
		t.Error("'thief datadog' has no 'auth' subcommand")
	}
}

// datadogAuthStubServer は Datadog の DCR とトークンの 2 つのエンドポイントを模した
// サーバ。エンドツーエンドのテストで実ホストの代わりに使う。
type datadogAuthStubServer struct {
	mu           sync.Mutex
	registerBody map[string]any
	tokenForm    url.Values
}

func (s *datadogAuthStubServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/api/v2/oauth2/register":
		s.mu.Lock()
		_ = json.Unmarshal(raw, &s.registerBody)
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"client_id":"e2e-cid","client_name":%q,"redirect_uris":[%q,%q]}`,
			"thief", datadogCLIRedirectURI, datadogServerRedirectURI())
	case "/oauth2/v1/token":
		form, perr := url.ParseQuery(string(raw))
		if perr != nil {
			http.Error(w, "parse form", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.tokenForm = form
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"access_token":"e2e-at","refresh_token":"e2e-rt","token_type":"Bearer","expires_in":3600,"scope":"usage_read"}`)
	default:
		http.Error(w, "unexpected path", http.StatusNotFound)
	}
}

func (s *datadogAuthStubServer) snapshot() (map[string]any, url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registerBody, s.tokenForm
}

// rewritingTransport は本番コードが組み立てた https://api.{site}/... 宛のリクエストを
// httptest のサーバへ向け直す。URL の組み立てをテストの対象から外さないために、
// URL ではなく HTTP クライアントを差し替える形にしている。
type rewritingTransport struct{ base *url.URL }

func (t rewritingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = t.base.Scheme
	r.URL.Host = t.base.Host
	r.Host = ""
	return http.DefaultTransport.RoundTrip(r)
}

// TestDatadogAuthLoginEndToEnd は `thief datadog auth login` を、Datadog の
// エンドポイントだけを httptest へ置き換えて通しで実行する。
//
// 置き換えるのは HTTP の宛先とブラウザの起動だけで、Dynamic Client Registration、
// PKCE と state の生成、ループバックのコールバック受理、トークンの引き換え、
// 保存はすべて本番の実装を通る。実際の Datadog へは接続しない。
func TestDatadogAuthLoginEndToEnd(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	stub := &datadogAuthStubServer{}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	stubURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse stub server URL: %v", err)
	}
	client := datadogauth.Client{HTTP: &http.Client{Transport: rewritingTransport{base: stubURL}}}

	authDeps := datadogauth.DefaultDeps()
	authDeps.RegisterClient = client.RegisterClient
	authDeps.ExchangeCode = client.ExchangeCode
	authDeps.RefreshToken = client.Refresh

	// 固定ポート 8400 は他のテストや開発サーバと衝突しうるため、待ち受けだけは
	// ランダムポートにする。redirect_uri は本番と同じ固定ポートの値が登録される。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on a random port: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var openedURL string
	var openMu sync.Mutex
	deps := datadogAuthDeps{
		prepareLogin: func(ctx context.Context, p datadogauth.PrepareParams) (*datadogauth.Login, error) {
			return datadogauth.PrepareLogin(ctx, p, authDeps)
		},
		completeLogin: func(ctx context.Context, login *datadogauth.Login, state, code string) (*datadogauth.TokenSet, error) {
			return datadogauth.CompleteLogin(ctx, login, state, code, authDeps)
		},
		ensureFreshToken: func(ctx context.Context, site, org string) (*datadogauth.TokenSet, bool, error) {
			return datadogauth.EnsureFreshToken(ctx, site, org, authDeps)
		},
		logout:        func(site, org string) error { return datadogauth.Logout(site, org, authDeps) },
		listen:        func(string) (net.Listener, error) { return ln, nil },
		serveCallback: serveDatadogAuthCallback,
		readPastedCode: func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		// ブラウザの代わりに、認可 URL の state を使ってコールバックを撃つ。
		// serveCallback は openBrowser の後に待ち受けを始めるため、非同期に撃つ。
		openBrowser: func(rawURL string) error {
			openMu.Lock()
			openedURL = rawURL
			openMu.Unlock()
			u, perr := url.Parse(rawURL)
			if perr != nil {
				return perr
			}
			go func() {
				callback := "http://" + ln.Addr().String() + datadogAuthCallbackPath +
					"?code=e2e-code&state=" + url.QueryEscape(u.Query().Get("state"))
				resp, cerr := http.Get(callback) //nolint:noctx // テスト内の単発リクエスト
				if cerr == nil {
					_ = resp.Body.Close()
				}
			}()
			return nil
		},
	}

	cmd, out, errOut := newDatadogAuthTestCmd(t, "datadoghq.com")
	if err := datadogAuthLoginWith(cmd, deps); err != nil {
		t.Fatalf("datadogAuthLoginWith() err = %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Successfully logged into Datadog site: datadoghq.com") {
		t.Errorf("stdout does not report a successful login:\n%s", out.String())
	}

	registerBody, tokenForm := stub.snapshot()

	// Dynamic Client Registration は CLI と API サーバの redirect_uri をまとめて 1 回だけ登録する。
	wantURIs := []any{datadogCLIRedirectURI, datadogServerRedirectURI()}
	if diff := cmp.Diff(wantURIs, registerBody["redirect_uris"]); diff != "" {
		t.Errorf("registered redirect_uris mismatch (-want +got):\n%s", diff)
	}
	if got := registerBody["client_name"]; got != "thief" {
		t.Errorf("registered client_name = %v, want %q", got, "thief")
	}
	if diff := cmp.Diff([]any{"authorization_code", "refresh_token"}, registerBody["grant_types"]); diff != "" {
		t.Errorf("registered grant_types mismatch (-want +got):\n%s", diff)
	}

	// トークン要求は認可 URL の code_challenge に対応する code_verifier を送る (RFC 7636)。
	openMu.Lock()
	authURL := openedURL
	openMu.Unlock()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	q := u.Query()
	if u.Host != "app.datadoghq.com" {
		t.Errorf("authorization URL host = %q, want %q", u.Host, "app.datadoghq.com")
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want %q", got, "S256")
	}
	verifier := tokenForm.Get("code_verifier")
	if verifier == "" {
		t.Fatal("the token request has no code_verifier")
	}
	sum := sha256.Sum256([]byte(verifier))
	if want, got := base64.RawURLEncoding.EncodeToString(sum[:]), q.Get("code_challenge"); got != want {
		t.Errorf("code_challenge = %q, want the S256 hash of the sent code_verifier (%q)", got, want)
	}
	for key, want := range map[string]string{
		"grant_type":   "authorization_code",
		"client_id":    "e2e-cid",
		"code":         "e2e-code",
		"redirect_uri": datadogCLIRedirectURI,
	} {
		if got := tokenForm.Get(key); got != want {
			t.Errorf("token request %s = %q, want %q", key, got, want)
		}
	}

	// トークンとクライアント登録が 0600 で保存される。
	dir := filepath.Join(base, "thief", "datadog")
	for _, name := range []string{"token_datadoghq.com.json", "client_datadoghq.com.json"} {
		info, serr := os.Stat(filepath.Join(dir, name))
		if serr != nil {
			t.Fatalf("stat %s: %v", name, serr)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %v, want %v", name, got, os.FileMode(0o600))
		}
	}
	tok, ok, err := datadogauth.LoadToken(dir, "datadoghq.com", "")
	if err != nil || !ok {
		t.Fatalf("LoadToken() = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if tok.AccessTokenValue() != "e2e-at" || tok.RefreshTokenValue() != "e2e-rt" {
		t.Errorf("stored token = (%q, %q), want (%q, %q)", tok.AccessTokenValue(), tok.RefreshTokenValue(), "e2e-at", "e2e-rt")
	}
	if tok.ClientID != "e2e-cid" {
		t.Errorf("stored client_id = %q, want %q", tok.ClientID, "e2e-cid")
	}

	// 保存済みトークンは有効なので、更新はトークンエンドポイントを呼ばずに済む。
	refreshCmd, refreshOut, _ := newDatadogAuthTestCmd(t, "datadoghq.com")
	if err := datadogAuthRefreshWith(refreshCmd, deps); err != nil {
		t.Fatalf("datadogAuthRefreshWith() err = %v", err)
	}
	if !strings.Contains(refreshOut.String(), "is valid until") {
		t.Errorf("stdout does not report the expiry:\n%s", refreshOut.String())
	}

	// ログアウトで両方のファイルが消える。
	logoutCmd, _, _ := newDatadogAuthTestCmd(t, "datadoghq.com")
	if err := datadogAuthLogoutWith(logoutCmd, deps); err != nil {
		t.Fatalf("datadogAuthLogoutWith() err = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Errorf("%d files are left after logout, want 0", len(entries))
	}
}

// TestDatadogCLIRedirectURIMatchesCallbackAddr は、CLI が実際に待ち受けるアドレスと、
// API サーバと共有している redirect_uri の定数が食い違わないことを守る。定数は config
// パッケージにあり (internal/api は internal/cli を import できないため)、待ち受け側だけを
// 変えて定数を直し忘れうる。食い違うと、登録済みの redirect_uri と異なる URI で認可要求を
// 出すことになり、認可サーバに拒否される。
func TestDatadogCLIRedirectURIMatchesCallbackAddr(t *testing.T) {
	want := "http://" + datadogAuthCallbackAddr + datadogAuthCallbackPath
	if datadogCLIRedirectURI != want {
		t.Errorf("datadogCLIRedirectURI = %q, want %q", datadogCLIRedirectURI, want)
	}
}

func boolToCount(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestDatadogAuthOrgFlag は --org が 3 つのサブコマンドすべてに生えており、その値が
// datadogauth へそのまま渡って出力にも現れることを確かめる。org を取り違えると、
// 別の Sub Organization の認証情報を上書きしたり消したりすることになる。
func TestDatadogAuthOrgFlag(t *testing.T) {
	t.Run("the flag is registered on every subcommand", func(t *testing.T) {
		for _, c := range newDatadogAuthCmd().Commands() {
			f := c.Flags().Lookup("org")
			if f == nil {
				t.Errorf("'datadog auth %s' has no --org flag", c.Name())
				continue
			}
			if f.DefValue != "" {
				t.Errorf("'datadog auth %s' --org default = %q, want the parent organization (empty)", c.Name(), f.DefValue)
			}
		}
	})

	// --org を定義していないコマンドでも親組織として動く (loadConfig の override と同じ
	// 許容の仕方)。
	t.Run("a command without the flag means the parent organization", func(t *testing.T) {
		if got := datadogOrgFlag(&cobra.Command{}); got != "" {
			t.Errorf("datadogOrgFlag() = %q, want %q", got, "")
		}
	})

	tests := []struct {
		name string
		org  string
		// wantLabel は出力に現れるべき org の表示。親組織では空。
		wantLabel string
	}{
		{name: "parent organization", org: "", wantLabel: ""},
		{name: "sub organization", org: "suborg1", wantLabel: " (organization: suborg1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("login", func(t *testing.T) {
				f := newFakeDatadogAuth()
				f.prepareLogin.Org = tt.org
				f.callbackState, f.callbackCode = "state-value", "auth-code"
				cmd, out, _ := newDatadogAuthOrgTestCmd(t, "datadoghq.com", tt.org)

				if err := datadogAuthLoginWith(cmd, f.deps(t)); err != nil {
					t.Fatalf("datadogAuthLoginWith() err = %v", err)
				}
				want := datadogauth.PrepareParams{
					Site:                 "datadoghq.com",
					Org:                  tt.org,
					RedirectURI:          datadogCLIRedirectURI,
					RegisterRedirectURIs: []string{datadogCLIRedirectURI, datadogServerRedirectURI()},
				}
				if diff := cmp.Diff(want, f.prepared()[0]); diff != "" {
					t.Errorf("prepareLogin params mismatch (-want +got):\n%s", diff)
				}
				wantOut := "Successfully logged into Datadog site: datadoghq.com" + tt.wantLabel
				if !strings.Contains(out.String(), wantOut) {
					t.Errorf("stdout does not contain %q:\n%s", wantOut, out.String())
				}
			})

			t.Run("logout", func(t *testing.T) {
				f := newFakeDatadogAuth()
				cmd, out, _ := newDatadogAuthOrgTestCmd(t, "datadoghq.com", tt.org)

				if err := datadogAuthLogoutWith(cmd, f.deps(t)); err != nil {
					t.Fatalf("datadogAuthLogoutWith() err = %v", err)
				}
				if diff := cmp.Diff([]string{"datadoghq.com|" + tt.org}, f.logoutCalls); diff != "" {
					t.Errorf("logout calls mismatch (-want +got):\n%s", diff)
				}
				wantOut := "Removed the local Datadog OAuth credentials for site: datadoghq.com" + tt.wantLabel
				if !strings.Contains(out.String(), wantOut) {
					t.Errorf("stdout does not contain %q:\n%s", wantOut, out.String())
				}
			})

			t.Run("refresh", func(t *testing.T) {
				f := newFakeDatadogAuth()
				f.ensureTok = &datadogauth.TokenSet{AccessToken: "at", ExpiresIn: 3600, IssuedAt: time.Now()}
				f.ensureOK = true
				cmd, out, _ := newDatadogAuthOrgTestCmd(t, "datadoghq.com", tt.org)

				if err := datadogAuthRefreshWith(cmd, f.deps(t)); err != nil {
					t.Fatalf("datadogAuthRefreshWith() err = %v", err)
				}
				if diff := cmp.Diff([]string{tt.org}, f.ensureOrgs); diff != "" {
					t.Errorf("ensureFreshToken orgs mismatch (-want +got):\n%s", diff)
				}
				wantOut := "Datadog OAuth token for site datadoghq.com" + tt.wantLabel + " is valid until"
				if !strings.Contains(out.String(), wantOut) {
					t.Errorf("stdout does not contain %q:\n%s", wantOut, out.String())
				}
			})

			t.Run("refresh without a token names the org in the login hint", func(t *testing.T) {
				f := newFakeDatadogAuth()
				f.ensureOK = false
				cmd, _, _ := newDatadogAuthOrgTestCmd(t, "datadoghq.com", tt.org)

				err := datadogAuthRefreshWith(cmd, f.deps(t))
				if err == nil {
					t.Fatal("datadogAuthRefreshWith() err = nil, want an error")
				}
				want := "Run '" + datadogLoginHint(tt.org) + "' first"
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to contain %q", err.Error(), want)
				}
			})
		})
	}
}
