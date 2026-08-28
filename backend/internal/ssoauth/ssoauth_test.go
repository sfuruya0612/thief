package ssoauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"

	"github.com/google/go-cmp/cmp"
)

// stageError は errors.As による型の取り出しを検証するための独自エラー型。
// AWS SDK が返す型付き例外 (ssooidctypes.AuthorizationPendingException など) の代役である。
type stageError struct{ stage string }

func (e *stageError) Error() string { return "sso stage " + e.stage + " failed" }

// okDeps は各段が成功するダミーを返す。各テストはこのうち検証したい段だけを差し替える。
func okDeps() Deps {
	return Deps{
		RegisterClient: func(context.Context, string, string, string) (*awsinternal.SSOClientRegistration, error) {
			return &awsinternal.SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}, nil
		},
		StartDeviceAuth: func(context.Context, string, *awsinternal.SSOClientRegistration, string) (*awsinternal.SSODeviceAuthorization, error) {
			return &awsinternal.SSODeviceAuthorization{DeviceCode: "dc", UserCode: "uc"}, nil
		},
		WaitForToken: func(context.Context, string, *awsinternal.SSOClientRegistration, *awsinternal.SSODeviceAuthorization, string) (*awsinternal.SSOToken, error) {
			return &awsinternal.SSOToken{AccessToken: "token", ExpiresIn: 3600}, nil
		},
		SaveCache:   func(*TokenCache) error { return nil },
		RevokeToken: func(context.Context, string, string) error { return nil },
		ListTokens: func(string, string) ([]awsinternal.SSOCachedToken, error) {
			return []awsinternal.SSOCachedToken{}, nil
		},
		RemoveTokenCache: func(string, string) error { return nil },
		RemoveAllCache:   func(string) error { return nil },
	}
}

// okSession は Wait のテストが使う中間状態を返す。
func okSession() *Session {
	return &Session{
		Region:   "ap-northeast-1",
		StartURL: "https://example.awsapps.com/start/",
		Registration: &awsinternal.SSOClientRegistration{
			ClientID:              "cid",
			ClientSecret:          "secret",
			ClientSecretExpiresAt: 1767193200, // 2025-12-31T15:00:00Z
		},
		DeviceAuth: &awsinternal.SSODeviceAuthorization{
			DeviceCode:              "dc",
			UserCode:                "uc",
			VerificationURI:         "https://device.sso/verify",
			VerificationURIComplete: "https://device.sso/verify?user_code=uc",
			Interval:                7,
			ExpiresIn:               900,
		},
	}
}

// TestStartBuildsSessionFromServerResponse は成功時に、クライアント登録の結果と
// デバイス認可の応答が中間状態へそのまま写ることを検証する。
//
// Interval と ExpiresIn まで比較するのは、後続の Wait がサーバの指示 (RFC 8628 §3.2) を
// この中間状態経由で受け取るためである。ここで欠けると、指示が捨てられて既定値に落ちる。
func TestStartBuildsSessionFromServerResponse(t *testing.T) {
	const (
		region   = "ap-northeast-1"
		startURL = "https://example.awsapps.com/start/"
	)
	registration := &awsinternal.SSOClientRegistration{
		ClientID:              "cid",
		ClientSecret:          "secret",
		ClientSecretExpiresAt: 1767193200,
	}
	deviceAuth := &awsinternal.SSODeviceAuthorization{
		DeviceCode:              "dc",
		UserCode:                "uc",
		VerificationURI:         "https://device.sso/verify",
		VerificationURIComplete: "https://device.sso/verify?user_code=uc",
		Interval:                7,
		ExpiresIn:               900,
	}

	var gotClientName, gotClientType string
	var gotRegistration *awsinternal.SSOClientRegistration
	var gotStartURL string
	deps := okDeps()
	deps.RegisterClient = func(_ context.Context, _ string, name, kind string) (*awsinternal.SSOClientRegistration, error) {
		gotClientName = name
		gotClientType = kind
		return registration, nil
	}
	deps.StartDeviceAuth = func(_ context.Context, _ string, reg *awsinternal.SSOClientRegistration, url string) (*awsinternal.SSODeviceAuthorization, error) {
		gotRegistration = reg
		gotStartURL = url
		return deviceAuth, nil
	}

	sess, err := Start(context.Background(), region, startURL, deps)
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	want := &Session{Region: region, StartURL: startURL, Registration: registration, DeviceAuth: deviceAuth}
	if diff := cmp.Diff(want, sess); diff != "" {
		t.Errorf("session mismatch (-want +got):\n%s", diff)
	}
	if gotClientName != clientName {
		t.Errorf("RegisterClient client name = %q, want %q", gotClientName, clientName)
	}
	if gotClientType != clientType {
		t.Errorf("RegisterClient client type = %q, want %q", gotClientType, clientType)
	}
	if gotRegistration != registration {
		t.Errorf("StartDeviceAuth registration = %v, want the one returned by RegisterClient", gotRegistration)
	}
	if gotStartURL != startURL {
		t.Errorf("StartDeviceAuth start URL = %q, want %q", gotStartURL, startURL)
	}
}

