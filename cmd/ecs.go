package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/util"
	"github.com/spf13/cobra"
)

var ecsCmd = &cobra.Command{
	Use:   "ecs",
	Short: "Manage ECS",
}

var ecsClustersCmd = &cobra.Command{
	Use:     "clusters",
	Short:   "List ECS clusters",
	Long:    "Retrieves and displays a list of ECS clusters in the current region.",
	Aliases: []string{"c"},
	RunE:    displayECSClusters,
}

var ecsServicesCmd = &cobra.Command{
	Use:     "services",
	Short:   "List ECS services",
	Long:    "Retrieves and displays a list of ECS services in the specified cluster.",
	Aliases: []string{"s"},
	RunE:    displayECSServices,
}

var ecsTasksCmd = &cobra.Command{
	Use:     "tasks",
	Short:   "List ECS tasks",
	Long:    "Retrieves and displays a list of ECS tasks in the specified cluster.",
	Aliases: []string{"t"},
	RunE:    displayECSTasks,
}

var ecsExecCmd = &cobra.Command{
	Use:     "exec",
	Short:   "Execute a command in a container",
	Long:    "Executes a command in a container running in an ECS task using AWS SSM Session Manager.",
	Aliases: []string{"e"},
	RunE:    ecsExecuteCommand,
}

var ecsClusterColumns = []util.Column{
	{Header: "ClusterName", Width: 70},
	{Header: "Status", Width: 7},
	{Header: "ActiveServices", Width: 14},
	{Header: "RunningTasks", Width: 12},
	{Header: "PendingTasks", Width: 12},
	{Header: "RegisteredContainerInstances", Width: 28},
}

var ecsServiceColumns = []util.Column{
	{Header: "ClusterName", Width: 65},
	{Header: "ServiceName", Width: 65},
	{Header: "TaskDefinition", Width: 65},
	{Header: "Status", Width: 7},
	{Header: "DesiredTasks", Width: 12},
	{Header: "RunningTasks", Width: 12},
	{Header: "PendingTasks", Width: 12},
}

var ecsTaskColumns = []util.Column{
	{Header: "TaskDefinition", Width: 65},
	{Header: "Task", Width: 32},
	{Header: "Container", Width: 24},
	{Header: "LastStatus", Width: 10},
	{Header: "DesiredStatus", Width: 13},
	{Header: "HealthStatus", Width: 12},
	{Header: "LaunchType", Width: 12},
	{Header: "PlatformFamily", Width: 14},
	{Header: "PlatformVersion", Width: 15},
	{Header: "StartedAt", Width: 20},
	// {Header: "StoppedAt", Width: 20},
}

type TargetJSON struct {
	Target string `json:"Target"`
}

