package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
)

// mockSSOOidcRegisterClientAPI は ssoOidcRegisterClientAPI の手書きモック。
// 受け取った Input を記録し、用意したレスポンスかエラーを返す。
type mockSSOOidcRegisterClientAPI struct {
	out    *ssooidc.RegisterClientOutput
	err    error
	inputs []*ssooidc.RegisterClientInput
}

func (m *mockSSOOidcRegisterClientAPI) RegisterClient(_ context.Context, params *ssooidc.RegisterClientInput, _ ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error) {
	m.inputs = append(m.inputs, params)
	return m.out, m.err
}

// TestRegisterSSOClientSendsClientNameAndType は RegisterClientInput が引数どおりに
// 構築されること、およびレスポンスが SSOClientRegistration へ写ることを検証する。
func TestRegisterSSOClientSendsClientNameAndType(t *testing.T) {
	mock := &mockSSOOidcRegisterClientAPI{
		out: &ssooidc.RegisterClientOutput{
			ClientId:              aws.String("cid"),
			ClientSecret:          aws.String("secret"),
			ClientSecretExpiresAt: 1234567890,
		},
	}

	got, err := registerSSOClient(context.Background(), mock, "thief", "public")
	if err != nil {
		t.Fatalf("registerSSOClient() error = %v, want nil", err)
	}

	if len(mock.inputs) != 1 {
		t.Fatalf("RegisterClient called %d times, want 1", len(mock.inputs))
	}
	if name := ptrStr(mock.inputs[0].ClientName); name != "thief" {
		t.Errorf("ClientName = %q, want %q", name, "thief")
	}
	if typ := ptrStr(mock.inputs[0].ClientType); typ != "public" {
		t.Errorf("ClientType = %q, want %q", typ, "public")
	}

	want := &SSOClientRegistration{ClientID: "cid", ClientSecret: "secret", ClientSecretExpiresAt: 1234567890}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("registration mismatch (-want +got):\n%s", diff)
	}
}

// TestRegisterSSOClientWrapsError は失敗時に API 名を含む文言でラップし、
// 元のエラーへ errors.Is で到達できることを検証する。
func TestRegisterSSOClientWrapsError(t *testing.T) {
	base := errors.New("boom")
	mock := &mockSSOOidcRegisterClientAPI{err: base}

	got, err := registerSSOClient(context.Background(), mock, "thief", "public")
	if got != nil {
		t.Errorf("registration = %v, want nil on error", got)
	}
	if !errors.Is(err, base) {
		t.Fatalf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	if msg := err.Error(); msg != "register sso oidc client: boom" {
		t.Errorf("error = %q, want %q", msg, "register sso oidc client: boom")
	}
}

// mockSSOOidcStartDeviceAuthorizationAPI は ssoOidcStartDeviceAuthorizationAPI の手書きモック。
type mockSSOOidcStartDeviceAuthorizationAPI struct {
	out    *ssooidc.StartDeviceAuthorizationOutput
	err    error
	inputs []*ssooidc.StartDeviceAuthorizationInput
}

func (m *mockSSOOidcStartDeviceAuthorizationAPI) StartDeviceAuthorization(_ context.Context, params *ssooidc.StartDeviceAuthorizationInput, _ ...func(*ssooidc.Options)) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	m.inputs = append(m.inputs, params)
	return m.out, m.err
}

// TestStartSSODeviceAuthorizationSendsRegistrationAndStartURL は
// StartDeviceAuthorizationInput が登録情報と start URL から構築されること、
// およびレスポンスが SSODeviceAuthorization へ写ることを検証する。
//
// Interval と ExpiresIn は RFC 8628 §3.2 の interval / expires_in であり、ポーリングの
// 間隔と打ち切り期限を決める。写し漏らすと newSSOTokenPollPolicy が既定値に倒れ、
// サーバの指示が黙って無視される。値は既定値 (5 秒 / 600 秒) のどちらとも異なるものを
// 使う。既定値と同じにすると、写さずに既定へ倒す実装でもこのテストが通ってしまう。
func TestStartSSODeviceAuthorizationSendsRegistrationAndStartURL(t *testing.T) {
	mock := &mockSSOOidcStartDeviceAuthorizationAPI{
		out: &ssooidc.StartDeviceAuthorizationOutput{
			DeviceCode:              aws.String("dc"),
			UserCode:                aws.String("uc"),
			VerificationUriComplete: aws.String("https://device.sso/verify?user_code=uc"),
			Interval:                7,
			ExpiresIn:               900,
		},
	}
	reg := &SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}
	const startURL = "https://example.awsapps.com/start/"

	got, err := startSSODeviceAuthorization(context.Background(), mock, reg, startURL)
	if err != nil {
		t.Fatalf("startSSODeviceAuthorization() error = %v, want nil", err)
	}

	if len(mock.inputs) != 1 {
		t.Fatalf("StartDeviceAuthorization called %d times, want 1", len(mock.inputs))
	}
	in := mock.inputs[0]
	if id := ptrStr(in.ClientId); id != "cid" {
		t.Errorf("ClientId = %q, want %q", id, "cid")
	}
	if secret := ptrStr(in.ClientSecret); secret != "secret" {
		t.Errorf("ClientSecret = %q, want %q", secret, "secret")
	}
	if url := ptrStr(in.StartUrl); url != startURL {
		t.Errorf("StartUrl = %q, want %q", url, startURL)
	}

	want := &SSODeviceAuthorization{
		DeviceCode:              "dc",
		UserCode:                "uc",
		VerificationURIComplete: "https://device.sso/verify?user_code=uc",
		Interval:                7,
		ExpiresIn:               900,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("device authorization mismatch (-want +got):\n%s", diff)
	}
}

