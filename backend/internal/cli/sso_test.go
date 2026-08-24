package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/ssoauth"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
)

func TestAppendProfiles(t *testing.T) {
	profiles := []ProfileConfig{
		{
			Name:      "my-account-adminaccess",
			StartUrl:  "https://example.awsapps.com/start/",
			Region:    "ap-northeast-1",
			AccountId: "123456789012",
			RoleName:  "AdminAccess",
		},
	}

	t.Run("append to empty config", func(t *testing.T) {
		got, err := appendProfiles("", profiles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantContains := []string{
			"[profile my-account-adminaccess]",
			"sso_start_url = https://example.awsapps.com/start/",
			"sso_region = ap-northeast-1",
			"sso_account_id = 123456789012",
			"sso_role_name = AdminAccess",
			"region = ap-northeast-1",
		}
		for _, want := range wantContains {
			if !strings.Contains(got, want) {
				t.Errorf("output does not contain %q:\n%s", want, got)
			}
		}
	})

	t.Run("keep existing config", func(t *testing.T) {
		existing := "[profile existing]\nregion = us-east-1\n"
		got, err := appendProfiles(existing, profiles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "[profile existing]") {
			t.Errorf("existing profile is lost:\n%s", got)
		}
		if !strings.Contains(got, "[profile my-account-adminaccess]") {
			t.Errorf("new profile is missing:\n%s", got)
		}
	})

	t.Run("skip duplicated profile", func(t *testing.T) {
		existing := "[profile my-account-adminaccess]\nregion = ap-northeast-1\n"
		got, err := appendProfiles(existing, profiles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Count(got, "[profile my-account-adminaccess]") != 1 {
			t.Errorf("duplicated profile should be skipped:\n%s", got)
		}
	})
}

func TestSelectIndices(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  []int
	}{
		{name: "all", input: "all", max: 3, want: []int{0, 1, 2}},
		{name: "all uppercase", input: "ALL", max: 2, want: []int{0, 1}},
		{name: "comma separated", input: "1,3", max: 3, want: []int{0, 2}},
		{name: "with spaces", input: " 1 , 2 ", max: 3, want: []int{0, 1}},
		{name: "out of range skipped", input: "0,4,2", max: 3, want: []int{1}},
		{name: "non-numeric skipped", input: "a,2", max: 3, want: []int{1}},
		{name: "empty", input: "", max: 3, want: []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetOut(&bytes.Buffer{})

			got := selectIndices(cmd, tt.input, tt.max, "account")
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("selectIndices(%q) mismatch (-want +got):\n%s", tt.input, diff)
			}
		})
	}
}

// ssoStageError は errors.As による型の取り出しを検証するための独自エラー型。
// AWS SDK が返す型付き例外 (ssooidctypes.AuthorizationPendingException など) の代役である。
type ssoStageError struct{ stage string }

func (e *ssoStageError) Error() string { return "sso stage " + e.stage + " failed" }

// okSession はテストが使うデバイス認可の中間状態を返す。
func okSession() *ssoauth.Session {
	return &ssoauth.Session{
		Region:       "ap-northeast-1",
		StartURL:     "https://example.awsapps.com/start/",
		Registration: &awsinternal.SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"},
		DeviceAuth: &awsinternal.SSODeviceAuthorization{
			DeviceCode:              "dc",
			UserCode:                "uc",
			VerificationURI:         "https://device.sso/verify",
			VerificationURIComplete: "https://device.sso/verify?user_code=uc",
		},
	}
}

// okSSOTokenDeps は各段が成功するダミーを返す。
// 各テストはこのうち検証したい段だけを差し替える。
func okSSOTokenDeps() ssoTokenDeps {
	return ssoTokenDeps{
		start: func(context.Context, string, string) (*ssoauth.Session, error) {
			return okSession(), nil
		},
		openBrowser: func(string) error { return nil },
		wait: func(context.Context, *ssoauth.Session) (*ssoauth.TokenCache, error) {
			return &ssoauth.TokenCache{AccessToken: "token"}, nil
		},
		// 標準出力・標準エラー出力への書き込みはテストに不要なため差し替える。
		display:              func(string, string, bool) {},
		reportBrowserFailure: func(error) {},
	}
}

