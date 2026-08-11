package cli

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/spf13/cobra"
)

// newEC2SessionCmd は startEC2SessionWith が読むフラグだけを持つコマンドと、
// 標準エラー出力の受け皿を返す。profile と region は明示指定して config の解決結果に
// 依存しないようにする。
//
// startEC2SessionWith は loadConfig を通り、その先の config.Load が
// $XDG_CONFIG_HOME/thief/config.yaml と $HOME/.thief/config.yaml を読む。実行環境に
// 壊れた YAML があると config.Load 自体が失敗してテストの意図と無関係に落ちるため、
// 両方を空のディレクトリへ向ける。
func newEC2SessionCmd(t *testing.T, instanceID string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := &cobra.Command{Use: "session"}
	cmd.Flags().String("instance-id", "", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("region", "", "")
	if err := cmd.Flags().Set("profile", "test-profile"); err != nil {
		t.Fatalf("set profile flag: %v", err)
	}
	if err := cmd.Flags().Set("region", "ap-northeast-1"); err != nil {
		t.Fatalf("set region flag: %v", err)
	}
	if instanceID != "" {
		if err := cmd.Flags().Set("instance-id", instanceID); err != nil {
			t.Fatalf("set instance-id flag: %v", err)
		}
	}

	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	return cmd, &stderr
}

// execExitError は終了コード 3 で終わる子プロセスを実際に起動して *exec.ExitError を得る。
// *exec.ExitError が持つ *os.ProcessState は pid / status / rusage がいずれも非公開
// フィールドであり、任意の終了コードを持つ値を公開 API から組み立てる手段が無い。
// そのため実プロセスを起動して得るしかない。
func execExitError(t *testing.T) *exec.ExitError {
	t.Helper()

	err := exec.Command("sh", "-c", "exit 3").Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("could not obtain an *exec.ExitError: err = %v", err)
	}
	return exitErr
}

// ec2SessionCall は差し替えた依存が受け取った引数を記録する。
type ec2SessionCall struct {
	pluginArgs          []string
	terminateIDs        []string
	selectCalls         int
	startSessionIDs     []string
	startSessionProfile string
	startSessionRegion  string
	terminateProfile    string
	terminateRegion     string
}

// okEC2SessionDeps はすべて成功する依存を返す。個々のテストは必要なフィールドだけを
// 差し替える。
func okEC2SessionDeps(rec *ec2SessionCall) ec2SessionDeps {
	return ec2SessionDeps{
		selectInstance: func(context.Context, *config.Config) (string, error) {
			rec.selectCalls++
			return "i-selected", nil
		},
		startSession: func(_ context.Context, profile, region, target string) (*awsinternal.StartSessionResult, error) {
			rec.startSessionProfile = profile
			rec.startSessionRegion = region
			rec.startSessionIDs = append(rec.startSessionIDs, target)
			return &awsinternal.StartSessionResult{
				SessionID:  "sess-1",
				StreamURL:  "wss://ssmmessages.example/stream",
				TokenValue: "token-1",
			}, nil
		},
		lookupPlugin: func() (string, error) { return "/usr/local/bin/session-manager-plugin", nil },
		execPlugin: func(_ string, args ...string) error {
			rec.pluginArgs = args
			return nil
		},
		terminateSession: func(_ context.Context, profile, region, sessionID string) error {
			rec.terminateProfile = profile
			rec.terminateRegion = region
			rec.terminateIDs = append(rec.terminateIDs, sessionID)
			return nil
		},
	}
}