// TestStartSSODeviceAuthorizationWrapsError は失敗時に API 名を含む文言でラップし、
// 元のエラーへ errors.Is で到達できることを検証する。
func TestStartSSODeviceAuthorizationWrapsError(t *testing.T) {
	base := errors.New("boom")
	mock := &mockSSOOidcStartDeviceAuthorizationAPI{err: base}

	got, err := startSSODeviceAuthorization(context.Background(), mock, &SSOClientRegistration{}, "https://example.awsapps.com/start/")
	if got != nil {
		t.Errorf("device authorization = %v, want nil on error", got)
	}
	if !errors.Is(err, base) {
		t.Fatalf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	want := "start sso oidc device authorization: boom"
	if msg := err.Error(); msg != want {
		t.Errorf("error = %q, want %q", msg, want)
	}
}

// ssoCreateTokenStep は CreateToken の 1 回の呼び出しが返す結果。
type ssoCreateTokenStep struct {
	out *ssooidc.CreateTokenOutput
	err error
}

// mockSSOOidcCreateTokenAPI は ssoOidcCreateTokenAPI の手書きモック。
// steps に用意した結果を 1 呼び出しにつき 1 つ返し、受け取った Input を記録する。
// ctx は意図的に見ない。検証対象は waitForSSOToken の select による待機の中断であり、
// モックが先に ctx エラーを返してしまうとその分岐を通らなくなる。
// TestWaitForSSOTokenReturnsContextError は待機に入る瞬間にキャンセルするため、
// CreateToken 自体は生きた ctx で呼ばれる。本番との乖離はそこには無い。
type mockSSOOidcCreateTokenAPI struct {
	steps  []ssoCreateTokenStep
	inputs []*ssooidc.CreateTokenInput
}

func (m *mockSSOOidcCreateTokenAPI) CreateToken(_ context.Context, params *ssooidc.CreateTokenInput, _ ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	m.inputs = append(m.inputs, params)
	idx := len(m.inputs) - 1
	if idx >= len(m.steps) {
		return nil, fmt.Errorf("unexpected CreateToken call %d: only %d steps prepared", idx+1, len(m.steps))
	}
	return m.steps[idx].out, m.steps[idx].err
}

// ssoCreateTokenSuccess は成功レスポンスを返す step を作る。
func ssoCreateTokenSuccess() ssoCreateTokenStep {
	return ssoCreateTokenStep{out: &ssooidc.CreateTokenOutput{
		AccessToken: aws.String("token"),
		ExpiresIn:   3600,
	}}
}

// ssoOidcOperationError は実 SDK が返すのと同じ形にエラーを包む。
//
// *ssooidc.Client は middleware を抜けたエラーを必ず smithy.OperationError で包んで返す
// (api_client.go の invokeOperation)。つまり本番のポーリングに typed exception が
// 最上位のエラーとして届くことは一度もない。モックが裸の exception を返すと、
// waitForSSOToken の errors.As を型アサーションや型 switch に書き換えてもテストが通り、
// 本番では SlowDown と AuthorizationPending が両方とも即時失敗の分岐に落ちる。
// それを検出できるようにするため、モックも同じ深さで包む。
func ssoOidcOperationError(err error) error {
	return &smithy.OperationError{
		ServiceID:     "SSO OIDC",
		OperationName: "CreateToken",
		Err:           err,
	}
}

// ssoCreateTokenPending は承認待ちを返す step を作る。
func ssoCreateTokenPending() ssoCreateTokenStep {
	return ssoCreateTokenStep{err: ssoOidcOperationError(&ssooidctypes.AuthorizationPendingException{})}
}

// ssoCreateTokenSlowDown はレート制限を返す step を作る。
func ssoCreateTokenSlowDown() ssoCreateTokenStep {
	return ssoCreateTokenStep{err: ssoOidcOperationError(&ssooidctypes.SlowDownException{})}
}

// fakeSSOClock はテストの中だけで進む時計。after は要求された待ち時間を記録し、
// その分だけ時刻を進めて即座に発火する。実際には待たないため、テストの実行時間が
// ポーリング間隔にも device code の有効期限にも依存しない。
//
// after の中で時刻を進めるのが要点である。waitForSSOToken の打ち切りは now が返す時刻で
// 判定するため、時計が止まったままだと待機をいくら重ねても期限に到達せず、テストが
// 終わらない。逆に言えば、この時計は「本番で実際に流れる時間」をそのまま模している。
//
// waitForSSOToken は単一の goroutine から now と after を呼ぶため、排他は要らない。
type fakeSSOClock struct {
	origin   time.Time
	current  time.Time
	recorded []time.Duration
}

// newFakeSSOClock は固定の起点から始まる時計を返す。
// 起点の値そのものに意味は無く、Add で単調に進むだけである。ゼロ値の time.Time を
// 使わないのは、失敗時のログを絶対時刻として読めるようにするためだけである。
func newFakeSSOClock() *fakeSSOClock {
	origin := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &fakeSSOClock{origin: origin, current: origin}
}

func (c *fakeSSOClock) Now() time.Time { return c.current }

func (c *fakeSSOClock) After(d time.Duration) <-chan time.Time {
	c.recorded = append(c.recorded, d)
	c.current = c.current.Add(d)
	ch := make(chan time.Time, 1)
	ch <- c.current
	return ch
}

// elapsed は起点から進んだ時間、すなわち待機の合計を返す。
func (c *fakeSSOClock) elapsed() time.Duration {
	return c.current.Sub(c.origin)
}

const (
	// testSSOPollInterval はテストで使う初期間隔。本番の既定値
	// ssoTokenPollDefaultInterval とは別の値にしてある。同値にすると、policy.interval
	// ではなく定数を直接読むように壊してもテストが通ってしまう。
	testSSOPollInterval = 3 * time.Second

	// testSSOPollTimeout は打ち切りに達しないだけの十分な猶予。本番の既定値
	// ssoTokenPollDefaultTimeout とは別の値にしてある。理由は testSSOPollInterval と
	// 同じで、policy.timeout ではなく定数を直接読むように壊したときに検出できるように
	// するため。
	testSSOPollTimeout = 7 * time.Minute
)

// withFakeClock は方針の now と after だけを fake clock に差し替えた写しを返す。
// interval と timeout は呼び出し側が渡した値をそのまま残す。
func withFakeClock(policy ssoTokenPollPolicy, clock *fakeSSOClock) ssoTokenPollPolicy {
	policy.now = clock.Now
	policy.after = clock.After
	return policy
}

// testSSOTokenPollPolicy はテスト用の間隔と猶予を持ち、待機を記録するだけの方針を返す。
func testSSOTokenPollPolicy(clock *fakeSSOClock) ssoTokenPollPolicy {
	return withFakeClock(ssoTokenPollPolicy{
		interval: testSSOPollInterval,
		timeout:  testSSOPollTimeout,
	}, clock)
}

// TestWaitForSSOTokenPollIntervals は CreateToken が返すエラーの種類に応じて
// 次の試行までの待ち時間がどう変わるかを検証する。
//
// SlowDown は間隔を 5 秒増やし、AuthorizationPending は間隔を変えない。
// 5 秒は RFC 8628 §3.5 の "the interval MUST be increased by 5 seconds for this and all
// subsequent requests" である。期待値の 5 秒をリテラルで書いているのは意図的で、
// ssoTokenPollSlowDownIncrement を記号参照すると定数を変えたときに期待値も一緒に動き、
// 仕様から外れたことを検出できなくなる。
//
// 初期間隔を 3 秒にしているため、増分 5 秒と倍加 (3 秒 → 6 秒) は区別できる。
// 経過時間の実測ではなく policy.after へ要求された値を記録して比較するため、
// 計測誤差にも実行環境の負荷にも影響されない。
func TestWaitForSSOTokenPollIntervals(t *testing.T) {
	tests := []struct {
		name          string
		steps         []ssoCreateTokenStep
		wantIntervals []time.Duration
	}{
		{
			name:          "slow down increases the interval by five seconds",
			steps:         []ssoCreateTokenStep{ssoCreateTokenSlowDown(), ssoCreateTokenSlowDown(), ssoCreateTokenSuccess()},
			wantIntervals: []time.Duration{testSSOPollInterval + 5*time.Second, testSSOPollInterval + 10*time.Second},
		},
		{
			name:          "authorization pending keeps the interval",
			steps:         []ssoCreateTokenStep{ssoCreateTokenPending(), ssoCreateTokenPending(), ssoCreateTokenSuccess()},
			wantIntervals: []time.Duration{testSSOPollInterval, testSSOPollInterval},
		},
		{
			// 増えた間隔が、その後の承認待ちでも維持されることを確かめる。
			// RFC 8628 §3.5 の "and all subsequent requests" がこれである。
			name:          "interval increased by slow down is kept across pending",
			steps:         []ssoCreateTokenStep{ssoCreateTokenSlowDown(), ssoCreateTokenPending(), ssoCreateTokenSuccess()},
			wantIntervals: []time.Duration{testSSOPollInterval + 5*time.Second, testSSOPollInterval + 5*time.Second},
		},
		{
			name:          "success on the first call does not wait",
			steps:         []ssoCreateTokenStep{ssoCreateTokenSuccess()},
			wantIntervals: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newFakeSSOClock()
			mock := &mockSSOOidcCreateTokenAPI{steps: tt.steps}
			reg := &SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}

			got, err := waitForSSOToken(context.Background(), mock, reg, "dc", "grant", testSSOTokenPollPolicy(clock))
			if err != nil {
				t.Fatalf("waitForSSOToken() error = %v, want nil", err)
			}

			want := &SSOToken{AccessToken: "token", ExpiresIn: 3600}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("token mismatch (-want +got):\n%s", diff)
			}
			if len(mock.inputs) != len(tt.steps) {
				t.Errorf("CreateToken called %d times, want %d", len(mock.inputs), len(tt.steps))
			}
			if diff := cmp.Diff(tt.wantIntervals, clock.recorded); diff != "" {
				t.Errorf("poll intervals mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestWaitForSSOTokenSendsCreateTokenInput は CreateTokenInput が登録情報と
// デバイスコードから構築され、再試行しても同じ Input が送られることを検証する。
func TestWaitForSSOTokenSendsCreateTokenInput(t *testing.T) {
	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{ssoCreateTokenPending(), ssoCreateTokenSuccess()}}
	reg := &SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}

	if _, err := waitForSSOToken(context.Background(), mock, reg, "dc", "urn:ietf:params:oauth:grant-type:device_code",
		testSSOTokenPollPolicy(newFakeSSOClock())); err != nil {
		t.Fatalf("waitForSSOToken() error = %v, want nil", err)
	}

	if len(mock.inputs) != 2 {
		t.Fatalf("CreateToken called %d times, want 2", len(mock.inputs))
	}
	for i, in := range mock.inputs {
		if id := ptrStr(in.ClientId); id != "cid" {
			t.Errorf("call %d: ClientId = %q, want %q", i+1, id, "cid")
		}
		if secret := ptrStr(in.ClientSecret); secret != "secret" {
			t.Errorf("call %d: ClientSecret = %q, want %q", i+1, secret, "secret")
		}
		if code := ptrStr(in.DeviceCode); code != "dc" {
			t.Errorf("call %d: DeviceCode = %q, want %q", i+1, code, "dc")
		}
		want := "urn:ietf:params:oauth:grant-type:device_code"
		if grant := ptrStr(in.GrantType); grant != want {
			t.Errorf("call %d: GrantType = %q, want %q", i+1, grant, want)
		}
	}
}

// TestWaitForSSOTokenFailsImmediatelyOnOtherError は承認待ちでもレート制限でもない
// エラーで即座に失敗し、再試行しないことを検証する。
func TestWaitForSSOTokenFailsImmediatelyOnOtherError(t *testing.T) {
	base := &ssooidctypes.InvalidGrantException{Message: aws.String("bad grant")}
	clock := newFakeSSOClock()
	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{
		{err: ssoOidcOperationError(base)},
		// 2 回目が呼ばれたら再試行してしまっている。steps に用意しておき、
		// 呼び出し回数の検証で捕まえる。
		ssoCreateTokenSuccess(),
	}}

	got, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, "dc", "grant",
		testSSOTokenPollPolicy(clock))
	if got != nil {
		t.Errorf("token = %v, want nil on error", got)
	}
	if err == nil {
		t.Fatal("waitForSSOToken() error = nil, want an error")
	}
	if !errors.Is(err, base) {
		t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	var target *ssooidctypes.InvalidGrantException
	if !errors.As(err, &target) {
		t.Errorf("errors.As() = false, want true; the chain is severed: %v", err)
	}
	// issue 0125 で変更した本番のラップ文言を固定する。register / start device authorization の
	// 2 箇所は他のテストで固定しているが、create token だけが抜けていた。
	if !strings.HasPrefix(err.Error(), "create sso oidc token: ") {
		t.Errorf("error = %q, want it to start with %q", err.Error(), "create sso oidc token: ")
	}
	if len(mock.inputs) != 1 {
		t.Errorf("CreateToken called %d times, want 1 (must not retry)", len(mock.inputs))
	}
	if len(clock.recorded) != 0 {
		t.Errorf("waited %v, want no wait before failing", clock.recorded)
	}
}

// awsSSODeviceAuthorization は AWS が実際に返す指示を持つデバイス認可の応答を返す。
// interval 5 秒 / expires_in 600 秒である。
func awsSSODeviceAuthorization() *SSODeviceAuthorization {
	return &SSODeviceAuthorization{DeviceCode: "dc", UserCode: "uc", Interval: 5, ExpiresIn: 600}
}

// TestWaitForSSOTokenPollsUntilDeviceCodeExpires は打ち切りが device code の有効期限で
// 決まることを検証する。方針は newSSOTokenPollPolicy に組ませ、時計だけを差し替えるため、
// 本番の配線をそのまま通る。
//
// interval 5 秒 / expires_in 600 秒に対して、承認待ちが続いた場合の内訳は次のとおりである。
//
//   - n 回目の CreateToken は起点から 5*(n-1) 秒の時点で呼ばれる
//   - 次の待機を終える時刻 5*n 秒が 600 秒に届くのは n = 120
//   - よって CreateToken は 120 回、待機は 119 回、待機の合計は 595 秒
//
// 打ち切りまでの実時間が 595 秒であることが、この issue の主眼である。修正前は
// ssoTokenPollMaxAttempts = 60 と 1 秒間隔の積で約 60 秒で打ち切っており、ブラウザで
// MFA を通す間に CLI 側だけが先に失敗していた。ここを 60 秒に戻すと落ちる。
//
// 待機の回数が CreateToken の回数より 1 少ないことが、余分な待機が消えたことの検証である。
// 修正前は最後の試行のあとにも待ってから打ち切っていたため両者は同数だった。
func TestWaitForSSOTokenPollsUntilDeviceCodeExpires(t *testing.T) {
	// 打ち切りが効かず呼び続けた場合に、mock の step 切れによる別のエラーではなく
	// 呼び出し回数の不一致として捕まえられるよう、期待値より多めに用意する。
	steps := make([]ssoCreateTokenStep, 200)
	for i := range steps {
		steps[i] = ssoCreateTokenPending()
	}
	clock := newFakeSSOClock()
	mock := &mockSSOOidcCreateTokenAPI{steps: steps}
	deviceAuth := awsSSODeviceAuthorization()

	got, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, deviceAuth.DeviceCode, "grant",
		withFakeClock(newSSOTokenPollPolicy(deviceAuth), clock))
	if got != nil {
		t.Errorf("token = %v, want nil on timeout", got)
	}
	if !errors.Is(err, errSSOTokenTimeout) {
		t.Fatalf("errors.Is(err, errSSOTokenTimeout) = false, want true; got %v", err)
	}
	if len(mock.inputs) != 120 {
		t.Errorf("CreateToken called %d times, want %d", len(mock.inputs), 120)
	}
	if len(clock.recorded) != 119 {
		t.Errorf("waited %d times, want %d (one fewer than the number of attempts)", len(clock.recorded), 119)
	}
	if elapsed := clock.elapsed(); elapsed != 595*time.Second {
		t.Errorf("total wait until timeout = %v, want %v", elapsed, 595*time.Second)
	}
}

