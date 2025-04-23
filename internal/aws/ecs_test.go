package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
				ActiveServicesCount:               *aws.Int32(2),
				RunningTasksCount:                 *aws.Int32(5),
				PendingTasksCount:                 *aws.Int32(0),
				RegisteredContainerInstancesCount: *aws.Int32(3),
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
				DesiredCount:   *aws.Int32(2),
				RunningCount:   *aws.Int32(2),
				PendingCount:   *aws.Int32(0),
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
	mock.Mock
}

func (m *mockEcsApi) ListClusters(ctx context.Context, input *ecs.ListClustersInput, opts ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ecs.ListClustersOutput), args.Error(1)
}

func (m *mockEcsApi) DescribeClusters(ctx context.Context, input *ecs.DescribeClustersInput, opts ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ecs.DescribeClustersOutput), args.Error(1)
}

func (m *mockEcsApi) ListServices(ctx context.Context, input *ecs.ListServicesInput, opts ...func(*ecs.Options)) (*ecs.ListServicesOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ecs.ListServicesOutput), args.Error(1)
}

func (m *mockEcsApi) DescribeServices(ctx context.Context, input *ecs.DescribeServicesInput, opts ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ecs.DescribeServicesOutput), args.Error(1)
}

func (m *mockEcsApi) ListTasks(ctx context.Context, input *ecs.ListTasksInput, opts ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ecs.ListTasksOutput), args.Error(1)
}

func (m *mockEcsApi) DescribeTasks(ctx context.Context, input *ecs.DescribeTasksInput, opts ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ecs.DescribeTasksOutput), args.Error(1)
}

func (m *mockEcsApi) ExecuteCommand(ctx context.Context, input *ecs.ExecuteCommandInput, opts ...func(*ecs.Options)) (*ecs.ExecuteCommandOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ecs.ExecuteCommandOutput), args.Error(1)
}

func TestGenerateListClustersInput(t *testing.T) {
	opts := &EcsOpts{}
	input := GenerateListClustersInput(opts)
	assert.NotNil(t, input)
}

func TestListClusters(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ListClusters", mock.Anything, mock.Anything, mock.Anything).Return(mockListClustersOutput, nil)

	input := &ecs.ListClustersInput{}
	result, err := ListClusters(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mockListClustersOutput.ClusterArns, result)

	mockApi.AssertExpectations(t)
}

