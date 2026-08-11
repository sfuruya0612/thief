package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"

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

func TestGenerateSSOCacheKey(t *testing.T) {
	// AWS CLI と同じ SHA-1 hex 形式であること。
	got := generateSSOCacheKey("https://example.awsapps.com/start/")
	if len(got) != 40 {
		t.Errorf("cache key length = %d, want 40 (sha1 hex)", len(got))
	}
	// 同一入力に対して安定していること。
	if got != generateSSOCacheKey("https://example.awsapps.com/start/") {
		t.Error("cache key is not deterministic")
	}
	// 入力が違えばキーも変わること。
	if got == generateSSOCacheKey("https://other.awsapps.com/start/") {
		t.Error("different inputs should produce different keys")
	}
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

// okSSOTokenDeps は 4 段すべてが成功するダミーを返す。
// 各テストはこのうち 1 段だけを失敗に差し替える。
func okSSOTokenDeps() ssoTokenDeps {
	return ssoTokenDeps{
		registerClient: func(context.Context, string, string, string) (*awsinternal.SSOClientRegistration, error) {
			return &awsinternal.SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}, nil
		},
		startDeviceAuth: func(context.Context, string, *awsinternal.SSOClientRegistration, string) (*awsinternal.SSODeviceAuthorization, error) {
			return &awsinternal.SSODeviceAuthorization{DeviceCode: "dc", UserCode: "uc"}, nil
		},
		openBrowser: func(string) error { return nil },
		waitForToken: func(context.Context, string, *awsinternal.SSOClientRegistration, *awsinternal.SSODeviceAuthorization, string) (*awsinternal.SSOToken, error) {
			return &awsinternal.SSOToken{AccessToken: "token", ExpiresIn: 3600}, nil
		},
		// 標準出力への表示はテストに不要なため差し替える。
		display: func(string, string) {},
	}
}