// TestWaitForSSOTokenSlowDownStaysBounded は SlowDown が続いても待ち時間が正のまま
// 有効期限を超えて伸びないことを検証する。
//
// 修正前の実装は間隔を倍加しており、上限が無かった。5 秒を起点にすると 31 回の倍加で
// time.Duration (int64 のナノ秒) を超えて負になり、負の値を time.After に渡すと即座に
// 発火するためバックオフが連打に反転する。「すべての待ち時間が正である」の検証がそれを
// 捕まえる。倍加のまま上限だけ足しても、5 秒ずつの増加になっていなければ内訳の比較で落ちる。
//
// interval 5 秒 / expires_in 600 秒に対する内訳は次のとおりである。k 回目の SlowDown の
// あとの間隔は 5 + 5k 秒で、待機の累計は 2.5k^2 + 7.5k 秒になる。次の待機を終える時刻が
// 600 秒に届くのは k = 14 の時点であり、そこで打ち切る。
func TestWaitForSSOTokenSlowDownStaysBounded(t *testing.T) {
	steps := make([]ssoCreateTokenStep, 200)
	for i := range steps {
		steps[i] = ssoCreateTokenSlowDown()
	}
	clock := newFakeSSOClock()
	mock := &mockSSOOidcCreateTokenAPI{steps: steps}
	deviceAuth := awsSSODeviceAuthorization()
	policy := withFakeClock(newSSOTokenPollPolicy(deviceAuth), clock)

	got, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, deviceAuth.DeviceCode, "grant", policy)
	if got != nil {
		t.Errorf("token = %v, want nil on timeout", got)
	}
	if !errors.Is(err, errSSOTokenTimeout) {
		t.Fatalf("errors.Is(err, errSSOTokenTimeout) = false, want true; got %v", err)
	}

	// 5 秒ずつ増える内訳をリテラルで固定する。倍加なら 10s, 20s, 40s と伸びるため合わない。
	wantIntervals := []time.Duration{
		10 * time.Second, 15 * time.Second, 20 * time.Second, 25 * time.Second, 30 * time.Second,
		35 * time.Second, 40 * time.Second, 45 * time.Second, 50 * time.Second, 55 * time.Second,
		60 * time.Second, 65 * time.Second, 70 * time.Second, 75 * time.Second,
	}
	if diff := cmp.Diff(wantIntervals, clock.recorded); diff != "" {
		t.Errorf("poll intervals mismatch (-want +got):\n%s", diff)
	}
	if len(mock.inputs) != len(wantIntervals)+1 {
		t.Errorf("CreateToken called %d times, want %d", len(mock.inputs), len(wantIntervals)+1)
	}

	// 内訳の比較とは別に、待ち時間が満たすべき不変条件を明示しておく。オーバーフローや
	// 上限の消失は「正である」「猶予以下である」のどちらかを必ず破る。
	for i, d := range clock.recorded {
		if d <= 0 {
			t.Errorf("wait %d = %v, want a positive duration", i+1, d)
		}
		if d > policy.timeout {
			t.Errorf("wait %d = %v, want <= %v (the device code lifetime)", i+1, d, policy.timeout)
		}
	}
}

