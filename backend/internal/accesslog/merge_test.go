package accesslog

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func ptrTime(t time.Time) *time.Time { return &t }

func TestMergeWAFOverlay(t *testing.T) {
	from := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	sources := []Source{
		{ID: "alb-a", Type: SourceALB, Name: "alb-a"},
		{ID: "waf-a", Type: SourceWAF, Name: "waf-a"},
	}
	results := []SourceResult{
		{
			SourceID: "alb-a",
			Rows: []PairRow{{
				ClientIP:         "203.0.113.9",
				URIPath:          "/admin",
				RequestCount:     10,
				BlockedCount:     1,
				ClientErrorCount: 2,
				FirstSeen:        ptrTime(from),
				LastSeen:         ptrTime(to),
				SampleUserAgent:  "curl/8",
			}},
		},
		{
			SourceID: "waf-a",
			Rows: []PairRow{{
				ClientIP:        "203.0.113.9",
				URIPath:         "/admin",
				RequestCount:    10,
				BlockedCount:    3,
				ChallengedCount: 1,
				CountMatched:    2,
				SampleRuleID:    "AWSManagedRulesCommonRuleSet",
				FirstSeen:       ptrTime(from),
				LastSeen:        ptrTime(to),
			}},
		},
	}
	got := Merge(results, sources)
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.Rows))
	}
	row := got.Rows[0]
	if row.RequestCount != 10 {
		t.Errorf("request count = %d, want 10", row.RequestCount)
	}
	if row.BlockedCount != 3 {
		t.Errorf("blocked = %d, want 3", row.BlockedCount)
	}
	if row.ChallengedCount != 1 {
		t.Errorf("challenged = %d, want 1", row.ChallengedCount)
	}
	if row.CountMatched != 2 {
		t.Errorf("count matched = %d, want 2", row.CountMatched)
	}
	if row.ClientErrorCount != 2 {
		t.Errorf("client errors = %d, want 2", row.ClientErrorCount)
	}
	if diff := cmp.Diff([]string{"alb-a", "waf-a"}, row.Sources); diff != "" {
		t.Errorf("sources mismatch (-want +got):\n%s", diff)
	}
	if row.SourceCounts["alb-a"] != 10 || row.SourceCounts["waf-a"] != 10 {
		t.Errorf("source counts = %v", row.SourceCounts)
	}
	if row.SampleRuleID != "AWSManagedRulesCommonRuleSet" {
		t.Errorf("rule id = %q", row.SampleRuleID)
	}
	if !containsString(row.Flags, "blocked") || !containsString(row.Flags, "shared") {
		t.Errorf("flags = %v", row.Flags)
	}
	if !got.Summary.DuplicateWarning {
		t.Error("duplicate warning = false, want true")
	}
}

func TestMergeWAFOnlyUsesWAFCount(t *testing.T) {
	got := Merge([]SourceResult{{
		SourceID: "waf-a",
		Rows: []PairRow{{
			ClientIP:     "198.51.100.7",
			URIPath:      "/.env",
			RequestCount: 5,
			BlockedCount: 5,
		}},
	}}, []Source{{ID: "waf-a", Type: SourceWAF}})
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.Rows))
	}
	if got.Rows[0].RequestCount != 5 {
		t.Errorf("request count = %d, want 5", got.Rows[0].RequestCount)
	}
	if !containsString(got.Rows[0].Flags, "sensitive_path") {
		t.Errorf("flags = %v", got.Rows[0].Flags)
	}
}

func TestMergeExcludesOriginALB(t *testing.T) {
	sources := []Source{
		{ID: "cf-a", Type: SourceCloudFront, Name: "cf-a"},
		{ID: "alb-a", Type: SourceALB, Name: "alb-a", OriginOf: "cf-a"},
	}
	results := []SourceResult{
		{
			SourceID: "cf-a",
			Rows: []PairRow{{
				ClientIP:     "203.0.113.9",
				URIPath:      "/",
				RequestCount: 10,
			}},
		},
		{
			SourceID: "alb-a",
			Rows: []PairRow{{
				ClientIP:         "198.51.100.1",
				URIPath:          "/",
				RequestCount:     10,
				ClientErrorCount: 4,
			}},
		},
	}
	got := Merge(results, sources)
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.Rows))
	}
	if got.Rows[0].ClientIP != "203.0.113.9" {
		t.Errorf("client ip = %q, want the CloudFront viewer", got.Rows[0].ClientIP)
	}
	if got.Summary.RecordCount != 10 {
		t.Errorf("record count = %d, want 10", got.Summary.RecordCount)
	}
	if len(got.Summary.BySources) != 2 {
		t.Errorf("source summary = %d, want 2", len(got.Summary.BySources))
	}
	if !containsString(got.Warnings, "origin ALB sources are excluded from the client IP aggregation because CloudFront is the edge") {
		t.Errorf("warnings = %v", got.Warnings)
	}
}

func TestMergeSharedALB(t *testing.T) {
	sources := []Source{
		{ID: "alb-a", Type: SourceALB},
		{ID: "alb-b", Type: SourceALB},
	}
	results := []SourceResult{
		{SourceID: "alb-a", Rows: []PairRow{{ClientIP: "203.0.113.9", URIPath: "/api", RequestCount: 10}}},
		{SourceID: "alb-b", Rows: []PairRow{{ClientIP: "203.0.113.9", URIPath: "/api", RequestCount: 5}}},
	}
	got := Merge(results, sources)
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.Rows))
	}
	if got.Rows[0].RequestCount != 15 {
		t.Errorf("request count = %d, want 15", got.Rows[0].RequestCount)
	}
	if diff := cmp.Diff([]string{"alb-a", "alb-b"}, got.Rows[0].Sources); diff != "" {
		t.Errorf("sources mismatch (-want +got):\n%s", diff)
	}
	if !containsString(got.Rows[0].Flags, "shared") {
		t.Errorf("flags = %v", got.Rows[0].Flags)
	}
}

func TestMergeIPSummariesStayPerSource(t *testing.T) {
	sources := []Source{{ID: "alb-a", Type: SourceALB}, {ID: "alb-b", Type: SourceALB}}
	results := []SourceResult{
		{SourceID: "alb-a", IPs: []IPSummary{{ClientIP: "203.0.113.9", RequestCount: 10, DistinctURIs: 3}}},
		{SourceID: "alb-b", IPs: []IPSummary{{ClientIP: "203.0.113.9", RequestCount: 5, DistinctURIs: 2}}},
	}
	got := Merge(results, sources)
	if len(got.IPs) != 2 {
		t.Fatalf("ip rows = %d, want 2", len(got.IPs))
	}
	if got.IPs[0].Sources[0] == got.IPs[1].Sources[0] {
		t.Errorf("ip rows were merged across sources: %v", got.IPs)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
