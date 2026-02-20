package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

var (
	mockListClustersOutput = &ecs.ListClustersOutput{
		ClusterArns: []string{
			"arn:aws:ecs:ap-northeast-1:123456789012:cluster/test-cluster",
		},
	}

	mockDescribeClustersOutput = &ecs.DescribeClustersOutput{
		Clusters: []types.Cluster{
			{
				ClusterName:                       aws.String("test-cluster"),
				Status:                            aws.String("ACTIVE"),
				ActiveServicesCount:               2,
				RunningTasksCount:                 5,
				PendingTasksCount:                 0,
				RegisteredContainerInstancesCount: 3,
			},
		},
	}

	mockListServicesOutput = &ecs.ListServicesOutput{
		ServiceArns: []string{
			"arn:aws:ecs:ap-northeast-1:123456789012:service/test-cluster/test-service",
		},
	}

	mockDescribeServicesOutput = &ecs.DescribeServicesOutput{
		Services: []types.Service{
			{
				ClusterArn:     aws.String("arn:aws:ecs:ap-northeast-1:123456789012:cluster/test-cluster"),
				ServiceName:    aws.String("test-service"),
				TaskDefinition: aws.String("arn:aws:ecs:ap-northeast-1:123456789012:task-definition/test-task:1"),
				Status:         aws.String("ACTIVE"),
				DesiredCount:   2,
				RunningCount:   2,
				PendingCount:   0,
			},
		},
	}

	mockListTasksOutput = &ecs.ListTasksOutput{
		TaskArns: []string{
			"arn:aws:ecs:ap-northeast-1:123456789012:task/test-cluster/abcdef1234567890",
		},
	}

	mockTaskStartTime = time.Now()

	mockDescribeTasksOutput = &ecs.DescribeTasksOutput{
		Tasks: []types.Task{
			{
				TaskDefinitionArn: aws.String("arn:aws:ecs:ap-northeast-1:123456789012:task-definition/test-task:1"),
				TaskArn:           aws.String("arn:aws:ecs:ap-northeast-1:123456789012:task/test-cluster/abcdef1234567890"),
				LastStatus:        aws.String("RUNNING"),
				DesiredStatus:     aws.String("RUNNING"),
				LaunchType:        types.LaunchTypeFargate,
				PlatformFamily:    aws.String("Linux"),
				PlatformVersion:   aws.String("1.4.0"),
				StartedAt:         &mockTaskStartTime,
				Containers: []types.Container{
					{
						Name:         aws.String("app"),
						HealthStatus: types.HealthStatusHealthy,
					},
				},
			},
		},
	}

	mockExecuteCommandOutput = &ecs.ExecuteCommandOutput{
		ContainerName: aws.String("app"),
		Interactive:   true,
		TaskArn:       aws.String("arn:aws:ecs:ap-northeast-1:123456789012:task/test-cluster/abcdef1234567890"),
	}
)

type mockEcsApi struct {
	listClustersOutput     *ecs.ListClustersOutput
	listClustersErr        error
	describeClustersOutput *ecs.DescribeClustersOutput
	describeClustersErr    error
	listServicesOutput     *ecs.ListServicesOutput
	listServicesErr        error
	describeServicesOutput *ecs.DescribeServicesOutput
	describeServicesErr    error
	listTasksOutput        *ecs.ListTasksOutput
	listTasksErr           error
	describeTasksOutput    *ecs.DescribeTasksOutput
	describeTasksErr       error
	executeCommandOutput   *ecs.ExecuteCommandOutput
	executeCommandErr      error
}

func (m *mockEcsApi) ListClusters(ctx context.Context, input *ecs.ListClustersInput, opts ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	return m.listClustersOutput, m.listClustersErr
}

func (m *mockEcsApi) DescribeClusters(ctx context.Context, input *ecs.DescribeClustersInput, opts ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
	return m.describeClustersOutput, m.describeClustersErr
}

func (m *mockEcsApi) ListServices(ctx context.Context, input *ecs.ListServicesInput, opts ...func(*ecs.Options)) (*ecs.ListServicesOutput, error) {
	return m.listServicesOutput, m.listServicesErr
}

func (m *mockEcsApi) DescribeServices(ctx context.Context, input *ecs.DescribeServicesInput, opts ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	return m.describeServicesOutput, m.describeServicesErr
}

func (m *mockEcsApi) ListTasks(ctx context.Context, input *ecs.ListTasksInput, opts ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	return m.listTasksOutput, m.listTasksErr
}