// TestWaitForSSOTokenReturnsContextError は待機に入ってから ctx がキャンセルされたときに
// ctx.Err() を返すことを検証する。
//
// キャンセルを after の中で行うのが要点である。呼び出し前にキャンセルしてしまうと、
// select を「待機に入る前に ctx.Err() を見るだけ」に書き換えた実装でもテストが通る。
// それは本番では待機中の Ctrl-C を最大 1 回分の間隔だけ無視する壊れた実装であり、
// 検出できなければならない。after の中でキャンセルすれば、CreateToken は生きた ctx で
// 呼ばれ、キャンセルされるのは待機に入る瞬間になる。
//
// 非決定性は入らない。cancel() は Done チャネルを閉じてから返るため、select が
// readiness を判定する時点で発火可能なのは ctx.Done() だけである。after が返す
// チャネルは 1 秒後に発火するので、ctx.Done() の case を削った実装は永久にブロックせず
// 1 秒で失敗する。
func TestWaitForSSOTokenReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{ssoCreateTokenPending()}}
	policy := ssoTokenPollPolicy{
		interval: testSSOPollInterval,
		timeout:  testSSOPollTimeout,
		now:      time.Now,
		after: func(time.Duration) <-chan time.Time {
			cancel()
			return time.After(time.Second)
		},
	}

	got, err := waitForSSOToken(ctx, mock, &SSOClientRegistration{}, "dc", "grant", policy)
	if got != nil {
		t.Errorf("token = %v, want nil on cancel", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(err, context.Canceled) = false, want true; got %v", err)
	}
	if len(mock.inputs) != 1 {
		t.Errorf("CreateToken called %d times, want 1", len(mock.inputs))
	}
}

