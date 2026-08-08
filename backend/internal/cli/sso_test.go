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
//   - display には start URL と UserCode を渡す。
func TestGetSSOTokenPassesDeviceAuthorizationThrough(t *testing.T) {
	const (
		region   = "ap-northeast-1"
		startURL = "https://example.awsapps.com/start/"
	)
	deviceAuth := &awsinternal.SSODeviceAuthorization{
		DeviceCode:              "dc",
		UserCode:                "uc",
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
	if gotDisplayURL != startURL {
		t.Errorf("display start url = %q, want %q", gotDisplayURL, startURL)
	}
	if gotDisplayCode != deviceAuth.UserCode {
		t.Errorf("display user code = %q, want %q", gotDisplayCode, deviceAuth.UserCode)
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