func (m *mockEcsApi) DescribeTasks(ctx context.Context, input *ecs.DescribeTasksInput, opts ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	return m.describeTasksOutput, m.describeTasksErr
}

func (m *mockEcsApi) ExecuteCommand(ctx context.Context, input *ecs.ExecuteCommandInput, opts ...func(*ecs.Options)) (*ecs.ExecuteCommandOutput, error) {
	return m.executeCommandOutput, m.executeCommandErr
}

func TestGenerateListClustersInput(t *testing.T) {
	opts := &EcsOpts{}
	input := GenerateListClustersInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
}

func TestListClusters(t *testing.T) {
	mockApi := &mockEcsApi{
		listClustersOutput: mockListClustersOutput,
		listClustersErr:    nil,
	}

	input := &ecs.ListClustersInput{}
	result, err := ListClusters(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != len(mockListClustersOutput.ClusterArns) {
		t.Fatalf("expected %d cluster ARNs, got %d", len(mockListClustersOutput.ClusterArns), len(result))
	}
	for i, arn := range mockListClustersOutput.ClusterArns {
		if result[i] != arn {
			t.Errorf("expected cluster ARN %q, got %q", arn, result[i])
		}
	}
}

func TestListClusters_Error(t *testing.T) {
	mockApi := &mockEcsApi{
		listClustersOutput: mockListClustersOutput,
		listClustersErr:    errors.New("error"),
	}

	input := &ecs.ListClustersInput{}
	result, err := ListClusters(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestGenerateDescribeClustersInput(t *testing.T) {
	clusters := []string{"test-cluster"}
	opts := &EcsOpts{Clusters: clusters}
	input := GenerateDescribeClustersInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if len(input.Clusters) != 1 || input.Clusters[0] != clusters[0] {
		t.Errorf("expected clusters %v, got %v", clusters, input.Clusters)
	}
}

func TestDescribeClusters(t *testing.T) {
	mockApi := &mockEcsApi{
		describeClustersOutput: mockDescribeClustersOutput,
		describeClustersErr:    nil,
	}

	input := &ecs.DescribeClustersInput{}
	result, err := DescribeClusters(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[0].Name != "test-cluster" {
		t.Errorf("expected Name 'test-cluster', got '%s'", result[0].Name)
	}
	if result[0].Status != "ACTIVE" {
		t.Errorf("expected Status 'ACTIVE', got '%s'", result[0].Status)
	}
	if result[0].ActiveServicesCount != 2 {
		t.Errorf("expected ActiveServicesCount 2, got %d", result[0].ActiveServicesCount)
	}
	if result[0].RunningTasksCount != 5 {
		t.Errorf("expected RunningTasksCount 5, got %d", result[0].RunningTasksCount)
	}
	if result[0].PendingTasksCount != 0 {
		t.Errorf("expected PendingTasksCount 0, got %d", result[0].PendingTasksCount)
	}
	if result[0].RegisteredContainerInstances != 3 {
		t.Errorf("expected RegisteredContainerInstances 3, got %d", result[0].RegisteredContainerInstances)
	}
}

func TestDescribeClusters_Error(t *testing.T) {
	mockApi := &mockEcsApi{
		describeClustersOutput: mockDescribeClustersOutput,
		describeClustersErr:    errors.New("error"),
	}

	input := &ecs.DescribeClustersInput{}
	result, err := DescribeClusters(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestGenerateListServicesInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	opts := &EcsOpts{Cluster: cluster}
	input := GenerateListServicesInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if input.Cluster != cluster {
		t.Errorf("expected Cluster %v, got %v", cluster, input.Cluster)
	}
}

func TestListServices(t *testing.T) {
	mockApi := &mockEcsApi{
		listServicesOutput: mockListServicesOutput,
		listServicesErr:    nil,
	}

	input := &ecs.ListServicesInput{}
	result, err := ListServices(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != len(mockListServicesOutput.ServiceArns) {
		t.Fatalf("expected %d service ARNs, got %d", len(mockListServicesOutput.ServiceArns), len(result))
	}
	for i, arn := range mockListServicesOutput.ServiceArns {
		if result[i] != arn {
			t.Errorf("expected service ARN %q, got %q", arn, result[i])
		}
	}
}

func TestListServices_Error(t *testing.T) {
	mockApi := &mockEcsApi{
		listServicesOutput: mockListServicesOutput,
		listServicesErr:    errors.New("error"),
	}

	input := &ecs.ListServicesInput{}
	result, err := ListServices(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestGenerateDescribeServicesInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	services := []string{"test-service"}
	opts := &EcsOpts{Cluster: cluster, Services: services}
	input := GenerateDescribeServicesInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if input.Cluster != cluster {
		t.Errorf("expected Cluster %v, got %v", cluster, input.Cluster)
	}
	if len(input.Services) != 1 || input.Services[0] != services[0] {
		t.Errorf("expected Services %v, got %v", services, input.Services)
	}
}

func TestDescribeServices(t *testing.T) {
	mockApi := &mockEcsApi{
		describeServicesOutput: mockDescribeServicesOutput,
		describeServicesErr:    nil,
	}

	input := &ecs.DescribeServicesInput{}
	result, err := DescribeServices(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[0].ClusterName != "test-cluster" {
		t.Errorf("expected ClusterName 'test-cluster', got '%s'", result[0].ClusterName)
	}
	if result[0].ServiceName != "test-service" {
		t.Errorf("expected ServiceName 'test-service', got '%s'", result[0].ServiceName)
	}
	if result[0].TaskDefinition != "test-task:1" {
		t.Errorf("expected TaskDefinition 'test-task:1', got '%s'", result[0].TaskDefinition)
	}
	if result[0].Status != "ACTIVE" {
		t.Errorf("expected Status 'ACTIVE', got '%s'", result[0].Status)
	}
	if result[0].DesiredCount != 2 {
		t.Errorf("expected DesiredCount 2, got %d", result[0].DesiredCount)
	}
	if result[0].RunningCount != 2 {
		t.Errorf("expected RunningCount 2, got %d", result[0].RunningCount)
	}
	if result[0].PendingCount != 0 {
		t.Errorf("expected PendingCount 0, got %d", result[0].PendingCount)
	}
}

func TestDescribeServices_Error(t *testing.T) {
	mockApi := &mockEcsApi{
		describeServicesOutput: mockDescribeServicesOutput,
		describeServicesErr:    errors.New("error"),
	}

	input := &ecs.DescribeServicesInput{}
	result, err := DescribeServices(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestGenerateListTasksInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	service := aws.String("test-service")
	opts := &EcsOpts{Cluster: cluster, Service: service}
	input := GenerateListTasksInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if input.Cluster != cluster {
		t.Errorf("expected Cluster %v, got %v", cluster, input.Cluster)
	}
	if input.ServiceName != service {
		t.Errorf("expected ServiceName %v, got %v", service, input.ServiceName)
	}
	if input.DesiredStatus != "" {
		t.Errorf("expected empty DesiredStatus, got %v", input.DesiredStatus)
	}
}

func TestGenerateListTasksInput_Running(t *testing.T) {
	cluster := aws.String("test-cluster")
	opts := &EcsOpts{Cluster: cluster, Status: "RUNNING"}
	input := GenerateListTasksInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if input.DesiredStatus != types.DesiredStatusRunning {
		t.Errorf("expected DesiredStatus RUNNING, got %v", input.DesiredStatus)
	}
}

func TestGenerateListTasksInput_Stopped(t *testing.T) {
	cluster := aws.String("test-cluster")
	opts := &EcsOpts{Cluster: cluster, Status: "STOPPED"}
	input := GenerateListTasksInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if input.DesiredStatus != types.DesiredStatusStopped {
		t.Errorf("expected DesiredStatus STOPPED, got %v", input.DesiredStatus)
	}
}

func TestListTasks(t *testing.T) {
	mockApi := &mockEcsApi{
		listTasksOutput: mockListTasksOutput,
		listTasksErr:    nil,
	}

	input := &ecs.ListTasksInput{}
	result, err := ListTasks(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != len(mockListTasksOutput.TaskArns) {
		t.Fatalf("expected %d task ARNs, got %d", len(mockListTasksOutput.TaskArns), len(result))
	}
	for i, arn := range mockListTasksOutput.TaskArns {
		if result[i] != arn {
			t.Errorf("expected task ARN %q, got %q", arn, result[i])
		}
	}
}

func TestListTasks_Error(t *testing.T) {
	mockApi := &mockEcsApi{
		listTasksOutput: mockListTasksOutput,
		listTasksErr:    errors.New("error"),
	}

	input := &ecs.ListTasksInput{}
	result, err := ListTasks(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestGenerateDescribeTasksInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	tasks := []string{"task-id"}
	opts := &EcsOpts{Cluster: cluster, Tasks: tasks}
	input := GenerateDescribeTasksInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if input.Cluster != cluster {
		t.Errorf("expected Cluster %v, got %v", cluster, input.Cluster)
	}
	if len(input.Tasks) != 1 || input.Tasks[0] != tasks[0] {
		t.Errorf("expected Tasks %v, got %v", tasks, input.Tasks)
	}
}

func TestDescribeTasks(t *testing.T) {
	mockApi := &mockEcsApi{
		describeTasksOutput: mockDescribeTasksOutput,
		describeTasksErr:    nil,
	}

	input := &ecs.DescribeTasksInput{}
	result, err := DescribeTasks(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[0].TaskDefinition != "test-task:1" {
		t.Errorf("expected TaskDefinition 'test-task:1', got '%s'", result[0].TaskDefinition)
	}
	if result[0].TaskID != "abcdef1234567890" {
		t.Errorf("expected TaskID 'abcdef1234567890', got '%s'", result[0].TaskID)
	}
	if result[0].ContainerName != "app" {
		t.Errorf("expected ContainerName 'app', got '%s'", result[0].ContainerName)
	}
	if result[0].LastStatus != "RUNNING" {
		t.Errorf("expected LastStatus 'RUNNING', got '%s'", result[0].LastStatus)
	}
	if result[0].DesiredStatus != "RUNNING" {
		t.Errorf("expected DesiredStatus 'RUNNING', got '%s'", result[0].DesiredStatus)
	}
	if result[0].HealthStatus != "HEALTHY" {
		t.Errorf("expected HealthStatus 'HEALTHY', got '%s'", result[0].HealthStatus)
	}
	if result[0].LaunchType != "FARGATE" {
		t.Errorf("expected LaunchType 'FARGATE', got '%s'", result[0].LaunchType)
	}
	if result[0].PlatformFamily != "Linux" {
		t.Errorf("expected PlatformFamily 'Linux', got '%s'", result[0].PlatformFamily)
	}
	if result[0].PlatformVersion != "1.4.0" {
		t.Errorf("expected PlatformVersion '1.4.0', got '%s'", result[0].PlatformVersion)
	}
	if result[0].StartedAt == "" {
		t.Error("expected non-empty StartedAt")
	}
}

func TestDescribeTasks_Error(t *testing.T) {
	mockApi := &mockEcsApi{
		describeTasksOutput: mockDescribeTasksOutput,
		describeTasksErr:    errors.New("error"),
	}

	input := &ecs.DescribeTasksInput{}
	result, err := DescribeTasks(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestGenerateExecuteCommandInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	task := aws.String("task-id")
	container := aws.String("app")
	command := aws.String("ls -la")
	interactive := true
	opts := &EcsOpts{
		Cluster:     cluster,
		Task:        task,
		Container:   container,
		Command:     command,
		Interactive: interactive,
	}
	input := GenerateExecuteCommandInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if input.Cluster != cluster {
		t.Errorf("expected Cluster %v, got %v", cluster, input.Cluster)
	}
	if input.Task != task {
		t.Errorf("expected Task %v, got %v", task, input.Task)
	}
	if input.Container != container {
		t.Errorf("expected Container %v, got %v", container, input.Container)
	}
	if input.Command != command {
		t.Errorf("expected Command %v, got %v", command, input.Command)
	}
	if input.Interactive != interactive {
		t.Errorf("expected Interactive %v, got %v", interactive, input.Interactive)
	}
}

func TestExecuteCommand(t *testing.T) {
	mockApi := &mockEcsApi{
		executeCommandOutput: mockExecuteCommandOutput,
		executeCommandErr:    nil,
	}

	input := &ecs.ExecuteCommandInput{}
	result, err := ExecuteCommand(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result.ContainerName != "app" {
		t.Errorf("expected ContainerName 'app', got '%s'", *result.ContainerName)
	}
	if !result.Interactive {
		t.Error("expected Interactive to be true")
	}
}

func TestExecuteCommand_Error(t *testing.T) {
	mockApi := &mockEcsApi{
		executeCommandOutput: mockExecuteCommandOutput,
		executeCommandErr:    errors.New("error"),
	}

	input := &ecs.ExecuteCommandInput{}
	result, err := ExecuteCommand(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}