// TestWaitForSSOTokenPrefersContextErrorOverTimeout は打ち切りと ctx のキャンセルが
// 同時に成立しているとき、ctx.Err() を返すことを検証する。
//
// 打ち切りの判定は時刻だけを見るため、その直前に ctx がキャンセルされていると、理由が
// 利用者の中断であっても打ち切りとして返ってしまう。呼び出し側は errors.Is で
// context.Canceled を判別できなくなる。
//
// 猶予より間隔を長くすることで、初回の判定で必ず打ち切りに達する状況を決定的に作る。
// ctx は呼び出し前にキャンセルしてあるので、両方の条件が同時に成立している。
func TestWaitForSSOTokenPrefersContextErrorOverTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{ssoCreateTokenPending()}}
	policy := ssoTokenPollPolicy{
		interval: 10 * time.Second,
		timeout:  time.Second,
		now:      time.Now,
		after: func(d time.Duration) <-chan time.Time {
			t.Errorf("after(%v) was called; the deadline had already passed", d)
			return time.After(0)
		},
	}

	got, err := waitForSSOToken(ctx, mock, &SSOClientRegistration{}, "dc", "grant", policy)
	if got != nil {
		t.Errorf("token = %v, want nil on cancel", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(err, context.Canceled) = false, want true; got %v", err)
	}
	if errors.Is(err, errSSOTokenTimeout) {
		t.Errorf("error = %v, want the cancellation reason rather than the timeout", err)
	}
}

