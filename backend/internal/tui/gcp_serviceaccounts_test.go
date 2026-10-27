package tui

import (
	"context"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

func TestGCPServiceAccountService_ColumnsAndRows(t *testing.T) {
	svc := NewGCPServiceAccountService(func(context.Context, string) ([]gcp.ServiceAccountInfo, error) {
		return []gcp.ServiceAccountInfo{
			{
				Email:       "runner@example.iam.gserviceaccount.com",
				DisplayName: "Runner",
				Description: "CI runner",
				ProjectID:   "proj",
				UniqueID:    "123",
			},
			{
				Email:     "disabled@example.iam.gserviceaccount.com",
				ProjectID: "proj",
				Disabled:  true,
			},
		}, nil
	})

	if svc.ID() != "gcpserviceaccounts" || svc.Title() != "Service Accounts" || svc.SupportsSession() {
		t.Errorf("service = %s/%s", svc.ID(), svc.Title())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
	wantColumns := []Column{{Title: "Email"}, {Title: "Display name"}, {Title: "Status"}, {Title: "Description"}}
	if !reflect.DeepEqual(svc.Columns(), wantColumns) {
		t.Errorf("columns = %#v, want %#v", svc.Columns(), wantColumns)
	}

	rows, err := svc.Load(context.Background(), "proj", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	first := rows[0].(gcpServiceAccountRow)
	if first.Name() != "Runner" || first.State() != "enabled" {
		t.Errorf("first name/state = %q/%q", first.Name(), first.State())
	}
	if got, want := first.Cells(), []string{
		"runner@example.iam.gserviceaccount.com", "Runner", "enabled", "CI runner",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("cells = %v, want %v", got, want)
	}
	wantOverview := []KV{
		{Key: "Email", Value: "runner@example.iam.gserviceaccount.com"},
		{Key: "Display name", Value: "Runner"},
		{Key: "Description", Value: "CI runner"},
		{Key: "Project", Value: "proj"},
		{Key: "Unique ID", Value: "123"},
		{Key: "Status", Value: "enabled"},
	}
	if !reflect.DeepEqual(first.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", first.Overview(), wantOverview)
	}

	second := rows[1].(gcpServiceAccountRow)
	if second.Name() != "disabled@example.iam.gserviceaccount.com" || second.State() != "disabled" {
		t.Errorf("second name/state = %q/%q", second.Name(), second.State())
	}
	if got, want := second.Cells(), []string{
		"disabled@example.iam.gserviceaccount.com", dash, "disabled", dash,
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("second cells = %v, want %v", got, want)
	}
}
