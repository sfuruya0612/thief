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
	list func(ctx context.Context, profile, region string) ([]aws.ECSResource, error)
}

// NewECSService は ECS の Service を返す。list には aws.ListECSResources を渡す。
func NewECSService(list func(ctx context.Context, profile, region string) ([]aws.ECSResource, error)) Service {
	return &ecsService{list: list}
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
// Deps.StartECSExec で ECS Exec を始める (startECSExec)。
func (s *ecsService) SupportsSession() bool { return true }

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
// Web UI の ECSExecLauncher が arnSuffix で取るものと同じで、ListECSContainers と StartECSExec の task に渡す。
// "/" が無ければそのまま返す。
func ecsTaskID(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}