// TestWaitForSSOTokenRejectsInvalidPolicy は間隔か猶予が正でない方針を渡されたときに、
// CreateToken を一度も呼ばずにエラーを返すことを検証する。
//
// この検証は無限ループの防波堤である。間隔が 0 以下だと待機で時刻が進まないため、
// 打ち切り判定が永久に成立しないまま CreateToken を連打し続ける。newSSOTokenPollPolicy は
// この不変条件を必ず満たすが、ssoTokenPollPolicy は構造体リテラルでも組めるため、
// waitForSSOToken 自身が前提を検証しなければ契約が呼び出し元の善意に依存する。
//
// モックに成功を置いているのが要点である。検証を外すと打ち切りに達する前に成功が返るので、
// テストはハングせず「エラーが nil」として即座に落ちる。
func TestWaitForSSOTokenRejectsInvalidPolicy(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		timeout  time.Duration
	}{
		{name: "zero interval", interval: 0, timeout: testSSOPollTimeout},
		{name: "negative interval", interval: -1 * time.Second, timeout: testSSOPollTimeout},
		{name: "zero timeout", interval: testSSOPollInterval, timeout: 0},
		{name: "negative timeout", interval: testSSOPollInterval, timeout: -1 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{ssoCreateTokenSuccess()}}
			policy := ssoTokenPollPolicy{
				interval: tt.interval,
				timeout:  tt.timeout,
				now:      time.Now,
				after:    time.After,
			}

			got, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, "dc", "grant", policy)
			if got != nil {
				t.Errorf("token = %v, want nil on an invalid policy", got)
			}
			if !errors.Is(err, errInvalidSSOTokenPollPolicy) {
				t.Fatalf("errors.Is(err, errInvalidSSOTokenPollPolicy) = false, want true; got %v", err)
			}
			if len(mock.inputs) != 0 {
				t.Errorf("CreateToken called %d times, want 0", len(mock.inputs))
			}
		})
	}
}

