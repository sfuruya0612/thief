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
		waitForToken: func(context.Context, string, *awsinternal.SSOClientRegistration, string, string) (*awsinternal.SSOToken, error) {
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
				deps.waitForToken = func(context.Context, string, *awsinternal.SSOClientRegistration, string, string) (*awsinternal.SSOToken, error) {
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
