package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ecsDescribeClustersBatchSize / ecsDescribeServicesBatchSize / ecsDescribeTasksBatchSize は
// 各 Describe API が 1 回で受け付ける最大件数。
const (
	ecsDescribeClustersBatchSize = 100
	ecsDescribeServicesBatchSize = 10
	ecsDescribeTasksBatchSize    = 100
)

// ECSClusterInfo はレガシー CLI 互換の ECS クラスタ表示用フィールドを保持する。
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

// ECSServiceInfo はレガシー CLI 互換の ECS サービス表示用フィールドを保持する。
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

// ECSTaskInfo はレガシー CLI 互換の ECS タスク表示用フィールド (コンテナ単位) を保持する。
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

// arnPart は ARN を "/" 区切りで分割した idx 番目の要素を返す。
// 想定外のフォーマットで要素が足りない場合は ARN 全体を返す (panic を避ける)。
func arnPart(arn string, idx int) string {
	parts := strings.Split(arn, "/")
	if idx < 0 || idx >= len(parts) {
		return arn
	}
	return parts[idx]
}

// newECSClient は ECS API クライアントを生成する。
func newECSClient(ctx context.Context, profile, region string) (*ecs.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *ecs.Client {
		return ecs.NewFromConfig(cfg)
	})
}

// ListECSClusterArns は ECS クラスタの ARN 一覧を返す。
func ListECSClusterArns(ctx context.Context, profile, region string) ([]string, error) {
	client, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var arns []string
	paginator := ecs.NewListClustersPaginator(client, &ecs.ListClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs clusters: %w", err)
		}
		arns = append(arns, page.ClusterArns...)
	}
	return arns, nil
}

// GetECSClusterInfos は指定クラスタ群の詳細をレガシー CLI 互換フィールドで返す。
func GetECSClusterInfos(ctx context.Context, profile, region string, arns []string) ([]ECSClusterInfo, error) {
	client, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var clusters []ECSClusterInfo
	for i := 0; i < len(arns); i += ecsDescribeClustersBatchSize {
		end := min(i+ecsDescribeClustersBatchSize, len(arns))
		out, err := client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: arns[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs clusters: %w", err)
		}
		for _, c := range out.Clusters {
			clusters = append(clusters, ECSClusterInfo{
				Name:                         ptrStr(c.ClusterName),
				Status:                       ptrStr(c.Status),
				ActiveServicesCount:          c.ActiveServicesCount,
				RunningTasksCount:            c.RunningTasksCount,
				PendingTasksCount:            c.PendingTasksCount,
				RegisteredContainerInstances: c.RegisteredContainerInstancesCount,
			})
		}
	}
	return clusters, nil
}

// GetECSServiceInfos は指定クラスタ内のサービス一覧をレガシー CLI 互換フィールドで返す。
func GetECSServiceInfos(ctx context.Context, profile, region, cluster string) ([]ECSServiceInfo, error) {
	client, err := newECSClient(ctx, profile, region)
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

	var services []ECSServiceInfo
	for i := 0; i < len(arns); i += ecsDescribeServicesBatchSize {
		end := min(i+ecsDescribeServicesBatchSize, len(arns))
		out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(cluster),
			Services: arns[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs services: %w", err)
		}
		for _, s := range out.Services {
			services = append(services, ECSServiceInfo{
				ClusterName:    arnPart(ptrStr(s.ClusterArn), 1),
				ServiceName:    ptrStr(s.ServiceName),
				TaskDefinition: arnPart(ptrStr(s.TaskDefinition), 1),
				Status:         ptrStr(s.Status),
				DesiredCount:   s.DesiredCount,
				RunningCount:   s.RunningCount,
				PendingCount:   s.PendingCount,
			})
		}
	}
	return services, nil
}

// ListECSTaskInfos は指定クラスタ内のタスクをコンテナ単位の行に展開して返す。
// desiredStatus が非空のとき (例: "RUNNING") はタスクの希望ステータスで絞り込む。
func ListECSTaskInfos(ctx context.Context, profile, region, cluster, desiredStatus string) ([]ECSTaskInfo, error) {
	client, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	input := &ecs.ListTasksInput{Cluster: aws.String(cluster)}
	if desiredStatus != "" {
		input.DesiredStatus = ecstypes.DesiredStatus(desiredStatus)
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

	var tasks []ECSTaskInfo
	for i := 0; i < len(arns); i += ecsDescribeTasksBatchSize {
		end := min(i+ecsDescribeTasksBatchSize, len(arns))
		out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Tasks:   arns[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs tasks: %w", err)
		}
		for _, t := range out.Tasks {
			tasks = append(tasks, ecsTaskInfosFromSDK(t)...)
		}
	}
	return tasks, nil
}

func ecsTaskInfosFromSDK(t ecstypes.Task) []ECSTaskInfo {
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

	infos := make([]ECSTaskInfo, 0, len(t.Containers))
	for _, c := range t.Containers {
		infos = append(infos, ECSTaskInfo{
			TaskDefinition:  arnPart(ptrStr(t.TaskDefinitionArn), 1),
			TaskID:          arnPart(ptrStr(t.TaskArn), 2),
			ContainerName:   ptrStr(c.Name),
			LastStatus:      ptrStr(t.LastStatus),
			DesiredStatus:   ptrStr(t.DesiredStatus),
			HealthStatus:    string(c.HealthStatus),
			LaunchType:      string(t.LaunchType),
			PlatformFamily:  platformFamily,
			PlatformVersion: platformVersion,
			StartedAt:       startedAt,
		})
	}
	return infos
}

// ECSExecSession は ECS Exec で開始した SSM セッション情報と、
// session-manager-plugin のターゲット文字列生成に必要な ARN 群を保持する。
type ECSExecSession struct {
	SessionID    string
	StreamURL    string
	TokenValue   string
	ClusterArn   string
	TaskArn      string
	ContainerArn string
}

// Target は session-manager-plugin に渡す ECS ターゲット文字列
// (ecs:<cluster>_<task-id>_<container-runtime-id>) を返す。
func (s ECSExecSession) Target() string {
	return fmt.Sprintf("ecs:%s_%s_%s",
		arnPart(s.ClusterArn, 1),
		arnPart(s.TaskArn, 2),
		arnPart(s.ContainerArn, 3),
	)
}

// ExecuteECSCommandSession は ECS Exec を開始し、セッション情報と関連 ARN を返す。
// WebSocket ブリッジ用の ExecuteECSCommand と異なり、CLI (session-manager-plugin) 向け。
func ExecuteECSCommandSession(ctx context.Context, profile, region, cluster, task, container, command string) (*ECSExecSession, error) {
	client, err := newECSClient(ctx, profile, region)
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

	return &ECSExecSession{
		SessionID:    ptrStr(out.Session.SessionId),
		StreamURL:    ptrStr(out.Session.StreamUrl),
		TokenValue:   ptrStr(out.Session.TokenValue),
		ClusterArn:   ptrStr(out.ClusterArn),
		TaskArn:      ptrStr(out.TaskArn),
		ContainerArn: ptrStr(out.ContainerArn),
	}, nil
}
