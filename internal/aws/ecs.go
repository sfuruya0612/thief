// Package aws provides AWS service clients and utilities for interacting with AWS services.
package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// EcsOpts defines options for ECS API operations.
type EcsOpts struct {
	Clusters    []string // List of cluster ARNs or names
	Cluster     *string  // Single cluster ARN or name
	Services    []string // List of service ARNs or names
	Service     *string  // Single service ARN or name
	Tasks       []string // List of task ARNs
	Task        *string  // Single task ARN
	Status      string   // Task status filter
	Container   *string  // Container name for execute-command
	Command     *string  // Command to execute in container
	Interactive bool     // Whether to execute command in interactive mode
}

// Ecs represents an ECS resource for selection UI.
type Ecs struct {
	Name string
}

// Title returns the display name of the ECS resource.
func (i Ecs) Title() string {
	return i.Name
}

// ID returns the identifier of the ECS resource.
func (i Ecs) ID() string {
	return i.Name
}

// ecsApi defines the interface for ECS API operations.
// This interface helps with testing by allowing mock implementations.
type ecsApi interface {
	ListClusters(ctx context.Context, input *ecs.ListClustersInput, opts ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeClusters(ctx context.Context, input *ecs.DescribeClustersInput, opts ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
	ListServices(ctx context.Context, input *ecs.ListServicesInput, opts ...func(*ecs.Options)) (*ecs.ListServicesOutput, error)
	DescribeServices(ctx context.Context, input *ecs.DescribeServicesInput, opts ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	ListTasks(ctx context.Context, input *ecs.ListTasksInput, opts ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(ctx context.Context, inupt *ecs.DescribeTasksInput, opts ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	ExecuteCommand(ctx context.Context, input *ecs.ExecuteCommandInput, opts ...func(*ecs.Options)) (*ecs.ExecuteCommandOutput, error)
}

// NewECSClient creates a new ECS client using the specified AWS profile and region.
func NewECSClient(profile, region string) (ecsApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create ECS client: %w", err)
	}
	return ecs.NewFromConfig(cfg), nil
}

// GenerateListClustersInput creates the input for the ListClusters API call.
// Returns an empty input to fetch all clusters.
func GenerateListClustersInput(opts *EcsOpts) *ecs.ListClustersInput {
	return &ecs.ListClustersInput{}
}

// ListClusters calls the ECS ListClusters API and returns the list of cluster ARNs.
func ListClusters(client ecsApi, input *ecs.ListClustersInput) ([]string, error) {
	output, err := client.ListClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output.ClusterArns, nil
}

// GenerateDescribeClustersInput creates the input for the DescribeClusters API call
// with the specified clusters from options.
func GenerateDescribeClustersInput(opts *EcsOpts) *ecs.DescribeClustersInput {
	return &ecs.DescribeClustersInput{
		Clusters: opts.Clusters,
	}
}

// DescribeClusters calls the ECS DescribeClusters API and formats the results
// as string arrays suitable for table display.
// Each cluster is represented as a string array containing name, status, service count,
// running task count, pending task count, and registered container instance count.
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

// GenerateListServicesInput creates the input for the ListServices API call
// with the specified cluster from options.
func GenerateListServicesInput(opts *EcsOpts) *ecs.ListServicesInput {
	return &ecs.ListServicesInput{
		Cluster: opts.Cluster,
	}
}

// ListServices calls the ECS ListServices API and returns the list of service ARNs
// for the specified cluster.
func ListServices(client ecsApi, input *ecs.ListServicesInput) ([]string, error) {
	output, err := client.ListServices(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output.ServiceArns, nil
}

// GenerateDescribeServicesInput creates the input for the DescribeServices API call
// with the specified cluster and services from options.
func GenerateDescribeServicesInput(opts *EcsOpts) *ecs.DescribeServicesInput {
	return &ecs.DescribeServicesInput{
		Cluster:  opts.Cluster,
		Services: opts.Services,
	}
}

// DescribeServices calls the ECS DescribeServices API and formats the results
// as string arrays suitable for table display.
// Each service is represented as a string array containing cluster name, service name,
// task definition, status, desired count, running count, and pending count.
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

		// stoppedAt := "None"
		// if t.StoppedAt != nil {
		// 	stoppedAt = t.StoppedAt.Format(time.RFC3339)
		// }

		for _, c := range t.Containers {
			task := []string{
				strings.Split(*t.TaskDefinitionArn, "/")[1],
				strings.Split(*t.TaskArn, "/")[2],
				*c.Name,
				*t.LastStatus,
				*t.DesiredStatus,
				string(c.HealthStatus),
				string(t.LaunchType),
				platformFamily,
				platformVersion,
				startedAt,
				// stoppedAt,
			}
			tasks = append(tasks, task)
		}

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
