package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// rdsService は RDS インスタンスの一覧を提供する Service。
type rdsService struct {
	list           func(ctx context.Context, profile, region string) ([]aws.RDSResource, error)
	instanceParams func(ctx context.Context, profile, region, group string) ([]aws.RDSParameter, error)
	clusterParams  func(ctx context.Context, profile, region, clusterID string) (aws.RDSClusterParameterGroup, error)
	now            func() time.Time
}

// NewRDSService は RDS の Service を返す。list には aws.ListRDSResources を渡す。
// clusterParams を省略すると Cluster Parameters タブを表示しない (既存のテスト用)。
func NewRDSService(list func(ctx context.Context, profile, region string) ([]aws.RDSResource, error), clusterParams ...func(ctx context.Context, profile, region, clusterID string) (aws.RDSClusterParameterGroup, error)) Service {
	s := &rdsService{list: list, now: time.Now}
	if len(clusterParams) > 0 {
		s.clusterParams = clusterParams[0]
	}
	return s
}

// NewRDSInteractiveService は Instance Parameters と Cluster Parameters のタブを持つ RDS の Service を返す。
func NewRDSInteractiveService(
	list func(ctx context.Context, profile, region string) ([]aws.RDSResource, error),
	instanceParams func(ctx context.Context, profile, region, group string) ([]aws.RDSParameter, error),
	clusterParams func(ctx context.Context, profile, region, clusterID string) (aws.RDSClusterParameterGroup, error),
) Service {
	return &rdsService{list: list, instanceParams: instanceParams, clusterParams: clusterParams, now: time.Now}
}

func (s *rdsService) ID() string    { return "rds" }
func (s *rdsService) Title() string { return "RDS" }

// Columns は Web UI の rdsColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// 値が無いセルの表示も Web UI に合わせ、Cluster と Uptime は dash、Endpoint は空文字列にする。
// MultiAZ は真なら check、偽なら dash。
func (s *rdsService) Columns() []Column {
	return []Column{
		{Title: "Identifier"},
		{Title: "State"},
		{Title: "Engine"},
		{Title: "Engine Version"},
		{Title: "Cluster"},
		{Title: "Class"},
		{Title: "Region"},
		{Title: "MultiAZ"},
		{Title: "Endpoint"},
		{Title: "Uptime"},
	}
}

func (s *rdsService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list RDS instances: %w", err)
	}
	now := s.now()
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, rdsRow{res: it, region: region, now: now})
	}
	return rows, nil
}

func (s *rdsService) SupportsSession() bool { return false }

// Tabs は Overview、Instance Parameters、Cluster Parameters、Tags を返す。
func (s *rdsService) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.instanceParams != nil {
		tabs = append(tabs, TabSpec{
			ID: "instance-parameters", Title: "Instance Parameters",
			New: func(ctx context.Context, profile, region string, row Row) Tab {
				instance := row.(rdsRow)
				return newRDSInstanceParametersTab(ctx, profile, region, instance.res.ParameterGroups,
					func(ctx context.Context, profile, region, group string) ([][]string, error) {
						params, err := s.instanceParams(ctx, profile, region, group)
						if err != nil {
							return nil, err
						}
						rows := make([][]string, 0, len(params))
						for _, param := range params {
							modifiable := dash
							if param.IsModifiable {
								modifiable = check
							}
							rows = append(rows, []string{param.Name, orDash(param.Value), param.ApplyType, param.DataType, modifiable, param.Source})
						}
						return rows, nil
					})
			},
		})
	}
	if s.clusterParams != nil {
		tabs = append(tabs, contentTabSpec("cluster-parameters", "Cluster Parameters", func(ctx context.Context, profile, region string, row Row) (TabContent, error) {
			instance := row.(rdsRow)
			if instance.res.ClusterID == "" {
				return TabContent{Blocks: []TabBlock{{Title: "Cluster parameters", Text: "Not part of a DB cluster."}}}, nil
			}
			group, err := s.clusterParams(ctx, profile, region, instance.res.ClusterID)
			if err != nil {
				return TabContent{}, fmt.Errorf("list RDS cluster parameters: %w", err)
			}
			rows := make([][]string, 0, len(group.Parameters))
			for _, param := range group.Parameters {
				modifiable := dash
				if param.IsModifiable {
					modifiable = check
				}
				rows = append(rows, []string{param.Name, orDash(param.Value), param.ApplyType, param.DataType, modifiable, param.Source})
			}
			return TabContent{Blocks: []TabBlock{{
				Title: fmt.Sprintf("%s (%d)", group.GroupName, len(group.Parameters)),
				Table: &TabTable{
					Columns: []Column{{Title: "Name"}, {Title: "Value"}, {Title: "Apply type"}, {Title: "Data type"}, {Title: "Modifiable"}, {Title: "Source"}},
					Rows:    rows,
				},
			}}}, nil
		}))
	}
	tabs = append(tabs, TabSpec{ID: tabTags, Title: "Tags"})
	return tabs
}

// rdsRow は RDS インスタンス 1 つの行。ID はインスタンス識別子。
type rdsRow struct {
	res    aws.RDSResource
	region string
	now    time.Time
}

func (r rdsRow) ID() string   { return r.res.ID }
func (r rdsRow) Name() string { return r.res.Name }

// State は RDSResource.State の生の値 (available など) を返す。ResourceState() は aws.NormalizeState で
// available を running に変えるため、Web UI の StatusBadge と食い違うので使わない。
func (r rdsRow) State() string { return r.res.State }

func (r rdsRow) Cells() []string {
	multiAZ := dash
	if r.res.MultiAZ {
		multiAZ = check
	}
	return []string{
		r.res.Name,
		r.State(),
		r.res.Engine,
		r.res.EngineVersion,
		orDash(r.res.ClusterID),
		r.res.Class,
		r.region,
		multiAZ,
		r.res.Endpoint,
		orDash(formatUptime(r.res.LaunchTime, r.now)),
	}
}

// Overview は Web UI の rdsOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Multi-AZ は yes か no、Cluster、Uptime、Launched は空なら dash、Port は 10 進、他の項目は生の値にする。
func (r rdsRow) Overview() []KV {
	multiAZ := "no"
	if r.res.MultiAZ {
		multiAZ = "yes"
	}
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Engine", Value: r.res.Engine},
		{Key: "Engine version", Value: r.res.EngineVersion},
		{Key: "Cluster", Value: orDash(r.res.ClusterID)},
		{Key: "Class", Value: r.res.Class},
		{Key: "Multi-AZ", Value: multiAZ},
		{Key: "Endpoint", Value: r.res.Endpoint},
		{Key: "Port", Value: formatInt32(r.res.Port)},
		{Key: "VPC", Value: r.res.VpcID},
		{Key: "Region", Value: r.region},
		{Key: "Uptime", Value: orDash(formatUptime(r.res.LaunchTime, r.now))},
		{Key: "Launched", Value: orDash(formatLaunched(r.res.LaunchTime))},
	}
}

func (r rdsRow) Tags() map[string]string { return r.res.Tags }
