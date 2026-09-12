package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestDashboardsAPI は DashboardsV1API を httptest.Server へ向けて返す。
func newTestDashboardsAPI(t *testing.T, srv *httptest.Server) (*DashboardsV1API, context.Context) {
	t.Helper()
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := NewConfiguration("datadoghq.com")
	cfg.Host = host
	cfg.Scheme = "http"

	ctx := NewContext(context.Background(), "public", "private")
	return NewDashboardsV1API(cfg), ctx
}

// newJSONServer は固定の応答を返す httptest.Server と、受け取ったパスの記録先を返す。
func newJSONServer(t *testing.T, status int, body string, gotPath *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write test response: %v", err)
		}
	}))
}

func TestListDashboards(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   []DashboardInfo
		// wantErr が真なら ListDashboards がエラーを返すこと。
		wantErr bool
	}{
		{
			name:   "dashboards are returned in order",
			status: http.StatusOK,
			body: `{"dashboards":[
			  {"id":"abc-123","title":"Overview","description":"main","url":"/dashboard/abc-123/overview"},
			  {"id":"def-456","title":"Latency"}
			]}`,
			want: []DashboardInfo{
				{ID: "abc-123", Title: "Overview", Description: "main", URL: "/dashboard/abc-123/overview"},
				{ID: "def-456", Title: "Latency"},
			},
		},
		{
			name:   "a dashboard without an id is skipped",
			status: http.StatusOK,
			body:   `{"dashboards":[{"title":"No Id"},{"id":"abc-123","title":"Overview"}]}`,
			want:   []DashboardInfo{{ID: "abc-123", Title: "Overview"}},
		},
		{
			name:   "an empty list yields an empty slice",
			status: http.StatusOK,
			body:   `{"dashboards":[]}`,
			want:   []DashboardInfo{},
		},
		{
			name:   "a missing dashboards field yields an empty slice",
			status: http.StatusOK,
			body:   `{}`,
			want:   []DashboardInfo{},
		},
		{
			name:    "an api error is wrapped",
			status:  http.StatusForbidden,
			body:    `{"errors":["insufficient_scope"]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			api, ctx := newTestDashboardsAPI(t, newJSONServer(t, tt.status, tt.body, &gotPath))

			dashboards, err := ListDashboards(ctx, api)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ListDashboards() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), "list datadog dashboards") {
					t.Errorf("ListDashboards() error = %v, want it to be wrapped with the operation name", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListDashboards() error = %v", err)
			}
			if gotPath != "/api/v1/dashboard" {
				t.Errorf("requested path = %q, want /api/v1/dashboard", gotPath)
			}
			if dashboards == nil {
				t.Fatal("ListDashboards() returned a nil slice, want an empty slice")
			}
			if len(dashboards) != len(tt.want) {
				t.Fatalf("dashboards = %+v, want %+v", dashboards, tt.want)
			}
			for i, want := range tt.want {
				if dashboards[i] != want {
					t.Errorf("dashboards[%d] = %+v, want %+v", i, dashboards[i], want)
				}
			}
		})
	}
}

// dashboardBody は widgets だけを差し替えられるダッシュボードの応答本文を組み立てる。
func dashboardBody(widgets string) string {
	return `{"id":"abc-123","title":"Overview","description":"main","layout_type":"ordered",` +
		`"url":"/dashboard/abc-123/overview","widgets":[` + widgets + `]}`
}

func TestGetDashboard(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    []WidgetInfo
		wantErr bool
	}{
		{
			name:   "a timeseries widget keeps every query string",
			status: http.StatusOK,
			body: dashboardBody(`{"id":1,"definition":{"type":"timeseries","title":"CPU","requests":[
			  {"q":"avg:system.cpu.user{*}"},
			  {"q":"avg:system.cpu.system{*}"}
			]}}`),
			want: []WidgetInfo{{
				ID:      1,
				Kind:    WidgetKindTimeseries,
				Type:    "timeseries",
				Title:   "CPU",
				Queries: []string{"avg:system.cpu.user{*}", "avg:system.cpu.system{*}"},
			}},
		},
		{
			name:   "a query_value widget is extracted",
			status: http.StatusOK,
			body: dashboardBody(`{"id":2,"definition":{"type":"query_value","title":"Hosts",
			  "requests":[{"q":"sum:system.load.1{*}","aggregator":"avg"}]}}`),
			want: []WidgetInfo{{
				ID:      2,
				Kind:    WidgetKindQueryValue,
				Type:    "query_value",
				Title:   "Hosts",
				Queries: []string{"sum:system.load.1{*}"},
			}},
		},
		{
			name:   "a group widget is replaced by its nested widgets",
			status: http.StatusOK,
			body: dashboardBody(`{"id":10,"definition":{"type":"group","title":"Group","layout_type":"ordered","widgets":[
			  {"id":11,"definition":{"type":"timeseries","title":"Nested CPU","requests":[{"q":"avg:system.cpu.user{*}"}]}},
			  {"id":12,"definition":{"type":"toplist","title":"Nested Top","requests":[{"q":"top(avg:system.cpu.user{*},10,'mean','desc')"}]}}
			]}},
			{"id":13,"definition":{"type":"query_value","title":"After Group","requests":[{"q":"sum:system.load.1{*}"}]}}`),
			want: []WidgetInfo{
				{ID: 11, Kind: WidgetKindTimeseries, Type: "timeseries", Title: "Nested CPU", Queries: []string{"avg:system.cpu.user{*}"}},
				{ID: 12, Kind: WidgetKindUnsupported, Type: "toplist", Title: "Nested Top", Queries: []string{}},
				{ID: 13, Kind: WidgetKindQueryValue, Type: "query_value", Title: "After Group", Queries: []string{"sum:system.load.1{*}"}},
			},
		},
		{
			name:   "an unsupported widget keeps its type name",
			status: http.StatusOK,
			body: dashboardBody(`{"id":3,"definition":{"type":"note","content":"hello"}},
			{"id":4,"definition":{"type":"heatmap","title":"Heat","requests":[{"q":"avg:system.cpu.user{*}"}]}}`),
			want: []WidgetInfo{
				{ID: 3, Kind: WidgetKindUnsupported, Type: "note", Queries: []string{}},
				{ID: 4, Kind: WidgetKindUnsupported, Type: "heatmap", Title: "Heat", Queries: []string{}},
			},
		},
		{
			name:   "a widget type the sdk does not know keeps its type name",
			status: http.StatusOK,
			body:   dashboardBody(`{"id":5,"definition":{"type":"future_widget","title":"Future"}}`),
			want: []WidgetInfo{
				{ID: 5, Kind: WidgetKindUnsupported, Type: "future_widget", Title: "Future", Queries: []string{}},
			},
		},
		{
			name:   "a timeseries widget without a query string is unsupported",
			status: http.StatusOK,
			body: dashboardBody(`{"id":6,"definition":{"type":"timeseries","title":"Formula","requests":[
			  {"formulas":[{"formula":"a"}],"response_format":"timeseries",
			   "queries":[{"data_source":"metrics","name":"a","query":"avg:system.cpu.user{*}"}]}
			]}}`),
			want: []WidgetInfo{
				{ID: 6, Kind: WidgetKindUnsupported, Type: "timeseries", Title: "Formula", Queries: []string{}},
			},
		},
		{
			name:   "a dashboard without widgets yields an empty slice",
			status: http.StatusOK,
			body:   dashboardBody(``),
			want:   []WidgetInfo{},
		},
		{
			name:    "an api error is wrapped",
			status:  http.StatusNotFound,
			body:    `{"errors":["Dashboard not found"]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			api, ctx := newTestDashboardsAPI(t, newJSONServer(t, tt.status, tt.body, &gotPath))

			detail, err := GetDashboard(ctx, api, "abc-123")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetDashboard() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), `get datadog dashboard "abc-123"`) {
					t.Errorf("GetDashboard() error = %v, want it to be wrapped with the operation name and the id", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetDashboard() error = %v", err)
			}
			if gotPath != "/api/v1/dashboard/abc-123" {
				t.Errorf("requested path = %q, want /api/v1/dashboard/abc-123", gotPath)
			}
			if detail.ID != "abc-123" || detail.Title != "Overview" || detail.Description != "main" {
				t.Errorf("detail = %+v, want id abc-123, title Overview, description main", detail)
			}
			if detail.URL != "/dashboard/abc-123/overview" {
				t.Errorf("detail.URL = %q, want the relative path as returned by datadog", detail.URL)
			}
			assertWidgets(t, detail.Widgets, tt.want)
		})
	}
}

// assertWidgets はウィジェット列を要素ごとに比べる。WidgetInfo は slice を持つため
// 直接の比較ができない。
func assertWidgets(t *testing.T, got, want []WidgetInfo) {
	t.Helper()
	if got == nil {
		t.Fatal("widgets = nil, want an empty slice")
	}
	if len(got) != len(want) {
		t.Fatalf("widgets = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Kind != want[i].Kind ||
			got[i].Type != want[i].Type || got[i].Title != want[i].Title {
			t.Errorf("widgets[%d] = %+v, want %+v", i, got[i], want[i])
		}
		if len(got[i].Queries) != len(want[i].Queries) {
			t.Errorf("widgets[%d].Queries = %v, want %v", i, got[i].Queries, want[i].Queries)
			continue
		}
		for j := range want[i].Queries {
			if got[i].Queries[j] != want[i].Queries[j] {
				t.Errorf("widgets[%d].Queries[%d] = %q, want %q", i, j, got[i].Queries[j], want[i].Queries[j])
			}
		}
	}
}