// TestStartKeepsEmptyVerificationURIComplete は verification_uri_complete が欠落した応答を
// そのまま中間状態へ写し、start URL から組み立てた値で補わないことを検証する。
// この値は RFC 8628 §3.2 で OPTIONAL であり、組み立てた値には仕様上の裏付けが無い。
// 提示手段 (ブラウザを開くかどうか) の分岐は呼び出し側の責務であり、本パッケージは
// サーバの応答を歪めずに渡すことだけを保証する。
func TestStartKeepsEmptyVerificationURIComplete(t *testing.T) {
	const startURL = "https://example.awsapps.com/start/"

	deps := okDeps()
	deps.StartDeviceAuth = func(context.Context, string, *awsinternal.SSOClientRegistration, string) (*awsinternal.SSODeviceAuthorization, error) {
		return &awsinternal.SSODeviceAuthorization{
			DeviceCode:              "dc",
			UserCode:                "uc",
			VerificationURI:         "https://device.sso/verify",
			VerificationURIComplete: "",
		}, nil
	}

	sess, err := Start(context.Background(), "ap-northeast-1", startURL, deps)
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := sess.DeviceAuth.VerificationURIComplete; got != "" {
		t.Errorf("VerificationURIComplete = %q, want empty; サーバの応答に無い値を補ってはならない", got)
	}
	// 捨てるべき推測値そのものを名指しで否定する。
	if guessed := startURL + "#/device"; sess.DeviceAuth.VerificationURI == guessed {
		t.Errorf("VerificationURI = %q; start URL から組み立てた推測値を渡している", guessed)
	}
}