// TestGetSSOTokenKeepsErrorChainAndDoesNotRepeatWording は、合成の 2 段 (開始、待機) の
// それぞれで失敗したときに、getSSOTokenWith の戻り値が次の 2 つを満たすことを検証する。
//
//   - errors.Is と errors.As が元のエラーへ到達できること (%v で包むとチェーンが切れて到達できない)
//   - 呼び出し先が既に述べた語句を、この層が重ねて述べていないこと
//
// 期待するメッセージを完全一致で固定しているのは、ssoauth 側の文言をそのまま
// 伝播させることがこの層の仕様だからである。ラップを足せば文字列が伸びて落ちる。
// openBrowser は失敗してもフローを中断しないため、このテーブルには含まない
// (TestGetSSOTokenContinuesWhenBrowserFailsToOpen で別途検証する)。
func TestGetSSOTokenKeepsErrorChainAndDoesNotRepeatWording(t *testing.T) {
	tests := []struct {
		name string
		// fail は okSSOTokenDeps の 1 段だけを、与えられたエラーを返すよう差し替える。
		fail func(deps *ssoTokenDeps, err error)
		// inner は呼び出し先が返すエラー。ssoauth 側は自分でラップ済みのエラーを返す
		// ため、その形を再現する (start 段の最初の失敗は RegisterClient で起きるので
		// その文言、wait 段は CreateToken ポーリングの文言になる)。
		inner   func(base error) error
		wantMsg string
	}{
		{
			name: "start",
			fail: func(deps *ssoTokenDeps, err error) {
				deps.start = func(context.Context, string, string) (*ssoauth.Session, error) {
					return nil, err
				}
			},
			inner:   func(base error) error { return fmt.Errorf("register sso oidc client: %w", base) },
			wantMsg: "register sso oidc client: sso stage start failed",
		},
		{
			name: "wait",
			fail: func(deps *ssoTokenDeps, err error) {
				deps.wait = func(context.Context, *ssoauth.Session) (*ssoauth.TokenCache, error) {
					return nil, err
				}
			},
			inner:   func(base error) error { return fmt.Errorf("create sso oidc token: %w", base) },
			wantMsg: "create sso oidc token: sso stage wait failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// base の stage は失敗させる合成の段 (ケース名) と揃える。全ケースで同じ値に
			// すると、期待メッセージがどの段の失敗を指すのか読み取れなくなる。
			base := &ssoStageError{stage: tt.name}
			deps := okSSOTokenDeps()
			tt.fail(&deps, tt.inner(base))

			cache, err := getSSOTokenWith(context.Background(), "ap-northeast-1", "https://example.awsapps.com/start/", deps)
			if err == nil {
				t.Fatalf("getSSOTokenWith() error = nil, want an error")
			}
			if cache != nil {
				t.Errorf("getSSOTokenWith() cache = %v, want nil on error", cache)
			}
			if !errors.Is(err, base) {
				t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
			}
			var target *ssoStageError
			if !errors.As(err, &target) {
				t.Errorf("errors.As() = false, want true; the chain is severed: %v", err)
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("getSSOTokenWith() error = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

// TestGetSSOTokenContinuesWhenBrowserFailsToOpen は、ブラウザの起動が失敗しても
// display が呼ばれ、トークン待機へ進み、その失敗が利用者へ報告されることを検証する。
// RFC 8628 §3.3.1 はブラウザ等による非テキストでの提示を MAY と定めており、失敗は
// フロー全体を中断する理由にならない。
func TestGetSSOTokenContinuesWhenBrowserFailsToOpen(t *testing.T) {
	deps := okSSOTokenDeps()
	browserErr := errors.New(`exec: "xdg-open": executable file not found in $PATH`)

	// 呼び出し順を記録する。display はブラウザの起動より先に呼ばれるべきであり
	// (利用者が既に verification_uri と user_code を見ている状態でブラウザの起動を
	// 試みる)、順序が入れ替わるとこの並びで検出する。
	var calls []string
	deps.openBrowser = func(string) error {
		calls = append(calls, "openBrowser")
		return browserErr
	}
	deps.display = func(string, string, bool) { calls = append(calls, "display") }
	deps.wait = func(context.Context, *ssoauth.Session) (*ssoauth.TokenCache, error) {
		calls = append(calls, "wait")
		return &ssoauth.TokenCache{AccessToken: "token"}, nil
	}
	var reported error
	deps.reportBrowserFailure = func(err error) { reported = err }

	cache, err := getSSOTokenWith(context.Background(), "ap-northeast-1", "https://example.awsapps.com/start/", deps)
	if err != nil {
		t.Fatalf("getSSOTokenWith() error = %v, want nil", err)
	}
	if cache == nil {
		t.Fatal("getSSOTokenWith() cache = nil, want non-nil")
	}
	if cache.AccessToken != "token" {
		t.Errorf("AccessToken = %q, want %q", cache.AccessToken, "token")
	}
	if want := []string{"display", "openBrowser", "wait"}; !slices.Equal(calls, want) {
		t.Errorf("call order = %v, want %v", calls, want)
	}
	if reported == nil {
		t.Fatal("reportBrowserFailure was not called")
	}
	if !errors.Is(reported, browserErr) {
		t.Errorf("reportBrowserFailure error = %v, want it to wrap %v", reported, browserErr)
	}
	if wantPrefix := "open browser: "; !strings.HasPrefix(reported.Error(), wantPrefix) {
		t.Errorf("reportBrowserFailure error = %q, want it to start with %q", reported.Error(), wantPrefix)
	}
}

// TestGetSSOTokenSkipsBrowserWhenVerificationURICompleteIsEmpty は、
// verification_uri_complete が空のときに openBrowser を呼ばないことを検証する。
// RFC 8628 §3.2 でこの値は OPTIONAL であり、空文字列を渡しても開く先が無い。
// display はこの場合も呼ばれ (user_code の提示は §3.3.1 で MUST)、この後ブラウザの
// 起動を試みないことを attemptingBrowser = false で伝える。true のまま渡すと
// 「開こうとしています」と表示しながら開かない、実際の動作と矛盾した案内になる。
func TestGetSSOTokenSkipsBrowserWhenVerificationURICompleteIsEmpty(t *testing.T) {
	sess := okSession()
	sess.DeviceAuth.VerificationURIComplete = ""

	deps := okSSOTokenDeps()
	deps.start = func(context.Context, string, string) (*ssoauth.Session, error) {
		return sess, nil
	}
	var (
		displayCalled     bool
		gotDisplayURL     string
		gotDisplayCode    string
		gotAttemptBrowser bool
	)
	deps.display = func(url, userCode string, attemptingBrowser bool) {
		displayCalled = true
		gotDisplayURL = url
		gotDisplayCode = userCode
		gotAttemptBrowser = attemptingBrowser
	}
	var browserCalled, reportCalled bool
	deps.openBrowser = func(string) error {
		browserCalled = true
		return nil
	}
	deps.reportBrowserFailure = func(error) { reportCalled = true }

	if _, err := getSSOTokenWith(context.Background(), "ap-northeast-1", "https://example.awsapps.com/start/", deps); err != nil {
		t.Fatalf("getSSOTokenWith() error = %v, want nil", err)
	}
	if browserCalled {
		t.Error("openBrowser was called, want it not to be called when VerificationURIComplete is empty")
	}
	if reportCalled {
		t.Error("reportBrowserFailure was called, want it not to be called when openBrowser was never attempted")
	}
	if !displayCalled {
		t.Fatal("display was not called, want the verification uri and user code to be presented")
	}
	if gotDisplayURL != sess.DeviceAuth.VerificationURI {
		t.Errorf("display verification uri = %q, want %q", gotDisplayURL, sess.DeviceAuth.VerificationURI)
	}
	if gotDisplayCode != sess.DeviceAuth.UserCode {
		t.Errorf("display user code = %q, want %q", gotDisplayCode, sess.DeviceAuth.UserCode)
	}
	if gotAttemptBrowser {
		t.Error("display attemptingBrowser = true, want false (VerificationURIComplete is empty)")
	}
}

// TestGetSSOTokenPassesSessionThrough は開始段が返した中間状態が後続へ正しく渡ることを
// 検証する。
//
//   - wait には start が返した Session を丸ごと渡す。ssoauth.Wait はこの中の
//     DeviceAuth (Interval と ExpiresIn を含む) からポーリング間隔と打ち切り期限を
//     決める (RFC 8628 §3.2 / §3.5)。別の値を詰め直すと、サーバの指示が捨てられる。
//   - openBrowser には VerificationURIComplete を渡す。ここを取り違えるとユーザーコードが
//     埋まっていない URL や device code がブラウザに渡り、承認に進めない。
//   - display には VerificationURI と UserCode を渡す (RFC 8628 §3.2 / §3.3)。start URL から
//     組み立てた値を渡すと、サーバの指示と食い違ったときに利用者の退路が塞がる。
func TestGetSSOTokenPassesSessionThrough(t *testing.T) {
	const startURL = "https://example.awsapps.com/start/"
	sess := okSession()

	var (
		gotSession        *ssoauth.Session
		gotBrowserURL     string
		gotDisplayURL     string
		gotDisplayCode    string
		gotAttemptBrowser bool
	)
	deps := okSSOTokenDeps()
	deps.start = func(context.Context, string, string) (*ssoauth.Session, error) {
		return sess, nil
	}
	deps.openBrowser = func(url string) error {
		gotBrowserURL = url
		return nil
	}
	deps.display = func(url, userCode string, attemptingBrowser bool) {
		gotDisplayURL = url
		gotDisplayCode = userCode
		gotAttemptBrowser = attemptingBrowser
	}
	deps.wait = func(_ context.Context, s *ssoauth.Session) (*ssoauth.TokenCache, error) {
		gotSession = s
		return &ssoauth.TokenCache{AccessToken: "token"}, nil
	}
	reportBrowserFailureCalled := false
	deps.reportBrowserFailure = func(error) { reportBrowserFailureCalled = true }

	if _, err := getSSOTokenWith(context.Background(), "ap-northeast-1", startURL, deps); err != nil {
		t.Fatalf("getSSOTokenWith() error = %v, want nil", err)
	}

	if gotSession != sess {
		t.Errorf("wait received %v, want the session returned by start", gotSession)
	}
	if gotBrowserURL != sess.DeviceAuth.VerificationURIComplete {
		t.Errorf("openBrowser url = %q, want %q", gotBrowserURL, sess.DeviceAuth.VerificationURIComplete)
	}
	if gotDisplayURL != sess.DeviceAuth.VerificationURI {
		t.Errorf("display verification uri = %q, want %q", gotDisplayURL, sess.DeviceAuth.VerificationURI)
	}
	// start URL から組み立てた推測値が渡っていないことを明示的に見る。上の比較だけでは
	// 期待値を書き換えれば通ってしまうため、捨てるべき値そのものを名指しで否定する。
	if guessed := startURL + "#/device"; gotDisplayURL == guessed {
		t.Errorf("display verification uri = %q; start URL から組み立てた推測値を渡している", guessed)
	}
	if gotDisplayCode != sess.DeviceAuth.UserCode {
		t.Errorf("display user code = %q, want %q", gotDisplayCode, sess.DeviceAuth.UserCode)
	}
	// VerificationURIComplete が非空なのでこの後ブラウザの起動を試みる。display に渡る
	// attemptingBrowser は、この後の挙動と食い違ってはならない。
	if !gotAttemptBrowser {
		t.Error("display attemptingBrowser = false, want true (VerificationURIComplete is non-empty)")
	}
	// openBrowser が成功した (VerificationURIComplete が非空で、エラーを返さない) 場合に
	// reportBrowserFailure を呼んでしまうと、成功しているのに利用者へ警告が出る。
	if reportBrowserFailureCalled {
		t.Error("reportBrowserFailure was called, want it not to be called when openBrowser succeeds")
	}
}

// TestWriteSSOLoginPrompt は承認手順の表示内容を検証する。
//
// RFC 8628 §3.3 は user_code と verification_uri を利用者へ提示することを求める。
// §3.3.1 は user_code の表示を MUST と定めており、verification_uri が欠けていても
// 省いてはならない。
func TestWriteSSOLoginPrompt(t *testing.T) {
	const (
		verificationURI = "https://device.sso.ap-northeast-1.amazonaws.com/"
		userCode        = "ABCD-EFGH"
	)

	tests := []struct {
		name string
		uri  string
		// attemptingBrowser はこの後ブラウザの起動を試みるかどうか。
		attemptingBrowser bool
		// wantContains は出力に含まれていてほしい行。
		wantContains []string
		// wantOmits は出力に含まれてはならない断片。
		wantOmits []string
	}{
		{
			name:              "server returned a verification uri",
			uri:               verificationURI,
			attemptingBrowser: true,
			wantContains:      []string{verificationURI, userCode, "open the following URL:", "Attempting to automatically open"},
			// 仕様上の裏付けが無い推測値を混ぜてはならない。
			wantOmits: []string{"#/device", "warning:"},
		},
		{
			// サーバの仕様違反。URI の行は省き、代わりに欠けていることを伝える。
			// user_code は §3.3.1 の MUST であり必ず出す。
			name:              "server omitted the verification uri",
			uri:               "",
			attemptingBrowser: true,
			wantContains:      []string{userCode, "warning: the authorization server did not return a verification URI"},
			// display はブラウザの起動より先に呼ばれるため、この時点ではブラウザが
			// 開けたかどうかは分からない。「開いたブラウザで承認する」という
			// ブラウザ側の状態を前提にした文言を含めてはならない。
			wantOmits: []string{"#/device", "open the following URL:", "authorize in the browser"},
		},
		{
			// verification_uri_complete が空でこの後ブラウザの起動を試みない場合。
			// 「開こうとしています」と出すと実際の動作と矛盾する。
			name:              "not attempting to open a browser",
			uri:               verificationURI,
			attemptingBrowser: false,
			wantContains:      []string{verificationURI, userCode, "open the following URL:"},
			wantOmits:         []string{"Attempting to automatically open"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeSSOLoginPrompt(&buf, tt.uri, userCode, tt.attemptingBrowser)

			got := buf.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output = %q, want it to contain %q", got, want)
				}
			}
			for _, omit := range tt.wantOmits {
				if strings.Contains(got, omit) {
					t.Errorf("output = %q, want it not to contain %q", got, omit)
				}
			}
		})
	}
}

// TestDefaultSSOTokenDepsIsFullyWired は本番用の依存がすべて埋まっていることを検証する。
// いずれかが nil のままだと getSSOTokenWith が nil 関数を呼んで panic する。
// エラー経路のテストは差し替えたダミーを通るため、この漏れを検知できない。
func TestDefaultSSOTokenDepsIsFullyWired(t *testing.T) {
	deps := defaultSSOTokenDeps(ssoauth.DefaultDeps())
	if deps.start == nil {
		t.Error("start is nil")
	}
	if deps.wait == nil {
		t.Error("wait is nil")
	}
	if deps.openBrowser == nil {
		t.Error("openBrowser is nil")
	}
	if deps.display == nil {
		t.Error("display is nil")
	}
	if deps.reportBrowserFailure == nil {
		t.Error("reportBrowserFailure is nil")
	}
}

// TestSSOTokenDepsWithoutSavingDoesNotSaveCache は、generate-config 用の依存の組み立てが
// キャッシュ保存を無効化していることを、実際の合成 (ssoauth.Start / ssoauth.Wait) を通して
// 検証する。generate-config は従来からトークンをキャッシュへ保存しない (保存は sso login
// の責務)。ssoauth.Wait は保存まで含むため、SaveCache の差し替えが失われると
// ~/.aws/sso/cache への書き込みが復活し、外部挙動が変わる。
func TestSSOTokenDepsWithoutSavingDoesNotSaveCache(t *testing.T) {
	saveCalled := false
	authDeps := ssoauth.Deps{
		RegisterClient: func(context.Context, string, string, string) (*awsinternal.SSOClientRegistration, error) {
			return &awsinternal.SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}, nil
		},
		StartDeviceAuth: func(context.Context, string, *awsinternal.SSOClientRegistration, string) (*awsinternal.SSODeviceAuthorization, error) {
			// ブラウザの起動 (本物の openBrowser) に進まないよう VerificationURIComplete は
			// 空にする。空のときブラウザを開かないことは
			// TestGetSSOTokenSkipsBrowserWhenVerificationURICompleteIsEmpty が検証している。
			return &awsinternal.SSODeviceAuthorization{
				DeviceCode:      "dc",
				UserCode:        "uc",
				VerificationURI: "https://device.sso/verify",
			}, nil
		},
		WaitForToken: func(context.Context, string, *awsinternal.SSOClientRegistration, *awsinternal.SSODeviceAuthorization, string) (*awsinternal.SSOToken, error) {
			return &awsinternal.SSOToken{AccessToken: "token", ExpiresIn: 3600}, nil
		},
		SaveCache: func(*ssoauth.TokenCache) error {
			saveCalled = true
			return nil
		},
	}

	deps := ssoTokenDepsWithoutSaving(authDeps)
	// 合成 (start / wait) は本物のまま使い、SaveCache が実行経路上で呼ばれないことを見る。
	// 標準出力への書き込みはテストの出力を汚さないよう差し替える。openBrowser も
	// ダミーに差し替える。本物のままだと、VerificationURIComplete が空なら呼ばれない
	// という分岐が退行したときに、テストが実際に OS のブラウザを開いてしまう。
	deps.display = func(string, string, bool) {}
	browserCalled := false
	deps.openBrowser = func(string) error {
		browserCalled = true
		return nil
	}

	cache, err := getSSOTokenWith(context.Background(), "ap-northeast-1", "https://example.awsapps.com/start/", deps)
	if err != nil {
		t.Fatalf("getSSOTokenWith() error = %v, want nil", err)
	}
	if cache == nil || cache.AccessToken != "token" {
		t.Errorf("cache = %v, want a cache with AccessToken %q", cache, "token")
	}
	if saveCalled {
		t.Error("SaveCache was called, want it not to be called (generate-config must not save the token cache)")
	}
	if browserCalled {
		t.Error("openBrowser was called, want it not to be called when VerificationURIComplete is empty")
	}
}

// TestWriteSSOBrowserFailureWarning はブラウザの起動失敗を報告する文言を検証する。
// verification_uri と user_code は writeSSOLoginPrompt で既に提示済みのため、ここでは
// それを使って別の端末からでも承認できることを伝える。
func TestWriteSSOBrowserFailureWarning(t *testing.T) {
	err := errors.New(`exec: "xdg-open": executable file not found in $PATH`)

	var buf bytes.Buffer
	writeSSOBrowserFailureWarning(&buf, err)

	got := buf.String()
	if !strings.Contains(got, err.Error()) {
		t.Errorf("output = %q, want it to contain %q", got, err.Error())
	}
	if !strings.HasPrefix(got, "warning:") {
		t.Errorf("output = %q, want it to start with %q", got, "warning:")
	}
	// 別の端末からでも承認できることを伝える指示そのものがこの関数の存在理由であり、
	// 単に失敗を告げるだけでは writeSSOLoginPrompt の警告と役割が重複する。
	if want := "open the URL shown above manually"; !strings.Contains(got, want) {
		t.Errorf("output = %q, want it to contain %q", got, want)
	}
}

// ssoCtxKey は context に載せた値を取り出して同一性を確かめるためのキー。
type ssoCtxKey struct{}

// TestGetSSOTokenForwardsContextToBothStages は、合成の 2 段 (開始、待機) が呼び出し元から
// 渡された context をそのまま受け取ることを検証する。
//
// トークン待機は RFC 8628 §3.5 に従いユーザの承認をポーリングで待つ。ここで context が
// 落ちていると Ctrl-C が届かず、承認されるかサーバ側の期限が切れるまで待ち続ける。
// 開始段も AWS への往復であり、同じ理由で context が必要になる。
// ssoauth パッケージ内部の各段への転送は ssoauth 側のテストが検証する。
func TestGetSSOTokenForwardsContextToBothStages(t *testing.T) {
	want := context.WithValue(context.Background(), ssoCtxKey{}, "carried")

	got := make(map[string]context.Context)
	deps := okSSOTokenDeps()
	deps.start = func(ctx context.Context, _, _ string) (*ssoauth.Session, error) {
		got["start"] = ctx
		return okSession(), nil
	}
	deps.wait = func(ctx context.Context, _ *ssoauth.Session) (*ssoauth.TokenCache, error) {
		got["wait"] = ctx
		return &ssoauth.TokenCache{AccessToken: "token"}, nil
	}

	if _, err := getSSOTokenWith(want, "ap-northeast-1", "https://example.awsapps.com/start/", deps); err != nil {
		t.Fatalf("getSSOTokenWith() error = %v", err)
	}

	for _, stage := range []string{"start", "wait"} {
		ctx, ok := got[stage]
		if !ok {
			t.Errorf("%s was not called", stage)
			continue
		}
		if ctx != want {
			t.Errorf("%s received %v, want the context passed to getSSOTokenWith", stage, ctx)
		}
	}
}

// TestSSOLoginPassesCommandContextToTokenRetrieval は sso login がコマンドに載った
// context をトークン取得へ渡し、成功時に aws sso login と同じ文言を出力することを検証する。
//
// デバイス認可フローはユーザがブラウザで承認するまで待つ。この CLI で利用者が
// Ctrl-C を押す可能性が最も高い場所であり、context が届かなければ待ち続ける。
// getSSOTokenWith への引数を commandContext(cmd) から context.Background() に戻しても
// コンパイルも lint も通るため、ここで落とす。
func TestSSOLoginPassesCommandContextToTokenRetrieval(t *testing.T) {
	// loadConfig の先の config.Load が $XDG_CONFIG_HOME/thief/config.yaml と
	// $HOME/.thief/config.yaml を読む。実行環境の設定に依存しないよう空にする。
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := context.WithValue(context.Background(), ssoCtxKey{}, "carried")

	var out bytes.Buffer
	cmd := newSSOLoginCmd(t, &out)
	cmd.SetContext(want)

	var got context.Context
	waited := 0
	deps := okSSOTokenDeps()
	deps.start = func(ctx context.Context, _, _ string) (*ssoauth.Session, error) {
		got = ctx
		return okSession(), nil
	}
	deps.wait = func(context.Context, *ssoauth.Session) (*ssoauth.TokenCache, error) {
		waited++
		return &ssoauth.TokenCache{AccessToken: "token"}, nil
	}
	if err := ssoLoginWith(cmd, deps); err != nil {
		t.Fatalf("ssoLoginWith() error = %v", err)
	}

	if got != want {
		t.Fatalf("start received %v, want the context set on the command", got)
	}
	if waited != 1 {
		t.Errorf("wait called %d times, want 1", waited)
	}
	// aws sso login コマンドと同じ成功時の出力を保つ (外部挙動の一部)。
	if want := "Successfully logged into Start URL: https://example.awsapps.com/start/\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// TestSSOLoginFallsBackToBackgroundContext は Execute 系を通らないコマンドでも
// トークン取得が nil ではない context を受け取ることを検証する。
// nil の context をそのまま AWS SDK へ渡すと実行時に壊れる。
func TestSSOLoginFallsBackToBackgroundContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var out bytes.Buffer
	cmd := newSSOLoginCmd(t, &out)
	if cmd.Context() != nil {
		t.Fatal("cobra.Command.Context() != nil; フォールバックの前提が変わった")
	}

	var got context.Context
	deps := okSSOTokenDeps()
	deps.start = func(ctx context.Context, _, _ string) (*ssoauth.Session, error) {
		got = ctx
		return okSession(), nil
	}
	if err := ssoLoginWith(cmd, deps); err != nil {
		t.Fatalf("ssoLoginWith() error = %v", err)
	}
	if got == nil {
		t.Fatal("start received a nil context, want non-nil")
	}
	if err := got.Err(); err != nil {
		t.Errorf("start ctx.Err() = %v, want nil", err)
	}
}

// TestSSOLoginWrapsTokenErrors は sso login がトークン取得の失敗を get token: で包んで
// 返すことを検証する。利用者は register sso oidc client などの段の文言だけでは、どの
// コマンドの何の操作で失敗したかを読み取れない。
func TestSSOLoginWrapsTokenErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	base := &ssoStageError{stage: "register"}
	deps := okSSOTokenDeps()
	deps.start = func(context.Context, string, string) (*ssoauth.Session, error) {
		return nil, fmt.Errorf("register sso oidc client: %w", base)
	}

	var out bytes.Buffer
	err := ssoLoginWith(newSSOLoginCmd(t, &out), deps)
	if err == nil {
		t.Fatal("ssoLoginWith() error = nil, want an error")
	}
	if !errors.Is(err, base) {
		t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	if want := "get token: register sso oidc client: sso stage register failed"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// okSSOGenerateConfigDeps は全段が成功するダミーを返す。
// アカウント 1 件・ロール 1 件で完走できる最小構成にしている。各テストは検証したい
// 段だけを差し替える。
func okSSOGenerateConfigDeps() ssoGenerateConfigDeps {
	return ssoGenerateConfigDeps{
		getToken: func(context.Context, string, string) (*ssoauth.TokenCache, error) {
			return &ssoauth.TokenCache{AccessToken: "token"}, nil
		},
		listAccounts: func(context.Context, string, string) ([]awsinternal.SSOAccountInfo, error) {
			return []awsinternal.SSOAccountInfo{{AccountID: "111111111111", AccountName: "account-1"}}, nil
		},
		listRoles: func(context.Context, string, string, string) ([]string, error) {
			return []string{"AdminAccess"}, nil
		},
		configPath:  func() (string, error) { return "/dummy/config", nil },
		readConfig:  func(string) (string, error) { return "", nil },
		writeConfig: func(string, string) error { return nil },
	}
}

// newSSOGenerateConfigCmd は ssoGenerateConfigWith が読むフラグと標準入力を持つコマンドを
// 返す。stdin にはアカウント選択・ロール選択の入力を改行区切りで渡す。
func newSSOGenerateConfigCmd(t *testing.T, stdin string, out *bytes.Buffer) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "generate-config"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("region", "", "")
	for name, value := range map[string]string{
		"url":     "example",
		"profile": "test-profile",
		"region":  "ap-northeast-1",
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s flag: %v", name, err)
		}
	}

	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(out)
	cmd.SetErr(out)
	return cmd
}

// TestSSOGenerateConfigWith_NoValidAccountsSelected は問題 1 (アカウント選択の分岐) を
// 検証する。範囲外の番号だけが入力された場合、selectIndices は 1 件も残さないため、
// ssoGenerateConfigWith はロール取得に進む前に no valid accounts selected を返す。
func TestSSOGenerateConfigWith_NoValidAccountsSelected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var out bytes.Buffer
	cmd := newSSOGenerateConfigCmd(t, "9\n", &out)

	err := ssoGenerateConfigWith(cmd, nil, okSSOGenerateConfigDeps())
	if err == nil {
		t.Fatal("ssoGenerateConfigWith() error = nil, want an error")
	}
	if got, want := err.Error(), "no valid accounts selected"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestSSOGenerateConfigWith_ListRolesFailureIsWrapped は問題 2 のうち、ロール取得の失敗が
// 対象のアカウント ID を含む文言でラップされて伝播することを検証する。
func TestSSOGenerateConfigWith_ListRolesFailureIsWrapped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	base := &ssoStageError{stage: "list-roles"}
	deps := okSSOGenerateConfigDeps()
	deps.listRoles = func(context.Context, string, string, string) ([]string, error) {
		return nil, base
	}

	var out bytes.Buffer
	cmd := newSSOGenerateConfigCmd(t, "1\n", &out)

	err := ssoGenerateConfigWith(cmd, nil, deps)
	if err == nil {
		t.Fatal("ssoGenerateConfigWith() error = nil, want an error")
	}
	if !errors.Is(err, base) {
		t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	if want := "list account roles for 111111111111: "; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// TestSSOGenerateConfigWith_AccountWithNoRolesIsSkipped は問題 2 のうち、ロールが 0 件の
// アカウントを continue でスキップし、警告を表示することを検証する。唯一のアカウントが
// これに当たるため、最終的にプロファイルが 1 件も無く no roles selected for any accounts
// になることも併せて確認する (問題 3 の入口の 1 つ)。
func TestSSOGenerateConfigWith_AccountWithNoRolesIsSkipped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	deps := okSSOGenerateConfigDeps()
	deps.listRoles = func(context.Context, string, string, string) ([]string, error) {
		return []string{}, nil
	}

	var out bytes.Buffer
	// ロール選択の入力は消費されないため、アカウント選択の 1 行だけで足りる。
	cmd := newSSOGenerateConfigCmd(t, "1\n", &out)

	err := ssoGenerateConfigWith(cmd, nil, deps)
	if err == nil {
		t.Fatal("ssoGenerateConfigWith() error = nil, want an error")
	}
	if got, want := err.Error(), "no roles selected for any accounts"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if want := "No roles found for account account-1 (111111111111)"; !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to contain %q", out.String(), want)
	}
}

