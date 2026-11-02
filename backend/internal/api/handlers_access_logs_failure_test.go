package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/accesslog"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestAccessLogQueryPartialFailure(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	deps := accessLogTestDeps(t)
	albSource := accesslog.Source{
		ID:              accessLogTestSourceID,
		Type:            accesslog.SourceALB,
		Name:            "prod-alb",
		Region:          "ap-northeast-1",
		QueryRegion:     "ap-northeast-1",
		Engine:          accesslog.EngineAthena,
		SchemaProfile:   accesslog.SchemaALBLegacy,
		SourceKeyValues: []string{"app/prod-alb/abc"},
		Status: accesslog.SourceStatus{
			LoggingEnabled:      true,
			DestinationType:     accesslog.DestinationS3,
			DestinationReadable: true,
			Attribution:         true,
		},
	}
	wafSource := accesslog.Source{
		ID:              "waf-source",
		Type:            accesslog.SourceWAF,
		Name:            "prod-waf",
		Region:          "us-east-1",
		QueryRegion:     "us-east-1",
		Global:          true,
		Engine:          accesslog.EngineLogsInsights,
		SchemaProfile:   accesslog.SchemaWAFJSON,
		SourceKeyValues: []string{"arn:aws:wafv2:us-east-1:123:global/webacl/prod/abc"},
		Destination:     accesslog.Destination{Type: accesslog.DestinationCloudWatchLogs, LogGroup: "aws-waf-logs-prod"},
		Status: accesslog.SourceStatus{
			LoggingEnabled:      true,
			DestinationType:     accesslog.DestinationCloudWatchLogs,
			DestinationReadable: true,
			Attribution:         true,
		},
	}
	deps.discover = func(context.Context, string, string) (awsinternal.AccessLogSources, error) {
		return awsinternal.AccessLogSources{Sources: []accesslog.Source{albSource, wafSource}}, nil
	}
	deps.getInsights = func(context.Context, string, string, string) (*awsinternal.LogsInsightsQueryResult, error) {
		return &awsinternal.LogsInsightsQueryResult{Status: "Failed"}, nil
	}
	s.accessLogs = deps
	s.registerRoutes()

	start := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query", AccessLogQueryRequest{
		View: "pairs",
		Sources: []AccessLogQuerySourceRequest{
			{ID: albSource.ID, Table: &accesslog.TableMapping{Catalog: "AwsDataCatalog", Database: "logs", Table: "alb_access_logs"}},
			{ID: wafSource.ID},
		},
		From: "2026-11-02T00:00:00Z",
		To:   "2026-11-02T01:00:00Z",
	})
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", start.Code, start.Body.String())
	}
	var started AccessLogQueryStartResponse
	decodeAccessLogResponse(t, start, &started)

	statusRec := getAccessLog(t, s, "/api/aws/profiles/default/access-logs/query/"+started.JobID)
	var status AccessLogQueryResponse
	decodeAccessLogResponse(t, statusRec, &status)
	if status.State != accesslog.QueryStatePartial {
		t.Errorf("state = %q, want partial", status.State)
	}

	resultsRec := getAccessLog(t, s, "/api/aws/profiles/default/access-logs/query/"+started.JobID+"/results")
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("results status = %d, body = %s", resultsRec.Code, resultsRec.Body.String())
	}
	var results AccessLogResultsResponse
	decodeAccessLogResponse(t, resultsRec, &results)
	if len(results.Rows) != 1 {
		t.Errorf("rows = %d, want 1", len(results.Rows))
	}
	if len(results.Sources) != 2 {
		t.Errorf("sources = %d, want 2", len(results.Sources))
	}
}

func TestAccessLogJobStoreTTL(t *testing.T) {
	store := newAccessLogJobStore()
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	store.create(&accessLogJob{ID: "job-1", CreatedAt: now})
	if _, expired, ok := store.get("job-1"); !ok || expired {
		t.Fatalf("job should exist: expired=%v ok=%v", expired, ok)
	}
	now = now.Add(accessLogJobTTL + time.Second)
	if _, expired, ok := store.get("job-1"); ok || !expired {
		t.Fatalf("job should be expired: expired=%v ok=%v", expired, ok)
	}
	if _, ok := store.jobs["job-1"]; ok {
		t.Error("expired job was not deleted")
	}
}

func TestAccessLogQueryCancel(t *testing.T) {
	s := newTestServer(t)
	s.mux = http.NewServeMux()
	deps := accessLogTestDeps(t)
	deps.startAthena = func(context.Context, string, string, awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error) {
		return &awsinternal.AthenaQueryExecution{ID: "exec-running", State: "RUNNING"}, nil
	}
	deps.getAthena = func(context.Context, string, string, string) (*awsinternal.AthenaQueryExecution, error) {
		return &awsinternal.AthenaQueryExecution{ID: "exec-running", State: "RUNNING"}, nil
	}
	stopped := false
	deps.stopAthena = func(context.Context, string, string, string) error {
		stopped = true
		return nil
	}
	s.accessLogs = deps
	s.registerRoutes()

	start := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query", AccessLogQueryRequest{
		View: "pairs",
		Sources: []AccessLogQuerySourceRequest{{
			ID:    accessLogTestSourceID,
			Table: &accesslog.TableMapping{Catalog: "AwsDataCatalog", Database: "logs", Table: "alb_access_logs"},
		}},
		From: "2026-11-02T00:00:00Z",
		To:   "2026-11-02T01:00:00Z",
	})
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", start.Code, start.Body.String())
	}
	var started AccessLogQueryStartResponse
	decodeAccessLogResponse(t, start, &started)
	cancel := postAccessLogJSON(t, s, "/api/aws/profiles/default/access-logs/query/"+started.JobID+"/cancel", nil)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, body = %s", cancel.Code, cancel.Body.String())
	}
	if !stopped {
		t.Error("stopAthena was not called")
	}
	var status AccessLogQueryResponse
	decodeAccessLogResponse(t, cancel, &status)
	if status.State != accesslog.QueryStateCancelled {
		t.Errorf("state = %q, want cancelled", status.State)
	}
}