// TestStartKeepsErrorChainAndDoesNotRepeatWording は、開始段の 2 つの呼び出しのそれぞれで
// 失敗したときに、Start の戻り値が次の 2 つを満たすことを検証する。
//
//   - errors.Is と errors.As が元のエラーへ到達できること (%v で包むとチェーンが切れて到達できない)
//   - 呼び出し先が既に述べた語句を、この層が重ねて述べていないこと
//
// 期待するメッセージを完全一致で固定しているのは、awsinternal 側の文言をそのまま
// 伝播させることがこの層の仕様だからである。ラップを足せば文字列が伸びて落ちる。
func TestStartKeepsErrorChainAndDoesNotRepeatWording(t *testing.T) {
	tests := []struct {
		name    string
		fail    func(deps *Deps, err error)
		inner   func(base error) error
		wantMsg string
	}{
		{
			name: "register client",
			fail: func(deps *Deps, err error) {
				deps.RegisterClient = func(context.Context, string, string, string) (*awsinternal.SSOClientRegistration, error) {
					return nil, err
				}
			},
			inner:   func(base error) error { return fmt.Errorf("register sso oidc client: %w", base) },
			wantMsg: "register sso oidc client: sso stage start failed",
		},
		{
			name: "start device authorization",
			fail: func(deps *Deps, err error) {
				deps.StartDeviceAuth = func(context.Context, string, *awsinternal.SSOClientRegistration, string) (*awsinternal.SSODeviceAuthorization, error) {
					return nil, err
				}
			},
			inner:   func(base error) error { return fmt.Errorf("start sso oidc device authorization: %w", base) },
			wantMsg: "start sso oidc device authorization: sso stage start failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &stageError{stage: "start"}
			deps := okDeps()
			tt.fail(&deps, tt.inner(base))

			sess, err := Start(context.Background(), "ap-northeast-1", "https://example.awsapps.com/start/", deps)
			if err == nil {
				t.Fatalf("Start() error = nil, want an error")
			}
			if sess != nil {
				t.Errorf("Start() session = %v, want nil on error", sess)
			}
			if !errors.Is(err, base) {
				t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
			}
			var target *stageError
			if !errors.As(err, &target) {
				t.Errorf("errors.As() = false, want true; the chain is severed: %v", err)
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("Start() error = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

// TestWaitBuildsAndSavesCache は成功時に、取得したトークンと登録情報が AWS CLI 互換の
// キャッシュへ写り、そのキャッシュが保存されてから返ることを検証する。
func TestWaitBuildsAndSavesCache(t *testing.T) {
	sess := okSession()

	var gotDeviceAuth *awsinternal.SSODeviceAuthorization
	var gotGrantType string
	var saved []*TokenCache
	deps := okDeps()
	deps.WaitForToken = func(_ context.Context, _ string, _ *awsinternal.SSOClientRegistration, da *awsinternal.SSODeviceAuthorization, grant string) (*awsinternal.SSOToken, error) {
		gotDeviceAuth = da
		gotGrantType = grant
		return &awsinternal.SSOToken{AccessToken: "token", ExpiresIn: 3600}, nil
	}
	deps.SaveCache = func(cache *TokenCache) error {
		saved = append(saved, cache)
		return nil
	}

	before := time.Now().UTC()
	cache, err := Wait(context.Background(), sess, deps)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}

	// ポインタの同一性ではなく中身を比較する。無害な写しを取る実装を落としたいのではなく、
	// Interval と ExpiresIn が欠けること (サーバの指示が既定値に落ちること) を落としたい。
	if diff := cmp.Diff(sess.DeviceAuth, gotDeviceAuth); diff != "" {
		t.Errorf("WaitForToken device authorization mismatch (-want +got):\n%s", diff)
	}
	if gotGrantType != grantType {
		t.Errorf("WaitForToken grant type = %q, want %q", gotGrantType, grantType)
	}

	if cache.StartURL != sess.StartURL {
		t.Errorf("StartURL = %q, want %q", cache.StartURL, sess.StartURL)
	}
	if cache.Region != sess.Region {
		t.Errorf("Region = %q, want %q", cache.Region, sess.Region)
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
	if want := time.Unix(1767193200, 0).UTC().Format(time.RFC3339); cache.RegistrationExpiresAt != want {
		t.Errorf("RegistrationExpiresAt = %q, want %q", cache.RegistrationExpiresAt, want)
	}

	// ExpiresAt はトークンの有効秒数を現在時刻へ足した RFC 3339 の時刻である。
	// 実行時刻に依存するため、呼び出し前後の時刻 + 3600 秒の範囲に収まることで検証する。
	expiresAt, err := time.Parse(time.RFC3339, cache.ExpiresAt)
	if err != nil {
		t.Fatalf("ExpiresAt = %q is not RFC 3339: %v", cache.ExpiresAt, err)
	}
	lower := before.Add(3600 * time.Second).Truncate(time.Second)
	upper := after.Add(3600 * time.Second)
	if expiresAt.Before(lower) || expiresAt.After(upper) {
		t.Errorf("ExpiresAt = %v, want within [%v, %v]", expiresAt, lower, upper)
	}

	if len(saved) != 1 {
		t.Fatalf("SaveCache called %d times, want 1", len(saved))
	}
	if saved[0] != cache {
		t.Errorf("SaveCache received %v, want the same cache that Wait returns", saved[0])
	}
}

// TestWaitKeepsPollingErrorChainAndDoesNotRepeatWording は、ポーリングの打ち切り
// (タイムアウトを含む) で WaitForToken が失敗したときに、Wait の戻り値が errors.Is /
// errors.As で元のエラーへ到達でき、awsinternal 側の文言がそのまま伝播することを検証する。
// 失敗時に SaveCache を呼ばないこと (取得できていないトークンを保存しないこと) も併せて見る。
func TestWaitKeepsPollingErrorChainAndDoesNotRepeatWording(t *testing.T) {
	base := &stageError{stage: "wait"}
	deps := okDeps()
	deps.WaitForToken = func(context.Context, string, *awsinternal.SSOClientRegistration, *awsinternal.SSODeviceAuthorization, string) (*awsinternal.SSOToken, error) {
		return nil, fmt.Errorf("create sso oidc token: %w", base)
	}
	saveCalled := false
	deps.SaveCache = func(*TokenCache) error {
		saveCalled = true
		return nil
	}

	cache, err := Wait(context.Background(), okSession(), deps)
	if err == nil {
		t.Fatalf("Wait() error = nil, want an error")
	}
	if cache != nil {
		t.Errorf("Wait() cache = %v, want nil on error", cache)
	}
	if !errors.Is(err, base) {
		t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	var target *stageError
	if !errors.As(err, &target) {
		t.Errorf("errors.As() = false, want true; the chain is severed: %v", err)
	}
	if want := "create sso oidc token: sso stage wait failed"; err.Error() != want {
		t.Errorf("Wait() error = %q, want %q", err.Error(), want)
	}
	if saveCalled {
		t.Error("SaveCache was called, want it not to be called when polling fails")
	}
}

// TestWaitWrapsSaveCacheFailure はキャッシュ保存の失敗が文脈付きでラップされて返り、
// エラー時にキャッシュを返さないことを検証する。注入された SaveCache の実装が何を
// しようとしたかまで述べる保証は無いため、この層で文脈を足すのが仕様である。
func TestWaitWrapsSaveCacheFailure(t *testing.T) {
	base := &stageError{stage: "save"}
	deps := okDeps()
	deps.SaveCache = func(*TokenCache) error { return base }

	cache, err := Wait(context.Background(), okSession(), deps)
	if err == nil {
		t.Fatalf("Wait() error = nil, want an error")
	}
	if cache != nil {
		t.Errorf("Wait() cache = %v, want nil on error", cache)
	}
	if !errors.Is(err, base) {
		t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	if want := "save cache file: sso stage save failed"; err.Error() != want {
		t.Errorf("Wait() error = %q, want %q", err.Error(), want)
	}
}

// TestWaitRejectsIncompleteSession は、Start を経ていない不完全な Session を Wait が
// panic ではなくエラーで拒否することを検証する。Start と Wait は公開関数として分割
// されており、呼び出し順の契約 (Start → Wait) は破られうる。nil 参照の panic は
// リクエスト処理中の panic の禁止 (AGENTS.md) に反するため、境界で拒否する。
func TestWaitRejectsIncompleteSession(t *testing.T) {
	tests := []struct {
		name string
		sess *Session
	}{
		{name: "nil session", sess: nil},
		{name: "nil registration", sess: func() *Session { s := okSession(); s.Registration = nil; return s }()},
		{name: "nil device auth", sess: func() *Session { s := okSession(); s.DeviceAuth = nil; return s }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 不完全な Session では、どの外部処理 (ポーリング、保存) にも進んではならない。
			deps := okDeps()
			pollCalled := false
			deps.WaitForToken = func(context.Context, string, *awsinternal.SSOClientRegistration, *awsinternal.SSODeviceAuthorization, string) (*awsinternal.SSOToken, error) {
				pollCalled = true
				return &awsinternal.SSOToken{AccessToken: "token"}, nil
			}
			saveCalled := false
			deps.SaveCache = func(*TokenCache) error {
				saveCalled = true
				return nil
			}

			cache, err := Wait(context.Background(), tt.sess, deps)
			if err == nil {
				t.Fatal("Wait() error = nil, want an error for an incomplete session")
			}
			if want := "sso auth session is incomplete: it must be built by Start"; err.Error() != want {
				t.Errorf("Wait() error = %q, want %q", err.Error(), want)
			}
			if cache != nil {
				t.Errorf("Wait() cache = %v, want nil on error", cache)
			}
			if pollCalled {
				t.Error("WaitForToken was called, want it not to be called for an incomplete session")
			}
			if saveCalled {
				t.Error("SaveCache was called, want it not to be called for an incomplete session")
			}
		})
	}
}

// ctxKey は context に載せた値を取り出して同一性を確かめるためのキー。
type ctxKey struct{}

// TestStartAndWaitForwardContextToEveryStage は、デバイス認可フローの各段が呼び出し元から
// 渡された context をそのまま受け取ることを検証する。
//
// WaitForToken は RFC 8628 §3.5 に従いユーザの承認をポーリングで待つ。ここで context が
// 落ちていると Ctrl-C が届かず、承認されるかサーバ側の期限が切れるまで待ち続ける。
// RegisterClient と StartDeviceAuth も AWS への往復であり、同じ理由で context が必要になる。
func TestStartAndWaitForwardContextToEveryStage(t *testing.T) {
	want := context.WithValue(context.Background(), ctxKey{}, "carried")

	got := make(map[string]context.Context)
	deps := okDeps()
	deps.RegisterClient = func(ctx context.Context, _, _, _ string) (*awsinternal.SSOClientRegistration, error) {
		got["RegisterClient"] = ctx
		return &awsinternal.SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}, nil
	}
	deps.StartDeviceAuth = func(ctx context.Context, _ string, _ *awsinternal.SSOClientRegistration, _ string) (*awsinternal.SSODeviceAuthorization, error) {
		got["StartDeviceAuth"] = ctx
		return &awsinternal.SSODeviceAuthorization{DeviceCode: "dc", UserCode: "uc"}, nil
	}
	deps.WaitForToken = func(ctx context.Context, _ string, _ *awsinternal.SSOClientRegistration, _ *awsinternal.SSODeviceAuthorization, _ string) (*awsinternal.SSOToken, error) {
		got["WaitForToken"] = ctx
		return &awsinternal.SSOToken{AccessToken: "token", ExpiresIn: 3600}, nil
	}

	sess, err := Start(want, "ap-northeast-1", "https://example.awsapps.com/start/", deps)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := Wait(want, sess, deps); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	for _, stage := range []string{"RegisterClient", "StartDeviceAuth", "WaitForToken"} {
		ctx, ok := got[stage]
		if !ok {
			t.Errorf("%s was not called", stage)
			continue
		}
		if ctx != want {
			t.Errorf("%s received %v, want the context passed to Start / Wait", stage, ctx)
		}
	}
}

// TestDefaultDepsIsFullyWired は本番用の依存がすべて埋まっていることを検証する。
// いずれかが nil のままだと Start / Wait が nil 関数を呼んで panic する。
// 他のテストは差し替えたダミーを通るため、この漏れを検知できない。
func TestDefaultDepsIsFullyWired(t *testing.T) {
	deps := DefaultDeps()
	if deps.RegisterClient == nil {
		t.Error("RegisterClient is nil")
	}
	if deps.StartDeviceAuth == nil {
		t.Error("StartDeviceAuth is nil")
	}
	if deps.WaitForToken == nil {
		t.Error("WaitForToken is nil")
	}
	if deps.SaveCache == nil {
		t.Error("SaveCache is nil")
	}
	if deps.RevokeToken == nil {
		t.Error("RevokeToken is nil")
	}
	if deps.ListTokens == nil {
		t.Error("ListTokens is nil")
	}
	if deps.RemoveTokenCache == nil {
		t.Error("RemoveTokenCache is nil")
	}
	if deps.RemoveAllCache == nil {
		t.Error("RemoveAllCache is nil")
	}
}

// TestSaveCacheFileWritesAWSCLICompatibleFile は saveCacheFile が AWS CLI 互換の
// パス (~/.aws/sso/cache/<sha1(startUrl)>.json)、JSON 形状、パーミッションで
// キャッシュを書き出すことを検証する。HOME を一時ディレクトリへ差し替えて実際に書く。
func TestSaveCacheFileWritesAWSCLICompatibleFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cache := &TokenCache{
		StartURL:              "https://example.awsapps.com/start/",
		Region:                "ap-northeast-1",
		AccessToken:           "token",
		ExpiresAt:             "2026-08-24T00:00:00Z",
		ClientID:              "cid",
		ClientSecret:          "secret",
		RegistrationExpiresAt: "2026-11-22T00:00:00Z",
	}
	if err := saveCacheFile(cache); err != nil {
		t.Fatalf("saveCacheFile() error = %v, want nil", err)
	}

	path := filepath.Join(home, ".aws", "sso", "cache", cacheKey(cache.StartURL)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}

	var got TokenCache
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal cache file: %v", err)
	}
	if diff := cmp.Diff(*cache, got); diff != "" {
		t.Errorf("cache file mismatch (-want +got):\n%s", diff)
	}

	// AWS CLI 互換の JSON キー (camelCase) であること。構造体タグの退行を落とす。
	for _, key := range []string{"startUrl", "accessToken", "expiresAt", "clientId", "clientSecret", "registrationExpiresAt"} {
		if !strings.Contains(string(data), `"`+key+`"`) {
			t.Errorf("cache file %s does not contain key %q", data, key)
		}
	}

	// パーミッションのビット表現は Unix 系でのみ検証する (Windows では意味を持たない)。
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat cache file: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("cache file permission = %o, want 0600", perm)
		}
	}
}

// TestSaveCacheFileFailsWhenCacheDirIsNotCreatable はキャッシュディレクトリを作成できない
// 場合にエラーが返ることを検証する。~/.aws/sso をファイルとして先に置き、MkdirAll を失敗させる。
func TestSaveCacheFileFailsWhenCacheDirIsNotCreatable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".aws", "sso"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}

	err := saveCacheFile(&TokenCache{StartURL: "https://example.awsapps.com/start/"})
	if err == nil {
		t.Fatal("saveCacheFile() error = nil, want an error")
	}
	if want := "failed to create sso cache directory: "; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// TestCacheKey は AWS CLI と同じ SHA-1 hex 形式のキャッシュファイル名を検証する。
