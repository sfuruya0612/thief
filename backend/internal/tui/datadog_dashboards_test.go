package tui

import (
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

func TestDatadogAppURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "relative", raw: "/dashboard/abc/def?q=1", want: "https://app.datadoghq.com/dashboard/abc/def?q=1"},
		{name: "absolute", raw: "https://example.com/d/abc", want: "https://example.com/d/abc"},
		{name: "empty", raw: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datadogAppURL("datadoghq.com", tt.raw); got != tt.want {
				t.Errorf("datadogAppURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDatadogWidgetsContent(t *testing.T) {
	tests := []struct {
		name   string
		detail datadog.DashboardDetail
		want   TabContent
	}{
		{
			name:   "empty",
			detail: datadog.DashboardDetail{},
			want:   TabContent{Blocks: []TabBlock{{Text: "This dashboard has no widgets"}}},
		},
		{
			name: "widgets",
			detail: datadog.DashboardDetail{Widgets: []datadog.WidgetInfo{
				{
					Kind:    datadog.WidgetKindTimeseries,
					Type:    "timeseries",
					Title:   "Requests",
					Queries: []string{"sum:requests{env:prod}", "sum:errors{env:prod}"},
				},
				{Kind: datadog.WidgetKindUnsupported, Type: "toplist", Title: ""},
			}},
			want: TabContent{Blocks: []TabBlock{{
				Title: "Widgets",
				Table: &TabTable{
					Columns: []Column{{Title: "Kind"}, {Title: "Type"}, {Title: "Title"}, {Title: "Queries"}},
					Rows: [][]string{
						{"timeseries", "timeseries", "Requests", "sum:requests{env:prod} | sum:errors{env:prod}"},
						{"unsupported", "toplist", dash, dash},
					},
				},
			}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datadogWidgetsContent(tt.detail); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("content = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDatadogDashboardRow_CellsAndOverview(t *testing.T) {
	row := datadogDashboardRow{res: datadog.DashboardInfo{
		ID:          "abc",
		Title:       "Service overview",
		Description: "Requests and errors",
		URL:         "https://app.datadoghq.com/dashboard/abc",
	}}
	if row.ID() != "abc" || row.Name() != "Service overview" || row.State() != "" {
		t.Errorf("row = %q/%q/%q", row.ID(), row.Name(), row.State())
	}
	if got, want := row.Cells(), []string{"Service overview", "Requests and errors"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cells = %v, want %v", got, want)
	}
	wantOverview := []KV{
		{Key: "ID", Value: "abc"},
		{Key: "Title", Value: "Service overview"},
		{Key: "Description", Value: "Requests and errors"},
		{Key: "URL", Value: "https://app.datadoghq.com/dashboard/abc"},
	}
	if !reflect.DeepEqual(row.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", row.Overview(), wantOverview)
	}
}

func TestDatadogDashboardsService_Tabs(t *testing.T) {
	svc := NewDatadogDashboardsService(newDatadogAuth("datadoghq.com", "api", "app", testDatadogDeps()), nil, "datadoghq.com")
	if svc.ID() != "datadog-dashboards" || svc.Title() != "Dashboards" || svc.SupportsSession() {
		t.Errorf("service = %s/%s", svc.ID(), svc.Title())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview", "Widgets"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
}
