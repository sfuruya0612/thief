package cli

import (
	"context"
	"errors"
	"fmt"
	"sync"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/gcp"
	"github.com/sfuruya0612/thief/backend/internal/session"
	tidbclient "github.com/sfuruya0612/thief/backend/internal/tidb"
	"github.com/sfuruya0612/thief/backend/internal/tui"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive terminal UI",
		Long: `Opens a full-screen terminal UI that lists AWS, Google Cloud, Datadog, and TiDB Cloud
resources, shows their details, and starts interactive AWS sessions, like the web UI but
inside the terminal. Press P to switch providers.`,
		RunE: runTUI,
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	return tui.Run(newTUIDeps(cfg), tui.Target{
		Provider: tui.ProviderAWS,
		Scope:    cfg.Profile,
		Region:   cfg.Region,
	})
}

// newTUIDeps は TUI が使う取得層とセッション開始の処理を組み立てる。
// 各プロバイダーのサービスは、Web UI の同じ一覧のハンドラが使う取得層の関数をプロセス内で
// 直接呼ぶ。Providers の並びは Web UI のトップバー (frontend/src/components/TopBar.tsx) と
// 同じ AWS、Google Cloud、Datadog、TiDB の順にする。
func newTUIDeps(cfg *config.Config) tui.Deps {
	return tui.Deps{
		Providers: []tui.Provider{
			tui.NewAWSProvider(awsinternal.ListProfiles, awsinternal.ListRegions, tuiAWSServices()),
			tui.NewGCPProvider(func(ctx context.Context) ([]gcp.ProjectInfo, error) {
				return listTUIProjects(ctx, cfg)
			}, cfg.BigQuery.ProjectID, tuiGCPServices()),
			tui.NewDatadogProvider(tui.DatadogProviderOptions{
				Site:       cfg.Datadog.Site,
				APIKey:     cfg.DatadogAPIKey(),
				AppKey:     cfg.DatadogAppKey(),
				StartMonth: cfg.Datadog.StartMonth,
				EndMonth:   cfg.Datadog.EndMonth,
				View:       cfg.Datadog.View,
			}),
			tui.NewTiDBProvider(
				tidbclient.NewClient(cfg.TiDB.PublicKey, cfg.TiDBPrivateKey()),
				cfg.TiDB.BilledMonth,
			),
		},
		OpenEC2Session:     openTUIEC2Session,
		ListECSTasks:       awsinternal.ListECSTasks,
		ListECSContainers:  awsinternal.ListECSContainers,
		OpenECSExecSession: openTUIECSExecSession,
	}
}

// tuiAWSServices は AWS の TUI サービスを Web UI のサイドバーと同じ順で返す。
func tuiAWSServices() []tui.Service {
	return []tui.Service{
		tui.NewEC2Service(awsinternal.ListEC2Resources),
		tui.NewLambdaService(awsinternal.ListLambdaResources),
		tui.NewECRService(awsinternal.ListECRResources, awsinternal.ListECRImages),
		tui.NewECSInteractiveService(awsinternal.ListECSResources, awsinternal.ListECSServices, awsinternal.ListECSTasks, awsinternal.ListECSContainerInstances),
		tui.NewS3Service(awsinternal.ListS3Resources, awsinternal.ListS3Objects),
		tui.NewRDSInteractiveService(awsinternal.ListRDSResources, awsinternal.ListRDSParameters, awsinternal.ListRDSClusterParameters),
		tui.NewDynamoServiceWithTabs(awsinternal.ListDynamoResources, awsinternal.DescribeDynamoTable, awsinternal.QueryDynamoItems),
		tui.NewCacheService(awsinternal.ListElastiCacheResources, awsinternal.ListElastiCacheParameters),
		tui.NewELBServiceWithTabs(awsinternal.ListELBResources, awsinternal.ListELBListeners, awsinternal.ListELBRules, awsinternal.ListELBTargetGroups, awsinternal.DescribeELBTargetHealth),
		tui.NewCloudFrontService(awsinternal.ListCloudFrontResources),
		tui.NewAPIGatewayService(awsinternal.ListAPIGatewayResources),
		tui.NewNATGatewayService(awsinternal.ListNATGatewayResources),
		tui.NewKinesisService(awsinternal.ListKinesisResources),
		tui.NewSQSService(awsinternal.ListSQSResources),
		tui.NewIAMService(awsinternal.ListIAMResources),
		tui.NewWAFServiceWithRules(awsinternal.ListWAFResources, awsinternal.ListWAFRules),
		tui.NewSecretsService(awsinternal.ListSecretResources, awsinternal.GetSecretValue),
		tui.NewSSMService(awsinternal.ListSSMParameters, awsinternal.GetSSMParameter),
		tui.NewCFNServiceWithTabs(awsinternal.ListCFNStacks, awsinternal.DescribeCFNStackDetail, awsinternal.ListCFNStackEvents, awsinternal.ListCFNStackResources),
	}
}

