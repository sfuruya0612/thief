package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// ecsServiceID は ECS の Service.ID()。Model はこの値で ECS Exec の経路 (startECSExec) に分岐する。
const ecsServiceID = "ecs"

// ecsExecDefaultCommand は ECS Exec で実行するコマンド。CLI の thief ecs exec の --command の既定値と
// Web UI の ECSExecLauncher と同じ。
const ecsExecDefaultCommand = "/bin/sh"

// ecsService は ECS クラスタの一覧を提供する Service。
type ecsService struct {
	list      func(ctx context.Context, profile, region string) ([]aws.ECSResource, error)
	services  func(ctx context.Context, profile, region, cluster string) ([]aws.ECSServiceResource, error)
	tasks     func(ctx context.Context, profile, region, cluster, service string) ([]aws.ECSTaskResource, error)
	instances func(ctx context.Context, profile, region, cluster string) ([]aws.ECSContainerInstanceResource, error)
}

// NewECSService は ECS の Service を返す。list には aws.ListECSResources を渡す。
// services を省略すると Services タブを表示しない (既存のテスト用)。
func NewECSService(list func(ctx context.Context, profile, region string) ([]aws.ECSResource, error), services ...func(ctx context.Context, profile, region, cluster string) ([]aws.ECSServiceResource, error)) Service {
	s := &ecsService{list: list}
	if len(services) > 0 {
		s.services = services[0]
	}
	return s
}

// NewECSInteractiveService は Services、Tasks、Instances のタブを持つ ECS の Service を返す。
func NewECSInteractiveService(
	list func(ctx context.Context, profile, region string) ([]aws.ECSResource, error),
	services func(ctx context.Context, profile, region, cluster string) ([]aws.ECSServiceResource, error),
	tasks func(ctx context.Context, profile, region, cluster, service string) ([]aws.ECSTaskResource, error),
	instances func(ctx context.Context, profile, region, cluster string) ([]aws.ECSContainerInstanceResource, error),
) Service {
	return &ecsService{list: list, services: services, tasks: tasks, instances: instances}
}

func (s *ecsService) ID() string    { return ecsServiceID }
func (s *ecsService) Title() string { return "ECS" }

// Columns は Web UI の ecsColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// 件数は区切り無しの 10 進の文字列 (formatInt32) にし、Web UI に dash を描画するセルは無い。
func (s *ecsService) Columns() []Column {
	return []Column{
		{Title: "Cluster"},
		{Title: "State"},
		{Title: "Active svc"},
		{Title: "Running"},
		{Title: "Pending"},
		{Title: "Registered EC2"},
		{Title: "Region"},
	}
}

func (s *ecsService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list ECS clusters: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, ecsRow{res: it, region: region})
	}
	return rows, nil
}

// SupportsSession は true を返す。クラスタの行で s を押すと、Model がタスクとコンテナを選ばせて
// Deps.OpenECSExecSession で ECS Exec を始める (startECSExec)。
func (s *ecsService) SupportsSession() bool { return true }

// Tabs は Overview、Services、Terminal、Tags を返す。Tasks と Instances は後続の issue で足す。
func (s *ecsService) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.services != nil {
		tabs = append(tabs, contentTabSpec("services", "Services", func(ctx context.Context, profile, region string, row Row) (TabContent, error) {
			cluster := row.(ecsRow)
			services, err := s.services(ctx, profile, region, cluster.Name())
			if err != nil {
				return TabContent{}, fmt.Errorf("list ECS services: %w", err)
			}
			rows := make([][]string, 0, len(services))
			for _, service := range services {
				rows = append(rows, []string{
					service.Name,
					service.Status,
					formatInt32(service.DesiredCount),
					formatInt32(service.RunningCount),
					formatInt32(service.PendingCount),
					service.LaunchType,
					service.TaskDefinition,
				})
			}
			return TabContent{Blocks: []TabBlock{{
				Title: fmt.Sprintf("Services (%d)", len(services)),
				Table: &TabTable{
					Columns: []Column{{Title: "Service"}, {Title: "State"}, {Title: "Desired", Align: AlignRight}, {Title: "Running", Align: AlignRight}, {Title: "Pending", Align: AlignRight}, {Title: "Launch type"}, {Title: "Task definition"}},
					Rows:    rows,
				},
			}}}, nil
		}))
	}
	if s.tasks != nil {
		tabs = append(tabs, ecsTasksTabSpec(s))
	}
	if s.instances != nil && s.tasks != nil {
		tabs = append(tabs, ecsInstancesTabSpec(s))
	}
	tabs = append(tabs, TabSpec{ID: tabTerminal, Title: "Terminal"}, TabSpec{ID: tabTags, Title: "Tags"})
	return tabs
}

