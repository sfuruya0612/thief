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
	"github.com/spf13/cobra"
)

func TestArnName(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		num  int
		want string
	}{
		{
			name: "cluster name from cluster arn",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
			num:  1,
			want: "my-cluster",
		},
		{
			name: "task id from task arn",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:task/my-cluster/abcdef1234567890",
			num:  2,
			want: "abcdef1234567890",
		},
		{
			name: "out of range returns full arn",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
			num:  5,
			want: "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
		},
		{
			name: "negative index returns full arn",
			arn:  "a/b",
			num:  -1,
			want: "a/b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arnName(tt.arn, tt.num); got != tt.want {
				t.Errorf("arnName(%q, %d) = %q, want %q", tt.arn, tt.num, got, tt.want)
			}
		})
	}
}

// newECSExecCmd は ecsExecuteCommandWith が読むフラグだけを持つコマンドと、
// 標準エラー出力の受け皿を返す。ec2_test.go の newEC2SessionCmd と同じ理由で
// config.Load の参照先を空のディレクトリへ向ける。
func newECSExecCmd(t *testing.T, cluster, task, container string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := &cobra.Command{Use: "exec"}
	cmd.Flags().String("cluster", "", "")
	cmd.Flags().String("task", "", "")
	cmd.Flags().String("container", "", "")
	cmd.Flags().String("command", "/bin/sh", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("region", "", "")
	if err := cmd.Flags().Set("profile", "test-profile"); err != nil {
		t.Fatalf("set profile flag: %v", err)
	}
	if err := cmd.Flags().Set("region", "ap-northeast-1"); err != nil {
		t.Fatalf("set region flag: %v", err)
	}
	if err := cmd.Flags().Set("cluster", cluster); err != nil {
		t.Fatalf("set cluster flag: %v", err)
	}
	if err := cmd.Flags().Set("task", task); err != nil {
		t.Fatalf("set task flag: %v", err)
	}
	if err := cmd.Flags().Set("container", container); err != nil {
		t.Fatalf("set container flag: %v", err)
	}

	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	return cmd, &stderr
}

// ecsSessionCall は差し替えた依存が受け取った引数を記録する。
type ecsSessionCall struct {
	pluginArgs         []string
	terminateIDs       []string
	executeSessionArgs []string
	terminateProfile   string
	terminateRegion    string
}

// okECSSessionDeps はすべて成功する依存を返す。個々のテストは必要なフィールドだけを
// 差し替える。
func okECSSessionDeps(rec *ecsSessionCall) ecsExecSessionDeps {
	return ecsExecSessionDeps{
		executeSession: func(_ context.Context, profile, region, cluster, task, container, command string) (*awsinternal.ECSExecSession, error) {
			rec.executeSessionArgs = []string{profile, region, cluster, task, container, command}
			return &awsinternal.ECSExecSession{
				SessionID:    "sess-1",
				StreamURL:    "wss://ssmmessages.example/stream",
				TokenValue:   "token-1",
				ClusterArn:   "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
				TaskArn:      "arn:aws:ecs:ap-northeast-1:123456789012:task/my-cluster/abcdef1234567890",
				ContainerArn: "arn:aws:ecs:ap-northeast-1:123456789012:container/my-cluster/abcdef1234567890/runtime-id",
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

// TestECSExecuteCommandKeepsExecErrorChain は session-manager-plugin の実行が失敗したときに
// errors.As で *exec.ExitError へ到達できることを、セッション切断が成功する経路と失敗する
// 経路の両方について検証する (ec2.go の TestStartEC2SessionKeepsExecErrorChain と同じ観点)。
func TestECSExecuteCommandKeepsExecErrorChain(t *testing.T) {
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
			wantMsg:   "execute session-manager-plugin command: exit status 3",
		},
		{
			name:       "terminate also fails",
			terminate:  termErr,
			wantMsg:    "execute session-manager-plugin command: exit status 3; terminate session: terminate boom",
			wantTermIs: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitErr := execExitError(t)
			cmd, _ := newECSExecCmd(t, "my-cluster", "abcdef1234567890", "my-container")
			rec := &ecsSessionCall{}
			deps := okECSSessionDeps(rec)
			deps.execPlugin = func(string, ...string) error { return exitErr }
			deps.terminateSession = func(_ context.Context, profile, region, sessionID string) error {
				rec.terminateProfile = profile
				rec.terminateRegion = region
				rec.terminateIDs = append(rec.terminateIDs, sessionID)
				return tt.terminate
			}

			err := ecsExecuteCommandWith(cmd, deps)
			if err == nil {
				t.Fatal("ecsExecuteCommandWith() error = nil, want an error")
			}

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
			if got := errors.Is(err, termErr); got != tt.wantTermIs {
				t.Errorf("errors.Is(err, termErr) = %t, want %t", got, tt.wantTermIs)
			}
			if msg := err.Error(); msg != tt.wantMsg {
				t.Errorf("error = %q, want %q", msg, tt.wantMsg)
			}
			// 実行に失敗しても ECS Exec のセッションは AWS 側に残るため、必ず切断を試みる。
			if len(rec.terminateIDs) != 1 || rec.terminateIDs[0] != "sess-1" {
				t.Errorf("terminate called with %v, want exactly [sess-1]", rec.terminateIDs)
			}
			if rec.terminateProfile != "test-profile" || rec.terminateRegion != "ap-northeast-1" {
				t.Errorf("terminate called with profile %q region %q, want %q and %q",
					rec.terminateProfile, rec.terminateRegion, "test-profile", "ap-northeast-1")
			}
		})
	}
}

// TestECSExecuteCommandReportsExecFailureOnlyThroughReturnValue は実行の失敗を標準エラー
// 出力へ書かないことを検証する。返り値の表示は cli.Run が 1 箇所で行うため、ここで
// 書くと同じ内容が 2 回並ぶ。
func TestECSExecuteCommandReportsExecFailureOnlyThroughReturnValue(t *testing.T) {
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
			cmd, stderr := newECSExecCmd(t, "my-cluster", "abcdef1234567890", "my-container")
			rec := &ecsSessionCall{}
			deps := okECSSessionDeps(rec)
			deps.execPlugin = func(string, ...string) error { return exitErr }
			deps.terminateSession = func(context.Context, string, string, string) error { return tt.terminate }

			err := ecsExecuteCommandWith(cmd, deps)
			if err == nil {
				t.Fatal("ecsExecuteCommandWith() error = nil, want an error")
			}
			if got := stderr.String(); got != "" {
				t.Errorf("stderr = %q, want empty; cli.Run already prints the returned error", got)
			}
			if msg := err.Error(); !strings.Contains(msg, "exit status 3") {
				t.Errorf("error = %q, want it to mention the exec failure", msg)
			}
		})
	}
}