// tuiGCPServices は Google Cloud の TUI サービスを Web UI のサイドバーと同じ順で返す。
// BigQuery と Cloud Logging はクエリエディタ / ログビューアのため、AWS の Athena と
// CloudWatch Logs と同じく TUI の一覧には含めない。
func tuiGCPServices() []tui.Service {
	return []tui.Service{
		tui.NewGCPCloudRunService(gcp.ListCloudRun),
		tui.NewGCSBucketService(gcp.ListBuckets, gcp.ListObjects),
		tui.NewGCPIAMService(gcp.ListIAMBindings),
		tui.NewGCPServiceAccountService(gcp.ListServiceAccounts),
	}
}

// listTUIProjects は TUI のプロジェクト選択に出す一覧を返す。Web UI と CLI と同じく
// ローカルのキャッシュ (~/.config/thief/gcp-projects.json) を優先し、無いときだけ
// Cloud Resource Manager から取得する。
func listTUIProjects(ctx context.Context, cfg *config.Config) ([]gcp.ProjectInfo, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	projects, _, ok, err := gcp.LoadProjectsFromDisk(dir)
	if err != nil {
		return nil, err
	}
	if !ok || len(projects) == 0 {
		// キャッシュが無いときと、キャッシュが空のときは取り直す。空のキャッシュを
		// 返し続けると、プロジェクトを作った後に p を押しても選択肢が増えない。
		return gcp.RefreshProjectsOnDisk(ctx, dir)
	}
	return projects, nil
}

// tuiDataChannel は TUI のセッションが使うデータチャネルの操作。テストで差し替える。
type tuiDataChannel interface {
	Read(ctx context.Context) (session.ReadResult, error)
	SendInput(ctx context.Context, payloadType session.PayloadType, payload []byte) error
	SendSize(ctx context.Context, cols, rows uint32) error
	Close() error
}

// tuiAdapterDeps は TUI のセッション開始が使う AWS とデータチャネルの呼び出し。
type tuiAdapterDeps struct {
	startSession     func(ctx context.Context, profile, region, target string) (*awsinternal.StartSessionResult, error)
	executeSession   func(ctx context.Context, profile, region, cluster, task, container, command string) (*awsinternal.StartSessionResult, error)
	openDataChannel  func(ctx context.Context, streamURL, tokenValue, sessionID string) (tuiDataChannel, error)
	terminateSession func(ctx context.Context, profile, region, sessionID string) error
}

// defaultTUIAdapterDeps は本番で使う実装を返す。
func defaultTUIAdapterDeps() tuiAdapterDeps {
	return tuiAdapterDeps{
		startSession:     awsinternal.StartSSMSession,
		executeSession:   awsinternal.ExecuteECSCommand,
		openDataChannel:  openTUIDataChannel,
		terminateSession: awsinternal.TerminateSSMSession,
	}
}

// openTUIDataChannel は TUI のセッション用に SSM のデータチャネルを開く。
func openTUIDataChannel(ctx context.Context, streamURL, tokenValue, sessionID string) (tuiDataChannel, error) {
	return session.OpenDataChannel(ctx, streamURL, tokenValue, sessionID)
}

// tuiSession は backend/internal/session の DataChannel を TUI の InteractiveSession に合わせる。
type tuiSession struct {
	dc        tuiDataChannel
	terminate func(ctx context.Context) error

	closeOnce sync.Once
	closeErr  error
}

// Read はデータチャネルの出力を TUI の SessionOutput に変換する。
func (s *tuiSession) Read(ctx context.Context) (tui.SessionOutput, error) {
	result, err := s.dc.Read(ctx)
	if err != nil {
		return tui.SessionOutput{}, err
	}
	return tui.SessionOutput{
		Data:    result.Output,
		Closed:  result.Closed,
		Message: result.CloseMessage,
	}, nil
}