func TestCacheKey(t *testing.T) {
	got := cacheKey("https://example.awsapps.com/start/")
	if len(got) != 40 {
		t.Errorf("cache key length = %d, want 40 (sha1 hex)", len(got))
	}
	// 同一入力に対して安定していること。
	if got != cacheKey("https://example.awsapps.com/start/") {
		t.Error("cache key is not deterministic")
	}
	// 入力が違えばキーも変わること。
	if got == cacheKey("https://other.awsapps.com/start/") {
		t.Error("different inputs should produce different keys")
	}
}

// TestCacheDirPointsUnderHome はキャッシュディレクトリが $HOME/.aws/sso/cache を
// 指すことを検証する。AWS CLI と SDK が参照する場所からずれると、保存したトークンが
// リソースハンドラの SDK から見えなくなる。
func TestCacheDirPointsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error = %v, want nil", err)
	}
	if want := filepath.Join(home, ".aws", "sso", "cache"); got != want {
		t.Errorf("CacheDir() = %q, want %q", got, want)
	}
}

// --- Logout / LogoutAll ---

// logoutRecorder は Logout / LogoutAll のテストで Deps に差し込む記録用ダミー。
// ファイルシステムは使わず、呼び出しの順序と引数を記録する。
type logoutRecorder struct {
	tokens     []awsinternal.SSOCachedToken
	listErr    error
	revokeErr  func(region, accessToken string) error
	removeErr  error
	calls      []string // 呼び出し順 ("list", "revoke:<region>:<token>", "removeToken", "removeAll")
	listArgs   [][2]string
	revokeCtxs []context.Context
	removeArgs [][2]string
}

