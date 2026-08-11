package cli

import (
	"context"
	"errors"
	"fmt"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var ec2Columns = []util.Column{
	{Header: "Name"},
	{Header: "InstanceID"},
	{Header: "InstanceType"},
	{Header: "Lifecycle"},
	{Header: "PrivateIP"},
	{Header: "PublicIP"},
	{Header: "State"},
	{Header: "KeyName"},
	{Header: "AZ"},
	{Header: "LaunchTime"},
}

// ec2SelectItem は SSM セッション対象の対話選択に使う表示アイテム。
type ec2SelectItem struct {
	Name       string
	InstanceID string
}

// Title returns a formatted string representation of the EC2 instance for display.
func (i ec2SelectItem) Title() string {
	return fmt.Sprintf("%s (%s)", i.Name, i.InstanceID)
}

// ID returns the EC2 instance ID.
func (i ec2SelectItem) ID() string {
	return i.InstanceID
}

func newEC2Cmd() *cobra.Command {
	ec2Cmd := &cobra.Command{
		Use:   "ec2",
		Short: "Manage EC2 instances",
		Long:  `Provides commands to list and manage AWS EC2 instances, including starting SSM sessions.`,
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List EC2 instances",
		Long:  `Retrieves and displays a list of EC2 instances based on specified filters like region, running state, etc.`,
		RunE:  displayEC2Instances,
	}
	lsCmd.Flags().BoolP("running", "", false, "Show only running instances")
	lsCmd.Flags().BoolP("global", "", false, "Show instances in all regions")

	sessionCmd := &cobra.Command{
		Use:     "session",
		Aliases: []string{"s"},
		Short:   "Start a session to an EC2 instance",
		Long: `Starts an AWS Systems Manager (SSM) session to a specified EC2 instance.
If no instance ID is provided, it will prompt for selection from available instances.`,
		RunE: startEC2Session,
	}
	sessionCmd.Flags().StringP("instance-id", "i", "", "Instance ID")

	ec2Cmd.AddCommand(lsCmd, sessionCmd)
	return ec2Cmd
}

// displayEC2Instances retrieves and displays EC2 instances.
func displayEC2Instances(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	running, _ := cmd.Flags().GetBool("running")
	global, _ := cmd.Flags().GetBool("global")

	ctx := commandContext(cmd)
	opts := awsinternal.EC2ListOptions{Running: running}

	var list []awsinternal.EC2InstanceInfo
	if global {
		regions, err := awsinternal.ListRegions(ctx, cfg.Profile)
		if err != nil {
			return fmt.Errorf("describe regions: %w", err)
		}

		for _, r := range regions {
			instances, err := awsinternal.ListEC2Instances(ctx, cfg.Profile, r.Code, opts)
			if err != nil {
				cmd.PrintErrf("list EC2 instances in region %s: %v\n", r.Code, err)
				continue
			}
			list = append(list, instances...)
		}
	} else {
		list, err = awsinternal.ListEC2Instances(ctx, cfg.Profile, cfg.Region, opts)
		if err != nil {
			return fmt.Errorf("list EC2 instances: %w", err)
		}
	}

	if len(list) == 0 {
		cmd.Println("No EC2 instances found")
		return nil
	}

	return printRowsOrGroupBy(cfg, ec2Columns, toRows(list))
}

// ec2SessionDeps は startEC2Session が SSM セッションの確立と切断で呼ぶ外部処理をまとめる。
// selectInstance / startSession / terminateSession は AWS への接続を伴い、lookupPlugin は
// PATH の解決、execPlugin は session-manager-plugin の起動を伴うため、いずれもテストからは
// 実行できない。エラーの伝播を検証するために差し替える。
// これらは元から自由関数であり、絞り込む対象の具象型が無い。internal/aws のように
// SDK クライアントをコンシューマ定義インターフェースで受けるのではなく、関数値を
// 持たせているのはそのためである。
type ec2SessionDeps struct {
	selectInstance   func(ctx context.Context, cfg *config.Config) (string, error)
	startSession     func(ctx context.Context, profile, region, target string) (*awsinternal.StartSessionResult, error)
	lookupPlugin     func() (string, error)
	execPlugin       func(process string, args ...string) error
	terminateSession func(ctx context.Context, profile, region, sessionID string) error
}

// defaultEC2SessionDeps は本番で使う実装を返す。
func defaultEC2SessionDeps() ec2SessionDeps {
	return ec2SessionDeps{
		selectInstance:   selectEC2Instance,
		startSession:     awsinternal.StartSSMSession,
		lookupPlugin:     lookupSessionManagerPlugin,
		execPlugin:       util.ExecCommand,
		terminateSession: awsinternal.TerminateSSMSession,
	}
}

// startEC2Session starts an SSM session to an EC2 instance via session-manager-plugin.
func startEC2Session(cmd *cobra.Command, args []string) error {
	return startEC2SessionWith(cmd, defaultEC2SessionDeps())
}

// startEC2SessionWith は startEC2Session の本体。
// 失敗の報告は返り値だけに任せ、この関数は標準エラー出力へ何も書かない。
// 返り値のエラーは cli.Run が 1 箇所で表示するため、ここで書くと同じ内容が 2 回並ぶ。
func startEC2SessionWith(cmd *cobra.Command, deps ec2SessionDeps) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	instanceID := cmd.Flag("instance-id").Value.String()

	if instanceID == "" {
		instanceID, err = deps.selectInstance(ctx, cfg)
		if err != nil {
			return err
		}
	}

	session, err := deps.startSession(ctx, cfg.Profile, cfg.Region, instanceID)
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	sessJSON, err := sessionManagerSessionJSON(session.SessionID, session.StreamURL, session.TokenValue)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	paramsJSON, err := util.Parser(struct {
		Target string
	}{Target: instanceID})
	if err != nil {
		return fmt.Errorf("marshal start session input: %w", err)
	}

	plug, err := deps.lookupPlugin()
	if err != nil {
		return err
	}

	ssmEndpoint := fmt.Sprintf("https://ssm.%s.amazonaws.com", cfg.Region)
	execErr := deps.execPlugin(plug, string(sessJSON), cfg.Region, "StartSession", cfg.Profile, string(paramsJSON), ssmEndpoint)

	// 切断は ctx とは別の context で行う。session-manager-plugin の実行中の Ctrl-C は
	// util.ExecCommand が握りつぶして子プロセスに処理を委ねるが、os/signal は登録済みの
	// 全チャネルへ同じシグナルを配送するため、main の signal.NotifyContext にも届いて
	// ctx はキャンセル済みになる。SIGTERM も util.ExecCommand が子プロセスへ転送して
	// 委ねるが、同じ理由で ctx はキャンセル済みになる。キャンセル済みの context で
	// TerminateSSMSession を呼ぶと必ず失敗し、セッションが AWS 側に残る。
	//
	// internal/session/bridge.go の cleanup が同じ理由で専用の短命 context を使っている。
	termCtx, cancelTerm := context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)
	defer cancelTerm()

	if execErr != nil {
		// 実行が失敗しても SSM セッションは AWS 側に残るため、必ず切断を試みる。
		// 切断も失敗した場合は 2 つの失敗を両方 %w で包む。Go 1.20 以降 fmt.Errorf は
		// %w を複数取れるため、errors.Is / errors.As がどちらの側にも到達する。
		//
		// どちらの経路も execute command: で始める。実行の失敗の見え方が、無関係な
		// 後処理である切断の成否によって変わらないようにするためである。
		if termErr := deps.terminateSession(termCtx, cfg.Profile, cfg.Region, session.SessionID); termErr != nil {
			return fmt.Errorf("execute command: %w; terminate session: %w", execErr, termErr)
		}
		return fmt.Errorf("execute command: %w", execErr)
	}

	if err := deps.terminateSession(termCtx, cfg.Profile, cfg.Region, session.SessionID); err != nil {
		return fmt.Errorf("terminate session: %w", err)
	}

	return nil
}

