package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var ecsClusterColumns = []util.Column{
	{Header: "ClusterName"},
	{Header: "Status"},
	{Header: "ActiveServices"},
	{Header: "RunningTasks"},
	{Header: "PendingTasks"},
	{Header: "RegisteredContainerInstances"},
}

var ecsServiceColumns = []util.Column{
	{Header: "ClusterName"},
	{Header: "ServiceName"},
	{Header: "TaskDefinition"},
	{Header: "Status"},
	{Header: "DesiredTasks"},
	{Header: "RunningTasks"},
	{Header: "PendingTasks"},
}

var ecsTaskColumns = []util.Column{
	{Header: "TaskDefinition"},
	{Header: "Task"},
	{Header: "Container"},
	{Header: "LastStatus"},
	{Header: "DesiredStatus"},
	{Header: "HealthStatus"},
	{Header: "LaunchType"},
	{Header: "PlatformFamily"},
	{Header: "PlatformVersion"},
	{Header: "StartedAt"},
}

// ecsSelectItem は ECS クラスタの対話選択に使う表示アイテム。
type ecsSelectItem struct {
	Name string
}

// Title returns the display name of the ECS resource.
func (i ecsSelectItem) Title() string { return i.Name }

// ID returns the identifier of the ECS resource.
func (i ecsSelectItem) ID() string { return i.Name }

func newECSCmd() *cobra.Command {
	ecsCmd := &cobra.Command{
		Use:   "ecs",
		Short: "Manage ECS",
	}

	clustersCmd := &cobra.Command{
		Use:     "clusters",
		Short:   "List ECS clusters",
		Long:    "Retrieves and displays a list of ECS clusters in the current region.",
		Aliases: []string{"c"},
		RunE:    displayECSClusters,
	}

	servicesCmd := &cobra.Command{
		Use:     "services",
		Short:   "List ECS services",
		Long:    "Retrieves and displays a list of ECS services in the specified cluster.",
		Aliases: []string{"s"},
		RunE:    displayECSServices,
	}

	tasksCmd := &cobra.Command{
		Use:     "tasks",
		Short:   "List ECS tasks",
		Long:    "Retrieves and displays a list of ECS tasks in the specified cluster.",
		Aliases: []string{"t"},
		RunE:    displayECSTasks,
	}
	tasksCmd.Flags().StringP("cluster", "", "", "Cluster name")
	tasksCmd.Flags().BoolP("running", "", false, "Show only running tasks")

	execCmd := &cobra.Command{
		Use:     "exec",
		Short:   "Execute a command in a container",
		Long:    "Executes a command in a container running in an ECS task using AWS SSM Session Manager.",
		Aliases: []string{"e"},
		RunE:    ecsExecuteCommand,
	}
	execCmd.Flags().StringP("cluster", "", "", "Cluster name")
	execCmd.Flags().StringP("task", "", "", "Task name")
	execCmd.Flags().StringP("container", "", "", "Container name")
	execCmd.Flags().StringP("command", "", "/bin/sh", "Command")

	ecsCmd.AddCommand(clustersCmd, servicesCmd, tasksCmd, execCmd)
	return ecsCmd
}

func displayECSClusters(cmd *cobra.Command, args []string) error {
	return runList(cmd, ListConfig[awsinternal.ECSClusterInfo]{
		Columns:  ecsClusterColumns,
		EmptyMsg: "No ECS clusters found",
		Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ECSClusterInfo, error) {
			arns, err := awsinternal.ListECSClusterArns(ctx, cfg.Profile, cfg.Region)
			if err != nil {
				return nil, fmt.Errorf("list ECS clusters: %w", err)
			}
			if len(arns) == 0 {
				return nil, nil
			}
			return awsinternal.GetECSClusterInfos(ctx, cfg.Profile, cfg.Region, arns)
		},
	})
}

func displayECSServices(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	clusterArns, err := awsinternal.ListECSClusterArns(ctx, cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("list ECS clusters: %w", err)
	}

	var allRows [][]string
	for _, c := range clusterArns {
		services, err := awsinternal.GetECSServiceInfos(ctx, cfg.Profile, cfg.Region, c)
		if err != nil {
			return fmt.Errorf("describe ECS services for cluster %s: %w", c, err)
		}
		allRows = append(allRows, toRows(services)...)
	}

	if len(allRows) == 0 {
		cmd.Println("No ECS services found")
		return nil
	}

	return printRowsOrGroupBy(cfg, ecsServiceColumns, allRows)
}

func displayECSTasks(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	cluster := cmd.Flag("cluster").Value.String()
	if cluster == "" {
		arns, err := awsinternal.ListECSClusterArns(ctx, cfg.Profile, cfg.Region)
		if err != nil {
			return fmt.Errorf("list ECS clusters: %w", err)
		}

		selected, err := util.Select(ctx, ecsSelectItems(arns, 1), "Select an ECS cluster:")
		if err != nil {
			return fmt.Errorf("select cluster: %w", err)
		}

		cluster = selected.ID()
		cmd.Printf("Selected cluster: %s\n", cluster)
	}

	running, _ := cmd.Flags().GetBool("running")
	status := ""
	if running {
		status = "RUNNING"
	}

	tasks, err := awsinternal.ListECSTaskInfos(ctx, cfg.Profile, cfg.Region, cluster, status)
	if err != nil {
		return fmt.Errorf("list ECS tasks: %w", err)
	}

	if len(tasks) == 0 {
		cmd.Println("No ECS tasks found")
		return nil
	}

	return printRowsOrGroupBy(cfg, ecsTaskColumns, toRows(tasks))
}

