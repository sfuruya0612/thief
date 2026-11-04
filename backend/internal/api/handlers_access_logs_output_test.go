package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/accesslog"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func newAccessLogOutputTestServer(t *testing.T, workgroup *awsinternal.AthenaWorkgroupConfig, start func(context.Context, string, string, awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error)) *Server {
	t.Helper()
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	deps := accessLogTestDeps(t)
	deps.workgroup = func(context.Context, string, string, string) (*awsinternal.AthenaWorkgroupConfig, error) {
		return workgroup, nil
	}
	deps.startAthena = start
	s.accessLogs = deps
	s.registerRoutes()
	return s
}

func accessLogOutputRequest() AccessLogQueryRequest {
	return AccessLogQueryRequest{
		View: "pairs",
		Sources: []AccessLogQuerySourceRequest{{
			ID:    accessLogTestSourceID,
			Table: &accesslog.TableMapping{Catalog: "AwsDataCatalog", Database: "logs", Table: "alb_access_logs"},
		}},
		From: "2026-11-02T00:00:00Z",
		To:   "2026-11-02T01:00:00Z",
	}
}

func TestAccessLogQueryRequiresOutputLocation(t *testing.T) {
	started := false
	s := newAccessLogOutputTestServer(t,
		&awsinternal.AthenaWorkgroupConfig{Name: "primary"},
		func(context.Context, string, string, awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error) {
			started = true
			return &awsinternal.AthenaQueryExecution{ID: "exec-1", State: "QUEUED"}, nil
		},
	)
	rec := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query", accessLogOutputRequest())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ACCESS_LOGS_OUTPUT_LOCATION_REQUIRED") {
		t.Errorf("body = %s, want ACCESS_LOGS_OUTPUT_LOCATION_REQUIRED", rec.Body.String())
	}
	if started {
		t.Error("Athena query was started without an output location")
	}
}

func TestAccessLogQueryManagedQueryResults(t *testing.T) {
	var outputLocation string
	s := newAccessLogOutputTestServer(t,
		&awsinternal.AthenaWorkgroupConfig{Name: "primary", ManagedQueryResults: true},
		func(_ context.Context, _, _ string, in awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error) {
			outputLocation = in.OutputLocation
			return &awsinternal.AthenaQueryExecution{ID: "exec-1", State: "SUCCEEDED"}, nil
		},
	)
	rec := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query", accessLogOutputRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if outputLocation != "" {
		t.Errorf("output location = %q, want empty for managed query results", outputLocation)
	}
}

func TestAccessLogQueryUsesRequestOutputLocation(t *testing.T) {
	var outputLocation string
	s := newAccessLogOutputTestServer(t,
		&awsinternal.AthenaWorkgroupConfig{Name: "primary"},
		func(_ context.Context, _, _ string, in awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error) {
			outputLocation = in.OutputLocation
			return &awsinternal.AthenaQueryExecution{ID: "exec-1", State: "SUCCEEDED"}, nil
		},
	)
	req := accessLogOutputRequest()
	req.OutputLocation = "s3://request-results/"
	rec := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query", req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if outputLocation != "s3://request-results/" {
		t.Errorf("output location = %q, want the request output location", outputLocation)
	}
}
