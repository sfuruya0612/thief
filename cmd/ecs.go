package cmd

import (
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
	Aliases: []string{"c"},
	Run:     displayECSClusters,
}

var ecsServicesCmd = &cobra.Command{
	Use:     "services",
	Short:   "List ECS services",
	Aliases: []string{"s"},
	Run:     displayECSServices,
}

var ecsTasksCmd = &cobra.Command{
	Use:     "tasks",
	Short:   "List ECS tasks",
	Aliases: []string{"t"},
	Run:     displayECSTasks,
}

var ecsExecCmd = &cobra.Command{
	Use:     "exec",
	Short:   "Execute a command in a container",
	Aliases: []string{"e"},
	Run:     ecsExecuteCommand,
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

func displayECSClusters(cmd *cobra.Command, args []string) {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	client := aws.NewECSClient(profile, region)

	clustersArns, err := aws.ListClusters(client, aws.GenerateListClustersInput(&aws.EcsOpts{}))
	if err != nil {
		fmt.Printf("Unable to list ECS clusters: %v\n", err)
		return
	}

	input := aws.GenerateDescribeClustersInput(&aws.EcsOpts{
		Clusters: clustersArns,
	})

	list, err := aws.DescribeClusters(client, input)
	if err != nil {
		fmt.Printf("Unable to describe ECS clusters: %v\n", err)
		return
	}

	if len(list) == 0 {
		fmt.Println("No ECS clusters found")
		return
	}

	formatter := util.NewTableFormatter(ecsClusterColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(list)
}

func displayECSServices(cmd *cobra.Command, args []string) {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	client := aws.NewECSClient(profile, region)

	clusterArns, err := aws.ListClusters(client, aws.GenerateListClustersInput(&aws.EcsOpts{}))
	if err != nil {
		fmt.Printf("Unable to list ECS clusters: %v\n", err)
		return
	}

	formatter := util.NewTableFormatter(ecsServiceColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	list := [][]string{}
	for _, c := range clusterArns {
		input := aws.GenerateListServicesInput(&aws.EcsOpts{
			Cluster: &c,
		})

		output, err := aws.ListServices(client, input)
		if err != nil {
			fmt.Printf("Unable to list ECS services: %v\n", err)
			return
		}

		i := aws.GenerateDescribeServicesInput(&aws.EcsOpts{
			Cluster:  &c,
			Services: output,
		})

		list, err = aws.DescribeServices(client, i)
		if err != nil {
			fmt.Printf("Unable to describe ECS services: %v\n", err)
			return
		}

		formatter.PrintRows(list)
	}
}

func displayECSTasks(cmd *cobra.Command, args []string) {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	client := aws.NewECSClient(profile, region)

	cluster := cmd.Flag("cluster").Value.String()
	if cluster == "" {
		output, err := aws.ListClusters(client, aws.GenerateListClustersInput(&aws.EcsOpts{}))
		if err != nil {
			fmt.Printf("Unable to list ECS clusters: %v\n", err)
			return
		}

		selected, err := util.Select(selecterEcs(output, 1), "Select an ECS cluster:")
		if err != nil {
			fmt.Printf("Unable to select cluster: %v\n", err)
			return
		}

		cluster = selected.ID()
		fmt.Printf("Selected cluster: %s\n", cluster)
	}

	input := aws.GenerateListTasksInput(&aws.EcsOpts{
		Cluster: &cluster,
	})

	output, err := aws.ListTasks(client, input)
	if err != nil {
		fmt.Printf("Unable to list ECS tasks: %v\n", err)
		return
	}

	if len(output) == 0 {
		fmt.Println("No ECS tasks found")
		return
	}

	i := aws.GenerateDescribeTasksInput(&aws.EcsOpts{
		Cluster: &cluster,
		Tasks:   output,
	})

	list, err := aws.DescribeTasks(client, i)
	if err != nil {
		fmt.Printf("Unable to describe ECS tasks: %v\n", err)
		return
	}

	formatter := util.NewTableFormatter(ecsTaskColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(list)
}

func ecsExecuteCommand(cmd *cobra.Command, args []string) {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	cluster := cmd.Flag("cluster").Value.String()
	task := cmd.Flag("task").Value.String()
	container := cmd.Flag("container").Value.String()
	command := cmd.Flag("command").Value.String()

	if cluster == "" || task == "" || container == "" {
		fmt.Println("--cluster, --task, and --container flags are required")
		return
	}

	// interactive, err := strconv.ParseBool(cmd.Flag("interactive").Value.String())
	// if err != nil {
	// 	fmt.Printf("Unable to parse interactive flag: %v\n", err)
	// 	return
	// }

	opts := &aws.EcsOpts{
		Cluster:     &cluster,
		Task:        &task,
		Container:   &container,
		Command:     &command,
		Interactive: true,
	}

	client := aws.NewECSClient(profile, region)

	input := aws.GenerateExecuteCommandInput(opts)

	output, err := aws.ExecuteCommand(client, input)
	if err != nil {
		fmt.Printf("Unable to execute command: %v\n", err)
		return
	}

	sessJson, err := util.Parser(output.Session)
	if err != nil {
		fmt.Printf("Unable to marshal session: %v\n", err)
		return
	}

	target := fmt.Sprintf("ecs:%s_%s_%s",
		strings.Split(*output.ClusterArn, "/")[1],
		strings.Split(*output.TaskArn, "/")[2],
		strings.Split(*output.ContainerArn, "/")[3],
	)

	targetJson, err := util.Parser(TargetJSON{Target: target})
	if err != nil {
		fmt.Printf("Unable to marshal target: %v\n", err)
		return
	}

	plug, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		fmt.Println("Unable to find session-manager-plugin")
		return
	}

	if err = util.ExecCommand(plug, string(sessJson), region, "StartSession", profile, string(targetJson), fmt.Sprintf("https://ecs.%s.amazonaws.com", region)); err != nil {
		fmt.Printf("Unable to execute command: %v\n", err)
	}
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
