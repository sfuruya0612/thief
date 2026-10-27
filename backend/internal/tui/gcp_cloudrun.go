package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

// gcpCloudRunService は Cloud Run のサービスとジョブの一覧を提供する Service。
type gcpCloudRunService struct {
	list func(ctx context.Context, projectID string) ([]gcp.RunResourceInfo, error)
}

// NewGCPCloudRunService は Cloud Run の Service を返す。list には gcp.ListCloudRun を渡す。
func NewGCPCloudRunService(list func(ctx context.Context, projectID string) ([]gcp.RunResourceInfo, error)) Service {
	return &gcpCloudRunService{list: list}
}

func (s *gcpCloudRunService) ID() string    { return "cloudrun" }
func (s *gcpCloudRunService) Title() string { return "Cloud Run" }

// Columns は Web UI の cloudRunColumns (frontend/src/components/tables/gcpColumns.tsx) と同じ列。
func (s *gcpCloudRunService) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "Kind"},
		{Title: "Region"},
		{Title: "URI"},
		{Title: "Updated"},
	}
}

func (s *gcpCloudRunService) Load(ctx context.Context, projectID, _ string) ([]Row, error) {
	items, err := s.list(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list Cloud Run resources: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, gcpCloudRunRow{res: it})
	}
	return rows, nil
}

func (s *gcpCloudRunService) SupportsSession() bool { return false }

// Tabs は Web UI と同じく Overview だけを返す。
func (s *gcpCloudRunService) Tabs() []TabSpec {
	return []TabSpec{{ID: tabOverview, Title: "Overview"}}
}

// gcpCloudRunRow は Cloud Run のサービスまたはジョブ 1 件の行。
type gcpCloudRunRow struct {
	res gcp.RunResourceInfo
}

// ID は Web UI の cloudRunResourceFromRaw と同じ kind/region/name を返す。
func (r gcpCloudRunRow) ID() string { return r.res.Kind + "/" + r.res.Region + "/" + r.res.Name }

func (r gcpCloudRunRow) Name() string  { return r.res.Name }
func (r gcpCloudRunRow) State() string { return r.res.Kind }

func (r gcpCloudRunRow) Cells() []string {
	return []string{
		r.res.Name,
		r.res.Kind,
		r.res.Region,
		orDash(r.res.URI),
		orDash(r.res.UpdateTime),
	}
}

// Overview は Web UI の cloudRunOverviewRows と同じ項目。
func (r gcpCloudRunRow) Overview() []KV {
	return []KV{
		{Key: "Name", Value: r.res.Name},
		{Key: "Kind", Value: r.res.Kind},
		{Key: "Region", Value: r.res.Region},
		{Key: "Project", Value: r.res.ProjectID},
		{Key: "URI", Value: orDash(r.res.URI)},
		{Key: "Created", Value: orDash(r.res.CreateTime)},
		{Key: "Updated", Value: orDash(r.res.UpdateTime)},
	}
}

func (r gcpCloudRunRow) Tags() map[string]string { return nil }