func ecsTasksTabSpec(s *ecsService) TabSpec {
	return TabSpec{
		ID:    "tasks",
		Title: "Tasks",
		New: func(ctx context.Context, profile, region string, row Row) Tab {
			cluster := row.(ecsRow).Name()
			return newListDetailTab(ctx,
				[]Column{{Title: "Group"}, {Title: "Containers"}, {Title: "Last status"}, {Title: "Desired status"}, {Title: "Launch type"}, {Title: "Exec enabled"}, {Title: "ARN"}},
				func(ctx context.Context) ([]listRow, error) {
					tasks, err := s.tasks(ctx, profile, region, cluster, "")
					if err != nil {
						return nil, fmt.Errorf("list ECS tasks: %w", err)
					}
					rows := make([]listRow, 0, len(tasks))
					for _, task := range tasks {
						names := make([]string, 0, len(task.Containers))
						for _, container := range task.Containers {
							names = append(names, container.Name)
						}
						exec := dash
						if task.EnableExecuteCommand {
							exec = check
						}
						rows = append(rows, listRow{
							cells: []string{task.Group, strings.Join(names, ", "), task.LastStatus, task.DesiredStatus, task.LaunchType, exec, task.ARN},
							value: task,
						})
					}
					return rows, nil
				},
				func(row listRow) *TabDetail {
					task := row.value.(aws.ECSTaskResource)
					kvs := []KV{
						{Key: "ARN", Value: task.ARN},
						{Key: "Group", Value: task.Group},
						{Key: "Last status", Value: task.LastStatus},
						{Key: "Desired status", Value: task.DesiredStatus},
						{Key: "Launch type", Value: task.LaunchType},
						{Key: "CPU", Value: orDash(task.CPU)},
						{Key: "Memory", Value: orDash(task.Memory)},
						{Key: "Started at", Value: orDash(task.StartedAt)},
						{Key: "Stopped at", Value: orDash(task.StoppedAt)},
						{Key: "Stopped reason", Value: orDash(task.StoppedReason)},
					}
					rows := make([][]string, 0, len(task.Containers))
					for _, container := range task.Containers {
						cpu := container.CPU
						if cpu == "0" || cpu == "" {
							cpu = dash
						}
						memory := formatContainerMemory(container.Memory, container.MemoryReservation)
						exit := dash
						if container.ExitCode != nil {
							exit = fmt.Sprintf("%d", *container.ExitCode)
						}
						exec := dash
						if container.ExecEnabled {
							exec = check
						}
						rows = append(rows, []string{container.Name, container.Image, container.LastStatus, orDash(container.HealthStatus), cpu, memory, exit, orDash(container.Reason), exec})
					}
					detail := listSummary("Task detail", kvs, nil)
					detail.Table = &TabTable{
						Columns: []Column{{Title: "Name"}, {Title: "Image"}, {Title: "Last status"}, {Title: "Health"}, {Title: "CPU"}, {Title: "Memory"}, {Title: "Exit code"}, {Title: "Reason"}, {Title: "Exec"}},
						Rows:    rows,
					}
					if len(rows) == 0 {
						detail.Text = "No containers"
					}
					detail.Selectable = true
					detail.Intent = func(index int) *TabIntent {
						if index < 0 || index >= len(task.Containers) {
							return nil
						}
						container := task.Containers[index]
						if !container.ExecEnabled {
							return &TabIntent{Status: fmt.Sprintf("container %s does not support ECS Exec", container.Name)}
						}
						return &TabIntent{ECSExec: &ECSExecTarget{Cluster: cluster, Task: ecsTaskID(task.ARN), Container: container.Name}}
					}
					return detail
				},
				nil,
			)
		},
	}
}

type ecsInstanceGroup struct {
	instance *aws.ECSContainerInstanceResource
	tasks    []aws.ECSTaskResource
}