// TestStartEC2SessionKeepsExecErrorChain は session-manager-plugin の実行が失敗したときに
// errors.As で *exec.ExitError へ到達できることを、セッション切断が成功する経路と失敗する
// 経路の両方について検証する。
//
// 切断の成否という無関係な後処理の結果によって、同じ失敗が判別可能かどうか変わっては
// ならない。以前は切断も失敗した場合だけ execErr を %v で埋め込んでおり、その経路では
// 終了コードで分岐できなかった。
func TestStartEC2SessionKeepsExecErrorChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}

	termErr := errors.New("terminate boom")

	tests := []struct {
		name       string
		terminate  error
		wantMsg    string
		wantTermIs bool
	}{
		{
			name:      "terminate succeeds",
			terminate: nil,
			wantMsg:   "execute command: exit status 3",
		},
		{
			name: "terminate also fails",
			// 実行の失敗の見え方が切断の成否で変わらないよう、どちらの経路も
			// execute command: で始まる。
			terminate:  termErr,
			wantMsg:    "execute command: exit status 3; terminate session: terminate boom",
			wantTermIs: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitErr := execExitError(t)
			cmd, _ := newEC2SessionCmd(t, "i-1234567890abcdef0")
			rec := &ec2SessionCall{}
			deps := okEC2SessionDeps(rec)
			deps.execPlugin = func(string, ...string) error { return exitErr }
			deps.terminateSession = func(_ context.Context, profile, region, sessionID string) error {
				rec.terminateProfile = profile
				rec.terminateRegion = region
				rec.terminateIDs = append(rec.terminateIDs, sessionID)
				return tt.terminate
			}

			err := startEC2SessionWith(cmd, deps)
			if err == nil {
				t.Fatal("startEC2SessionWith() error = nil, want an error")
			}

			// 本 issue の主眼。どちらの経路でも終了コードで分岐できる必要がある。
			var gotExit *exec.ExitError
			if !errors.As(err, &gotExit) {
				t.Fatalf("errors.As(*exec.ExitError) = false, want true; the chain is severed: %v", err)
			}
			if code := gotExit.ExitCode(); code != 3 {
				t.Errorf("exit code = %d, want 3", code)
			}
			if !errors.Is(err, exitErr) {
				t.Errorf("errors.Is(err, exitErr) = false, want true: %v", err)
			}
			// 切断の失敗にも到達できる (2 つの %w のうちもう一方)。
			if got := errors.Is(err, termErr); got != tt.wantTermIs {
				t.Errorf("errors.Is(err, termErr) = %t, want %t", got, tt.wantTermIs)
			}
			if msg := err.Error(); msg != tt.wantMsg {
				t.Errorf("error = %q, want %q", msg, tt.wantMsg)
			}
			// 実行に失敗しても AWS 側にセッションが残るため、必ず切断を試みる。
			if len(rec.terminateIDs) != 1 || rec.terminateIDs[0] != "sess-1" {
				t.Errorf("terminate called with %v, want exactly [sess-1]", rec.terminateIDs)
			}
			// 切断先を誤ると別のセッションを切ることになる。
			if rec.terminateProfile != "test-profile" || rec.terminateRegion != "ap-northeast-1" {
				t.Errorf("terminate called with profile %q region %q, want %q and %q",
					rec.terminateProfile, rec.terminateRegion, "test-profile", "ap-northeast-1")
			}
		})
	}
}

// TestStartEC2SessionReportsExecFailureOnlyThroughReturnValue は実行の失敗を標準エラー
// 出力へ書かないことを検証する。返り値の表示は cli.Run が 1 箇所で行うため、ここで
// 書くと同じ内容が 2 回並ぶ。
func TestStartEC2SessionReportsExecFailureOnlyThroughReturnValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}

	tests := []struct {
		name      string
		terminate error
	}{
		{name: "terminate succeeds", terminate: nil},
		{name: "terminate also fails", terminate: errors.New("terminate boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitErr := execExitError(t)
			cmd, stderr := newEC2SessionCmd(t, "i-1234567890abcdef0")
			rec := &ec2SessionCall{}
			deps := okEC2SessionDeps(rec)
			deps.execPlugin = func(string, ...string) error { return exitErr }
			deps.terminateSession = func(context.Context, string, string, string) error { return tt.terminate }

			err := startEC2SessionWith(cmd, deps)
			if err == nil {
				t.Fatal("startEC2SessionWith() error = nil, want an error")
			}
			if got := stderr.String(); got != "" {
				t.Errorf("stderr = %q, want empty; cli.Run already prints the returned error", got)
			}
			// 返り値には失敗の内容が残っている (標準エラー出力を消した代わりに情報が
			// 落ちていないこと)。
			if msg := err.Error(); !strings.Contains(msg, "exit status 3") {
				t.Errorf("error = %q, want it to mention the exec failure", msg)
			}
		})
	}
}

