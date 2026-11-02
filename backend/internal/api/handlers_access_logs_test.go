package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/accesslog"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestAccessLogHandlers(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.accessLogs = accessLogTestDeps(t)
	s.registerRoutes()

	sources := getAccessLogSources(t, s)
	if len(sources.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(sources.Sources))
	}
	sourceID := sources.Sources[0].ID

	validate := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/table/validate", AccessLogTableValidateRequest{
		SourceID: sourceID,
		Catalog:  "AwsDataCatalog",
		Database: "logs",
		Table:    "alb_access_logs",
	})
	if validate.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", validate.Code, validate.Body.String())
	}
	var validation AccessLogTableValidateResponse
	decodeAccessLogResponse(t, validate, &validation)
	if !validation.Validation.Valid || !validation.Validation.Attribution {
		t.Errorf("validation = %+v", validation.Validation)
	}
	if !strings.Contains(validation.SetupSQL, "CREATE EXTERNAL TABLE") {
		t.Errorf("setup sql = %q", validation.SetupSQL)
	}

	start := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query", AccessLogQueryRequest{
		View: "pairs",
		Sources: []AccessLogQuerySourceRequest{{
			ID: sourceID,
			Table: &accesslog.TableMapping{
				Catalog:  "AwsDataCatalog",
				Database: "logs",
				Table:    "alb_access_logs",
			},
		}},
		From: "2026-11-02T00:00:00Z",
		To:   "2026-11-02T01:00:00Z",
	})
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", start.Code, start.Body.String())
	}
	var started AccessLogQueryStartResponse
	decodeAccessLogResponse(t, start, &started)
	if started.JobID == "" {
		t.Fatal("job id is empty")
	}

	statusRec := getAccessLog(t, s, "/api/aws/profiles/default/access-logs/query/"+started.JobID)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status status = %d, body = %s", statusRec.Code, statusRec.Body.String())
	}
	var status AccessLogQueryResponse
	decodeAccessLogResponse(t, statusRec, &status)
	if status.State != accesslog.QueryStateSucceeded {
		t.Errorf("state = %q, want succeeded", status.State)
	}

	resultsRec := getAccessLog(t, s, "/api/aws/profiles/default/access-logs/query/"+started.JobID+"/results")
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("results status = %d, body = %s", resultsRec.Code, resultsRec.Body.String())
	}
	var results AccessLogResultsResponse
	decodeAccessLogResponse(t, resultsRec, &results)
	if len(results.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(results.Rows))
	}
	if results.Rows[0].RequestCount != 10 || results.Rows[0].BlockedCount != 3 {
		t.Errorf("row = %+v", results.Rows[0])
	}
	if results.Summary.RecordCount != 10 {
		t.Errorf("summary = %+v", results.Summary)
	}
}

