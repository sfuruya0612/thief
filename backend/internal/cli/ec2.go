package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

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

	ctx := context.Background()
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

// startEC2Session starts an SSM session to an EC2 instance via session-manager-plugin.
func startEC2Session(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	instanceID := cmd.Flag("instance-id").Value.String()

	if instanceID == "" {
		instanceID, err = selectEC2Instance(ctx, cfg)
		if err != nil {
			return err
		}
	}

	session, err := awsinternal.StartSSMSession(ctx, cfg.Profile, cfg.Region, instanceID)
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	// session-manager-plugin が期待する StartSession API レスポンス/リクエストの JSON 形状。
	sessJSON, err := util.Parser(struct {
		SessionId  string
		StreamUrl  string
		TokenValue string
	}{
		SessionId:  session.SessionID,
		StreamUrl:  session.StreamURL,
		TokenValue: session.TokenValue,
	})
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	paramsJSON, err := util.Parser(struct {
		Target string
	}{Target: instanceID})
	if err != nil {
		return fmt.Errorf("marshal start session input: %w", err)
	}

	plug, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		return errors.New("session-manager-plugin not found in PATH")
	}

	ssmEndpoint := fmt.Sprintf("https://ssm.%s.amazonaws.com", cfg.Region)
	execErr := util.ExecCommand(plug, string(sessJSON), cfg.Region, "StartSession", cfg.Profile, string(paramsJSON), ssmEndpoint)

	if execErr != nil {
		cmd.PrintErrf("execute command: %v\n", execErr)
		if termErr := awsinternal.TerminateSSMSession(ctx, cfg.Profile, cfg.Region, session.SessionID); termErr != nil {
			return fmt.Errorf("terminate session after exec error: %w (original exec error: %v)", termErr, execErr)
		}
		return fmt.Errorf("execute command: %w", execErr)
	}

	if err := awsinternal.TerminateSSMSession(ctx, cfg.Profile, cfg.Region, session.SessionID); err != nil {
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
