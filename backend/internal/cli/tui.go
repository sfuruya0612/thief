package cli

import (
	"context"
	"fmt"
	"os/exec"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/tui"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive terminal UI",
		Long: `Opens a full-screen terminal UI that lists AWS resources, shows their details,
and starts interactive sessions, like the web UI but inside the terminal.`,
		RunE: runTUI,
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	return tui.Run(newTUIDeps(), cfg.Profile, cfg.Region)
}

// newTUIDeps は TUI が使う取得層とセッション開始の処理を組み立てる。
// Services の各要素は、Web UI の同じ一覧のハンドラが使う internal/aws の関数をプロセス内で直接呼ぶ。
func newTUIDeps() tui.Deps {
	return tui.Deps{
		ListProfiles: awsinternal.ListProfiles,
		ListRegions:  awsinternal.ListRegions,
		Services: []tui.Service{
			tui.NewEC2Service(awsinternal.ListEC2Resources),
		},
		StartSession: sessionManagerCommand,
	}
}

// tuiSessionDeps は sessionManagerCommandWith が使う AWS の呼び出しと plugin の解決。
// ec2SessionDeps (ec2.go) と同じく、テストで差し替えるために関数で持つ。
type tuiSessionDeps struct {
	startSession     func(ctx context.Context, profile, region, target string) (*awsinternal.StartSessionResult, error)
	lookupPlugin     func() (string, error)
	terminateSession func(ctx context.Context, profile, region, sessionID string) error
}

// defaultTUISessionDeps は本番で使う実装を返す。
func defaultTUISessionDeps() tuiSessionDeps {
	return tuiSessionDeps{
		startSession:     awsinternal.StartSSMSession,
		lookupPlugin:     lookupSessionManagerPlugin,
		terminateSession: awsinternal.TerminateSSMSession,
	}
}

// sessionManagerCommand は SSM セッションを開始し、session-manager-plugin を起動する exec.Cmd と、
// プロセスの終了後に AWS 側のセッションを切断する finalize を返す。
// startEC2SessionWith (ec2.go) と同じ手順を、TUI が子プロセスの実行を bubbletea に委ねられる
// 形に分けたもの。セッションの開始後に失敗した場合は、ここで切断してから返す。
func sessionManagerCommand(ctx context.Context, profile, region, target string) (*exec.Cmd, func() error, error) {
	return sessionManagerCommandWith(ctx, defaultTUISessionDeps(), profile, region, target)
}

// sessionManagerCommandWith は sessionManagerCommand の本体。deps で AWS の呼び出しを差し替えられる。
func sessionManagerCommandWith(ctx context.Context, deps tuiSessionDeps, profile, region, target string) (*exec.Cmd, func() error, error) {
	session, err := deps.startSession(ctx, profile, region, target)
	if err != nil {
		return nil, nil, fmt.Errorf("start session: %w", err)
	}

	// 切断は呼び出し元の ctx とは別の短命 context で行う (startEC2SessionWith と同じ理由)。
	finalize := func() error {
		termCtx, cancel := context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)
		defer cancel()
		if err := deps.terminateSession(termCtx, profile, region, session.SessionID); err != nil {
			return fmt.Errorf("terminate session: %w", err)
		}
		return nil
	}
	failAfterStart := func(cause error) (*exec.Cmd, func() error, error) {
		if termErr := finalize(); termErr != nil {
			return nil, nil, fmt.Errorf("%w; %w", cause, termErr)
		}
		return nil, nil, cause
	}

	sessJSON, err := sessionManagerSessionJSON(session.SessionID, session.StreamURL, session.TokenValue)
	if err != nil {
		return failAfterStart(fmt.Errorf("marshal session: %w", err))
	}
	paramsJSON, err := util.Parser(struct {
		Target string
	}{Target: target})
	if err != nil {
		return failAfterStart(fmt.Errorf("marshal start session input: %w", err))
	}
	plug, err := deps.lookupPlugin()
	if err != nil {
		return failAfterStart(err)
	}

	ssmEndpoint := fmt.Sprintf("https://ssm.%s.amazonaws.com", region)
	c := exec.Command(plug, string(sessJSON), region, "StartSession", profile, string(paramsJSON), ssmEndpoint)
	return c, finalize, nil
}