func ecsInstancesTabSpec(s *ecsService) TabSpec {
	return TabSpec{
		ID:    "instances",
		Title: "Instances",
		New: func(ctx context.Context, profile, region string, row Row) Tab {
			cluster := row.(ecsRow).Name()
			return newListDetailTab(ctx,
				[]Column{{Title: "Instance"}, {Title: "Status"}, {Title: "Agent"}, {Title: "Running"}, {Title: "Pending"}, {Title: "CPU (remaining/registered)"}, {Title: "Memory (remaining/registered)"}},
				func(ctx context.Context) ([]listRow, error) {
					instances, err := s.instances(ctx, profile, region, cluster)
					if err != nil {
						return nil, fmt.Errorf("list ECS container instances: %w", err)
					}
					tasks, err := s.tasks(ctx, profile, region, cluster, "")
					if err != nil {
						return nil, fmt.Errorf("list ECS tasks: %w", err)
					}
					byARN := map[string][]aws.ECSTaskResource{}
					for _, task := range tasks {
						if task.ContainerInstanceArn == "" {
							continue
						}
						byARN[task.ContainerInstanceArn] = append(byARN[task.ContainerInstanceArn], task)
					}
					rows := make([]listRow, 0, len(instances)+1)
					known := map[string]bool{}
					for i := range instances {
						instance := instances[i]
						known[instance.ARN] = true
						rows = append(rows, listRow{
							cells: []string{instance.EC2InstanceID, instance.Status, yesNoBool(instance.AgentConnected), formatInt32(instance.RunningTasksCount), formatInt32(instance.PendingTasksCount), resourceRatio(instance.RemainingCPU, instance.RegisteredCPU), resourceRatio(instance.RemainingMemory, instance.RegisteredMemory)},
							value: ecsInstanceGroup{instance: &instance, tasks: byARN[instance.ARN]},
						})
					}
					var unknown []aws.ECSTaskResource
					for _, task := range tasks {
						if task.ContainerInstanceArn != "" && !known[task.ContainerInstanceArn] {
							unknown = append(unknown, task)
						}
					}
					if len(unknown) > 0 {
						rows = append(rows, listRow{
							cells: []string{"Unknown container instance", "", "", formatInt32(int32(len(unknown))), "0", dash, dash},
							value: ecsInstanceGroup{tasks: unknown},
						})
					}
					return rows, nil
				},
				func(row listRow) *TabDetail {
					group := row.value.(ecsInstanceGroup)
					kvs := []KV{}
					if group.instance != nil {
						kvs = append(kvs,
							KV{Key: "ARN", Value: group.instance.ARN},
							KV{Key: "EC2 instance", Value: group.instance.EC2InstanceID},
							KV{Key: "Status", Value: group.instance.Status},
							KV{Key: "Agent", Value: connectedLabel(group.instance.AgentConnected)},
							KV{Key: "ECS reported", Value: fmt.Sprintf("running %d / pending %d", group.instance.RunningTasksCount, group.instance.PendingTasksCount)},
							KV{Key: "CPU (remaining / registered)", Value: resourceRatio(group.instance.RemainingCPU, group.instance.RegisteredCPU)},
							KV{Key: "Memory (remaining / registered)", Value: resourceRatio(group.instance.RemainingMemory, group.instance.RegisteredMemory)},
						)
					}
					rows := make([][]string, 0, len(group.tasks))
					for _, task := range group.tasks {
						rows = append(rows, []string{task.Group, task.LastStatus, orDash(task.CPU), orDash(task.Memory), orDash(task.StartedAt)})
					}
					detail := listSummary("Instance detail", kvs, nil)
					detail.Table = &TabTable{
						Columns: []Column{{Title: "Group"}, {Title: "Last status"}, {Title: "CPU"}, {Title: "Memory"}, {Title: "Started at"}},
						Rows:    rows,
					}
					if len(rows) == 0 {
						detail.Text = "No tasks"
					}
					return detail
				},
				nil,
			)
		},
	}
}

func yesNoBool(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func connectedLabel(v bool) string {
	if v {
		return "connected"
	}
	return "disconnected"
}

func formatInt32Ptr(v *int32) string {
	if v == nil {
		return dash
	}
	return formatInt32(*v)
}

func resourceRatio(remaining, registered *int32) string {
	return formatInt32Ptr(remaining) + " / " + formatInt32Ptr(registered)
}

func formatContainerMemory(hard, soft string) string {
	values := make([]string, 0, 2)
	for _, v := range []string{hard, soft} {
		if v != "" && v != "0" {
			values = append(values, v)
		}
	}
	if len(values) == 0 {
		return dash
	}
	return strings.Join(values, " / ")
}

// ecsRow は ECS クラスタ 1 つの行。ID はクラスタの ARN。
type ecsRow struct {
	res    aws.ECSResource
	region string
}

func (r ecsRow) ID() string   { return r.res.ID }
func (r ecsRow) Name() string { return r.res.Name }

// State は ECSResource.State の生の値 (active など) を返す。ResourceState() は aws.NormalizeState で
// active を running に変えるため、Web UI の StatusBadge と食い違うので使わない。
func (r ecsRow) State() string { return r.res.State }

func (r ecsRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		formatInt32(r.res.ActiveServices),
		formatInt32(r.res.RunningTasks),
		formatInt32(r.res.PendingTasks),
		formatInt32(r.res.RegisteredEC2),
		r.region,
	}
}

// Overview は Web UI の ecsOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r ecsRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Active services", Value: formatInt32(r.res.ActiveServices)},
		{Key: "Running tasks", Value: formatInt32(r.res.RunningTasks)},
		{Key: "Pending tasks", Value: formatInt32(r.res.PendingTasks)},
		{Key: "Registered EC2", Value: formatInt32(r.res.RegisteredEC2)},
		{Key: "Region", Value: r.region},
	}
}

func (r ecsRow) Tags() map[string]string { return r.res.Tags }

// ecsTaskID はタスクの ARN (arn:aws:ecs:<region>:<account>:task/<cluster>/<task-id>) からタスク ID を返す。
// Web UI の ECSExecLauncher が arnSuffix で取るものと同じで、ListECSContainers と OpenECSExecSession の task に渡す。
// "/" が無ければそのまま返す。
func ecsTaskID(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}