func TestAccessLogQueryValidation(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.accessLogs = accessLogTestDeps(t)
	s.registerRoutes()

	tests := []struct {
		name string
		body AccessLogQueryRequest
		want int
	}{
		{
			name: "no sources",
			body: AccessLogQueryRequest{View: "pairs", From: "2026-11-02T00:00:00Z", To: "2026-11-02T01:00:00Z"},
			want: http.StatusBadRequest,
		},
		{
			name: "unknown source",
			body: AccessLogQueryRequest{View: "pairs", Sources: []AccessLogQuerySourceRequest{{ID: "unknown"}}, From: "2026-11-02T00:00:00Z", To: "2026-11-02T01:00:00Z"},
			want: http.StatusBadRequest,
		},
		{
			name: "duplicate source",
			body: AccessLogQueryRequest{View: "pairs", Sources: []AccessLogQuerySourceRequest{{ID: accessLogTestSourceID}, {ID: accessLogTestSourceID}}, From: "2026-11-02T00:00:00Z", To: "2026-11-02T01:00:00Z"},
			want: http.StatusBadRequest,
		},
		{
			name: "range too long",
			body: AccessLogQueryRequest{View: "pairs", Sources: []AccessLogQuerySourceRequest{{ID: accessLogTestSourceID}}, From: "2026-11-01T00:00:00Z", To: "2026-11-03T01:00:00Z"},
			want: http.StatusBadRequest,
		},
		{
			name: "invalid view",
			body: AccessLogQueryRequest{View: "bad", Sources: []AccessLogQuerySourceRequest{{ID: accessLogTestSourceID}}, From: "2026-11-02T00:00:00Z", To: "2026-11-02T01:00:00Z"},
			want: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query", tt.body)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestAccessLogJobNotFound(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	s.accessLogs = accessLogTestDeps(t)
	s.registerRoutes()
	rec := getAccessLog(t, s, "/api/aws/profiles/default/access-logs/query/missing")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

const accessLogTestSourceID = "alb-source"

func accessLogTestDeps(t *testing.T) *accessLogDeps {
	t.Helper()
	source := accesslog.Source{
		ID:              accessLogTestSourceID,
		Type:            accesslog.SourceALB,
		Name:            "prod-alb",
		Region:          "ap-northeast-1",
		QueryRegion:     "ap-northeast-1",
		Engine:          accesslog.EngineAthena,
		SchemaProfile:   accesslog.SchemaALBLegacy,
		SourceKeyValues: []string{"app/prod-alb/abc"},
		Destination:     accesslog.Destination{Type: accesslog.DestinationS3, Bucket: "logs", Prefix: "alb"},
		Status: accesslog.SourceStatus{
			LoggingEnabled:      true,
			DestinationType:     accesslog.DestinationS3,
			DestinationReadable: true,
			Attribution:         true,
			SourceKey:           "app/prod-alb/abc",
		},
	}
	return &accessLogDeps{
		discover: func(context.Context, string, string) (awsinternal.AccessLogSources, error) {
			return awsinternal.AccessLogSources{Sources: []accesslog.Source{source}}, nil
		},
		tableMetadata: func(context.Context, string, string, string, string, string, string) (*awsinternal.AthenaTableMetadata, error) {
			return &awsinternal.AthenaTableMetadata{
				Columns: []awsinternal.AthenaColumn{
					{Name: "elb", Type: "string"},
					{Name: "client_ip", Type: "string"},
					{Name: "request_url", Type: "string"},
					{Name: "elb_status_code", Type: "int"},
					{Name: "actions_executed", Type: "string"},
					{Name: "user_agent", Type: "string"},
					{Name: "time", Type: "string"},
				},
				PartitionKeys: []awsinternal.AthenaColumn{{Name: "day", Type: "string"}},
			}, nil
		},
		workgroup: func(context.Context, string, string, string) (*awsinternal.AthenaWorkgroupConfig, error) {
			return &awsinternal.AthenaWorkgroupConfig{Name: "primary"}, nil
		},
		startAthena: func(_ context.Context, _, _ string, in awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error) {
			if len(in.ExecutionParameters) == 0 {
				t.Error("execution parameters are empty")
			}
			return &awsinternal.AthenaQueryExecution{ID: "exec-1", State: "SUCCEEDED"}, nil
		},
		getAthena: func(context.Context, string, string, string) (*awsinternal.AthenaQueryExecution, error) {
			return &awsinternal.AthenaQueryExecution{ID: "exec-1", State: "SUCCEEDED", BytesScanned: 1234}, nil
		},
		resultsAthena: func(context.Context, string, string, string, int) (*awsinternal.AthenaResultPage, error) {
			columns := []awsinternal.AthenaResultColumn{
				{Name: "source_key"}, {Name: "client_ip"}, {Name: "uri_path"},
				{Name: "request_count"}, {Name: "blocked_count"}, {Name: "challenged_count"},
				{Name: "count_matched"}, {Name: "client_error_count"}, {Name: "server_error_count"},
				{Name: "first_seen_ms"}, {Name: "last_seen_ms"}, {Name: "sample_user_agent"},
				{Name: "sample_rule_id"}, {Name: "has_query"}, {Name: "sample_query"},
			}
			return &awsinternal.AthenaResultPage{
				Columns: columns,
				Rows: [][]string{{
					"app/prod-alb/abc", "203.0.113.9", "/admin", "10", "3", "1", "0", "4", "0",
					"1762000000000", "1762003600000", "curl/8", "rule-a", "0", "",
				}},
			}, nil
		},
		stopAthena: func(context.Context, string, string, string) error { return nil },
		startInsights: func(context.Context, string, string, []string, string, time.Time, time.Time, int) (string, error) {
			return "insights-1", nil
		},
		getInsights: func(context.Context, string, string, string) (*awsinternal.LogsInsightsQueryResult, error) {
			return &awsinternal.LogsInsightsQueryResult{Status: "Complete"}, nil
		},
		stopInsights: func(context.Context, string, string, string) error { return nil },
	}
}

func getAccessLogSources(t *testing.T, s *Server) AccessLogSourcesResponse {
	t.Helper()
	rec := getAccessLog(t, s, "/api/aws/profiles/default/access-logs/sources?region=ap-northeast-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("sources status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out AccessLogSourcesResponse
	decodeAccessLogResponse(t, rec, &out)
	return out
}

func getAccessLog(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Host = "127.0.0.1:8089"
	r.Header.Set("Origin", "http://localhost:8088")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, r)
	return w
}

func postAccessLogJSON(t *testing.T, s *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	r.Host = "127.0.0.1:8089"
	r.Header.Set("Origin", "http://localhost:8088")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, r)
	return w
}

func decodeAccessLogResponse(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode response: %v, body = %s", err, rec.Body.String())
	}
}
