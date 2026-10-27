package tui

import (
	"context"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

func TestGroupGCPIAMBindings(t *testing.T) {
	bindings := []gcp.IAMBindingInfo{
		{Member: "user:b@example.com", Role: "roles/viewer", ProjectID: "proj"},
		{Member: "user:a@example.com", Role: "roles/editor", ProjectID: "proj"},
		{Member: "user:a@example.com", Role: "roles/viewer", ProjectID: "proj"},
		{Member: "user:a@example.com", Role: "roles/editor", ProjectID: "proj"},
		{Member: "", Role: "roles/ignored", ProjectID: "proj"},
	}
	got := groupGCPIAMBindings(bindings)
	want := []gcpIAMMember{
		{Member: "user:a@example.com", ProjectID: "proj", Roles: []string{"roles/editor", "roles/viewer"}},
		{Member: "user:b@example.com", ProjectID: "proj", Roles: []string{"roles/viewer"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("members = %#v, want %#v", got, want)
	}
}

func TestGCPIAMService_ColumnsAndRow(t *testing.T) {
	svc := NewGCPIAMService(func(context.Context, string) ([]gcp.IAMBindingInfo, error) {
		return []gcp.IAMBindingInfo{
			{Member: "user:a@example.com", Role: "roles/editor", ProjectID: "proj"},
			{Member: "user:a@example.com", Role: "roles/viewer", ProjectID: "proj"},
		}, nil
	})

	if svc.ID() != "gcpiam" || svc.Title() != "IAM" || svc.SupportsSession() {
		t.Errorf("service = %s/%s", svc.ID(), svc.Title())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
	wantColumns := []Column{{Title: "Member"}, {Title: "Roles"}, {Title: "Count", Align: AlignRight}}
	if !reflect.DeepEqual(svc.Columns(), wantColumns) {
		t.Errorf("columns = %#v, want %#v", svc.Columns(), wantColumns)
	}

	rows, err := svc.Load(context.Background(), "proj", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	row := rows[0].(gcpIAMRow)
	if row.ID() != "user:a@example.com" {
		t.Errorf("ID = %q", row.ID())
	}
	if got, want := row.Cells(), []string{"user:a@example.com", "roles/editor, roles/viewer", "2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cells = %v, want %v", got, want)
	}
	wantOverview := []KV{
		{Key: "Member", Value: "user:a@example.com"},
		{Key: "Project", Value: "proj"},
		{Key: "Roles", Value: "2"},
		{Key: "Role 1", Value: "roles/editor"},
		{Key: "Role 2", Value: "roles/viewer"},
	}
	if !reflect.DeepEqual(row.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", row.Overview(), wantOverview)
	}
}
