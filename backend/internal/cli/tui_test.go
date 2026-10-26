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
