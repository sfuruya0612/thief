package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// TestNewTUIDeps_ServicesFollowWebUIOrder は TUI に載せるサービスの集合と並びを固定する。
// 並びは Web UI のサイドバー (frontend/src/lib/serviceMeta.ts の SERVICES) と同じで、第 1 段階の 19 サービス
// (docs/prd/thief.md の成功指標) が揃っている。サービスを足したら、ここの期待値も更新する。
func TestNewTUIDeps_ServicesFollowWebUIOrder(t *testing.T) {
	deps := newTUIDeps()

	var got []string
	seen := map[string]bool{}
	for _, s := range deps.Services {
		if seen[s.ID()] {
			t.Errorf("service %q is registered twice", s.ID())
		}
		seen[s.ID()] = true
		got = append(got, s.ID())
	}
	want := []string{"ec2", "lambda", "ecr", "ecs", "s3", "rds", "dynamo", "cache", "elb", "cloudfront", "apigw", "natgw", "kinesis", "sqs", "iam", "waf", "secrets", "ssm", "cfn"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("service IDs = %v, want %v", got, want)
	}

	if deps.ListProfiles == nil || deps.ListRegions == nil || deps.StartSession == nil {
		t.Errorf("deps must wire ListProfiles, ListRegions and StartSession: %+v", deps)
	}
	// ECS Exec の 3 関数は、同じシグネチャの別の関数を配線しても非 nil の確認では通るため、関数の同一性で固定する。
	wired := map[string][2]uintptr{
		"ListECSTasks":      {reflect.ValueOf(deps.ListECSTasks).Pointer(), reflect.ValueOf(awsinternal.ListECSTasks).Pointer()},
		"ListECSContainers": {reflect.ValueOf(deps.ListECSContainers).Pointer(), reflect.ValueOf(awsinternal.ListECSContainers).Pointer()},
		"StartECSExec":      {reflect.ValueOf(deps.StartECSExec).Pointer(), reflect.ValueOf(ecsExecCommand).Pointer()},
	}
	for name, pair := range wired {
		if pair[0] == 0 || pair[0] != pair[1] {
			t.Errorf("deps.%s must be the production function (awsinternal.%s or ecsExecCommand)", name, name)
		}
	}
}

// tuiSessionCall は tuiSessionDeps の偽物が記録する呼び出し。
type tuiSessionCall struct {
	startTargets []string
	terminateIDs []string
}

func okTUISessionDeps(rec *tuiSessionCall) tuiSessionDeps {
	return tuiSessionDeps{
		startSession: func(_ context.Context, _, _, target string) (*awsinternal.StartSessionResult, error) {
			rec.startTargets = append(rec.startTargets, target)
			return &awsinternal.StartSessionResult{
				SessionID:  "sess-1",
				StreamURL:  "wss://ssmmessages.example/stream",
				TokenValue: "token-1",
			}, nil
		},
		lookupPlugin: func() (string, error) { return "/usr/local/bin/session-manager-plugin", nil },
		terminateSession: func(_ context.Context, _, _, sessionID string) error {
			rec.terminateIDs = append(rec.terminateIDs, sessionID)
			return nil
		},
	}
}

func TestSessionManagerCommandWith_Success(t *testing.T) {
	rec := &tuiSessionCall{}
	cmd, finalize, err := sessionManagerCommandWith(context.Background(), okTUISessionDeps(rec), "prof", "ap-northeast-1", "i-web")
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if cmd == nil || finalize == nil {
		t.Fatal("cmd and finalize must be returned on success")
	}
	if got := rec.startTargets; len(got) != 1 || got[0] != "i-web" {
		t.Errorf("StartSSMSession targets = %v, want [i-web]", got)
	}
	// 引数は startEC2SessionWith と同じ並び: セッション JSON、リージョン、StartSession、プロファイル、入力 JSON、エンドポイント。
	args := cmd.Args
	if len(args) != 7 || args[0] != "/usr/local/bin/session-manager-plugin" || args[2] != "ap-northeast-1" ||
		args[3] != "StartSession" || args[4] != "prof" || args[6] != "https://ssm.ap-northeast-1.amazonaws.com" {
		t.Errorf("cmd.Args = %q", args)
	}
	if !strings.Contains(args[1], "sess-1") || !strings.Contains(args[5], "i-web") {
		t.Errorf("session JSON / params JSON = %q / %q", args[1], args[5])
	}
	if len(rec.terminateIDs) != 0 {
		t.Errorf("terminate must not run before finalize: %v", rec.terminateIDs)
	}
	if err := finalize(); err != nil {
		t.Fatalf("finalize() error = %v", err)
	}
	if got := rec.terminateIDs; len(got) != 1 || got[0] != "sess-1" {
		t.Errorf("TerminateSSMSession IDs = %v, want [sess-1]", got)
	}
}

