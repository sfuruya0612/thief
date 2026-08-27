package aws

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
	ARN                  string   `json:"arn"`
	Group                string   `json:"group"`
	LastStatus           string   `json:"last_status"`
	DesiredStatus        string   `json:"desired_status"`
	LaunchType           string   `json:"launch_type"`
	EnableExecuteCommand bool     `json:"enable_execute_command"`
	ContainerNames       []string `json:"container_names"`
	CPU                  string   `json:"cpu"`
	Memory               string   `json:"memory"`
	StartedAt            string   `json:"started_at"`
	StoppedAt            string   `json:"stopped_at"`
	StoppedReason        string   `json:"stopped_reason"`
	// ContainerInstanceArn はタスクが載っているコンテナインスタンス (EC2) の ARN。
	// Fargate のタスクでは空文字列になる。
	ContainerInstanceArn string                   `json:"container_instance_arn"`
	Containers           []ECSTaskContainerDetail `json:"containers"`
}

// ecsDescribeContainerInstancesBatchSize は DescribeContainerInstances が 1 回に受け付ける
// コンテナインスタンス数の上限 (API 仕様)。
const ecsDescribeContainerInstancesBatchSize = 100

// ecsResourceTypeInteger は ecstypes.Resource.Type の値のうち IntegerValue が有効なもの。
const ecsResourceTypeInteger = "INTEGER"

// ecsResourceNameCPU と ecsResourceNameMemory は ecstypes.Resource.Name のうち
// コンテナインスタンスの CPU ユニットとメモリ (MiB) を表す値。
const (
	ecsResourceNameCPU    = "CPU"
	ecsResourceNameMemory = "MEMORY"
)

// ECSContainerInstanceResource はクラスタに登録されたコンテナインスタンス (EC2) 1 台の情報。
// RegisteredCPU / RegisteredMemory / RemainingCPU / RemainingMemory は該当する要素が
// 無いとき nil (JSON では null) になる。0 は「残り 0」を意味するため未取得と区別する。
type ECSContainerInstanceResource struct {
	ARN               string `json:"arn"`
	EC2InstanceID     string `json:"ec2_instance_id"`
	Status            string `json:"status"`
	AgentConnected    bool   `json:"agent_connected"`
	RunningTasksCount int32  `json:"running_tasks_count"`
	PendingTasksCount int32  `json:"pending_tasks_count"`
	RegisteredCPU     *int32 `json:"registered_cpu"`
	RegisteredMemory  *int32 `json:"registered_memory"`
	RemainingCPU      *int32 `json:"remaining_cpu"`
	RemainingMemory   *int32 `json:"remaining_memory"`
}

// ECSTaskContainerDetail はタスク詳細ペインに表示するコンテナ単位の情報。
// ListECSContainers が返す ECSContainerResource (Exec 対象選択用) とは異なり、
// タスク一覧取得時に DescribeTasks から一度に得られる情報のみを保持する。
type ECSTaskContainerDetail struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	LastStatus   string `json:"last_status"`
	HealthStatus string `json:"health_status"`
	ExitCode     *int32 `json:"exit_code,omitempty"`
	Reason       string `json:"reason"`
	RuntimeID    string `json:"runtime_id"`
	// CPU / Memory / MemoryReservation は DescribeTasks の Container が返す設定値
	// (タスク定義の指定値) をそのまま持つ。未指定の場合 SDK は CPU に "0"、Memory と
	// MemoryReservation に nil を返す。nil は空文字列にする。
	CPU               string `json:"cpu"`
	Memory            string `json:"memory"`
	MemoryReservation string `json:"memory_reservation"`
	// ExecEnabled は Task.EnableExecuteCommand とコンテナの RuntimeID 有無から判定する
	// (ListECSContainers の ExecEnabled と同じ判定)。RuntimeID が空の場合、
	// タスクがまだ Exec 可能な状態まで起動していない。
	ExecEnabled bool `json:"exec_enabled"`
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

	var resources []ECSServiceResource
	for i := 0; i < len(arns); i += ecsDescribeServicesBatchSize {
		end := min(i+ecsDescribeServicesBatchSize, len(arns))
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
	client, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listECSTasks(ctx, client, cluster, service)
}

// listECSTasks は生成済みクライアントでタスク一覧を取得するコア。
// ListTasksInput に載せる ServiceName を単体テストで固定できるよう、
// クライアントの生成と分離してある。
func listECSTasks(ctx context.Context, client ecsTaskListClient, cluster, service string) ([]ECSTaskResource, error) {
	// DesiredStatus を指定しないため ECS の既定で desiredStatus が RUNNING のタスクだけが返る。
	// コンテナインスタンス (EC2) ごとのタスク一覧を表示する Drawer のタブは、この既定に
	// 依存して「動いているタスク」を表示している。ステータスの指定を追加するとタブの前提が崩れる。
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
			resources = append(resources, ecsTaskFromSDK(t))
		}
	}
	return resources, nil
}