func (r *logoutRecorder) deps() Deps {
	d := okDeps()
	d.ListTokens = func(cacheDir, startURL string) ([]awsinternal.SSOCachedToken, error) {
		r.calls = append(r.calls, "list")
		r.listArgs = append(r.listArgs, [2]string{cacheDir, startURL})
		if r.listErr != nil {
			return nil, r.listErr
		}
		return r.tokens, nil
	}
	d.RevokeToken = func(ctx context.Context, region, accessToken string) error {
		r.calls = append(r.calls, "revoke:"+region+":"+accessToken)
		r.revokeCtxs = append(r.revokeCtxs, ctx)
		if r.revokeErr != nil {
			return r.revokeErr(region, accessToken)
		}
		return nil
	}
	d.RemoveTokenCache = func(cacheDir, startURL string) error {
		r.calls = append(r.calls, "removeToken")
		r.removeArgs = append(r.removeArgs, [2]string{cacheDir, startURL})
		return r.removeErr
	}
	d.RemoveAllCache = func(cacheDir string) error {
		r.calls = append(r.calls, "removeAll")
		r.removeArgs = append(r.removeArgs, [2]string{cacheDir, ""})
		return r.removeErr
	}
	return d
}

// captureDefaultLogs は既定の slog ロガーをバッファへ書くハンドラに差し替える。
// 既定ロガーはプロセス全体で共有されるため、これを使うテストは t.Parallel を呼ばない。
func captureDefaultLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// logoutTestHome は HOME を一時ディレクトリに差し替え、CacheDir() が返す値を返す。
func logoutTestHome(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error = %v", err)
	}
	return dir
}

const (
	logoutStartURL = "https://example.awsapps.com/start/"
	futureExpires  = "2999-01-01T00:00:00Z"
	pastExpires    = "2000-01-01T00:00:00Z"
)

func cachedToken(fileName, region, token, expiresAt string) awsinternal.SSOCachedToken {
	return awsinternal.SSOCachedToken{FileName: fileName, StartURL: logoutStartURL, Region: region, AccessToken: token, ExpiresAt: expiresAt}
}