func TestSessionManagerCommandWith_Failures(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(d *tuiSessionDeps)
		wantErr       []string
		wantTerminate int
	}{
		{
			name: "start session fails before anything to terminate",
			modify: func(d *tuiSessionDeps) {
				d.startSession = func(context.Context, string, string, string) (*awsinternal.StartSessionResult, error) {
					return nil, errors.New("AccessDeniedException")
				}
			},
			wantErr:       []string{"start session: AccessDeniedException"},
			wantTerminate: 0,
		},
		{
			name: "plugin missing terminates the started session",
			modify: func(d *tuiSessionDeps) {
				d.lookupPlugin = func() (string, error) { return "", errors.New("session-manager-plugin not found in PATH") }
			},
			wantErr:       []string{"session-manager-plugin not found in PATH"},
			wantTerminate: 1,
		},
		{
			name: "plugin missing and terminate fails reports both",
			modify: func(d *tuiSessionDeps) {
				d.lookupPlugin = func() (string, error) { return "", errors.New("session-manager-plugin not found in PATH") }
				d.terminateSession = func(context.Context, string, string, string) error { return errors.New("terminate boom") }
			},
			wantErr:       []string{"session-manager-plugin not found in PATH", "terminate session: terminate boom"},
			wantTerminate: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &tuiSessionCall{}
			deps := okTUISessionDeps(rec)
			tt.modify(&deps)
			cmd, finalize, err := sessionManagerCommandWith(context.Background(), deps, "prof", "ap-northeast-1", "i-web")
			if err == nil {
				t.Fatal("error = nil, want an error")
			}
			if cmd != nil || finalize != nil {
				t.Errorf("cmd = %v, finalize nil = %v, want both nil on failure", cmd, finalize == nil)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should contain %q", err, want)
				}
			}
			if len(rec.terminateIDs) != tt.wantTerminate {
				t.Errorf("terminate calls = %v, want %d", rec.terminateIDs, tt.wantTerminate)
			}
		})
	}
}

// TestECSExecCommandWith_Success は組み立てる exec.Cmd の引数が ecsExecuteCommandWith (ecs.go) の
// TestECSExecuteCommandSendsPluginArgsAndTerminates と同じ並びであることと、finalize が切断を呼ぶことを確かめる。
func TestECSExecCommandWith_Success(t *testing.T) {
	rec := &ecsSessionCall{}
	cmd, finalize, err := ecsExecCommandWith(context.Background(), "test-profile", "ap-northeast-1", "my-cluster", "abcdef1234567890", "my-container", "/bin/sh", okECSSessionDeps(rec))
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if cmd == nil || finalize == nil {
		t.Fatal("cmd and finalize must be returned on success")
	}
	wantArgs := []string{
		"/usr/local/bin/session-manager-plugin",
		`{"SessionId":"sess-1","StreamUrl":"wss://ssmmessages.example/stream","TokenValue":"token-1"}`,
		"ap-northeast-1",
		"StartSession",
		"test-profile",
		`{"Target":"ecs:my-cluster_abcdef1234567890_runtime-id"}`,
		"https://ecs.ap-northeast-1.amazonaws.com",
	}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Errorf("cmd.Args = %q, want %q", cmd.Args, wantArgs)
	}
	wantExecute := []string{"test-profile", "ap-northeast-1", "my-cluster", "abcdef1234567890", "my-container", "/bin/sh"}
	if !reflect.DeepEqual(rec.executeSessionArgs, wantExecute) {
		t.Errorf("executeSession args = %v, want %v", rec.executeSessionArgs, wantExecute)
	}
	if rec.pluginArgs != nil {
		t.Errorf("execPlugin must not run (bubbletea runs the process): %v", rec.pluginArgs)
	}
	if len(rec.terminateIDs) != 0 {
		t.Errorf("terminate must not run before finalize: %v", rec.terminateIDs)
	}
	if err := finalize(); err != nil {
		t.Fatalf("finalize() error = %v", err)
	}
	if got := rec.terminateIDs; len(got) != 1 || got[0] != "sess-1" || rec.terminateProfile != "test-profile" || rec.terminateRegion != "ap-northeast-1" {
		t.Errorf("terminate IDs = %v (profile %q region %q), want [sess-1] with test-profile / ap-northeast-1", got, rec.terminateProfile, rec.terminateRegion)
	}
}

