package tui

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/tidb"
)

func TestTiDBProjectsService_ColumnsAndRow(t *testing.T) {
	created := time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	svc := NewTiDBProjectsService(
		func(context.Context) ([]tidb.Project, error) {
			return []tidb.Project{{
				ID: "proj-1", Name: "analytics", OrgID: "org-1",
				ClusterCount: 2, UserCount: 3, CreatedAt: created,
			}}, nil
		},
		func(context.Context, string) ([]tidb.Cluster, error) { return nil, nil },
	)

	if svc.ID() != "tidb-projects" || svc.Title() != "Projects" || svc.SupportsSession() {
		t.Errorf("service = %s/%s", svc.ID(), svc.Title())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview", "Clusters"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
	wantColumns := []Column{
		{Title: "Project"}, {Title: "Clusters", Align: AlignRight},
		{Title: "Users", Align: AlignRight}, {Title: "Org ID"}, {Title: "Created"},
	}
	if !reflect.DeepEqual(svc.Columns(), wantColumns) {
		t.Errorf("columns = %#v, want %#v", svc.Columns(), wantColumns)
	}

	rows, err := svc.Load(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	row := rows[0].(tidbProjectRow)
	if row.ID() != "proj-1" || row.Name() != "analytics" || row.State() != "" {
		t.Errorf("row = %q/%q/%q", row.ID(), row.Name(), row.State())
	}
	wantCells := []string{"analytics", "2", "3", "org-1", "2026-10-01T09:30:00Z"}
	if !reflect.DeepEqual(row.Cells(), wantCells) {
		t.Errorf("cells = %v, want %v", row.Cells(), wantCells)
	}
	wantOverview := []KV{
		{Key: "Project", Value: "analytics"},
		{Key: "Project ID", Value: "proj-1"},
		{Key: "Org ID", Value: "org-1"},
		{Key: "Clusters", Value: "2"},
		{Key: "Users", Value: "3"},
		{Key: "Created", Value: "2026-10-01T09:30:00Z"},
	}
	if !reflect.DeepEqual(row.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", row.Overview(), wantOverview)
	}
}

func TestTiDBProjectsService_ClustersTab(t *testing.T) {
	created := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	svc := NewTiDBProjectsService(
		func(context.Context) ([]tidb.Project, error) { return nil, nil },
		func(_ context.Context, projectID string) ([]tidb.Cluster, error) {
			if projectID != "proj-1" {
				t.Errorf("ListClusters project = %q, want proj-1", projectID)
			}
			return []tidb.Cluster{{
				ID: "c-1", Name: "primary", Status: "AVAILABLE", Region: "ap-northeast-1",
				ClusterType: "SERVERLESS", CloudProvider: "AWS", CreatedAt: created,
			}}, nil
		},
	)
	specs := svc.(TabProvider).Tabs()
	if len(specs) != 2 || specs[1].Title != "Clusters" {
		t.Fatalf("tabs = %#v", specs)
	}
	tab := specs[1].New(context.Background(), "", "", tidbProjectRow{res: tidb.Project{ID: "proj-1", Name: "analytics"}})
	content, err := loadTabContent(t, tab)
	if err != nil {
		t.Fatalf("clusters tab: %v", err)
	}
	if len(content.Blocks) != 1 || content.Blocks[0].Table == nil {
		t.Fatalf("content = %#v", content)
	}
	wantColumns := []Column{
		{Title: "Cluster"}, {Title: "Status"}, {Title: "Type"},
		{Title: "Cloud"}, {Title: "Region"}, {Title: "Created"},
	}
	if !reflect.DeepEqual(content.Blocks[0].Table.Columns, wantColumns) {
		t.Errorf("columns = %#v, want %#v", content.Blocks[0].Table.Columns, wantColumns)
	}
	wantRow := []string{"primary", "AVAILABLE", "SERVERLESS", "AWS", "ap-northeast-1", "2026-10-02T10:00:00Z"}
	if len(content.Blocks[0].Table.Rows) != 1 || !reflect.DeepEqual(content.Blocks[0].Table.Rows[0], wantRow) {
		t.Errorf("rows = %#v, want %#v", content.Blocks[0].Table.Rows, wantRow)
	}
}

// loadTabContent は contentTab の Init を 1 回実行して内容を取り出す。
func loadTabContent(t *testing.T, tab Tab) (TabContent, error) {
	t.Helper()
	cmd := tab.Init()
	if cmd == nil {
		t.Fatal("tab.Init returned no cmd")
	}
	msg := cmd()
	loaded, ok := msg.(contentTabLoadedMsg)
	if !ok {
		t.Fatalf("tab.Init msg = %#v, want contentTabLoadedMsg", msg)
	}
	return loaded.content, loaded.err
}
