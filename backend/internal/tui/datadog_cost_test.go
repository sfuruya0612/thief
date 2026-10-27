package tui

import (
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

func TestFormatCost(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{in: 0, want: "$0.00"},
		{in: 12.3, want: "$12.30"},
		{in: 100, want: "$100.00"},
		{in: 101.4, want: "$101"},
		{in: 1234.6, want: "$1,235"},
		{in: -12.3, want: "$-12.30"},
	}
	for _, tt := range tests {
		if got := formatCost(tt.in); got != tt.want {
			t.Errorf("formatCost(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDatadogCostRow_CellsAndOverview(t *testing.T) {
	row := datadogCostRow{index: 2, res: datadog.CostInfo{
		Month:       "2026-10",
		AccountName: "acct",
		OrgName:     "org",
		ProductName: "infra",
		ChargeType:  "usage",
		Cost:        12.3,
	}}
	if row.ID() != "2026-10/acct/org/infra/usage#2" {
		t.Errorf("ID = %q", row.ID())
	}
	if row.Name() != "infra" || row.State() != "" {
		t.Errorf("name/state = %q/%q", row.Name(), row.State())
	}
	wantCells := []string{"2026-10", "acct", "org", "infra", "usage", "$12.30"}
	if !reflect.DeepEqual(row.Cells(), wantCells) {
		t.Errorf("cells = %v, want %v", row.Cells(), wantCells)
	}
	wantOverview := []KV{
		{Key: "Month", Value: "2026-10"},
		{Key: "Account", Value: "acct"},
		{Key: "Org", Value: "org"},
		{Key: "Product", Value: "infra"},
		{Key: "Charge type", Value: "usage"},
		{Key: "Cost", Value: "$12.30"},
	}
	if !reflect.DeepEqual(row.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", row.Overview(), wantOverview)
	}
}

func TestDatadogCostService_ColumnsAndTabs(t *testing.T) {
	svc := NewDatadogCostService(
		"datadog-cost-historical", "Cost (Historical)",
		newDatadogAuth("datadoghq.com", "api", "app", testDatadogDeps()),
		nil, datadog.GetHistoricalCost, "", "", "summary",
	)
	if svc.ID() != "datadog-cost-historical" || svc.Title() != "Cost (Historical)" || svc.SupportsSession() {
		t.Errorf("service = %s/%s", svc.ID(), svc.Title())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
	wantColumns := []Column{
		{Title: "Month"}, {Title: "Account"}, {Title: "Org"},
		{Title: "Product"}, {Title: "Charge type"}, {Title: "Cost", Align: AlignRight},
	}
	if !reflect.DeepEqual(svc.Columns(), wantColumns) {
		t.Errorf("columns = %#v, want %#v", svc.Columns(), wantColumns)
	}
}
