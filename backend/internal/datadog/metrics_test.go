package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// newTestMetricsAPI は MetricsV1API を httptest.Server へ向けて返す。
func newTestMetricsAPI(t *testing.T, srv *httptest.Server) (*MetricsV1API, context.Context) {
	t.Helper()
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := NewConfiguration("datadoghq.com")
	cfg.Host = host
	cfg.Scheme = "http"

	ctx := NewContext(context.Background(), "public", "private")
	return NewMetricsV1API(cfg), ctx
}

func ptr(v float64) *float64 { return &v }

func TestQueryMetrics(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   MetricQueryResult
		// wantErr が空でなければ、エラーにこの文字列が含まれること。
		wantErr string
	}{
		{
			name:   "points and metadata are mapped",
			status: http.StatusOK,
			body: `{"status":"ok","query":"avg:system.cpu.user{*}","series":[
			  {"expression":"avg:system.cpu.user{host:web-1}","scope":"host:web-1",
			   "unit":[{"short_name":"%"}],
			   "pointlist":[[1700000000000,12.5],[1700000060000,13.5]]}
			]}`,
			want: MetricQueryResult{
				Query: "avg:system.cpu.user{*}",
				Series: []MetricSeries{{
					Name:  "avg:system.cpu.user{host:web-1}",
					Scope: "host:web-1",
					Unit:  "%",
					Points: []MetricPoint{
						{T: 1700000000000, V: ptr(12.5)},
						{T: 1700000060000, V: ptr(13.5)},
					},
				}},
			},
		},
		{
			name:   "a null value stays null instead of becoming zero",
			status: http.StatusOK,
			body: `{"status":"ok","query":"q","series":[
			  {"expression":"q","pointlist":[[1700000000000,null],[1700000060000,4]]}
			]}`,
			want: MetricQueryResult{
				Query: "q",
				Series: []MetricSeries{{
					Name: "q",
					Points: []MetricPoint{
						{T: 1700000000000, V: nil},
						{T: 1700000060000, V: ptr(4)},
					},
				}},
			},
		},
		{
			name:   "a point without a timestamp is dropped",
			status: http.StatusOK,
			body: `{"status":"ok","query":"q","series":[
			  {"expression":"q","pointlist":[[null,1],[1700000000000,2],[]]}
			]}`,
			want: MetricQueryResult{
				Query:  "q",
				Series: []MetricSeries{{Name: "q", Points: []MetricPoint{{T: 1700000000000, V: ptr(2)}}}},
			},
		},
		{
			name:   "a per unit is joined with the primary unit",
			status: http.StatusOK,
			body: `{"status":"ok","query":"q","series":[
			  {"expression":"q","unit":[{"short_name":"B"},{"short_name":"s"}],"pointlist":[]}
			]}`,
			want: MetricQueryResult{
				Query:  "q",
				Series: []MetricSeries{{Name: "q", Unit: "B/s", Points: []MetricPoint{}}},
			},
		},
		{
			name:   "a null per unit leaves the primary unit alone",
			status: http.StatusOK,
			body: `{"status":"ok","query":"q","series":[
			  {"expression":"q","unit":[{"short_name":"B"}],"pointlist":[]}
			]}`,
			want: MetricQueryResult{
				Query:  "q",
				Series: []MetricSeries{{Name: "q", Unit: "B", Points: []MetricPoint{}}},
			},
		},
		{
			name:   "the unit falls back to the long name",
			status: http.StatusOK,
			body: `{"status":"ok","query":"q","series":[
			  {"expression":"q","unit":[{"name":"request"}],"pointlist":[]}
			]}`,
			want: MetricQueryResult{
				Query:  "q",
				Series: []MetricSeries{{Name: "q", Unit: "request", Points: []MetricPoint{}}},
			},
		},
		{
			name:   "the series name falls back to the metric and scope",
			status: http.StatusOK,
			body: `{"status":"ok","query":"q","series":[
			  {"metric":"system.cpu.user","scope":"host:web-1","pointlist":[]}
			]}`,
			want: MetricQueryResult{
				Query: "q",
				Series: []MetricSeries{{
					Name:   "system.cpu.user{host:web-1}",
					Scope:  "host:web-1",
					Points: []MetricPoint{},
				}},
			},
		},
		{
			name:   "the series name falls back to the display name when there is no metric",
			status: http.StatusOK,
			body:   `{"status":"ok","query":"q","series":[{"display_name":"cpu","pointlist":[]}]}`,
			want: MetricQueryResult{
				Query:  "q",
				Series: []MetricSeries{{Name: "cpu", Points: []MetricPoint{}}},
			},
		},
		{
			name:   "a query matching nothing yields an empty series list",
			status: http.StatusOK,
			body:   `{"status":"ok","query":"q","series":[]}`,
			want:   MetricQueryResult{Query: "q", Series: []MetricSeries{}},
		},
		{
			name:   "the requested query is kept when the response omits it",
			status: http.StatusOK,
			body:   `{"status":"ok","series":[]}`,
			want:   MetricQueryResult{Query: "avg:system.cpu.user{*}", Series: []MetricSeries{}},
		},
		{
			// Datadog はクエリの誤りを HTTP 200 と status:"error" で返す。
			name:    "a query error reported with http 200 becomes an error",
			status:  http.StatusOK,
			body:    `{"status":"error","error":"Invalid query: unknown metric"}`,
			wantErr: "Invalid query: unknown metric",
		},
		{
			name:    "an api error is wrapped",
			status:  http.StatusForbidden,
			body:    `{"errors":["Forbidden"]}`,
			wantErr: "query datadog metrics",
		},
	}

	from := time.Unix(1700000000, 0)
	to := time.Unix(1700003600, 0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			api, ctx := newTestMetricsAPI(t, newJSONServer(t, tt.status, tt.body, &gotPath))

			got, err := QueryMetrics(ctx, api, "avg:system.cpu.user{*}", from, to)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("QueryMetrics() error = nil, want an error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("QueryMetrics() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("QueryMetrics() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("QueryMetrics() mismatch (-want +got):\n%s", diff)
			}
			if gotPath != "/api/v1/query" {
				t.Errorf("request path = %q, want %q", gotPath, "/api/v1/query")
			}
		})
	}
}

// TestQueryMetricsSendsWindow は時間窓とクエリが Unix 秒のクエリパラメータとして
// 渡ることを確認する。ここを取り違えると常に誤った期間のデータを描いてしまう。
func TestQueryMetricsSendsWindow(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"status":"ok","series":[]}`)); err != nil {
			t.Errorf("write test response: %v", err)
		}
	}))
	api, ctx := newTestMetricsAPI(t, srv)

	from := time.Unix(1700000000, 0)
	to := time.Unix(1700003600, 0)
	if _, err := QueryMetrics(ctx, api, "avg:system.cpu.user{*}", from, to); err != nil {
		t.Fatalf("QueryMetrics() error = %v", err)
	}

	for _, want := range []string{"from=1700000000", "to=1700003600", "query=avg%3Asystem.cpu.user%7B%2A%7D"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("request query = %q, want it to contain %q", gotQuery, want)
		}
	}
}