// TestSSOGenerateConfigWith_SkippedAccountDoesNotBlockLaterAccounts は問題 2 の continue が
// 後続のアカウントの処理を妨げないことを検証する。ロール 0 件のアカウントはロール選択の
// プロンプトを出さずに次のアカウントへ進むため、continue を外すと 1 個目のアカウントが
// 2 個目のアカウント用のロール選択の入力行を奪って読み、ロールを 1 つも選べずに
// no roles selected for any accounts へ落ちる (2 個目のアカウントの入力が届かなくなる)。
// あわせて account-2 の 2 件目のロール (ReadOnly) が選ばれることも確認し、
// accounts[accountIndex] と roles[roleIndex] が呼び出しのたびに正しい要素を指すことも
// 併せて検証する。
func TestSSOGenerateConfigWith_SkippedAccountDoesNotBlockLaterAccounts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	deps := okSSOGenerateConfigDeps()
	deps.listAccounts = func(context.Context, string, string) ([]awsinternal.SSOAccountInfo, error) {
		return []awsinternal.SSOAccountInfo{
			{AccountID: "111111111111", AccountName: "account-1"},
			{AccountID: "222222222222", AccountName: "account-2"},
		}, nil
	}
	deps.listRoles = func(_ context.Context, _, _, accountID string) ([]string, error) {
		if accountID == "111111111111" {
			return []string{}, nil
		}
		return []string{"AdminAccess", "ReadOnly"}, nil
	}
	var gotContent string
	deps.writeConfig = func(_, content string) error {
		gotContent = content
		return nil
	}

	var out bytes.Buffer
	// 1 行目: アカウント選択 (all)。2 行目: ロール選択は account-2 の分だけ届く
	// (account-1 はロール 0 件で continue し、ロール選択のプロンプトを出さないため)。
	cmd := newSSOGenerateConfigCmd(t, "all\n2\n", &out)

	if err := ssoGenerateConfigWith(cmd, nil, deps); err != nil {
		t.Fatalf("ssoGenerateConfigWith() error = %v, want nil", err)
	}
	if want := "No roles found for account account-1 (111111111111)"; !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to contain %q", out.String(), want)
	}
	if want := "[profile account-2-readonly]"; !strings.Contains(gotContent, want) {
		t.Errorf("writeConfig content = %q, want it to contain %q", gotContent, want)
	}
	if strings.Contains(gotContent, "account-1") {
		t.Errorf("writeConfig content = %q, want it not to contain account-1 (its only role list is empty)", gotContent)
	}
}

