package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/tidb"
)

// tidbProjectsService は TiDB Cloud のプロジェクト一覧を提供する Service。
type tidbProjectsService struct {
	list     func(ctx context.Context) ([]tidb.Project, error)
	clusters func(ctx context.Context, projectID string) ([]tidb.Cluster, error)
}

// NewTiDBProjectsService は Projects の Service を返す。list には
// (*tidb.Client).ListProjects を、clusters には (*tidb.Client).ListClusters を渡す。
// clusters を省略すると Clusters タブを表示しない。
func NewTiDBProjectsService(
	list func(ctx context.Context) ([]tidb.Project, error),
	clusters ...func(ctx context.Context, projectID string) ([]tidb.Cluster, error),
) Service {
	s := &tidbProjectsService{list: list}
	if len(clusters) > 0 {
		s.clusters = clusters[0]
	}
	return s
}

func (s *tidbProjectsService) ID() string    { return "tidb-projects" }
func (s *tidbProjectsService) Title() string { return "Projects" }

// Columns は Web UI の tidbProjectColumns (frontend/src/components/tables/nonAwsColumns.tsx) と同じ列。
func (s *tidbProjectsService) Columns() []Column {
	return []Column{
		{Title: "Project"},
		{Title: "Clusters", Align: AlignRight},
		{Title: "Users", Align: AlignRight},
		{Title: "Org ID"},
		{Title: "Created"},
	}
}

func (s *tidbProjectsService) Load(ctx context.Context, _, _ string) ([]Row, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("list TiDB projects: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, tidbProjectRow{res: it})
	}
	return rows, nil
}

func (s *tidbProjectsService) SupportsSession() bool { return false }

// Tabs は Overview と、プロジェクトのクラスタを並べる Clusters を返す。
func (s *tidbProjectsService) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.clusters != nil {
		tabs = append(tabs, contentTabSpec("clusters", "Clusters", func(ctx context.Context, _, _ string, row Row) (TabContent, error) {
			project := row.(tidbProjectRow)
			items, err := s.clusters(ctx, project.res.ID)
			if err != nil {
				return TabContent{}, fmt.Errorf("list TiDB clusters: %w", err)
			}
			rows := make([][]string, 0, len(items))
			for _, c := range items {
				rows = append(rows, []string{
					c.Name,
					c.Status,
					c.ClusterType,
					c.CloudProvider,
					c.Region,
					orDash(formatTiDBTime(c.CreatedAt)),
				})
			}
			return TabContent{Blocks: []TabBlock{{
				Title: "Clusters",
				Table: &TabTable{
					Columns: []Column{
						{Title: "Cluster"}, {Title: "Status"}, {Title: "Type"},
						{Title: "Cloud"}, {Title: "Region"}, {Title: "Created"},
					},
					Rows: rows,
				},
			}}}, nil
		}))
	}
	return tabs
}

// tidbProjectRow は TiDB Cloud のプロジェクト 1 件の行。ID はプロジェクト ID。
type tidbProjectRow struct {
	res tidb.Project
}

func (r tidbProjectRow) ID() string    { return r.res.ID }
func (r tidbProjectRow) Name() string  { return r.res.Name }
func (r tidbProjectRow) State() string { return "" }

func (r tidbProjectRow) Cells() []string {
	return []string{
		r.res.Name,
		fmt.Sprintf("%d", r.res.ClusterCount),
		fmt.Sprintf("%d", r.res.UserCount),
		r.res.OrgID,
		orDash(formatTiDBTime(r.res.CreatedAt)),
	}
}

// Overview は Web UI のプロジェクト一覧の項目に合わせる。
func (r tidbProjectRow) Overview() []KV {
	return []KV{
		{Key: "Project", Value: r.res.Name},
		{Key: "Project ID", Value: r.res.ID},
		{Key: "Org ID", Value: r.res.OrgID},
		{Key: "Clusters", Value: fmt.Sprintf("%d", r.res.ClusterCount)},
		{Key: "Users", Value: fmt.Sprintf("%d", r.res.UserCount)},
		{Key: "Created", Value: orDash(formatTiDBTime(r.res.CreatedAt))},
	}
}

func (r tidbProjectRow) Tags() map[string]string { return nil }

// formatTiDBTime は TiDB Cloud の時刻を RFC3339 で返す。ゼロ値は空文字。
func formatTiDBTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

var (
	_ Service     = (*tidbProjectsService)(nil)
	_ TabProvider = (*tidbProjectsService)(nil)
)