// TestECSExecuteCommandSendsPluginArgsAndTerminates は成功経路で
// session-manager-plugin へ渡す引数と、その後の切断を検証する。
func TestECSExecuteCommandSendsPluginArgsAndTerminates(t *testing.T) {
	cmd, stderr := newECSExecCmd(t, "my-cluster", "abcdef1234567890", "my-container")
	rec := &ecsSessionCall{}

	if err := ecsExecuteCommandWith(cmd, okECSSessionDeps(rec)); err != nil {
		t.Fatalf("ecsExecuteCommandWith() error = %v, want nil", err)
	}

	wantArgs := []string{
		`{"SessionId":"sess-1","StreamUrl":"wss://ssmmessages.example/stream","TokenValue":"token-1"}`,
		"ap-northeast-1",
		"StartSession",
		"test-profile",
		`{"Target":"ecs:my-cluster_abcdef1234567890_runtime-id"}`,
		"https://ecs.ap-northeast-1.amazonaws.com",
	}
	if diff := cmp.Diff(wantArgs, rec.pluginArgs); diff != "" {
		t.Errorf("plugin args mismatch (-want +got):\n%s", diff)
	}

	wantExecuteSessionArgs := []string{"test-profile", "ap-northeast-1", "my-cluster", "abcdef1234567890", "my-container", "/bin/sh"}
	if diff := cmp.Diff(wantExecuteSessionArgs, rec.executeSessionArgs); diff != "" {
		t.Errorf("executeSession args mismatch (-want +got):\n%s", diff)
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

// TestECSExecuteCommandRequiresFlags は --cluster / --task / --container のいずれかが
// 空のときにセッションを確立せずエラーを返すことを検証する。
func TestECSExecuteCommandRequiresFlags(t *testing.T) {
	tests := []struct {
		name      string
		cluster   string
		task      string
		container string
	}{
		{name: "missing cluster", cluster: "", task: "t", container: "c"},
		{name: "missing task", cluster: "cl", task: "", container: "c"},
		{name: "missing container", cluster: "cl", task: "t", container: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _ := newECSExecCmd(t, tt.cluster, tt.task, tt.container)
			rec := &ecsSessionCall{}

			err := ecsExecuteCommandWith(cmd, okECSSessionDeps(rec))
			if err == nil {
				t.Fatal("ecsExecuteCommandWith() error = nil, want an error")
			}
			if rec.executeSessionArgs != nil {
				t.Errorf("executeSession called with %v, want no call", rec.executeSessionArgs)
			}
		})
	}
}

// TestECSExecuteCommandPropagatesSetupErrors はセッション確立前後の各段の失敗が、
// チェーンを保ったまま伝播することを検証する。executeSession 自体の失敗はまだ
// セッションが存在しないため切断を呼ばないが、それより後段 (plugin の探索など) の
// 失敗は、既に AWS 側にセッションが確立済みのため必ず切断を試みる。
func TestECSExecuteCommandPropagatesSetupErrors(t *testing.T) {
	base := errors.New("boom")

	tests := []struct {
		name             string
		mutate           func(*ecsExecSessionDeps)
		wantMsg          string
		wantTerminateIDs []string
	}{
		{
			name: "execute session fails",
			mutate: func(d *ecsExecSessionDeps) {
				d.executeSession = func(context.Context, string, string, string, string, string, string) (*awsinternal.ECSExecSession, error) {
					return nil, base
				}
			},
			wantMsg: "execute command: boom",
			// セッションがまだ存在しないため切断は呼ばない。
			wantTerminateIDs: nil,
		},
		{
			name: "plugin lookup fails",
			mutate: func(d *ecsExecSessionDeps) {
				d.lookupPlugin = func() (string, error) { return "", base }
			},
			// lookupSessionManagerPlugin は PATH に無いことを述べるためラップしない。
			wantMsg: "boom",
			// executeSession は既に成功しているため、この失敗でも切断を試みなければ
			// セッションが AWS 側に残る (issue 0135 が問題にした症状の再発)。
			wantTerminateIDs: []string{"sess-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, stderr := newECSExecCmd(t, "my-cluster", "abcdef1234567890", "my-container")
			rec := &ecsSessionCall{}
			deps := okECSSessionDeps(rec)
			tt.mutate(&deps)

			err := ecsExecuteCommandWith(cmd, deps)
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

// TestECSExecuteCommandTerminatesOnLookupPluginFailureAfterSessionEstablished は
// TestECSExecuteCommandPropagatesSetupErrors/plugin_lookup_fails の切断失敗経路を
// 別途検証する。切断も失敗した場合、双方のエラーへ errors.Is で到達できる必要がある。
func TestECSExecuteCommandTerminatesOnLookupPluginFailureAfterSessionEstablished(t *testing.T) {
	lookupErr := errors.New("session-manager-plugin: executable file not found in $PATH")
	termErr := errors.New("terminate boom")

	cmd, _ := newECSExecCmd(t, "my-cluster", "abcdef1234567890", "my-container")
	rec := &ecsSessionCall{}
	deps := okECSSessionDeps(rec)
	deps.lookupPlugin = func() (string, error) { return "", lookupErr }
	deps.terminateSession = func(_ context.Context, profile, region, sessionID string) error {
		rec.terminateProfile = profile
		rec.terminateRegion = region
		rec.terminateIDs = append(rec.terminateIDs, sessionID)
		return termErr
	}

	err := ecsExecuteCommandWith(cmd, deps)
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
}

// TestECSExecuteCommandWrapsTerminateErrorOnSuccessPath は実行が成功したあとの切断の失敗が
// ラップされて伝播することを検証する。
func TestECSExecuteCommandWrapsTerminateErrorOnSuccessPath(t *testing.T) {
	base := errors.New("terminate boom")
	cmd, _ := newECSExecCmd(t, "my-cluster", "abcdef1234567890", "my-container")
	rec := &ecsSessionCall{}
	deps := okECSSessionDeps(rec)
	deps.terminateSession = func(context.Context, string, string, string) error { return base }

	err := ecsExecuteCommandWith(cmd, deps)
	if !errors.Is(err, base) {
		t.Fatalf("errors.Is() = false, want true; the chain is severed: %v", err)
	}
	if msg := err.Error(); msg != "terminate session: terminate boom" {
		t.Errorf("error = %q, want %q", msg, "terminate session: terminate boom")
	}
}

// TestDefaultECSExecSessionDepsIsFullyWired は本番の依存が全フィールド埋まっていることを
// 検証する。フィールドを増やしたときに defaultECSExecSessionDeps への追加を忘れると、
// 本番で nil 関数の呼び出しになる (ec2.go の TestDefaultEC2SessionDepsIsFullyWired と同じ観点)。
func TestDefaultECSExecSessionDepsIsFullyWired(t *testing.T) {
	v := reflect.ValueOf(defaultECSExecSessionDeps())
	if v.NumField() == 0 {
		t.Fatal("ecsExecSessionDeps has no field")
	}
	for i := range v.NumField() {
		if v.Field(i).IsNil() {
			t.Errorf("%s is nil", v.Type().Field(i).Name)
		}
	}
}

// TestECSExecuteCommandTerminatesWithLiveContextAfterInterrupt は Ctrl-C でコマンドの
// context がキャンセルされた後も SSM セッションの切断が通ることを検証する
// (ec2.go の TestStartEC2SessionTerminatesWithLiveContextAfterInterrupt と同じ観点。理由も
// 同じで、util.ExecCommand の実行中の Ctrl-C は main の signal.NotifyContext にも届いて
// コマンドの context をキャンセル済みにするため、切断はそれとは別の context で行う必要が
// ある)。
func TestECSExecuteCommandTerminatesWithLiveContextAfterInterrupt(t *testing.T) {
	tests := []struct {
		name    string
		execErr error
	}{
		{name: "exec succeeds"},
		{name: "exec fails", execErr: errors.New("plugin boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _ := newECSExecCmd(t, "my-cluster", "abcdef1234567890", "my-container")

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			cmd.SetContext(ctx)

			var executeCtxErr, termCtxErr error
			var termHasDeadline bool
			rec := &ecsSessionCall{}
			deps := okECSSessionDeps(rec)
			deps.executeSession = func(ctx context.Context, _, _, _, _, _, _ string) (*awsinternal.ECSExecSession, error) {
				executeCtxErr = ctx.Err()
				return &awsinternal.ECSExecSession{SessionID: "sess-1"}, nil
			}
			deps.execPlugin = func(string, ...string) error { return tt.execErr }
			deps.terminateSession = func(ctx context.Context, _, _, sessionID string) error {
				termCtxErr = ctx.Err()
				_, termHasDeadline = ctx.Deadline()
				rec.terminateIDs = append(rec.terminateIDs, sessionID)
				return nil
			}

			err := ecsExecuteCommandWith(cmd, deps)
			if (err != nil) != (tt.execErr != nil) {
				t.Fatalf("ecsExecuteCommandWith() error = %v, want error presence %t", err, tt.execErr != nil)
			}

			if !errors.Is(executeCtxErr, context.Canceled) {
				t.Errorf("executeSession ctx.Err() = %v, want %v", executeCtxErr, context.Canceled)
			}
			if termCtxErr != nil {
				t.Errorf("terminateSession ctx.Err() = %v, want nil", termCtxErr)
			}
			if !termHasDeadline {
				t.Error("terminateSession ctx has no deadline, want one")
			}
			if len(rec.terminateIDs) != 1 || rec.terminateIDs[0] != "sess-1" {
				t.Errorf("terminate called with %v, want exactly [sess-1]", rec.terminateIDs)
			}
		})
	}
}