// ecsExecSessionDeps は ecsExecuteCommand が ECS Exec セッションの確立と切断で呼ぶ
// 外部処理をまとめる。ec2SessionDeps と同じ理由 (AWS 呼び出しと session-manager-plugin の
// 起動を伴い、テストから実行できない) で差し替え可能にする。
type ecsExecSessionDeps struct {
	executeSession   func(ctx context.Context, profile, region, cluster, task, container, command string) (*awsinternal.ECSExecSession, error)
	lookupPlugin     func() (string, error)
	execPlugin       func(process string, args ...string) error
	terminateSession func(ctx context.Context, profile, region, sessionID string) error
}

// defaultECSExecSessionDeps は本番で使う実装を返す。
func defaultECSExecSessionDeps() ecsExecSessionDeps {
	return ecsExecSessionDeps{
		executeSession:   awsinternal.ExecuteECSCommandSession,
		lookupPlugin:     lookupSessionManagerPlugin,
		execPlugin:       util.ExecCommand,
		terminateSession: awsinternal.TerminateSSMSession,
	}
}

func ecsExecuteCommand(cmd *cobra.Command, args []string) error {
	return ecsExecuteCommandWith(cmd, defaultECSExecSessionDeps())
}

// ecsExecuteCommandWith は ecsExecuteCommand の本体。
func ecsExecuteCommandWith(cmd *cobra.Command, deps ecsExecSessionDeps) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	cluster := cmd.Flag("cluster").Value.String()
	task := cmd.Flag("task").Value.String()
	container := cmd.Flag("container").Value.String()
	command := cmd.Flag("command").Value.String()

	if cluster == "" || task == "" || container == "" {
		return errors.New("--cluster, --task, and --container flags are required")
	}

	ctx := commandContext(cmd)
	session, err := deps.executeSession(ctx, cfg.Profile, cfg.Region, cluster, task, container, command)
	if err != nil {
		return fmt.Errorf("execute command: %w", err)
	}

	// この時点で AWS 側に ECS Exec のセッションが確立している。以降のどの失敗経路
	// (JSON の組み立て、plugin の探索、plugin の実行) で return する場合も、切断を
	// 試みてからでなければセッションが AWS 側に残ってしまう。切断は ctx とは別の
	// 短命 context で行う。理由は startEC2SessionWith (ec2.go) と同じで、
	// util.ExecCommand の実行中に届いた Ctrl-C / SIGTERM は main の signal.NotifyContext
	// にも配送されて ctx をキャンセル済みにするため、ctx のままでは TerminateSSMSession
	// が必ず失敗し、セッションが AWS 側に残る。猶予の長さは internal/aws の
	// TerminateSessionGracePeriod を ec2.go の同じ後始末と共有する。
	termCtx, cancelTerm := context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)
	defer cancelTerm()

	terminateThen := func(cause error) error {
		if termErr := deps.terminateSession(termCtx, cfg.Profile, cfg.Region, session.SessionID); termErr != nil {
			return fmt.Errorf("%w; terminate session: %w", cause, termErr)
		}
		return cause
	}

	sessJSON, err := sessionManagerSessionJSON(session.SessionID, session.StreamURL, session.TokenValue)
	if err != nil {
		return terminateThen(fmt.Errorf("marshal session: %w", err))
	}

	targetJSON, err := util.Parser(struct {
		Target string `json:"Target"`
	}{Target: session.Target()})
	if err != nil {
		return terminateThen(fmt.Errorf("marshal target: %w", err))
	}

	plug, err := deps.lookupPlugin()
	if err != nil {
		return terminateThen(err)
	}

	execErr := deps.execPlugin(plug, string(sessJSON), cfg.Region, "StartSession", cfg.Profile, string(targetJSON), fmt.Sprintf("https://ecs.%s.amazonaws.com", cfg.Region))
	if execErr != nil {
		return terminateThen(fmt.Errorf("execute session-manager-plugin command: %w", execErr))
	}

	if err := deps.terminateSession(termCtx, cfg.Profile, cfg.Region, session.SessionID); err != nil {
		return fmt.Errorf("terminate session: %w", err)
	}

	return nil
}

// ecsSelectItems は ARN 一覧を "/" 区切りの num 番目の要素を名前とする選択アイテムに変換する。
func ecsSelectItems(arns []string, num int) []util.Item {
	var items []util.Item
	for _, arn := range arns {
		items = append(items, ecsSelectItem{Name: arnName(arn, num)})
	}
	return items
}

// arnName は ARN を "/" 区切りで分割した num 番目の要素を返す。
// 要素が足りない場合は ARN 全体を返す。
func arnName(arn string, num int) string {
	parts := strings.Split(arn, "/")
	if num < 0 || num >= len(parts) {
		return arn
	}
	return parts[num]
}