// selectEC2Instance は SSM 接続可能なインスタンスを対話式に選択させ、インスタンス ID を返す。
func selectEC2Instance(ctx context.Context, cfg *config.Config) (string, error) {
	instanceIDs, err := awsinternal.ListSSMOnlineInstanceIDs(ctx, cfg.Profile, cfg.Region)
	if err != nil {
		return "", fmt.Errorf("describe instance information: %w", err)
	}

	if len(instanceIDs) == 0 {
		return "", errors.New("no online EC2 instances found for SSM session")
	}

	instances, err := awsinternal.ListEC2Instances(ctx, cfg.Profile, cfg.Region, awsinternal.EC2ListOptions{
		InstanceIDs: instanceIDs,
	})
	if err != nil {
		return "", fmt.Errorf("get target instance: %w", err)
	}

	// DescribeInstances の結果順は不定なため、SSM が返した ID 順に並べる。
	byID := make(map[string]awsinternal.EC2InstanceInfo, len(instances))
	for _, inst := range instances {
		byID[inst.InstanceID] = inst
	}

	var items []util.Item
	for _, id := range instanceIDs {
		inst, ok := byID[id]
		if !ok {
			continue
		}
		items = append(items, ec2SelectItem{Name: inst.Name, InstanceID: inst.InstanceID})
	}

	if len(items) == 0 {
		return "", errors.New("no matching EC2 instances found for SSM selection")
	}

	selected, err := util.Select(items, "Select an EC2 instance:")
	if err != nil {
		return "", fmt.Errorf("select instance: %w", err)
	}

	return selected.ID(), nil
}
