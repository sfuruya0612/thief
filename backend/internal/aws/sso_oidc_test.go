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
func TestStartSSODeviceAuthorizationSendsRegistrationAndStartURL(t *testing.T) {
	mock := &mockSSOOidcStartDeviceAuthorizationAPI{
		out: &ssooidc.StartDeviceAuthorizationOutput{
			DeviceCode:              aws.String("dc"),
			UserCode:                aws.String("uc"),
			VerificationUriComplete: aws.String("https://device.sso/verify?user_code=uc"),
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

// recordingAfter は要求された待ち時間を記録し、即座に発火するチャネルを返す。
// 実際には待たないため、テストの実行時間がポーリング間隔に依存しない。
func recordingAfter(recorded *[]time.Duration) func(time.Duration) <-chan time.Time {
	return func(d time.Duration) <-chan time.Time {
		*recorded = append(*recorded, d)
		ch := make(chan time.Time, 1)
		ch <- time.Time{}
		return ch
	}
}

const (
	// testSSOPollInterval はテストで使う初期間隔。本番の既定値 ssoTokenPollInterval とは
	// 別の値にしてある。同値にすると、policy.interval ではなく定数を直接読むように壊しても
	// テストが通ってしまう。
	testSSOPollInterval = 100 * time.Millisecond

	// testSSOPollMaxAttempts は打ち切りに達しないだけの十分な試行回数。本番の既定値
	// ssoTokenPollMaxAttempts とは別の値にしてある。理由は testSSOPollInterval と同じで、
	// policy.maxAttempts ではなく定数を直接読むように壊したときに検出できるようにするため。
	testSSOPollMaxAttempts = 5
)

// testSSOTokenPollPolicy は待ち時間を記録するだけの方針を返す。
func testSSOTokenPollPolicy(interval time.Duration, maxAttempts int, recorded *[]time.Duration) ssoTokenPollPolicy {
	return ssoTokenPollPolicy{
		interval:    interval,
		maxAttempts: maxAttempts,
		after:       recordingAfter(recorded),
	}
}

// TestWaitForSSOTokenPollIntervals は CreateToken が返すエラーの種類に応じて
// 次の試行までの待ち時間がどう変わるかを検証する。
//
// SlowDown は間隔を倍にし、AuthorizationPending は間隔を変えない。
// 経過時間の実測ではなく policy.after へ要求された値を記録して比較するため、
// 計測誤差にも実行環境の負荷にも影響されない。
//
// 固定しているのは連続 2 回までの倍加である。倍加に上限が無いこと、およびそれが
// 34 回で int64 を超えて負の待ち時間になることは意図的に固定していない。RFC 8628 §3.5
// への違反であり、直すと挙動が変わるため issue 0129 で扱う。
// つまりこのテストは「SlowDown の扱いが正しい」ことは主張しない。
func TestWaitForSSOTokenPollIntervals(t *testing.T) {
	tests := []struct {
		name          string
		steps         []ssoCreateTokenStep
		wantIntervals []time.Duration
	}{
		{
			name:          "slow down doubles the interval",
			steps:         []ssoCreateTokenStep{ssoCreateTokenSlowDown(), ssoCreateTokenSlowDown(), ssoCreateTokenSuccess()},
			wantIntervals: []time.Duration{2 * testSSOPollInterval, 4 * testSSOPollInterval},
		},
		{
			name:          "authorization pending keeps the interval",
			steps:         []ssoCreateTokenStep{ssoCreateTokenPending(), ssoCreateTokenPending(), ssoCreateTokenSuccess()},
			wantIntervals: []time.Duration{testSSOPollInterval, testSSOPollInterval},
		},
		{
			// 倍加した間隔が、その後の承認待ちでも維持されることを確かめる。
			name:          "interval doubled by slow down is kept across pending",
			steps:         []ssoCreateTokenStep{ssoCreateTokenSlowDown(), ssoCreateTokenPending(), ssoCreateTokenSuccess()},
			wantIntervals: []time.Duration{2 * testSSOPollInterval, 2 * testSSOPollInterval},
		},
		{
			name:          "success on the first call does not wait",
			steps:         []ssoCreateTokenStep{ssoCreateTokenSuccess()},
			wantIntervals: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorded []time.Duration
			mock := &mockSSOOidcCreateTokenAPI{steps: tt.steps}
			reg := &SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}

			got, err := waitForSSOToken(context.Background(), mock, reg, "dc", "grant",
				testSSOTokenPollPolicy(testSSOPollInterval, testSSOPollMaxAttempts, &recorded))
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
			if diff := cmp.Diff(tt.wantIntervals, recorded); diff != "" {
				t.Errorf("poll intervals mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestWaitForSSOTokenSendsCreateTokenInput は CreateTokenInput が登録情報と
// デバイスコードから構築され、再試行しても同じ Input が送られることを検証する。
func TestWaitForSSOTokenSendsCreateTokenInput(t *testing.T) {
	var recorded []time.Duration
	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{ssoCreateTokenPending(), ssoCreateTokenSuccess()}}
	reg := &SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"}

	if _, err := waitForSSOToken(context.Background(), mock, reg, "dc", "urn:ietf:params:oauth:grant-type:device_code",
		testSSOTokenPollPolicy(testSSOPollInterval, testSSOPollMaxAttempts, &recorded)); err != nil {
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
	var recorded []time.Duration
	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{
		{err: ssoOidcOperationError(base)},
		// 2 回目が呼ばれたら再試行してしまっている。steps に用意しておき、
		// 呼び出し回数の検証で捕まえる。
		ssoCreateTokenSuccess(),
	}}

	got, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, "dc", "grant",
		testSSOTokenPollPolicy(testSSOPollInterval, testSSOPollMaxAttempts, &recorded))
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
	if len(recorded) != 0 {
		t.Errorf("waited %v, want no wait before failing", recorded)
	}
}

// TestWaitForSSOTokenTimesOutAfterMaxAttempts は最大試行回数を使い切ったときに
// timeout を返すことを検証する。
func TestWaitForSSOTokenTimesOutAfterMaxAttempts(t *testing.T) {
	const maxAttempts = 3
	steps := make([]ssoCreateTokenStep, maxAttempts)
	for i := range steps {
		steps[i] = ssoCreateTokenPending()
	}
	var recorded []time.Duration
	mock := &mockSSOOidcCreateTokenAPI{steps: steps}

	got, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, "dc", "grant",
		testSSOTokenPollPolicy(testSSOPollInterval, maxAttempts, &recorded))
	if got != nil {
		t.Errorf("token = %v, want nil on timeout", got)
	}
	if err == nil {
		t.Fatal("waitForSSOToken() error = nil, want a timeout error")
	}
	if msg := err.Error(); msg != "timeout waiting for authentication" {
		t.Errorf("error = %q, want %q", msg, "timeout waiting for authentication")
	}
	if len(mock.inputs) != maxAttempts {
		t.Errorf("CreateToken called %d times, want %d", len(mock.inputs), maxAttempts)
	}
	// 最後の試行のあとにも待機してから打ち切る現行の挙動を固定する。
	if len(recorded) != maxAttempts {
		t.Errorf("waited %d times, want %d", len(recorded), maxAttempts)
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
		interval:    testSSOPollInterval,
		maxAttempts: testSSOPollMaxAttempts,
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

// TestProductionSSOTokenPollPolicyPinsWaitTimes は本番のポーリング方針が定数どおりに
// 組まれていること、およびその定数から決まる最悪ケースの総待ち時間を固定する。
//
// 期待値をリテラルで書いているのは意図的である。ssoTokenPollInterval や
// ssoTokenPollMaxAttempts を記号参照すると、定数を変えたときに期待値も一緒に動いてしまい、
// 待ち時間が化けたことを検出できない。定数を変えたらここで落ちるのが正しい。
func TestProductionSSOTokenPollPolicyPinsWaitTimes(t *testing.T) {
	policy := productionSSOTokenPollPolicy()

	if policy.interval != time.Second {
		t.Errorf("interval = %v, want %v", policy.interval, time.Second)
	}
	if policy.maxAttempts != 60 {
		t.Errorf("maxAttempts = %d, want %d", policy.maxAttempts, 60)
	}
	if policy.after == nil {
		t.Fatal("after is nil; WaitForSSOToken would panic on the first wait")
	}

	// 承認待ちが続いた場合の総待ち時間を固定する。SlowDown が来なければ間隔は倍にならない
	// ため、これが打ち切りまでの実際の待ち時間になる。
	var recorded []time.Duration
	steps := make([]ssoCreateTokenStep, policy.maxAttempts)
	for i := range steps {
		steps[i] = ssoCreateTokenPending()
	}
	mock := &mockSSOOidcCreateTokenAPI{steps: steps}

	// after だけを記録用に差し替える。interval と maxAttempts は本番の値をそのまま使う。
	measured := policy
	measured.after = recordingAfter(&recorded)

	_, err := waitForSSOToken(context.Background(), mock, &SSOClientRegistration{}, "dc", "grant", measured)
	if err == nil {
		t.Fatal("waitForSSOToken() error = nil, want a timeout error")
	}
	if msg := err.Error(); msg != "timeout waiting for authentication" {
		t.Fatalf("error = %q, want %q", msg, "timeout waiting for authentication")
	}
	if len(mock.inputs) != 60 {
		t.Errorf("CreateToken called %d times, want %d", len(mock.inputs), 60)
	}

	var total time.Duration
	for _, d := range recorded {
		total += d
	}
	if total != 60*time.Second {
		t.Errorf("total wait until timeout = %v, want %v", total, 60*time.Second)
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
// interval を本番の 1 秒から短くしているため、テストの実行時間は 1 秒に依存しない。
func TestWaitForSSOTokenUsesTimeAfterWhenNotStubbed(t *testing.T) {
	mock := &mockSSOOidcCreateTokenAPI{steps: []ssoCreateTokenStep{ssoCreateTokenPending(), ssoCreateTokenSuccess()}}
	policy := productionSSOTokenPollPolicy()
	policy.interval = 30 * time.Millisecond
	policy.maxAttempts = 2

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
