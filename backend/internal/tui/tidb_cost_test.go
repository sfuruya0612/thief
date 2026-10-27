package tui

import (
	"context"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/tidb"
)

func TestTiDBCostService_ColumnsAndRow(t *testing.T) {
	svc := NewTiDBCostService(func(_ context.Context, start, end string) ([]tidb.Cost, error) {
		if start != "2026-10" || end != "2026-10" {
			t.Errorf("GetCostRange(%q, %q), want 2026-10/2026-10", start, end)
		}
		return []tidb.Cost{{
			BilledDate: "2026-10-27", ProjectName: "analytics", ClusterName: "primary",
			ServicePathName: "TiDB Cloud", Credits: 1.5, Discounts: 0.5,
			RunningTotal: 10, TotalCost: 11,
		}}, nil
	}, "2026-10")

	if svc.ID() != "tidb-cost" || svc.Title() != "Cost" || svc.SupportsSession() {
		t.Errorf("service = %s/%s", svc.ID(), svc.Title())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
	wantColumns := []Column{
		{Title: "Billed date"}, {Title: "Project"}, {Title: "Cluster"}, {Title: "Service"},
		{Title: "Credits", Align: AlignRight}, {Title: "Discounts", Align: AlignRight},
		{Title: "Total", Align: AlignRight},
	}
	if !reflect.DeepEqual(svc.Columns(), wantColumns) {
		t.Errorf("columns = %#v, want %#v", svc.Columns(), wantColumns)
	}

	rows, err := svc.Load(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	row := rows[0].(tidbCostRow)
	if row.ID() != "2026-10-27/analytics/primary/TiDB Cloud#0" {
		t.Errorf("ID = %q", row.ID())
	}
	wantCells := []string{
		"2026-10-27", "analytics", "primary", "TiDB Cloud",
		"$1.50", "$0.50", "$11.00",
	}
	if !reflect.DeepEqual(row.Cells(), wantCells) {
		t.Errorf("cells = %v, want %v", row.Cells(), wantCells)
	}
	wantOverview := []KV{
		{Key: "Billed date", Value: "2026-10-27"},
		{Key: "Project", Value: "analytics"},
		{Key: "Cluster", Value: "primary"},
		{Key: "Service", Value: "TiDB Cloud"},
		{Key: "Credits", Value: "$1.50"},
		{Key: "Discounts", Value: "$0.50"},
		{Key: "Running total", Value: "$10.00"},
		{Key: "Total", Value: "$11.00"},
	}
	if !reflect.DeepEqual(row.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", row.Overview(), wantOverview)
	}
}