// TestWaitForSSOTokenRejectsNilDeviceAuthorization は WaitForSSOToken が nil の応答を
// 参照外しせずエラーを返すことを検証する。
//
// WaitForSSOToken は device code とポーリング方針の両方を deviceAuth から読むため、
// nil なら参照外しで panic する。AGENTS.md は「リクエスト処理中の panic は禁止」と定めており、
// CLI のログイン処理もこれに当たる。検証を外すとこのテストは panic して落ちる。
//
// 検証を AWS のクライアント生成より前に置いてあるため、このテストは認証情報も
// ネットワークも要求しない。生成より後ろに移すと環境依存のエラーが先に返り、
// errors.Is の判定で落ちる。
func TestWaitForSSOTokenRejectsNilDeviceAuthorization(t *testing.T) {
	got, err := WaitForSSOToken(context.Background(), "ap-northeast-1", &SSOClientRegistration{}, nil, "grant")
	if got != nil {
		t.Errorf("token = %v, want nil on a nil device authorization", got)
	}
	if !errors.Is(err, errNilSSODeviceAuthorization) {
		t.Fatalf("errors.Is(err, errNilSSODeviceAuthorization) = false, want true; got %v", err)
	}
}

// TestSSOTokenPollSentinelMessages はセンチネルエラーの文言を固定する。
//
// 判別を errors.Is に移したことで、文言そのものを見るアサーションがどのテストからも
// 消えた。打ち切りのエラーは thief sso login が利用者に表示するメッセージであり、
// 黙って変わってよいものではないのでここで押さえる。
//
// あわせてリポジトリの文言規約 (エラーメッセージは英語) の検査も兼ねる。
func TestSSOTokenPollSentinelMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: errSSOTokenTimeout, want: "timeout waiting for authentication"},
		{name: "nil device authorization", err: errNilSSODeviceAuthorization, want: "nil sso device authorization"},
		{name: "invalid poll policy", err: errInvalidSSOTokenPollPolicy, want: "invalid sso token poll policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNewSSOTokenPollPolicyFollowsServerInstructions は device authorization response の
// 指示がポーリング方針へどう写るかを固定する。
//
// 期待値をリテラルで書いているのは意図的である。ssoTokenPollDefaultInterval や
// ssoTokenPollDefaultTimeout を記号参照すると、定数を変えたときに期待値も一緒に動いてしまい、
// RFC 8628 §3.2 が定める既定値から外れたことを検出できない。定数を変えたらここで落ちるのが
// 正しい。
//
// 0 以下を既定に倒すのは仕様上の既定値だからだけではない。間隔が 0 以下だと打ち切り判定に
// 使う時刻が進まないまま CreateToken を呼び続けることになり、猶予が 0 以下だと打ち切りが
// 消える。どちらも待機の無いループになるため、ここは無限ループの防波堤でもある。
func TestNewSSOTokenPollPolicyFollowsServerInstructions(t *testing.T) {
	tests := []struct {
		name         string
		deviceAuth   *SSODeviceAuthorization
		wantInterval time.Duration
		wantTimeout  time.Duration
	}{
		{
			name:         "server instructs both",
			deviceAuth:   &SSODeviceAuthorization{Interval: 7, ExpiresIn: 900},
			wantInterval: 7 * time.Second,
			wantTimeout:  900 * time.Second,
		},
		{
			// RFC 8628 §3.2: "If no value is provided, clients MUST use 5 as the default."
			name:         "interval is absent",
			deviceAuth:   &SSODeviceAuthorization{Interval: 0, ExpiresIn: 900},
			wantInterval: 5 * time.Second,
			wantTimeout:  900 * time.Second,
		},
		{
			name:         "interval is negative",
			deviceAuth:   &SSODeviceAuthorization{Interval: -1, ExpiresIn: 900},
			wantInterval: 5 * time.Second,
			wantTimeout:  900 * time.Second,
		},
		{
			// RFC 8628 §3.2 は expires_in を REQUIRED と定めるため、無いのは仕様に
			// 従っていない応答である。それでも際限なくポーリングしないよう既定に倒す。
			name:         "expires in is absent",
			deviceAuth:   &SSODeviceAuthorization{Interval: 7, ExpiresIn: 0},
			wantInterval: 7 * time.Second,
			wantTimeout:  600 * time.Second,
		},
		{
			name:         "expires in is negative",
			deviceAuth:   &SSODeviceAuthorization{Interval: 7, ExpiresIn: -1},
			wantInterval: 7 * time.Second,
			wantTimeout:  600 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newSSOTokenPollPolicy(tt.deviceAuth)

			if policy.interval != tt.wantInterval {
				t.Errorf("interval = %v, want %v", policy.interval, tt.wantInterval)
			}
			if policy.timeout != tt.wantTimeout {
				t.Errorf("timeout = %v, want %v", policy.timeout, tt.wantTimeout)
			}
		})
	}
}