// TestECSExecCommandWith_Failures は ExecuteCommand の失敗では切断を呼ばず、セッションの確立後の失敗
// (plugin の探索、切断の失敗) では切断を試みてから失敗を返すことを確かめる。
func TestECSExecCommandWith_Failures(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(d *ecsExecSessionDeps)
		wantErr       []string
		wantTerminate int
	}{
		{
			name: "execute command fails before anything to terminate",
			modify: func(d *ecsExecSessionDeps) {
				d.executeSession = func(context.Context, string, string, string, string, string, string) (*awsinternal.ECSExecSession, error) {
					return nil, errors.New("AccessDeniedException")
				}
			},
			wantErr:       []string{"execute command: AccessDeniedException"},
			wantTerminate: 0,
		},
		{
			name: "plugin missing terminates the started session",
			modify: func(d *ecsExecSessionDeps) {
				d.lookupPlugin = func() (string, error) { return "", errors.New("session-manager-plugin not found in PATH") }
			},
			wantErr:       []string{"session-manager-plugin not found in PATH"},
			wantTerminate: 1,
		},
		{
			name: "plugin missing and terminate failing reports both",
			modify: func(d *ecsExecSessionDeps) {
				d.lookupPlugin = func() (string, error) { return "", errors.New("session-manager-plugin not found in PATH") }
				prev := d.terminateSession
				d.terminateSession = func(ctx context.Context, profile, region, sessionID string) error {
					// 切断の呼び出しは記録したうえで失敗させる。
					if err := prev(ctx, profile, region, sessionID); err != nil {
						return err
					}
					return errors.New("terminate boom")
				}
			},
			wantErr:       []string{"session-manager-plugin not found in PATH", "terminate session: terminate boom"},
			wantTerminate: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &ecsSessionCall{}
			deps := okECSSessionDeps(rec)
			tt.modify(&deps)
			cmd, finalize, err := ecsExecCommandWith(context.Background(), "test-profile", "ap-northeast-1", "my-cluster", "abcdef1234567890", "my-container", "/bin/sh", deps)
			if err == nil {
				t.Fatal("error = nil, want an error")
			}
			if cmd != nil || finalize != nil {
				t.Errorf("cmd = %v, finalize nil = %v, want both nil on failure", cmd, finalize == nil)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should contain %q", err, want)
				}
			}
			if len(rec.terminateIDs) != tt.wantTerminate {
				t.Errorf("terminate calls = %v, want %d", rec.terminateIDs, tt.wantTerminate)
			}
			for _, id := range rec.terminateIDs {
				if id != "sess-1" {
					t.Errorf("terminate must use the started session ID: %v", rec.terminateIDs)
				}
			}
		})
	}
}

// TestECSExecCommandWith_FinalizeError は finalize が切断の失敗を包んで返すことを確かめる。
func TestECSExecCommandWith_FinalizeError(t *testing.T) {
	rec := &ecsSessionCall{}
	deps := okECSSessionDeps(rec)
	deps.terminateSession = func(context.Context, string, string, string) error { return errors.New("terminate boom") }
	_, finalize, err := ecsExecCommandWith(context.Background(), "test-profile", "ap-northeast-1", "my-cluster", "abcdef1234567890", "my-container", "/bin/sh", deps)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if err := finalize(); err == nil || !strings.Contains(err.Error(), "terminate session: terminate boom") {
		t.Errorf("finalize() error = %v, want the terminate failure", err)
	}
}