func displayECSClusters(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	client, err := aws.NewECSClient(profile, region)
	if err != nil {
		return fmt.Errorf("create ECS client: %w", err)
	}

	clustersArns, err := aws.ListClusters(client, aws.GenerateListClustersInput(&aws.EcsOpts{}))
	if err != nil {
		return fmt.Errorf("list ECS clusters: %w", err)
	}

	input := aws.GenerateDescribeClustersInput(&aws.EcsOpts{
		Clusters: clustersArns,
	})

	list, err := aws.DescribeClusters(client, input)
	if err != nil {
		return fmt.Errorf("describe ECS clusters: %w", err)
	}

	if len(list) == 0 {
		cmd.Println("No ECS clusters found")
		return nil
	}

	formatter := util.NewTableFormatter(ecsClusterColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(list)
	return nil
}

func displayECSServices(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	client, err := aws.NewECSClient(profile, region)
	if err != nil {
		return fmt.Errorf("create ECS client: %w", err)
	}

	clusterArns, err := aws.ListClusters(client, aws.GenerateListClustersInput(&aws.EcsOpts{}))
	if err != nil {
		return fmt.Errorf("list ECS clusters: %w", err)
	}

	formatter := util.NewTableFormatter(ecsServiceColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	for _, c := range clusterArns {
		input := aws.GenerateListServicesInput(&aws.EcsOpts{
			Cluster: &c,
		})

		output, err := aws.ListServices(client, input)
		if err != nil {
			return fmt.Errorf("list ECS services for cluster %s: %w", c, err)
		}

		i := aws.GenerateDescribeServicesInput(&aws.EcsOpts{
			Cluster:  &c,
			Services: output,
		})

		list, err := aws.DescribeServices(client, i)
		if err != nil {
			return fmt.Errorf("describe ECS services for cluster %s: %w", c, err)
		}

		formatter.PrintRows(list)
	}

	return nil
}

func displayECSTasks(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	client, err := aws.NewECSClient(profile, region)
	if err != nil {
		return fmt.Errorf("create ECS client: %w", err)
	}

	cluster := cmd.Flag("cluster").Value.String()
	if cluster == "" {
		output, err := aws.ListClusters(client, aws.GenerateListClustersInput(&aws.EcsOpts{}))
		if err != nil {
			return fmt.Errorf("list ECS clusters: %w", err)
		}

		selected, err := util.Select(selecterEcs(output, 1), "Select an ECS cluster:")
		if err != nil {
			return fmt.Errorf("select cluster: %w", err)
		}

		cluster = selected.ID()
		cmd.Printf("Selected cluster: %s\n", cluster)
	}

	input := aws.GenerateListTasksInput(&aws.EcsOpts{
		Cluster: &cluster,
	})

	output, err := aws.ListTasks(client, input)
	if err != nil {
		return fmt.Errorf("list ECS tasks: %w", err)
	}

	if len(output) == 0 {
		cmd.Println("No ECS tasks found")
		return nil
	}

	i := aws.GenerateDescribeTasksInput(&aws.EcsOpts{
		Cluster: &cluster,
		Tasks:   output,
	})

	list, err := aws.DescribeTasks(client, i)
	if err != nil {
		return fmt.Errorf("describe ECS tasks: %w", err)
	}

	formatter := util.NewTableFormatter(ecsTaskColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(list)
	return nil
}

func ecsExecuteCommand(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	cluster := cmd.Flag("cluster").Value.String()
	task := cmd.Flag("task").Value.String()
	container := cmd.Flag("container").Value.String()
	command := cmd.Flag("command").Value.String()

	if cluster == "" || task == "" || container == "" {
		return errors.New("--cluster, --task, and --container flags are required")
	}

	// interactive, err := strconv.ParseBool(cmd.Flag("interactive").Value.String())
	// if err != nil {
	// 	return fmt.Errorf("parse interactive flag: %w", err)
	// }

	opts := &aws.EcsOpts{
		Cluster:     &cluster,
		Task:        &task,
		Container:   &container,
		Command:     &command,
		Interactive: true,
	}

	client, err := aws.NewECSClient(profile, region)
	if err != nil {
		return fmt.Errorf("create ECS client: %w", err)
	}

	input := aws.GenerateExecuteCommandInput(opts)

	output, err := aws.ExecuteCommand(client, input)
	if err != nil {
		return fmt.Errorf("execute command: %w", err)
	}

	sessJson, err := util.Parser(output.Session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	target := fmt.Sprintf("ecs:%s_%s_%s",
		strings.Split(*output.ClusterArn, "/")[1],
		strings.Split(*output.TaskArn, "/")[2],
		strings.Split(*output.ContainerArn, "/")[3],
	)

	targetJson, err := util.Parser(TargetJSON{Target: target})
	if err != nil {
		return fmt.Errorf("marshal target: %w", err)
	}

	plug, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		return errors.New("session-manager-plugin not found in PATH")
	}

	if err = util.ExecCommand(plug, string(sessJson), region, "StartSession", profile, string(targetJson), fmt.Sprintf("https://ecs.%s.amazonaws.com", region)); err != nil {
		return fmt.Errorf("execute session-manager-plugin command: %w", err)
	}

	return nil
}

func selecterEcs(Arns []string, num int) []util.Item {
	var items []util.Item
	for _, arn := range Arns {
		items = append(items, aws.Ecs{
			Name: strings.Split(arn, "/")[num],
		})
	}

	return items
}