// TestNewSSOTokenPollPolicyUsesRealClock は本番の方針が実時間の時計を持つことを検証する。
//
// after が nil なら最初の待機で panic するため気付ける。now が nil でも同様である。
// 危険なのは nil ではなく止まった時計や別の時刻を返す時計を入れてしまう場合で、
// その場合は打ち切りが効かず CreateToken を延々と呼び続ける。now() が実際に
// time.Now() の範囲に収まることまで見て検出する。
//
// 2 回の time.Now() で挟むだけなので、待たずに済みフレークもしない。time.Now() は
// 単調増加するため、その間に取った値が範囲外に出ることはない。
//
// ただし範囲の検査だけでは、方針を組んだ時点の時刻を捕まえて以後それを返し続ける時計
// (now: func() time.Time { return start }) を見逃す。範囲は構築直後に取るため、
// 固定された値もその中に収まってしまう。この時計は本番で最悪の壊れ方をする。deadline も
// 判定側も同じ値になり、now()+interval < now()+timeout が恒真になって打ち切りが消え、
// CreateToken を永久に呼び続ける。実時間を少しだけ進めてから 2 回目を読み、時刻が
// 進むことまで確認して塞ぐ。
//
// 1 ミリ秒の待機は Go のタイマーの下限保証 (指定より早くは発火しない) で担保され、
// time.Now() の単調成分はナノ秒精度なので、この検査はフレークしない。
func TestNewSSOTokenPollPolicyUsesRealClock(t *testing.T) {
	policy := newSSOTokenPollPolicy(awsSSODeviceAuthorization())

	if policy.after == nil {
		t.Fatal("after is nil; waitForSSOToken would panic on the first wait")
	}
	if policy.now == nil {
		t.Fatal("now is nil; waitForSSOToken would panic before the first attempt")
	}

	before := time.Now()
	got := policy.now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("now() = %v, want a value in [%v, %v]", got, before, after)
	}

	time.Sleep(time.Millisecond)
	if again := policy.now(); !again.After(got) {
		t.Errorf("now() = %v on the second read, want a value after %v; the clock does not advance", again, got)
	}
}

// TestWaitForSSOTokenUsesTimeAfterWhenNotStubbed は本番の after (time.After) を
// 差し替えずに通したときに、待機が実際に発火して次の試行へ進むこと、および要求した
// 間隔が本当に待機に使われていることを検証する。
//
// 経過時間は下限だけを見る。Go のタイマーは指定より早くは発火しないため下限は
// フレークしない。上限を見ると実行環境の負荷でフレークするので見ない。
// この下限が無いと、本番の after を time.After(0) を返す実装に書き換えても
// テストが通ってしまう (待機ゼロで 60 回連打する実装になる)。
// interval を本番の 5 秒から短くしているため、テストの実行時間は 5 秒に依存しない。
// timeout は本番の値 (expires_in 600 秒) をそのまま使う。打ち切りに達する前に成功するため、
// 実際に 600 秒待つことはない。
func TestWaitForSSOTokenUsesTimeAfterWhenNotStubbed(t *testing.T) {
	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{ssoCreateTokenPending(), ssoCreateTokenSuccess()}}
	policy := newSSOTokenPollPolicy(awsSSODeviceAuthorization())
	policy.interval = 30 * time.Millisecond

	start := time.Now()
	got, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, "dc", "grant", policy)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("waitForSSOToken() error = %v, want nil", err)
	}
	if elapsed < policy.interval {
		t.Errorf("elapsed = %v, want >= %v; the requested interval did not reach the timer", elapsed, policy.interval)
	}
	want := &SSOToken{AccessToken: "token", ExpiresIn: 3600}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("token mismatch (-want +got):\n%s", diff)
	}
	if len(mock.inputs) != 2 {
		t.Errorf("CreateToken called %d times, want 2", len(mock.inputs))
	}
}