// TestSSOGenerateConfigWith_NoRolesSelectedForAnyAccount は問題 3 を検証する。
// アカウントにロールは存在するが、ロール選択で範囲外の番号だけを入力した場合、
// selectedRoles が空になりプロファイルが 1 件も作られない。
func TestSSOGenerateConfigWith_NoRolesSelectedForAnyAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var out bytes.Buffer
	// 1 行目: アカウント選択。2 行目: ロール選択 (範囲外の番号)。
	cmd := newSSOGenerateConfigCmd(t, "1\n9\n", &out)

	err := ssoGenerateConfigWith(cmd, nil, okSSOGenerateConfigDeps())
	if err == nil {
		t.Fatal("ssoGenerateConfigWith() error = nil, want an error")
	}
	if got, want := err.Error(), "no roles selected for any accounts"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestSSOGenerateConfigWith_ExistingConfigReadFailureFallsBackToEmpty は問題 4 を検証する。
// readConfig が失敗しても警告を表示するだけで処理を止めず、空文字列を既存設定として
// 続行し、最終的に新しい設定の書き込みまで完走することを確認する。
func TestSSOGenerateConfigWith_ExistingConfigReadFailureFallsBackToEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	readErr := &ssoStageError{stage: "read-config"}
	deps := okSSOGenerateConfigDeps()
	deps.readConfig = func(string) (string, error) {
		return "", readErr
	}

	var gotPath, gotContent string
	deps.writeConfig = func(path, content string) error {
		gotPath = path
		gotContent = content
		return nil
	}

	var out bytes.Buffer
	// 1 行目: アカウント選択。2 行目: ロール選択 (all)。
	cmd := newSSOGenerateConfigCmd(t, "1\nall\n", &out)

	if err := ssoGenerateConfigWith(cmd, nil, deps); err != nil {
		t.Fatalf("ssoGenerateConfigWith() error = %v, want nil", err)
	}

	if want := "Warning: Reading existing config: sso stage read-config failed"; !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to contain %q", out.String(), want)
	}
	if gotPath != "/dummy/config" {
		t.Errorf("writeConfig path = %q, want %q", gotPath, "/dummy/config")
	}
	if want := "[profile account-1-adminaccess]"; !strings.Contains(gotContent, want) {
		t.Errorf("writeConfig content = %q, want it to contain %q", gotContent, want)
	}
}

