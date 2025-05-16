package cmd

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/util"
)

// ec2Cmd represents the base command for EC2 operations.
var ec2Cmd = &cobra.Command{
	Use:   "ec2",
	Short: "Manage EC2 instances",
	Long:  `Provides commands to list and manage AWS EC2 instances, including starting SSM sessions.`,
}

// ec2ListCmd represents the command to list EC2 instances.
var ec2ListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List EC2 instances",
	Long:  `Retrieves and displays a list of EC2 instances based on specified filters like region, running state, etc.`,
	RunE:  displayEC2Instances,
}

// ec2StartSessionCmd represents the command to start an SSM session to an EC2 instance.
var ec2StartSessionCmd = &cobra.Command{
	Use:     "session",
	Aliases: []string{"s"},
	Short:   "Start a session to an EC2 instance",
	Long: `Starts an AWS Systems Manager (SSM) session to a specified EC2 instance.
If no instance ID is provided, it will prompt for selection from available instances.`,
	RunE: startSession,
}

var ec2Columns = []util.Column{
	{Header: "Name", Width: 48},
	{Header: "InstanceID", Width: 20},
	{Header: "InstanceType", Width: 12},
	{Header: "Lifecycle", Width: 9},
	{Header: "PrivateIP", Width: 12},
	{Header: "PublicIP", Width: 14},
	{Header: "State", Width: 10},
	{Header: "KeyName", Width: 16},
	{Header: "AZ", Width: 10},
	{Header: "LaunchTime", Width: 30},
}

// displayEC2Instances retrieves and displays EC2 instances.
func displayEC2Instances(cmd *cobra.Command, args []string) error {
	profile := viper.GetString("profile")
	region := viper.GetString("region")

	opts := &aws.Ec2Opts{
		Running: viper.GetBool("running"),
	}

	list := [][]string{}
	if viper.GetBool("global") {
		client, err := aws.NewEC2Client(profile, region)
		if err != nil {
			return fmt.Errorf("create EC2 client: %w", err)
		}

		regions, err := aws.DescribeRegions(client, aws.GenerateDescribeRegionsInput())
		if err != nil {
			return fmt.Errorf("describe regions: %w", err)
		}

		for _, r := range regions {
			instances, err := listEC2Instances(profile, r, opts)
			if err != nil {
				// Log or collect errors for each region instead of returning immediately
				cmd.PrintErrf("list EC2 instances in region %s: %v\n", r, err)
				continue // Continue to the next region
			}
			list = append(list, instances...)
		}
	} else {
		var err error
		list, err = listEC2Instances(profile, region, opts)
		if err != nil {
			return fmt.Errorf("list EC2 instances: %w", err)
		}
	}

	if len(list) == 0 {
		cmd.Println("No EC2 instances found")
		return nil
	}

	formatter := util.NewTableFormatter(ec2Columns, viper.GetString("output"))

	if !viper.GetBool("no-header") {
		formatter.PrintHeader()
	}

	formatter.PrintRows(list)
	return nil
}

// listEC2Instances lists EC2 instances for a given profile, region, and options.
func listEC2Instances(profile, region string, opts *aws.Ec2Opts) ([][]string, error) {
	client, err := aws.NewEC2Client(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create EC2 client: %w", err)
	}

	input, err := aws.GenerateDescribeInstancesInput(opts)
	if err != nil {
		return nil, fmt.Errorf("generate describe instances input: %w", err)
	}

	instances, err := aws.DescribeInstances(client, input)
	if err != nil {
		return nil, fmt.Errorf("describe instances: %w", err)
	}

	return instances, nil
}