func TestListClusters_Error(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ListClusters", mock.Anything, mock.Anything, mock.Anything).Return(mockListClustersOutput, errors.New("error"))

	input := &ecs.ListClustersInput{}
	result, err := ListClusters(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateDescribeClustersInput(t *testing.T) {
	clusters := []string{"test-cluster"}
	opts := &EcsOpts{Clusters: clusters}
	input := GenerateDescribeClustersInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, clusters, input.Clusters)
}

func TestDescribeClusters(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("DescribeClusters", mock.Anything, mock.Anything, mock.Anything).Return(mockDescribeClustersOutput, nil)

	input := &ecs.DescribeClustersInput{}
	result, err := DescribeClusters(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-cluster", result[0][0])
	assert.Equal(t, "ACTIVE", result[0][1])
	assert.Equal(t, "2", result[0][2])
	assert.Equal(t, "5", result[0][3])
	assert.Equal(t, "0", result[0][4])
	assert.Equal(t, "3", result[0][5])

	mockApi.AssertExpectations(t)
}

func TestDescribeClusters_Error(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("DescribeClusters", mock.Anything, mock.Anything, mock.Anything).Return(mockDescribeClustersOutput, errors.New("error"))

	input := &ecs.DescribeClustersInput{}
	result, err := DescribeClusters(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateListServicesInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	opts := &EcsOpts{Cluster: cluster}
	input := GenerateListServicesInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, cluster, input.Cluster)
}

func TestListServices(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return(mockListServicesOutput, nil)

	input := &ecs.ListServicesInput{}
	result, err := ListServices(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mockListServicesOutput.ServiceArns, result)

	mockApi.AssertExpectations(t)
}

func TestListServices_Error(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return(mockListServicesOutput, errors.New("error"))

	input := &ecs.ListServicesInput{}
	result, err := ListServices(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateDescribeServicesInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	services := []string{"test-service"}
	opts := &EcsOpts{Cluster: cluster, Services: services}
	input := GenerateDescribeServicesInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, cluster, input.Cluster)
	assert.Equal(t, services, input.Services)
}

func TestDescribeServices(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("DescribeServices", mock.Anything, mock.Anything, mock.Anything).Return(mockDescribeServicesOutput, nil)

	input := &ecs.DescribeServicesInput{}
	result, err := DescribeServices(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-cluster", result[0][0])
	assert.Equal(t, "test-service", result[0][1])
	assert.Equal(t, "test-task:1", result[0][2])
	assert.Equal(t, "ACTIVE", result[0][3])
	assert.Equal(t, "2", result[0][4])
	assert.Equal(t, "2", result[0][5])
	assert.Equal(t, "0", result[0][6])

	mockApi.AssertExpectations(t)
}

func TestDescribeServices_Error(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("DescribeServices", mock.Anything, mock.Anything, mock.Anything).Return(mockDescribeServicesOutput, errors.New("error"))

	input := &ecs.DescribeServicesInput{}
	result, err := DescribeServices(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateListTasksInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	service := aws.String("test-service")
	opts := &EcsOpts{Cluster: cluster, Service: service}
	input := GenerateListTasksInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, cluster, input.Cluster)
	assert.Equal(t, service, input.ServiceName)
}

func TestListTasks(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ListTasks", mock.Anything, mock.Anything, mock.Anything).Return(mockListTasksOutput, nil)

	input := &ecs.ListTasksInput{}
	result, err := ListTasks(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mockListTasksOutput.TaskArns, result)

	mockApi.AssertExpectations(t)
}

func TestListTasks_Error(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ListTasks", mock.Anything, mock.Anything, mock.Anything).Return(mockListTasksOutput, errors.New("error"))

	input := &ecs.ListTasksInput{}
	result, err := ListTasks(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateDescribeTasksInput(t *testing.T) {
	cluster := aws.String("test-cluster")
	tasks := []string{"task-id"}
	opts := &EcsOpts{Cluster: cluster, Tasks: tasks}
	input := GenerateDescribeTasksInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, cluster, input.Cluster)
	assert.Equal(t, tasks, input.Tasks)
}

func TestDescribeTasks(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("DescribeTasks", mock.Anything, mock.Anything, mock.Anything).Return(mockDescribeTasksOutput, nil)

	input := &ecs.DescribeTasksInput{}
	result, err := DescribeTasks(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-task:1", result[0][0])
	assert.Equal(t, "abcdef1234567890", result[0][1])
	assert.Equal(t, "app", result[0][2])
	assert.Equal(t, "RUNNING", result[0][3])
	assert.Equal(t, "RUNNING", result[0][4])
	assert.Equal(t, "HEALTHY", result[0][5])
	assert.Equal(t, "FARGATE", result[0][6])
	assert.Equal(t, "Linux", result[0][7])
	assert.Equal(t, "1.4.0", result[0][8])
	assert.NotEmpty(t, result[0][9])

	mockApi.AssertExpectations(t)
}

func TestDescribeTasks_Error(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("DescribeTasks", mock.Anything, mock.Anything, mock.Anything).Return(mockDescribeTasksOutput, errors.New("error"))

	input := &ecs.DescribeTasksInput{}
	result, err := DescribeTasks(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
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
	assert.NotNil(t, input)
	assert.Equal(t, cluster, input.Cluster)
	assert.Equal(t, task, input.Task)
	assert.Equal(t, container, input.Container)
	assert.Equal(t, command, input.Command)
	assert.Equal(t, interactive, input.Interactive)
}

func TestExecuteCommand(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything).Return(mockExecuteCommandOutput, nil)

	input := &ecs.ExecuteCommandInput{}
	result, err := ExecuteCommand(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "app", *result.ContainerName)
	assert.True(t, result.Interactive)

	mockApi.AssertExpectations(t)
}

func TestExecuteCommand_Error(t *testing.T) {
	mockApi := new(mockEcsApi)
	mockApi.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything).Return(mockExecuteCommandOutput, errors.New("error"))

	input := &ecs.ExecuteCommandInput{}
	result, err := ExecuteCommand(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}
