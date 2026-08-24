package ssoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		SaveCache: func(*TokenCache) error { return nil },
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
