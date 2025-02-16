package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type EcsOpts struct {
	Clusters    []string
	Cluster     *string
	Services    []string
	Service     *string
	Tasks       []string
	Task        *string
	Status      string
	Container   *string
	Command     *string
	Interactive bool
}

type ecsApi interface {
	ListClusters(ctx context.Context, input *ecs.ListClustersInput, opts ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeClusters(ctx context.Context, input *ecs.DescribeClustersInput, opts ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
	ListServices(ctx context.Context, input *ecs.ListServicesInput, opts ...func(*ecs.Options)) (*ecs.ListServicesOutput, error)
	DescribeServices(ctx context.Context, input *ecs.DescribeServicesInput, opts ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	ListTasks(ctx context.Context, input *ecs.ListTasksInput, opts ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(ctx context.Context, inupt *ecs.DescribeTasksInput, opts ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	ExecuteCommand(ctx context.Context, input *ecs.ExecuteCommandInput, opts ...func(*ecs.Options)) (*ecs.ExecuteCommandOutput, error)
}

func NewECSClient(profile, region string) ecsApi {
	return ecs.NewFromConfig(GetSession(profile, region))
}

func GenerateListClustersInput(opts *EcsOpts) *ecs.ListClustersInput {
	return &ecs.ListClustersInput{}
}

func ListClusters(client ecsApi, input *ecs.ListClustersInput) ([]string, error) {
	output, err := client.ListClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output.ClusterArns, nil
}

func GenerateDescribeClustersInput(opts *EcsOpts) *ecs.DescribeClustersInput {
	return &ecs.DescribeClustersInput{
		Clusters: opts.Clusters,
	}
}

func DescribeClusters(client ecsApi, input *ecs.DescribeClustersInput) ([][]string, error) {
	output, err := client.DescribeClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var clusters [][]string
	for _, c := range output.Clusters {
		cluster := []string{
			*c.ClusterName,
			*c.Status,
			fmt.Sprintf("%d", c.ActiveServicesCount),
			fmt.Sprintf("%d", c.RunningTasksCount),
			fmt.Sprintf("%d", c.PendingTasksCount),
			fmt.Sprintf("%d", c.RegisteredContainerInstancesCount),
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

func GenerateListServicesInput(opts *EcsOpts) *ecs.ListServicesInput {
	return &ecs.ListServicesInput{
		Cluster: opts.Cluster,
	}
}

func ListServices(client ecsApi, input *ecs.ListServicesInput) ([]string, error) {
	output, err := client.ListServices(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output.ServiceArns, nil
}

func GenerateDescribeServicesInput(opts *EcsOpts) *ecs.DescribeServicesInput {
	return &ecs.DescribeServicesInput{
		Cluster:  opts.Cluster,
		Services: opts.Services,
	}
}

func DescribeServices(client ecsApi, input *ecs.DescribeServicesInput) ([][]string, error) {
	output, err := client.DescribeServices(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var services [][]string
	for _, s := range output.Services {
		service := []string{
			strings.Split(*s.ClusterArn, "/")[1],
			*s.ServiceName,
			strings.Split(*s.TaskDefinition, "/")[1],
			*s.Status,
			fmt.Sprintf("%d", s.DesiredCount),
			fmt.Sprintf("%d", s.RunningCount),
			fmt.Sprintf("%d", s.PendingCount),
		}
		services = append(services, service)
	}

	return services, nil
}

func GenerateListTasksInput(opts *EcsOpts) *ecs.ListTasksInput {
	return &ecs.ListTasksInput{
		Cluster:     opts.Cluster,
		ServiceName: opts.Service,
	}
}

func ListTasks(client ecsApi, input *ecs.ListTasksInput) ([]string, error) {
	output, err := client.ListTasks(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output.TaskArns, nil
}

func GenerateDescribeTasksInput(opts *EcsOpts) *ecs.DescribeTasksInput {
	return &ecs.DescribeTasksInput{
		Cluster: opts.Cluster,
		Tasks:   opts.Tasks,
	}
}

func DescribeTasks(client ecsApi, input *ecs.DescribeTasksInput) ([][]string, error) {
	output, err := client.DescribeTasks(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var tasks [][]string
	for _, t := range output.Tasks {
		platformFamily := "None"
		if t.PlatformFamily != nil {
			platformFamily = *t.PlatformFamily
		}

		platformVersion := "None"
		if t.PlatformVersion != nil {
			platformVersion = *t.PlatformVersion
		}

		startedAt := "None"
		if t.StartedAt != nil {
			startedAt = t.StartedAt.Format(time.RFC3339)
		}

		stoppedAt := "None"
		if t.StoppedAt != nil {
			stoppedAt = t.StoppedAt.Format(time.RFC3339)
		}

		task := []string{
			strings.Split(*t.ClusterArn, "/")[1],
			strings.Split(*t.TaskDefinitionArn, "/")[1],
			strings.Split(*t.TaskArn, "/")[2],
			*t.LastStatus,
			*t.DesiredStatus,
			string(t.HealthStatus),
			string(t.LaunchType),
			*t.Cpu,
			*t.Memory,
			platformFamily,
			platformVersion,
			startedAt,
			stoppedAt,
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func GenerateExecuteCommandInput(opts *EcsOpts) *ecs.ExecuteCommandInput {
	return &ecs.ExecuteCommandInput{
		Cluster:     opts.Cluster,
		Task:        opts.Task,
		Container:   opts.Container,
		Command:     opts.Command,
		Interactive: opts.Interactive,
	}
}

func ExecuteCommand(client ecsApi, input *ecs.ExecuteCommandInput) (*ecs.ExecuteCommandOutput, error) {
	output, err := client.ExecuteCommand(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output, nil
}