// TestStartEC2SessionSendsPluginArgsAndTerminates は成功経路で
// session-manager-plugin へ渡す引数と、その後の切断を検証する。
func TestStartEC2SessionSendsPluginArgsAndTerminates(t *testing.T) {
	cmd, stderr := newEC2SessionCmd(t, "i-1234567890abcdef0")
	rec := &ec2SessionCall{}

	if err := startEC2SessionWith(cmd, okEC2SessionDeps(rec)); err != nil {
		t.Fatalf("startEC2SessionWith() error = %v, want nil", err)
	}

	// 引数の位置と値はプラグインとの契約であり、順序を入れ替えると別の意味になる。
	// JSON はキー名まで完全一致で固定する。session_plugin.go が「フィールド名は
	// プラグインとの契約であり変更してはならない」と述べている対象がこれである。
	wantArgs := []string{
		`{"SessionId":"sess-1","StreamUrl":"wss://ssmmessages.example/stream","TokenValue":"token-1"}`,
		"ap-northeast-1",
		"StartSession",
		"test-profile",
		`{"Target":"i-1234567890abcdef0"}`,
		"https://ssm.ap-northeast-1.amazonaws.com",
	}
	if diff := cmp.Diff(wantArgs, rec.pluginArgs); diff != "" {
		t.Errorf("plugin args mismatch (-want +got):\n%s", diff)
	}

	if len(rec.startSessionIDs) != 1 || rec.startSessionIDs[0] != "i-1234567890abcdef0" {
		t.Errorf("StartSSMSession called with %v, want exactly [i-1234567890abcdef0]", rec.startSessionIDs)
	}
	if rec.startSessionProfile != "test-profile" || rec.startSessionRegion != "ap-northeast-1" {
		t.Errorf("StartSSMSession called with profile %q region %q, want %q and %q",
			rec.startSessionProfile, rec.startSessionRegion, "test-profile", "ap-northeast-1")
	}
	// --instance-id を指定したので対話選択は呼ばれない。
	if rec.selectCalls != 0 {
		t.Errorf("selectInstance called %d times, want 0", rec.selectCalls)
	}
	if len(rec.terminateIDs) != 1 || rec.terminateIDs[0] != "sess-1" {
		t.Errorf("terminate called with %v, want exactly [sess-1]", rec.terminateIDs)
	}
	if rec.terminateProfile != "test-profile" || rec.terminateRegion != "ap-northeast-1" {
		t.Errorf("terminate called with profile %q region %q, want %q and %q",
			rec.terminateProfile, rec.terminateRegion, "test-profile", "ap-northeast-1")
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty on success", got)
	}
}

// TestStartEC2SessionSelectsInstanceWhenFlagIsEmpty は --instance-id 未指定のときに
// 対話選択の結果が StartSSMSession へ渡ることを検証する。
func TestStartEC2SessionSelectsInstanceWhenFlagIsEmpty(t *testing.T) {
	cmd, _ := newEC2SessionCmd(t, "")
	rec := &ec2SessionCall{}

	if err := startEC2SessionWith(cmd, okEC2SessionDeps(rec)); err != nil {
		t.Fatalf("startEC2SessionWith() error = %v, want nil", err)
	}

	if rec.selectCalls != 1 {
		t.Errorf("selectInstance called %d times, want 1", rec.selectCalls)
	}
	if len(rec.startSessionIDs) != 1 || rec.startSessionIDs[0] != "i-selected" {
		t.Errorf("StartSSMSession called with %v, want exactly [i-selected]", rec.startSessionIDs)
	}
}

// TestStartEC2SessionPropagatesSetupErrors はセッション確立前後の各段の失敗が、
// チェーンを保ったまま伝播することを検証する。startSession 自体の失敗はまだ
// セッションが存在しないため切断を呼ばないが、それより後段 (plugin の探索など) の
// 失敗は、既に AWS 側にセッションが確立済みのため必ず切断を試みる。
func TestStartEC2SessionPropagatesSetupErrors(t *testing.T) {
	base := errors.New("boom")

	tests := []struct {
		name             string
		mutate           func(*ec2SessionDeps)
		wantMsg          string
		wantTerminateIDs []string
	}{
		{
			name: "select instance fails",
			mutate: func(d *ec2SessionDeps) {
				d.selectInstance = func(context.Context, *config.Config) (string, error) { return "", base }
			},
			// selectEC2Instance は既にどの取得で失敗したかを述べるためラップしない。
			wantMsg: "boom",
			// セッションがまだ存在しないため切断は呼ばない。
			wantTerminateIDs: nil,
		},
		{
			name: "start session fails",
			mutate: func(d *ec2SessionDeps) {
				d.startSession = func(context.Context, string, string, string) (*awsinternal.StartSessionResult, error) {
					return nil, base
				}
			},
			wantMsg: "start session: boom",
			// セッションがまだ存在しないため切断は呼ばない。
			wantTerminateIDs: nil,
		},
		{
			name: "plugin lookup fails",
			mutate: func(d *ec2SessionDeps) {
				d.lookupPlugin = func() (string, error) { return "", base }
			},
			// lookupSessionManagerPlugin は PATH に無いことを述べるためラップしない。
			wantMsg: "boom",
			// startSession は既に成功しているため、この失敗でも切断を試みなければ
			// セッションが AWS 側に残る (issue 0142 が問題にした症状)。
			wantTerminateIDs: []string{"sess-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, stderr := newEC2SessionCmd(t, "")
			rec := &ec2SessionCall{}
			deps := okEC2SessionDeps(rec)
			tt.mutate(&deps)

			err := startEC2SessionWith(cmd, deps)
			if !errors.Is(err, base) {
				t.Fatalf("errors.Is() = false, want true; the chain is severed: %v", err)
			}
			if msg := err.Error(); msg != tt.wantMsg {
				t.Errorf("error = %q, want %q", msg, tt.wantMsg)
			}
			if diff := cmp.Diff(tt.wantTerminateIDs, rec.terminateIDs); diff != "" {
				t.Errorf("terminate call mismatch (-want +got):\n%s", diff)
			}
			if got := stderr.String(); got != "" {
				t.Errorf("stderr = %q, want empty", got)
			}
		})
	}
}