// SendInput は端末入力をデータチャネルへ送る。
func (s *tuiSession) SendInput(ctx context.Context, data []byte) error {
	return s.dc.SendInput(ctx, session.PayloadTypeOutput, data)
}

// SendSize は端末サイズをデータチャネルへ送る。
func (s *tuiSession) SendSize(ctx context.Context, cols, rows uint32) error {
	return s.dc.SendSize(ctx, cols, rows)
}

// Close はデータチャネルを閉じ、AWS 側のセッションを終了する。2 回目以降は最初の結果を返す。
func (s *tuiSession) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		// リモートの channel_closed の後に呼ばれることがある。データチャネルの Close は
		// すでに閉じている場合にエラーを返し得るため、切断の成否は TerminateSSMSession で判定する。
		_ = s.dc.Close()
		if s.terminate != nil {
			// 呼び出し元の context がキャンセル済みでも AWS 側のセッションを終えるため、
			// キャンセルを切り離した短命 context で切断する。
			termCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), awsinternal.TerminateSessionGracePeriod)
			defer cancel()
			s.closeErr = s.terminate(termCtx)
		}
	})
	return s.closeErr
}

// openTUIEC2Session は EC2 の Session Manager のセッションを開始し、TUI 用のアダプタを返す。
func openTUIEC2Session(ctx context.Context, profile, region, target string) (tui.InteractiveSession, error) {
	return openTUIEC2SessionWith(ctx, defaultTUIAdapterDeps(), profile, region, target)
}

// openTUIEC2SessionWith は openTUIEC2Session の本体。deps で外部呼び出しを差し替えられる。
func openTUIEC2SessionWith(ctx context.Context, deps tuiAdapterDeps, profile, region, target string) (tui.InteractiveSession, error) {
	started, err := deps.startSession(ctx, profile, region, target)
	if err != nil {
		return nil, &tui.SessionStartError{Started: false, Err: fmt.Errorf("start session: %w", err)}
	}

	dc, err := deps.openDataChannel(ctx, started.StreamURL, started.TokenValue, started.SessionID)
	if err != nil {
		termErr := terminateTUISession(deps, profile, region, started.SessionID)
		return nil, &tui.SessionStartError{
			Started: true,
			Err:     errors.Join(fmt.Errorf("open data channel: %w", err), termErr),
		}
	}

	return &tuiSession{
		dc: dc,
		terminate: func(ctx context.Context) error {
			if err := deps.terminateSession(ctx, profile, region, started.SessionID); err != nil {
				return fmt.Errorf("terminate session: %w", err)
			}
			return nil
		},
	}, nil
}

// openTUIECSExecSession は ECS Exec のセッションを開始し、TUI 用のアダプタを返す。
func openTUIECSExecSession(ctx context.Context, profile, region, cluster, task, container, command string) (tui.InteractiveSession, error) {
	return openTUIECSExecSessionWith(ctx, defaultTUIAdapterDeps(), profile, region, cluster, task, container, command)
}

// openTUIECSExecSessionWith は openTUIECSExecSession の本体。deps で外部呼び出しを差し替えられる。
func openTUIECSExecSessionWith(ctx context.Context, deps tuiAdapterDeps, profile, region, cluster, task, container, command string) (tui.InteractiveSession, error) {
	started, err := deps.executeSession(ctx, profile, region, cluster, task, container, command)
	if err != nil {
		return nil, &tui.SessionStartError{Started: false, Err: fmt.Errorf("execute command: %w", err)}
	}

	dc, err := deps.openDataChannel(ctx, started.StreamURL, started.TokenValue, started.SessionID)
	if err != nil {
		termErr := terminateTUISession(deps, profile, region, started.SessionID)
		return nil, &tui.SessionStartError{
			Started: true,
			Err:     errors.Join(fmt.Errorf("open data channel: %w", err), termErr),
		}
	}

	return &tuiSession{
		dc: dc,
		terminate: func(ctx context.Context) error {
			if err := deps.terminateSession(ctx, profile, region, started.SessionID); err != nil {
				return fmt.Errorf("terminate session: %w", err)
			}
			return nil
		},
	}, nil
}

// terminateTUISession は開始後の後始末で AWS 側のセッションを切断する。
func terminateTUISession(deps tuiAdapterDeps, profile, region, sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)
	defer cancel()
	if err := deps.terminateSession(ctx, profile, region, sessionID); err != nil {
		return fmt.Errorf("terminate session: %w", err)
	}
	return nil
}