// TestDefaultSSOGenerateConfigDepsIsFullyWired は ssoGenerateConfig の本番用の依存が
// すべて埋まっていることを検証する。いずれかが nil のままだと ssoGenerateConfigWith が
// nil 関数を呼んで panic する。他のテストは差し替えたダミーを通るため、この漏れを
// 検知できない。
func TestDefaultSSOGenerateConfigDepsIsFullyWired(t *testing.T) {
	deps := defaultSSOGenerateConfigDeps()
	if deps.getToken == nil {
		t.Error("getToken is nil")
	}
	if deps.listAccounts == nil {
		t.Error("listAccounts is nil")
	}
	if deps.listRoles == nil {
		t.Error("listRoles is nil")
	}
	if deps.configPath == nil {
		t.Error("configPath is nil")
	}
	if deps.readConfig == nil {
		t.Error("readConfig is nil")
	}
	if deps.writeConfig == nil {
		t.Error("writeConfig is nil")
	}
}

// newSSOLoginCmd は ssoLoginWith が読むフラグだけを持つコマンドを返す。
// profile と region は明示指定して config の解決結果に依存しないようにする。
func newSSOLoginCmd(t *testing.T, out *bytes.Buffer) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "login"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("region", "", "")
	for name, value := range map[string]string{
		"url":     "example",
		"profile": "test-profile",
		"region":  "ap-northeast-1",
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s flag: %v", name, err)
		}
	}

	cmd.SetOut(out)
	cmd.SetErr(out)
	return cmd
}