// TestStartEC2SessionTerminatesOnLookupPluginFailureAfterSessionEstablished は
// TestStartEC2SessionPropagatesSetupErrors/plugin_lookup_fails の切断失敗経路を
// 別途検証する。切断も失敗した場合、双方のエラーへ errors.Is で到達できる必要がある。
func TestStartEC2SessionTerminatesOnLookupPluginFailureAfterSessionEstablished(t *testing.T) {
	lookupErr := errors.New("session-manager-plugin: executable file not found in $PATH")
	termErr := errors.New("terminate boom")

	cmd, _ := newEC2SessionCmd(t, "i-1234567890abcdef0")
	rec := &ec2SessionCall{}
	deps := okEC2SessionDeps(rec)
	deps.lookupPlugin = func() (string, error) { return "", lookupErr }
	deps.terminateSession = func(_ context.Context, profile, region, sessionID string) error {
		rec.terminateProfile = profile
		rec.terminateRegion = region
		rec.terminateIDs = append(rec.terminateIDs, sessionID)
		return termErr
	}

	err := startEC2SessionWith(cmd, deps)
	if !errors.Is(err, lookupErr) {
		t.Errorf("errors.Is(err, lookupErr) = false, want true: %v", err)
	}
	if !errors.Is(err, termErr) {
		t.Errorf("errors.Is(err, termErr) = false, want true: %v", err)
	}
	wantMsg := "session-manager-plugin: executable file not found in $PATH; terminate session: terminate boom"
	if msg := err.Error(); msg != wantMsg {
		t.Errorf("error = %q, want %q", msg, wantMsg)
	}
	// 切断先が正しいセッションであることまで確認する。誤って別の profile / region /
	// sessionID を渡しても、上記の errors.Is とメッセージ比較だけでは検出できない。
	if rec.terminateProfile != "test-profile" {
		t.Errorf("terminate profile = %q, want %q", rec.terminateProfile, "test-profile")
	}
	if rec.terminateRegion != "ap-northeast-1" {
		t.Errorf("terminate region = %q, want %q", rec.terminateRegion, "ap-northeast-1")
	}
	if diff := cmp.Diff([]string{"sess-1"}, rec.terminateIDs); diff != "" {
		t.Errorf("terminate call mismatch (-want +got):\n%s", diff)
	}
}