func TestLogoutRevokesBeforeRemovingAndPassesCacheDirAndStartURL(t *testing.T) {
	cacheDir := logoutTestHome(t)
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{cachedToken("a.json", "ap-northeast-1", "tok-a", futureExpires)}}
	got, err := Logout(context.Background(), logoutStartURL, "us-east-1", rec.deps())
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if diff := cmp.Diff([]string{"list", "revoke:ap-northeast-1:tok-a", "removeToken"}, rec.calls); diff != "" {
		t.Errorf("call order mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([][2]string{{cacheDir, logoutStartURL}}, rec.listArgs); diff != "" {
		t.Errorf("ListTokens args mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([][2]string{{cacheDir, logoutStartURL}}, rec.removeArgs); diff != "" {
		t.Errorf("RemoveTokenCache args mismatch (-want +got):\n%s", diff)
	}
	if got.Revoked != 1 || len(got.RevokeFailed) != 0 {
		t.Errorf("result = %+v, want Revoked 1 and no failures", got)
	}
}

func TestLogoutAllRevokesBeforeRemovingAllAndListsEveryToken(t *testing.T) {
	cacheDir := logoutTestHome(t)
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{
		cachedToken("b.json", "us-east-1", "tok-b", futureExpires),
		cachedToken("a.json", "ap-northeast-1", "tok-a", futureExpires),
	}}
	got, err := LogoutAll(context.Background(), rec.deps())
	if err != nil {
		t.Fatalf("LogoutAll() error = %v", err)
	}
	// 失効はファイル名の辞書順に逐次呼ばれ、削除はその後。
	if diff := cmp.Diff([]string{"list", "revoke:ap-northeast-1:tok-a", "revoke:us-east-1:tok-b", "removeAll"}, rec.calls); diff != "" {
		t.Errorf("call order mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([][2]string{{cacheDir, ""}}, rec.listArgs); diff != "" {
		t.Errorf("ListTokens args mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([][2]string{{cacheDir, ""}}, rec.removeArgs); diff != "" {
		t.Errorf("RemoveAllCache args mismatch (-want +got):\n%s", diff)
	}
	if got.Revoked != 2 {
		t.Errorf("Revoked = %d, want 2", got.Revoked)
	}
}

func TestLogoutListFailureSkipsRevokeAndRemove(t *testing.T) {
	logoutTestHome(t)
	listErr := errors.New("read sso cache dir: boom")
	for name, run := range map[string]func(Deps) (LogoutResult, error){
		"Logout":    func(d Deps) (LogoutResult, error) { return Logout(context.Background(), logoutStartURL, "", d) },
		"LogoutAll": func(d Deps) (LogoutResult, error) { return LogoutAll(context.Background(), d) },
	} {
		t.Run(name, func(t *testing.T) {
			rec := &logoutRecorder{listErr: listErr}
			got, err := run(rec.deps())
			if !errors.Is(err, listErr) {
				t.Fatalf("error = %v, want %v", err, listErr)
			}
			if diff := cmp.Diff(LogoutResult{}, got); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"list"}, rec.calls); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLogoutCacheDirFailureSkipsList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.UserHomeDir does not depend on HOME on windows")
	}
	t.Setenv("HOME", "")
	for name, run := range map[string]func(Deps) (LogoutResult, error){
		"Logout":    func(d Deps) (LogoutResult, error) { return Logout(context.Background(), logoutStartURL, "", d) },
		"LogoutAll": func(d Deps) (LogoutResult, error) { return LogoutAll(context.Background(), d) },
	} {
		t.Run(name, func(t *testing.T) {
			rec := &logoutRecorder{}
			got, err := run(rec.deps())
			if err == nil {
				t.Fatal("error = nil, want cache directory error")
			}
			if diff := cmp.Diff(LogoutResult{}, got); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if len(rec.calls) != 0 {
				t.Errorf("calls = %v, want none", rec.calls)
			}
		})
	}
}

func TestLogoutRegionSelection(t *testing.T) {
	logoutTestHome(t)
	tests := []struct {
		name           string
		token          awsinternal.SSOCachedToken
		fallbackRegion string
		wantCalls      []string
		wantFailedErr  error
	}{
		{
			name:           "cache region wins over fallback",
			token:          cachedToken("a.json", "ap-northeast-1", "tok", futureExpires),
			fallbackRegion: "us-east-1",
			wantCalls:      []string{"list", "revoke:ap-northeast-1:tok", "removeToken"},
		},
		{
			name:           "missing region uses fallback",
			token:          cachedToken("a.json", "", "tok", futureExpires),
			fallbackRegion: "us-east-1",
			wantCalls:      []string{"list", "revoke:us-east-1:tok", "removeToken"},
		},
		{
			name:          "missing region and fallback is recorded as failure",
			token:         cachedToken("a.json", "", "tok", futureExpires),
			wantCalls:     []string{"list", "removeToken"},
			wantFailedErr: errRegionUnknown,
		},
		{
			name:           "empty access token is recorded as failure",
			token:          cachedToken("a.json", "ap-northeast-1", "", futureExpires),
			fallbackRegion: "us-east-1",
			wantCalls:      []string{"list", "removeToken"},
			wantFailedErr:  errAccessTokenMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{tt.token}}
			got, err := Logout(context.Background(), logoutStartURL, tt.fallbackRegion, rec.deps())
			if err != nil {
				t.Fatalf("Logout() error = %v", err)
			}
			if diff := cmp.Diff(tt.wantCalls, rec.calls); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			if tt.wantFailedErr == nil {
				if got.Revoked != 1 || len(got.RevokeFailed) != 0 {
					t.Errorf("result = %+v, want Revoked 1", got)
				}
				return
			}
			if got.Revoked != 0 || len(got.RevokeFailed) != 1 {
				t.Fatalf("result = %+v, want exactly 1 failure", got)
			}
			if !errors.Is(got.RevokeFailed[0].Err, tt.wantFailedErr) {
				t.Errorf("RevokeFailed[0].Err = %v, want %v", got.RevokeFailed[0].Err, tt.wantFailedErr)
			}
			if diff := cmp.Diff([]string{"a.json"}, got.RevokeFailed[0].FileNames); diff != "" {
				t.Errorf("FileNames mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLogoutGroupsEmptyAccessTokensIntoOneFailure は accessToken が空のファイルが複数
// あるとき、空文字列を同一トークンとして 1 つの RevokeFailure にまとめ、全ファイル名を
// 記録することを検証する (RevokeToken は呼ばれず、削除は行われる)。
func TestLogoutGroupsEmptyAccessTokensIntoOneFailure(t *testing.T) {
	logoutTestHome(t)
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{
		cachedToken("b.json", "ap-northeast-1", "", futureExpires),
		cachedToken("a.json", "ap-northeast-1", "", futureExpires),
	}}
	got, err := Logout(context.Background(), logoutStartURL, "", rec.deps())
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if diff := cmp.Diff([]string{"list", "removeToken"}, rec.calls); diff != "" {
		t.Errorf("calls mismatch (-want +got):\n%s", diff)
	}
	if got.Revoked != 0 || len(got.RevokeFailed) != 1 {
		t.Fatalf("result = %+v, want exactly 1 failure", got)
	}
	if !errors.Is(got.RevokeFailed[0].Err, errAccessTokenMissing) {
		t.Errorf("Err = %v, want errAccessTokenMissing", got.RevokeFailed[0].Err)
	}
	if diff := cmp.Diff([]string{"a.json", "b.json"}, got.RevokeFailed[0].FileNames); diff != "" {
		t.Errorf("FileNames mismatch (-want +got):\n%s", diff)
	}
}

func TestLogoutAllMissingRegionIsFailure(t *testing.T) {
	logoutTestHome(t)
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{cachedToken("a.json", "", "tok", futureExpires)}}
	got, err := LogoutAll(context.Background(), rec.deps())
	if err != nil {
		t.Fatalf("LogoutAll() error = %v", err)
	}
	if diff := cmp.Diff([]string{"list", "removeAll"}, rec.calls); diff != "" {
		t.Errorf("calls mismatch (-want +got):\n%s", diff)
	}
	if len(got.RevokeFailed) != 1 || !errors.Is(got.RevokeFailed[0].Err, errRegionUnknown) {
		t.Errorf("RevokeFailed = %+v, want one errRegionUnknown", got.RevokeFailed)
	}
}

func TestLogoutExpiresAtHandling(t *testing.T) {
	logoutTestHome(t)
	tests := []struct {
		name       string
		expiresAt  string
		wantRevoke bool
	}{
		{name: "expired token is skipped", expiresAt: pastExpires, wantRevoke: false},
		{name: "future token is revoked", expiresAt: futureExpires, wantRevoke: true},
		{name: "missing expiresAt is revoked", expiresAt: "", wantRevoke: true},
		{name: "unparseable expiresAt is revoked", expiresAt: "2020-06-14T05:26:13UTC", wantRevoke: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureDefaultLogs(t)
			rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{cachedToken("a.json", "ap-northeast-1", "tok", tt.expiresAt)}}
			got, err := Logout(context.Background(), logoutStartURL, "", rec.deps())
			if err != nil {
				t.Fatalf("Logout() error = %v", err)
			}
			wantCalls := []string{"list", "removeToken"}
			wantRevoked := 0
			if tt.wantRevoke {
				wantCalls = []string{"list", "revoke:ap-northeast-1:tok", "removeToken"}
				wantRevoked = 1
			}
			if diff := cmp.Diff(wantCalls, rec.calls); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			if got.Revoked != wantRevoked || len(got.RevokeFailed) != 0 {
				t.Errorf("result = %+v, want Revoked %d and no failures", got, wantRevoked)
			}
			want := fmt.Sprintf("level=INFO msg=\"sso logout completed\" revoked=%d revoke_failed=0", wantRevoked)
			if !strings.Contains(logs.String(), want) {
				t.Errorf("logs do not contain %q: %s", want, logs.String())
			}
		})
	}
}

func TestLogoutDeduplicatesSameAccessTokenUsingFirstFileByName(t *testing.T) {
	logoutTestHome(t)
	// 辞書順で最初の a.json (region ap-northeast-1、有効期限あり) が代表になる。
	// b.json は同じトークンだが region と expiresAt が食い違う。
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{
		cachedToken("b.json", "us-east-1", "tok", pastExpires),
		cachedToken("a.json", "ap-northeast-1", "tok", futureExpires),
	}}
	got, err := Logout(context.Background(), logoutStartURL, "", rec.deps())
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if diff := cmp.Diff([]string{"list", "revoke:ap-northeast-1:tok", "removeToken"}, rec.calls); diff != "" {
		t.Errorf("calls mismatch (-want +got):\n%s", diff)
	}
	if got.Revoked != 1 {
		t.Errorf("Revoked = %d, want 1 (unique access token)", got.Revoked)
	}
}

func TestLogoutRevokeFailureContinuesToRemoveAndRecordsAllFileNames(t *testing.T) {
	logoutTestHome(t)
	logs := captureDefaultLogs(t)
	revokeErr := errors.New("sso logout: UnauthorizedException")
	rec := &logoutRecorder{
		tokens: []awsinternal.SSOCachedToken{
			cachedToken("a.json", "ap-northeast-1", "tok-shared", futureExpires),
			cachedToken("b.json", "ap-northeast-1", "tok-shared", futureExpires),
			cachedToken("c.json", "ap-northeast-1", "tok-ok", futureExpires),
		},
		revokeErr: func(_, accessToken string) error {
			if accessToken == "tok-shared" {
				return revokeErr
			}
			return nil
		},
	}
	got, err := Logout(context.Background(), logoutStartURL, "", rec.deps())
	if err != nil {
		t.Fatalf("Logout() error = %v, want nil (revoke failure must not fail logout)", err)
	}
	if diff := cmp.Diff([]string{"list", "revoke:ap-northeast-1:tok-shared", "revoke:ap-northeast-1:tok-ok", "removeToken"}, rec.calls); diff != "" {
		t.Errorf("calls mismatch (-want +got):\n%s", diff)
	}
	if got.Revoked != 1 {
		t.Errorf("Revoked = %d, want 1", got.Revoked)
	}
	if len(got.RevokeFailed) != 1 {
		t.Fatalf("RevokeFailed = %+v, want 1 entry", got.RevokeFailed)
	}
	if diff := cmp.Diff([]string{"a.json", "b.json"}, got.RevokeFailed[0].FileNames); diff != "" {
		t.Errorf("FileNames mismatch (-want +got):\n%s", diff)
	}
	if !errors.Is(got.RevokeFailed[0].Err, revokeErr) {
		t.Errorf("Err = %v, want %v", got.RevokeFailed[0].Err, revokeErr)
	}
	for _, want := range []string{
		"level=WARN msg=\"sso token revoke failed\" files=\"[a.json b.json]\"",
		"level=INFO msg=\"sso logout completed\" revoked=1 revoke_failed=1",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs do not contain %q: %s", want, logs.String())
		}
	}
}