// TestGetSSOTokenKeepsErrorChainAndDoesNotRepeatWording は、デバイス認可フローの
// 4 段それぞれで失敗したときに、getSSOToken の戻り値が次の 2 つを満たすことを検証する。
//
//   - errors.Is と errors.As が元のエラーへ到達できること (%v で包むとチェーンが切れて到達できない)
//   - 呼び出し先が既に述べた語句を、この層が重ねて述べていないこと
//
// 期待するメッセージを完全一致で固定しているのは、awsinternal 側の文言をそのまま
// 伝播させることがこの層の仕様だからである。ラップを足せば文字列が伸びて落ちる。
func TestGetSSOTokenKeepsErrorChainAndDoesNotRepeatWording(t *testing.T) {
	tests := []struct {
		name string
		// fail は okSSOTokenDeps の 1 段だけを、与えられたエラーを返すよう差し替える。
		fail func(deps *ssoTokenDeps, err error)
		// inner は呼び出し先が返すエラー。awsinternal の 3 つは自分でラップ済みの
		// エラーを返すため、その形を再現する。openBrowser は exec の裸のエラーを返す。
		inner   func(base error) error
		wantMsg string
	}{
		{
			name: "register client",
			fail: func(deps *ssoTokenDeps, err error) {
				deps.registerClient = func(context.Context, string, string, string) (*awsinternal.SSOClientRegistration, error) {
					return nil, err
				}
			},
			inner:   func(base error) error { return fmt.Errorf("register sso oidc client: %w", base) },
			wantMsg: "register sso oidc client: sso stage register failed",
		},
		{
			name: "start device authorization",
			fail: func(deps *ssoTokenDeps, err error) {
				deps.startDeviceAuth = func(context.Context, string, *awsinternal.SSOClientRegistration, string) (*awsinternal.SSODeviceAuthorization, error) {
					return nil, err
				}
			},
			inner:   func(base error) error { return fmt.Errorf("start sso oidc device authorization: %w", base) },
			wantMsg: "start sso oidc device authorization: sso stage register failed",
		},
		{
			name: "open browser",
			fail: func(deps *ssoTokenDeps, err error) {
				deps.openBrowser = func(string) error { return err }
			},
			// exec のエラーには文脈が無いため、この層でだけラップを足す。
			inner:   func(base error) error { return base },
			wantMsg: "open browser: sso stage register failed",
		},
		{
			name: "wait for token",
			fail: func(deps *ssoTokenDeps, err error) {
				deps.waitForToken = func(context.Context, string, *awsinternal.SSOClientRegistration, *awsinternal.SSODeviceAuthorization, string) (*awsinternal.SSOToken, error) {
					return nil, err
				}
			},
			inner:   func(base error) error { return fmt.Errorf("create sso oidc token: %w", base) },
			wantMsg: "create sso oidc token: sso stage register failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &ssoStageError{stage: "register"}
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

// TestGetSSOTokenBuildsCacheFromDeps は 4 段すべてが成功したときに、
// 取得したトークンと登録情報がキャッシュへ写ることを検証する。
// エラー経路のテストが使う okSSOTokenDeps が、そもそも成功経路を通ることの裏付けでもある。
func TestGetSSOTokenBuildsCacheFromDeps(t *testing.T) {
	const (
		region   = "ap-northeast-1"
		startURL = "https://example.awsapps.com/start/"
	)

	cache, err := getSSOTokenWith(context.Background(), region, startURL, okSSOTokenDeps())
	if err != nil {
		t.Fatalf("getSSOTokenWith() error = %v, want nil", err)
	}
	if cache.StartURL != startURL {
		t.Errorf("StartURL = %q, want %q", cache.StartURL, startURL)
	}
	if cache.Region != region {
		t.Errorf("Region = %q, want %q", cache.Region, region)
	}
	if cache.AccessToken != "token" {
		t.Errorf("AccessToken = %q, want %q", cache.AccessToken, "token")
	}
	if cache.ClientID != "cid" {
		t.Errorf("ClientID = %q, want %q", cache.ClientID, "cid")
	}
	if cache.ClientSecret != "secret" {
		t.Errorf("ClientSecret = %q, want %q", cache.ClientSecret, "secret")
	}
}

// TestGetSSOTokenPassesDeviceAuthorizationThrough は startDeviceAuth が返した応答が
// 後続の 3 段へ正しく渡ることを検証する。
//
//   - waitForToken には応答を丸ごと渡す。awsinternal 側はこの Interval と ExpiresIn から
//     ポーリング間隔と打ち切り期限を決める (RFC 8628 §3.2 / §3.5)。DeviceCode だけ取り出して
//     詰め直すと、サーバの指示が捨てられて既定値 (5 秒 / 600 秒) に落ちる。見た目には
//     動いてしまうため、渡った値の中身まで比較して検出する。
//   - openBrowser には VerificationURIComplete を渡す。ここを取り違えるとユーザーコードが
//     埋まっていない URL や device code がブラウザに渡り、承認に進めない。
//   - display には VerificationURI と UserCode を渡す (RFC 8628 §3.2 / §3.3)。start URL から
//     組み立てた値を渡すと、サーバの指示と食い違ったときに利用者の退路が塞がる。
func TestGetSSOTokenPassesDeviceAuthorizationThrough(t *testing.T) {
	const (
		region   = "ap-northeast-1"
		startURL = "https://example.awsapps.com/start/"
	)
	deviceAuth := &awsinternal.SSODeviceAuthorization{
		DeviceCode:              "dc",
		UserCode:                "uc",
		VerificationURI:         "https://device.sso/verify",
		VerificationURIComplete: "https://device.sso/verify?user_code=uc",
		Interval:                7,
		ExpiresIn:               900,
	}

	var (
		gotDeviceAuth  *awsinternal.SSODeviceAuthorization
		gotGrantType   string
		gotBrowserURL  string
		gotDisplayURL  string
		gotDisplayCode string
	)
	deps := okSSOTokenDeps()
	deps.startDeviceAuth = func(context.Context, string, *awsinternal.SSOClientRegistration, string) (*awsinternal.SSODeviceAuthorization, error) {
		return deviceAuth, nil
	}
	deps.openBrowser = func(url string) error {
		gotBrowserURL = url
		return nil
	}
	deps.display = func(url, userCode string) {
		gotDisplayURL = url
		gotDisplayCode = userCode
	}
	deps.waitForToken = func(_ context.Context, _ string, _ *awsinternal.SSOClientRegistration, da *awsinternal.SSODeviceAuthorization, grantType string) (*awsinternal.SSOToken, error) {
		gotDeviceAuth = da
		gotGrantType = grantType
		return &awsinternal.SSOToken{AccessToken: "token", ExpiresIn: 3600}, nil
	}

	if _, err := getSSOTokenWith(context.Background(), region, startURL, deps); err != nil {
		t.Fatalf("getSSOTokenWith() error = %v, want nil", err)
	}

	// ポインタの同一性ではなく中身を比較する。無害な写しを取る実装を落としたいのではなく、
	// Interval と ExpiresIn が欠けることを落としたい。
	if diff := cmp.Diff(deviceAuth, gotDeviceAuth); diff != "" {
		t.Errorf("waitForToken device authorization mismatch (-want +got):\n%s", diff)
	}
	if gotGrantType != ssoGrantType {
		t.Errorf("waitForToken grant type = %q, want %q", gotGrantType, ssoGrantType)
	}
	if gotBrowserURL != deviceAuth.VerificationURIComplete {
		t.Errorf("openBrowser url = %q, want %q", gotBrowserURL, deviceAuth.VerificationURIComplete)
	}
	if gotDisplayURL != deviceAuth.VerificationURI {
		t.Errorf("display verification uri = %q, want %q", gotDisplayURL, deviceAuth.VerificationURI)
	}
	// start URL から組み立てた推測値が渡っていないことを明示的に見る。上の比較だけでは
	// 期待値を書き換えれば通ってしまうため、捨てるべき値そのものを名指しで否定する。
	if guessed := startURL + "#/device"; gotDisplayURL == guessed {
		t.Errorf("display verification uri = %q; start URL から組み立てた推測値を渡している", guessed)
	}
	if gotDisplayCode != deviceAuth.UserCode {
		t.Errorf("display user code = %q, want %q", gotDisplayCode, deviceAuth.UserCode)
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
		// wantContains は出力に含まれていてほしい行。
		wantContains []string
		// wantOmits は出力に含まれてはならない断片。
		wantOmits []string
	}{
		{
			name:         "server returned a verification uri",
			uri:          verificationURI,
			wantContains: []string{verificationURI, userCode, "open the following URL:"},
			// 仕様上の裏付けが無い推測値を混ぜてはならない。
			wantOmits: []string{"#/device", "warning:"},
		},
		{
			// サーバの仕様違反。URI の行は省き、代わりに欠けていることを伝える。
			// user_code は §3.3.1 の MUST であり必ず出す。
			name:         "server omitted the verification uri",
			uri:          "",
			wantContains: []string{userCode, "warning: the authorization server did not return a verification URI"},
			wantOmits:    []string{"#/device", "open the following URL:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeSSOLoginPrompt(&buf, tt.uri, userCode)

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
// いずれかが nil のままだと getSSOToken が nil 関数を呼んで panic する。
// エラー経路のテストは差し替えたダミーを通るため、この漏れを検知できない。
func TestDefaultSSOTokenDepsIsFullyWired(t *testing.T) {
	deps := defaultSSOTokenDeps()
	if deps.registerClient == nil {
		t.Error("registerClient is nil")
	}
	if deps.startDeviceAuth == nil {
		t.Error("startDeviceAuth is nil")
	}
	if deps.openBrowser == nil {
		t.Error("openBrowser is nil")
	}
	if deps.waitForToken == nil {
		t.Error("waitForToken is nil")
	}
	if deps.display == nil {
		t.Error("display is nil")
	}
}

// TestDefaultSSOLoginDepsIsFullyWired は ssoLogin の本番用の依存がすべて埋まっていることを
// 検証する。いずれかが nil のままだと ssoLoginWith が nil 関数を呼んで panic する。
// ssoLoginWith のテストは差し替えたダミーを通るため、この漏れを検知できない。
func TestDefaultSSOLoginDepsIsFullyWired(t *testing.T) {
	deps := defaultSSOLoginDeps()
	if deps.getToken == nil {
		t.Error("getToken is nil")
	}
	if deps.saveCache == nil {
		t.Error("saveCache is nil")
	}
}

// ssoCtxKey は context に載せた値を取り出して同一性を確かめるためのキー。
type ssoCtxKey struct{}

// TestGetSSOTokenForwardsContextToEveryStage は、デバイス認可フローの各段が呼び出し元から
// 渡された context をそのまま受け取ることを検証する。
//
// waitForToken は RFC 8628 §3.5 に従いユーザの承認をポーリングで待つ。ここで context が
// 落ちていると Ctrl-C が届かず、承認されるかサーバ側の期限が切れるまで待ち続ける。
// registerClient と startDeviceAuth も AWS への往復であり、同じ理由で context が必要になる。
func TestGetSSOTokenForwardsContextToEveryStage(t *testing.T) {
	want := context.WithValue(context.Background(), ssoCtxKey{}, "carried")

	got := make(map[string]context.Context)
	deps := okSSOTokenDeps()
	deps.registerClient = func(ctx context.Context, _, _, _ string) (*awsinternal.SSOClientRegistration, error) {
		got["registerClient"] = ctx
		return &awsinternal.SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}, nil
	}
	deps.startDeviceAuth = func(ctx context.Context, _ string, _ *awsinternal.SSOClientRegistration, _ string) (*awsinternal.SSODeviceAuthorization, error) {
		got["startDeviceAuth"] = ctx
		return &awsinternal.SSODeviceAuthorization{DeviceCode: "dc", UserCode: "uc"}, nil
	}
	deps.waitForToken = func(ctx context.Context, _ string, _ *awsinternal.SSOClientRegistration, _ *awsinternal.SSODeviceAuthorization, _ string) (*awsinternal.SSOToken, error) {
		got["waitForToken"] = ctx
		return &awsinternal.SSOToken{AccessToken: "token", ExpiresIn: 3600}, nil
	}

	if _, err := getSSOTokenWith(want, "ap-northeast-1", "https://example.awsapps.com/start/", deps); err != nil {
		t.Fatalf("getSSOTokenWith() error = %v", err)
	}

	for _, stage := range []string{"registerClient", "startDeviceAuth", "waitForToken"} {
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
// context をトークン取得へ渡すことを検証する。
//
// デバイス認可フローはユーザがブラウザで承認するまで待つ。この CLI で利用者が
// Ctrl-C を押す可能性が最も高い場所であり、context が届かなければ待ち続ける。
// getSSOToken への引数を commandContext(cmd) から context.Background() に戻しても
// コンパイルも lint も通るため、ここで落とす。
func TestSSOLoginPassesCommandContextToTokenRetrieval(t *testing.T) {
	// loadConfig の先の config.Load が $XDG_CONFIG_HOME/thief/config.yaml と
	// $HOME/.thief/config.yaml を読む。実行環境の設定に依存しないよう空にする。
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := context.WithValue(context.Background(), ssoCtxKey{}, "carried")

	cmd := newSSOLoginCmd(t)
	cmd.SetContext(want)

	var got context.Context
	saved := 0
	err := ssoLoginWith(cmd, ssoLoginDeps{
		getToken: func(ctx context.Context, _, _ string) (*SSOTokenCache, error) {
			got = ctx
			return &SSOTokenCache{AccessToken: "token"}, nil
		},
		saveCache: func(*SSOTokenCache) error {
			saved++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ssoLoginWith() error = %v", err)
	}

	if got != want {
		t.Fatalf("getToken received %v, want the context set on the command", got)
	}
	if saved != 1 {
		t.Errorf("saveCache called %d times, want 1", saved)
	}
}

// TestSSOLoginFallsBackToBackgroundContext は Execute 系を通らないコマンドでも
// トークン取得が nil ではない context を受け取ることを検証する。
// nil の context をそのまま AWS SDK へ渡すと実行時に壊れる。
func TestSSOLoginFallsBackToBackgroundContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newSSOLoginCmd(t)
	if cmd.Context() != nil {
		t.Fatal("cobra.Command.Context() != nil; フォールバックの前提が変わった")
	}

	var got context.Context
	err := ssoLoginWith(cmd, ssoLoginDeps{
		getToken: func(ctx context.Context, _, _ string) (*SSOTokenCache, error) {
			got = ctx
			return &SSOTokenCache{AccessToken: "token"}, nil
		},
		saveCache: func(*SSOTokenCache) error { return nil },
	})
	if err != nil {
		t.Fatalf("ssoLoginWith() error = %v", err)
	}
	if got == nil {
		t.Fatal("getToken received a nil context, want non-nil")
	}
	if err := got.Err(); err != nil {
		t.Errorf("getToken ctx.Err() = %v, want nil", err)
	}
}

// okSSOGenerateConfigDeps は全段が成功するダミーを返す。
// アカウント 1 件・ロール 1 件で完走できる最小構成にしている。各テストは検証したい
// 段だけを差し替える。
func okSSOGenerateConfigDeps() ssoGenerateConfigDeps {
	return ssoGenerateConfigDeps{
		getToken: func(context.Context, string, string) (*SSOTokenCache, error) {
			return &SSOTokenCache{AccessToken: "token"}, nil
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
func newSSOLoginCmd(t *testing.T) *cobra.Command {
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

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}
