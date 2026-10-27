package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

func TestGCPCloudRunService_ColumnsAndRow(t *testing.T) {
	svc := NewGCPCloudRunService(func(_ context.Context, projectID string) ([]gcp.RunResourceInfo, error) {
		if projectID != "proj" {
			t.Errorf("projectID = %q, want proj", projectID)
		}
		return []gcp.RunResourceInfo{{
			Name:       "projects/proj/locations/asia-northeast1/services/api",
			Kind:       "service",
			Region:     "asia-northeast1",
			ProjectID:  "proj",
			URI:        "https://api.example.com",
			CreateTime: "2026-10-01T00:00:00Z",
			UpdateTime: "2026-10-02T00:00:00Z",
		}}, nil
	})

	if svc.ID() != "cloudrun" || svc.Title() != "Cloud Run" || svc.SupportsSession() {
		t.Errorf("service = %s/%s session=%v", svc.ID(), svc.Title(), svc.SupportsSession())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
	wantColumns := []Column{
		{Title: "Name"}, {Title: "Kind"}, {Title: "Region"}, {Title: "URI"}, {Title: "Updated"},
	}
	if !reflect.DeepEqual(svc.Columns(), wantColumns) {
		t.Errorf("columns = %#v, want %#v", svc.Columns(), wantColumns)
	}

	rows, err := svc.Load(context.Background(), "proj", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0].(gcpCloudRunRow)
	if row.ID() != "service/asia-northeast1/projects/proj/locations/asia-northeast1/services/api" {
		t.Errorf("ID = %q", row.ID())
	}
	if row.State() != "service" {
		t.Errorf("State = %q, want service", row.State())
	}
	wantCells := []string{
		"projects/proj/locations/asia-northeast1/services/api",
		"service", "asia-northeast1", "https://api.example.com", "2026-10-02T00:00:00Z",
	}
	if !reflect.DeepEqual(row.Cells(), wantCells) {
		t.Errorf("cells = %v, want %v", row.Cells(), wantCells)
	}
	wantOverview := []KV{
		{Key: "Name", Value: "projects/proj/locations/asia-northeast1/services/api"},
		{Key: "Kind", Value: "service"},
		{Key: "Region", Value: "asia-northeast1"},
		{Key: "Project", Value: "proj"},
		{Key: "URI", Value: "https://api.example.com"},
		{Key: "Created", Value: "2026-10-01T00:00:00Z"},
		{Key: "Updated", Value: "2026-10-02T00:00:00Z"},
	}
	if !reflect.DeepEqual(row.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", row.Overview(), wantOverview)
	}
	if row.Tags() != nil {
		t.Errorf("tags = %v, want nil", row.Tags())
	}
}

func TestGCPCloudRunService_LoadError(t *testing.T) {
	svc := NewGCPCloudRunService(func(context.Context, string) ([]gcp.RunResourceInfo, error) {
		return nil, errors.New("permission denied")
	})
	_, err := svc.Load(context.Background(), "proj", "")
	if err == nil || !strings.Contains(err.Error(), "list Cloud Run resources: permission denied") {
		t.Errorf("err = %v", err)
	}
}

func serviceTabTitles(svc Service) []string {
	p, ok := svc.(TabProvider)
	if !ok {
		return []string{"Overview", "Tags"}
	}
	out := make([]string, 0, len(p.Tabs()))
	for _, tab := range p.Tabs() {
		out = append(out, tab.Title)
	}
	return out
}