func TestLogoutRevokeContextHasThirtySecondDeadline(t *testing.T) {
	logoutTestHome(t)
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{cachedToken("a.json", "ap-northeast-1", "tok", futureExpires)}}
	before := time.Now()
	if _, err := Logout(context.Background(), logoutStartURL, "", rec.deps()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if len(rec.revokeCtxs) != 1 {
		t.Fatalf("RevokeToken called %d times, want 1", len(rec.revokeCtxs))
	}
	deadline, ok := rec.revokeCtxs[0].Deadline()
	if !ok {
		t.Fatal("RevokeToken ctx has no deadline, want 30s")
	}
	// before は Logout 呼び出しの直前に取るため、デッドラインは before + 30s より
	// わずかに後になる。前後 5 秒の幅で判定する。
	remaining := deadline.Sub(before)
	if remaining > revokeTimeout+5*time.Second || remaining < revokeTimeout-5*time.Second {
		t.Errorf("deadline is %v from start, want about %v", remaining, revokeTimeout)
	}
}

func TestLogoutCancelledParentContextSkipsRevokeButRemoves(t *testing.T) {
	logoutTestHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{
		cachedToken("a.json", "ap-northeast-1", "tok-a", futureExpires),
		cachedToken("b.json", "ap-northeast-1", "tok-b", futureExpires),
	}}
	got, err := Logout(ctx, logoutStartURL, "", rec.deps())
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if diff := cmp.Diff([]string{"list", "removeToken"}, rec.calls); diff != "" {
		t.Errorf("calls mismatch (-want +got):\n%s", diff)
	}
	if len(got.RevokeFailed) != 2 {
		t.Fatalf("RevokeFailed = %+v, want 2 entries", got.RevokeFailed)
	}
	for _, f := range got.RevokeFailed {
		if !errors.Is(f.Err, context.Canceled) {
			t.Errorf("Err = %v, want context.Canceled", f.Err)
		}
	}
}

