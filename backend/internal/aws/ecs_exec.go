package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ECSServiceResource represents a single ECS service.
type ECSServiceResource struct {
	ARN            string `json:"arn"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	DesiredCount   int32  `json:"desired_count"`
	RunningCount   int32  `json:"running_count"`
	PendingCount   int32  `json:"pending_count"`
	TaskDefinition string `json:"task_definition"`
	LaunchType     string `json:"launch_type"`
}

// ECSTaskResource represents a single ECS task.
type ECSTaskResource struct {
	ARN                  string `json:"arn"`
	Group                string `json:"group"`
	LastStatus           string `json:"last_status"`
	DesiredStatus        string `json:"desired_status"`
	LaunchType           string `json:"launch_type"`
	EnableExecuteCommand bool   `json:"enable_execute_command"`
}

// ECSContainerResource represents a single container within an ECS task.
type ECSContainerResource struct {
	Name       string `json:"name"`
	RuntimeID  string `json:"runtime_id"`
	LastStatus string `json:"last_status"`
	// ExecEnabled は Task.EnableExecuteCommand とコンテナの RuntimeID 有無から判定する。
	// RuntimeID が空の場合、タスクがまだ Exec 可能な状態まで起動していない。
	ExecEnabled bool `json:"exec_enabled"`
}

// ListECSServices returns all services in the given ECS cluster.
func ListECSServices(ctx context.Context, profile, region, cluster string) ([]ECSServiceResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ecs.Client {
		return ecs.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var arns []string
	paginator := ecs.NewListServicesPaginator(client, &ecs.ListServicesInput{Cluster: aws.String(cluster)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs services: %w", err)
		}
		arns = append(arns, page.ServiceArns...)
	}
	if len(arns) == 0 {
		return nil, nil
	}

	var resources []ECSServiceResource
	for i := 0; i < len(arns); i += 10 {
		end := i + 10
		if end > len(arns) {
			end = len(arns)
		}
		out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(cluster),
			Services: arns[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs services: %w", err)
		}
		for _, svc := range out.Services {
			resources = append(resources, ecsServiceFromSDK(svc))
		}
	}
	return resources, nil
}

func ecsServiceFromSDK(s ecstypes.Service) ECSServiceResource {
	return ECSServiceResource{
		ARN:            ptrStr(s.ServiceArn),
		Name:           ptrStr(s.ServiceName),
		Status:         DisplayState(ptrStr(s.Status)),
		DesiredCount:   s.DesiredCount,
		RunningCount:   s.RunningCount,
		PendingCount:   s.PendingCount,
		TaskDefinition: ptrStr(s.TaskDefinition),
		LaunchType:     string(s.LaunchType),
	}
}

// ListECSTasks returns all tasks in the given ECS cluster, optionally filtered by service.
func ListECSTasks(ctx context.Context, profile, region, cluster, service string) ([]ECSTaskResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ecs.Client {
		return ecs.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	input := &ecs.ListTasksInput{Cluster: aws.String(cluster)}
	if service != "" {
		input.ServiceName = aws.String(service)
	}

	var arns []string
	paginator := ecs.NewListTasksPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs tasks: %w", err)
		}
		arns = append(arns, page.TaskArns...)
	}
	if len(arns) == 0 {
		return nil, nil
	}

	var resources []ECSTaskResource
	for i := 0; i < len(arns); i += 100 {
		end := i + 100
		if end > len(arns) {
			end = len(arns)
		}
		out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Tasks:   arns[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs tasks: %w", err)
		}
		for _, t := range out.Tasks {
			resources = append(resources, ecsTaskFromSDK(t))
		}
	}
	return resources, nil
}

func ecsTaskFromSDK(t ecstypes.Task) ECSTaskResource {
	return ECSTaskResource{
		ARN:                  ptrStr(t.TaskArn),
		Group:                ptrStr(t.Group),
		LastStatus:           DisplayState(ptrStr(t.LastStatus)),
		DesiredStatus:        DisplayState(ptrStr(t.DesiredStatus)),
		LaunchType:           string(t.LaunchType),
		EnableExecuteCommand: t.EnableExecuteCommand,
	}
}

// ListECSContainers returns all containers within the given ECS task.
func ListECSContainers(ctx context.Context, profile, region, cluster, task string) ([]ECSContainerResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ecs.Client {
		return ecs.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster),
		Tasks:   []string{task},
	})
	if err != nil {
		return nil, fmt.Errorf("describe ecs task %s: %w", task, err)
	}
	if len(out.Tasks) == 0 {
		return nil, nil
	}

	t := out.Tasks[0]
	resources := make([]ECSContainerResource, 0, len(t.Containers))
	for _, c := range t.Containers {
		runtimeID := ptrStr(c.RuntimeId)
		resources = append(resources, ECSContainerResource{
			Name:        ptrStr(c.Name),
			RuntimeID:   runtimeID,
			LastStatus:  DisplayState(ptrStr(c.LastStatus)),
			ExecEnabled: t.EnableExecuteCommand && runtimeID != "",
		})
	}
	return resources, nil
}

// ExecuteECSCommand runs command interactively on the given container within the given task
// and returns the data channel connection info for the resulting SSM session.
func ExecuteECSCommand(ctx context.Context, profile, region, cluster, task, container, command string) (*StartSessionResult, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ecs.Client {
		return ecs.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.ExecuteCommand(ctx, &ecs.ExecuteCommandInput{
		Cluster:     aws.String(cluster),
		Task:        aws.String(task),
		Container:   aws.String(container),
		Command:     aws.String(command),
		Interactive: true,
	})
	if err != nil {
		return nil, fmt.Errorf("execute command on task %s container %s: %w", task, container, err)
	}
	if out.Session == nil {
		return nil, fmt.Errorf("execute command on task %s container %s: no session returned", task, container)
	}

	return &StartSessionResult{
		SessionID:  ptrStr(out.Session.SessionId),
		StreamURL:  ptrStr(out.Session.StreamUrl),
		TokenValue: ptrStr(out.Session.TokenValue),
	}, nil
}