// TestStartEC2SessionWrapsTerminateErrorOnSuccessPath は実行が成功したあとの切断の失敗が
// ラップされて伝播することを検証する。
func TestStartEC2SessionWrapsTerminateErrorOnSuccessPath(t *testing.T) {
	base := errors.New("terminate boom")
	cmd, _ := newEC2SessionCmd(t, "i-1234567890abcdef0")
	rec := &ec2SessionCall{}
	deps := okEC2SessionDeps(rec)
	deps.terminateSession = func(context.Context, string, string, string) error { return base }

	err := startEC2SessionWith(cmd, deps)
	if !errors.Is(err, base) {
		t.Fatalf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	if msg := err.Error(); msg != "terminate session: terminate boom" {
		t.Errorf("error = %q, want %q", msg, "terminate session: terminate boom")
	}
}

// TestDefaultEC2SessionDepsIsFullyWired は本番の依存が全フィールド埋まっていることを
// 検証する。フィールドを増やしたときに defaultEC2SessionDeps への追加を忘れると、
// 本番で nil 関数の呼び出しになる。
//
// 各フィールドに「正しい関数が入っているか」は検査していない。関数値の同一性は
// 比較できず、実際に呼べば AWS 接続や外部プロセス起動が起きるためである。
// 姉妹関数の TestDefaultSSOTokenDepsIsFullyWired も同じ範囲に留めている。
// util.ExecCommand 自体がエラーチェーンを保つことは
// internal/util/executer_test.go の TestExecCommand_PreservesExitErrorChain が検証する。
func TestDefaultEC2SessionDepsIsFullyWired(t *testing.T) {
	// 姉妹関数の TestDefaultSSOTokenDepsIsFullyWired はフィールドを 1 つずつ書いているが、
	// ここは reflect で全フィールドを走査する。フィールドを追加したときに検査の追加を
	// 忘れても自動的に対象になる。
	v := reflect.ValueOf(defaultEC2SessionDeps())
	if v.NumField() == 0 {
		t.Fatal("ec2SessionDeps has no field")
	}
	for i := range v.NumField() {
		if v.Field(i).IsNil() {
			t.Errorf("%s is nil", v.Type().Field(i).Name)
		}
	}
}

// TestStartEC2SessionTerminatesWithLiveContextAfterInterrupt は Ctrl-C でコマンドの
// context がキャンセルされた後も SSM セッションの切断が通ることを検証する。
//
// session-manager-plugin の実行中の Ctrl-C は util.ExecCommand が握りつぶして子プロセスへ
// 処理を委ねるが、os/signal は登録済みの全チャネルへ同じシグナルを配送するため、main の
// signal.NotifyContext にも届いてコマンドの context はキャンセル済みになる。切断をその
// context で行うと必ず失敗し、セッションが AWS 側に残り続ける。
//
// 一方で本流の呼び出しはコマンドの context を使わなければならない。中断が AWS への
// 問い合わせに届かなくなる。切断だけを切り離していることを両側から押さえる。
func TestStartEC2SessionTerminatesWithLiveContextAfterInterrupt(t *testing.T) {
	tests := []struct {
		name string
		// execErr は session-manager-plugin の実行結果。切断は成功時と失敗時の
		// 2 経路から呼ばれるため、両方を通す。
		execErr error
	}{
		{name: "exec succeeds"},
		{name: "exec fails", execErr: errors.New("plugin boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _ := newEC2SessionCmd(t, "i-1234567890abcdef0")

			// 中断でキャンセル済みになったコマンドの context を再現する。
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			cmd.SetContext(ctx)

			var startCtxErr, termCtxErr error
			var termHasDeadline bool
			rec := &ec2SessionCall{}
			deps := okEC2SessionDeps(rec)
			deps.startSession = func(ctx context.Context, _, _, _ string) (*awsinternal.StartSessionResult, error) {
				startCtxErr = ctx.Err()
				return &awsinternal.StartSessionResult{SessionID: "sess-1"}, nil
			}
			deps.execPlugin = func(string, ...string) error { return tt.execErr }
			deps.terminateSession = func(ctx context.Context, _, _, sessionID string) error {
				termCtxErr = ctx.Err()
				_, termHasDeadline = ctx.Deadline()
				rec.terminateIDs = append(rec.terminateIDs, sessionID)
				return nil
			}

			err := startEC2SessionWith(cmd, deps)
			if (err != nil) != (tt.execErr != nil) {
				t.Fatalf("startEC2SessionWith() error = %v, want error presence %t", err, tt.execErr != nil)
			}

			// 本流はコマンドの context を使う。Background に差し替えると中断が届かない。
			if !errors.Is(startCtxErr, context.Canceled) {
				t.Errorf("startSession ctx.Err() = %v, want %v", startCtxErr, context.Canceled)
			}
			// 切断はキャンセル済みの context から派生させてはならない。
			if termCtxErr != nil {
				t.Errorf("terminateSession ctx.Err() = %v, want nil", termCtxErr)
			}
			// かつ無期限でもない。応答が返らない場合に切断で止まり続ける。
			if !termHasDeadline {
				t.Error("terminateSession ctx has no deadline, want one")
			}
			if len(rec.terminateIDs) != 1 || rec.terminateIDs[0] != "sess-1" {
				t.Errorf("terminate called with %v, want exactly [sess-1]", rec.terminateIDs)
			}
		})
	}
}