// startSession starts an SSM session to an EC2 instance.
func startSession(cmd *cobra.Command, args []string) error {
	profile := viper.GetString("profile")
	region := viper.GetString("region")

	ssmClient, err := aws.NewSsmClient(profile, region)
	if err != nil {
		return fmt.Errorf("create SSM client: %w", err)
	}

	ssmOpts := &aws.SsmOpts{
		PingStatus:   "Online",
		ResourceType: "EC2Instance",
		InstanceId:   viper.GetString("instance-id"),
	}

	if viper.GetString("instance-id") == "" {
		ssmInput := aws.GenerateDescribeInstanceInformationInput(ssmOpts)

		instanceIds, err := aws.DescribeInstanceInformation(ssmClient, ssmInput)
		if err != nil {
			return fmt.Errorf("describe instance information: %w", err)
		}

		if len(instanceIds) == 0 {
			return errors.New("no online EC2 instances found for SSM session")
		}

		instances, err := ssmTargetInstance(profile, region, instanceIds)
		if err != nil {
			return fmt.Errorf("get target instance: %w", err)
		}

		selected, err := util.Select(instances, "Select an EC2 instance:")
		if err != nil {
			return fmt.Errorf("select instance: %w", err)
		}

		ssmOpts.InstanceId = selected.ID()
	}

	startSessionInput := aws.GenerateStartSessionInput(ssmOpts)

	session, err := aws.StartSession(ssmClient, startSessionInput)
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	sessJSON, err := util.Parser(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	paramsJSON, err := util.Parser(startSessionInput)
	if err != nil {
		return fmt.Errorf("marshal start session input: %w", err)
	}

	plug, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		return errors.New("session-manager-plugin not found in PATH")
	}

	ssmOpts.SessionId = *session.SessionId
	terminateSessionInput := aws.GenerateTerminateSessionInput(ssmOpts)

	ssmEndpoint := fmt.Sprintf("https://ssm.%s.amazonaws.com", region)
	execErr := util.ExecCommand(plug, string(sessJSON), region, "StartSession", profile, string(paramsJSON), ssmEndpoint)

	if execErr != nil {
		cmd.PrintErrf("execute command: %v\n", execErr)
		// Attempt to terminate the session even if ExecCommand failed
		if _, termErr := aws.TerminateSession(ssmClient, terminateSessionInput); termErr != nil {
			return fmt.Errorf("terminate session after exec error: %w (original exec error: %v)", termErr, execErr)
		}
		return fmt.Errorf("execute command: %w", execErr)
	}

	// Terminate session after successful execution or if ExecCommand is interrupted (though interruption handling is basic here)
	if _, err := aws.TerminateSession(ssmClient, terminateSessionInput); err != nil {
		return fmt.Errorf("terminate session: %w", err)
	}

	return nil
}

// ssmTargetInstance retrieves EC2 instance details for SSM target selection.
func ssmTargetInstance(profile, region string, instanceIds []string) ([]util.Item, error) {
	ec2Client, err := aws.NewEC2Client(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create EC2 client: %w", err)
	}

	var items [][]string
	for _, id := range instanceIds {
		ec2Opts := &aws.Ec2Opts{
			// Running:    true, // Already filtered by PingStatus="Online" in SSM
			InstanceId: id,
		}

		ec2Input, err := aws.GenerateDescribeInstancesInput(ec2Opts)
		if err != nil {
			return nil, fmt.Errorf("generate describe instances input for %s: %w", id, err)
		}

		instances, err := aws.DescribeInstances(ec2Client, ec2Input)
		if err != nil {
			return nil, fmt.Errorf("describe instances for %s: %w", id, err)
		}
		if len(instances) > 0 {
			items = append(items, instances[0]) // Assuming DescribeInstances returns at most one for a specific ID
		}
	}

	if len(items) == 0 {
		return nil, errors.New("no matching EC2 instances found for SSM selection")
	}

	var utilItems []util.Item
	for _, inst := range items {
		// Ensure inst has enough elements to avoid panic
		if len(inst) > 1 {
			utilItems = append(utilItems, aws.EC2Instance{
				Name:       inst[0],
				InstanceID: inst[1],
			})
		}
	}

	if len(utilItems) == 0 {
		return nil, errors.New("no instances could be processed for selection")
	}

	return utilItems, nil
}