func ecsTaskFromSDK(t ecstypes.Task) ECSTaskResource {
	names := make([]string, 0, len(t.Containers))
	containers := make([]ECSTaskContainerDetail, 0, len(t.Containers))
	for _, c := range t.Containers {
		names = append(names, ptrStr(c.Name))
		runtimeID := ptrStr(c.RuntimeId)
		containers = append(containers, ECSTaskContainerDetail{
			Name:              ptrStr(c.Name),
			Image:             ptrStr(c.Image),
			LastStatus:        DisplayState(ptrStr(c.LastStatus)),
			HealthStatus:      DisplayState(string(c.HealthStatus)),
			ExitCode:          c.ExitCode,
			Reason:            ptrStr(c.Reason),
			RuntimeID:         runtimeID,
			CPU:               ptrStr(c.Cpu),
			Memory:            ptrStr(c.Memory),
			MemoryReservation: ptrStr(c.MemoryReservation),
			ExecEnabled:       t.EnableExecuteCommand && runtimeID != "",
		})
	}
	startedAt := ""
	if t.StartedAt != nil {
		startedAt = t.StartedAt.Format(time.RFC3339)
	}
	stoppedAt := ""
	if t.StoppedAt != nil {
		stoppedAt = t.StoppedAt.Format(time.RFC3339)
	}
	return ECSTaskResource{
		ARN:                  ptrStr(t.TaskArn),
		Group:                ptrStr(t.Group),
		LastStatus:           DisplayState(ptrStr(t.LastStatus)),
		DesiredStatus:        DisplayState(ptrStr(t.DesiredStatus)),
		LaunchType:           string(t.LaunchType),
		EnableExecuteCommand: t.EnableExecuteCommand,
		ContainerNames:       names,
		CPU:                  ptrStr(t.Cpu),
		Memory:               ptrStr(t.Memory),
		StartedAt:            startedAt,
		StoppedAt:            stoppedAt,
		StoppedReason:        ptrStr(t.StoppedReason),
		ContainerInstanceArn: ptrStr(t.ContainerInstanceArn),
		Containers:           containers,
	}
}

// ListECSContainerInstances はクラスタに登録された全コンテナインスタンスを返す。
// ListContainerInstances に Status フィルタを渡さないため、既定 (INACTIVE 以外) の
// ACTIVE / DRAINING / REGISTERING / REGISTRATION_FAILED / DEREGISTERING がすべて含まれる。
func ListECSContainerInstances(ctx context.Context, profile, region, cluster string) ([]ECSContainerInstanceResource, error) {
	client, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listECSContainerInstances(ctx, client, cluster)
}

func listECSContainerInstances(ctx context.Context, client ecsContainerInstanceListClient, cluster string) ([]ECSContainerInstanceResource, error) {
	var arns []string
	paginator := ecs.NewListContainerInstancesPaginator(client, &ecs.ListContainerInstancesInput{Cluster: aws.String(cluster)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs container instances: %w", err)
		}
		arns = append(arns, page.ContainerInstanceArns...)
	}
	resources := make([]ECSContainerInstanceResource, 0, len(arns))
	// DescribeContainerInstancesInput.ContainerInstances は必須のため 0 件では呼ばない
	for i := 0; i < len(arns); i += ecsDescribeContainerInstancesBatchSize {
		end := min(i+ecsDescribeContainerInstancesBatchSize, len(arns))
		out, err := client.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
			Cluster:            aws.String(cluster),
			ContainerInstances: arns[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs container instances: %w", err)
		}
		// List と Describe の間に登録解除された等の個別の失敗は全体を失敗にせず、記録して除く
		for _, f := range out.Failures {
			slog.Warn("describe ecs container instance failed", "cluster", cluster, "arn", ptrStr(f.Arn), "reason", ptrStr(f.Reason))
		}
		for _, ci := range out.ContainerInstances {
			resources = append(resources, ecsContainerInstanceFromSDK(ci))
		}
	}
	return resources, nil
}

func ecsContainerInstanceFromSDK(ci ecstypes.ContainerInstance) ECSContainerInstanceResource {
	return ECSContainerInstanceResource{
		ARN:               ptrStr(ci.ContainerInstanceArn),
		EC2InstanceID:     ptrStr(ci.Ec2InstanceId),
		Status:            DisplayState(ptrStr(ci.Status)),
		AgentConnected:    ci.AgentConnected,
		RunningTasksCount: ci.RunningTasksCount,
		PendingTasksCount: ci.PendingTasksCount,
		RegisteredCPU:     ecsIntegerResource(ci.RegisteredResources, ecsResourceNameCPU),
		RegisteredMemory:  ecsIntegerResource(ci.RegisteredResources, ecsResourceNameMemory),
		RemainingCPU:      ecsIntegerResource(ci.RemainingResources, ecsResourceNameCPU),
		RemainingMemory:   ecsIntegerResource(ci.RemainingResources, ecsResourceNameMemory),
	}
}

// ecsIntegerResource は Resource の一覧から Name が name で Type が INTEGER の要素の
// IntegerValue を返す。該当が無ければ nil。Resource は Type で有効な値フィールドが決まる
// 構造のため、Type を確認せずに IntegerValue を読むとゼロ値の 0 を「残り 0」と誤る。
func ecsIntegerResource(resources []ecstypes.Resource, name string) *int32 {
	for _, r := range resources {
		if ptrStr(r.Name) != name || ptrStr(r.Type) != ecsResourceTypeInteger {
			continue
		}
		v := r.IntegerValue
		return &v
	}
	return nil
}

// ListECSContainers returns all containers within the given ECS task.
func ListECSContainers(ctx context.Context, profile, region, cluster, task string) ([]ECSContainerResource, error) {
	client, err := newECSClient(ctx, profile, region)
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

	return &StartSessionResult{
		SessionID:  ptrStr(out.Session.SessionId),
		StreamURL:  ptrStr(out.Session.StreamUrl),
		TokenValue: ptrStr(out.Session.TokenValue),
	}, nil
}
