// Package aws provides AWS service clients and utilities for interacting with AWS services.
package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
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

// Ecs represents an ECS resource for the interactive selection UI.
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

// ECSClusterInfo holds display fields for an ECS cluster.
type ECSClusterInfo struct {
	Name                         string
	Status                       string
	ActiveServicesCount          int32
	RunningTasksCount            int32
	PendingTasksCount            int32
	RegisteredContainerInstances int32
}

// ToRow converts ECSClusterInfo to a string slice suitable for table formatting.
func (c ECSClusterInfo) ToRow() []string {
	return []string{
		c.Name,
		c.Status,
		fmt.Sprintf("%d", c.ActiveServicesCount),
		fmt.Sprintf("%d", c.RunningTasksCount),
		fmt.Sprintf("%d", c.PendingTasksCount),
		fmt.Sprintf("%d", c.RegisteredContainerInstances),
	}
}

// ECSServiceInfo holds display fields for an ECS service.
type ECSServiceInfo struct {
	ClusterName    string
	ServiceName    string
	TaskDefinition string
	Status         string
	DesiredCount   int32
	RunningCount   int32
	PendingCount   int32
}

// ToRow converts ECSServiceInfo to a string slice suitable for table formatting.
func (s ECSServiceInfo) ToRow() []string {
	return []string{
		s.ClusterName,
		s.ServiceName,
		s.TaskDefinition,
		s.Status,
		fmt.Sprintf("%d", s.DesiredCount),
		fmt.Sprintf("%d", s.RunningCount),
		fmt.Sprintf("%d", s.PendingCount),
	}
}

// ECSTaskInfo holds display fields for an ECS task container.
type ECSTaskInfo struct {
	TaskDefinition  string
	TaskID          string
	ContainerName   string
	LastStatus      string
	DesiredStatus   string
	HealthStatus    string
	LaunchType      string
	PlatformFamily  string
	PlatformVersion string
	StartedAt       string
}

// ToRow converts ECSTaskInfo to a string slice suitable for table formatting.
func (t ECSTaskInfo) ToRow() []string {
	return []string{
		t.TaskDefinition,
		t.TaskID,
		t.ContainerName,
		t.LastStatus,
		t.DesiredStatus,
		t.HealthStatus,
		t.LaunchType,
		t.PlatformFamily,
		t.PlatformVersion,
		t.StartedAt,
	}
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
		return nil, fmt.Errorf("create ecs client: %w", err)
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

// DescribeClusters calls the ECS DescribeClusters API and returns the results
// as a typed slice of ECSClusterInfo.
func DescribeClusters(client ecsApi, input *ecs.DescribeClustersInput) ([]ECSClusterInfo, error) {
	output, err := client.DescribeClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var clusters []ECSClusterInfo
	for _, c := range output.Clusters {
		clusters = append(clusters, ECSClusterInfo{
			Name:                         *c.ClusterName,
			Status:                       *c.Status,
			ActiveServicesCount:          c.ActiveServicesCount,
			RunningTasksCount:            c.RunningTasksCount,
			PendingTasksCount:            c.PendingTasksCount,
			RegisteredContainerInstances: c.RegisteredContainerInstancesCount,
		})
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

// DescribeServices calls the ECS DescribeServices API and returns the results
// as a typed slice of ECSServiceInfo.
func DescribeServices(client ecsApi, input *ecs.DescribeServicesInput) ([]ECSServiceInfo, error) {
	output, err := client.DescribeServices(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var services []ECSServiceInfo
	for _, s := range output.Services {
		services = append(services, ECSServiceInfo{
			ClusterName:    strings.Split(*s.ClusterArn, "/")[1],
			ServiceName:    *s.ServiceName,
			TaskDefinition: strings.Split(*s.TaskDefinition, "/")[1],
			Status:         *s.Status,
			DesiredCount:   s.DesiredCount,
			RunningCount:   s.RunningCount,
			PendingCount:   s.PendingCount,
		})
	}

	return services, nil
}

// GenerateListTasksInput creates the input for the ListTasks API call.
// If opts.Status is set (e.g., "RUNNING" or "STOPPED"), it filters tasks by desired status.
func GenerateListTasksInput(opts *EcsOpts) *ecs.ListTasksInput {
	input := &ecs.ListTasksInput{
		Cluster:     opts.Cluster,
		ServiceName: opts.Service,
	}

	if opts.Status != "" {
		input.DesiredStatus = types.DesiredStatus(opts.Status)
	}

	return input
}

// ListTasks calls the ECS ListTasks API and returns the list of task ARNs.
func ListTasks(client ecsApi, input *ecs.ListTasksInput) ([]string, error) {
	output, err := client.ListTasks(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output.TaskArns, nil
}

// GenerateDescribeTasksInput creates the input for the DescribeTasks API call.
func GenerateDescribeTasksInput(opts *EcsOpts) *ecs.DescribeTasksInput {
	return &ecs.DescribeTasksInput{
		Cluster: opts.Cluster,
		Tasks:   opts.Tasks,
	}
}

// DescribeTasks calls the ECS DescribeTasks API and returns the results
// as a typed slice of ECSTaskInfo (one entry per container per task).
func DescribeTasks(client ecsApi, input *ecs.DescribeTasksInput) ([]ECSTaskInfo, error) {
	output, err := client.DescribeTasks(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var tasks []ECSTaskInfo
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

		for _, c := range t.Containers {
			tasks = append(tasks, ECSTaskInfo{
				TaskDefinition:  strings.Split(*t.TaskDefinitionArn, "/")[1],
				TaskID:          strings.Split(*t.TaskArn, "/")[2],
				ContainerName:   *c.Name,
				LastStatus:      *t.LastStatus,
				DesiredStatus:   *t.DesiredStatus,
				HealthStatus:    string(c.HealthStatus),
				LaunchType:      string(t.LaunchType),
				PlatformFamily:  platformFamily,
				PlatformVersion: platformVersion,
				StartedAt:       startedAt,
			})
		}
	}

	return tasks, nil
}

// GenerateExecuteCommandInput creates the input for the ExecuteCommand API call.
func GenerateExecuteCommandInput(opts *EcsOpts) *ecs.ExecuteCommandInput {
	return &ecs.ExecuteCommandInput{
		Cluster:     opts.Cluster,
		Task:        opts.Task,
		Container:   opts.Container,
		Command:     opts.Command,
		Interactive: opts.Interactive,
	}
}

// ExecuteCommand executes a command in an ECS container.
func ExecuteCommand(client ecsApi, input *ecs.ExecuteCommandInput) (*ecs.ExecuteCommandOutput, error) {
	output, err := client.ExecuteCommand(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return output, nil
}