func TestLogoutRemoveFailureReturnsErrorWithValidResult(t *testing.T) {
	logoutTestHome(t)
	removeErr := errors.New("remove sso cache file a.json: permission denied")
	for name, run := range map[string]func(Deps) (LogoutResult, error){
		"Logout":    func(d Deps) (LogoutResult, error) { return Logout(context.Background(), logoutStartURL, "", d) },
		"LogoutAll": func(d Deps) (LogoutResult, error) { return LogoutAll(context.Background(), d) },
	} {
		t.Run(name, func(t *testing.T) {
			rec := &logoutRecorder{
				tokens: []awsinternal.SSOCachedToken{
					cachedToken("a.json", "ap-northeast-1", "tok-a", futureExpires),
					cachedToken("b.json", "ap-northeast-1", "", futureExpires),
				},
				removeErr: removeErr,
			}
			got, err := run(rec.deps())
			if !errors.Is(err, removeErr) {
				t.Fatalf("error = %v, want %v", err, removeErr)
			}
			if got.Revoked != 1 || len(got.RevokeFailed) != 1 {
				t.Errorf("result = %+v, want Revoked 1 and 1 failure kept despite the removal error", got)
			}
		})
	}
}

func TestLogoutWithNoTokensOnlyRemovesAndLogsZero(t *testing.T) {
	logoutTestHome(t)
	logs := captureDefaultLogs(t)
	rec := &logoutRecorder{tokens: []awsinternal.SSOCachedToken{}}
	got, err := Logout(context.Background(), logoutStartURL, "", rec.deps())
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if diff := cmp.Diff([]string{"list", "removeToken"}, rec.calls); diff != "" {
		t.Errorf("calls mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(LogoutResult{}, got); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
	if want := "level=INFO msg=\"sso logout completed\" revoked=0 revoke_failed=0"; !strings.Contains(logs.String(), want) {
		t.Errorf("logs do not contain %q: %s", want, logs.String())
	}
}
