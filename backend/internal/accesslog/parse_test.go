package accesslog

import (
	"testing"
	"time"
)

func TestParseAthenaPairRows(t *testing.T) {
	columns := []string{"source_key", "client_ip", "uri_path", "request_count", "blocked_count", "challenged_count", "count_matched", "client_error_count", "server_error_count", "first_seen_ms", "last_seen_ms", "sample_user_agent", "sample_rule_id", "has_query", "sample_query"}
	rows := [][]string{{
		"app/prod/1", "203.0.113.9", "/admin", "10", "3", "1", "2", "4", "5", "1762000000000", "1762003600000", "curl/8", "rule-a", "1", "q=1",
	}}
	pairs, ips, err := ParseAthenaRows(columns, rows, "pairs", map[string]string{"app/prod/1": "alb-a"}, "alb-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 0 || len(pairs) != 1 {
		t.Fatalf("pairs = %d, ips = %d", len(pairs), len(ips))
	}
	pair := pairs[0]
	if pair.SourceID != "alb-a" || pair.RequestCount != 10 || pair.BlockedCount != 3 {
		t.Errorf("pair = %+v", pair)
	}
	if pair.FirstSeen == nil || !pair.FirstSeen.Equal(time.UnixMilli(1762000000000)) {
		t.Errorf("first seen = %v", pair.FirstSeen)
	}
	if !pair.HasQuery || pair.SampleQuery != "q=1" {
		t.Errorf("query = %v %q", pair.HasQuery, pair.SampleQuery)
	}
}

func TestParseAthenaIPRows(t *testing.T) {
	columns := []string{"source_key", "client_ip", "request_count", "blocked_count", "challenged_count", "count_matched", "client_error_count", "server_error_count", "distinct_uris", "first_seen_ms", "last_seen_ms"}
	rows := [][]string{{"", "198.51.100.7", "20", "1", "0", "0", "2", "0", "7", "1762000000000", "1762003600000"}}
	_, ips, err := ParseAthenaRows(columns, rows, "ips", nil, "alb-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 {
		t.Fatalf("ips = %d, want 1", len(ips))
	}
	if ips[0].SourceID != "alb-a" || ips[0].DistinctURIs != 7 || ips[0].RequestCount != 20 {
		t.Errorf("ip = %+v", ips[0])
	}
}

func TestParseInsightsRows(t *testing.T) {
	results := []map[string]string{
		{"webaclId": "arn:a", "ip": "203.0.113.9", "uri": "/admin", "action": "BLOCK", "request_count": "3", "first_seen_ms": "1762000000000", "last_seen_ms": "1762003600000"},
		{"webaclId": "arn:a", "ip": "203.0.113.9", "uri": "/admin", "action": "ALLOW", "request_count": "7", "first_seen_ms": "1762000000000", "last_seen_ms": "1762003600000"},
		{"webaclId": "arn:a", "ip": "203.0.113.9", "uri": "/admin", "action": "CAPTCHA", "request_count": "1", "first_seen_ms": "1762000000000", "last_seen_ms": "1762003600000"},
	}
	rows := ParseInsightsRows(results, map[string]string{"arn:a": "waf-a"}, "waf-a")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.SourceID != "waf-a" || row.RequestCount != 11 || row.BlockedCount != 3 || row.ChallengedCount != 1 {
		t.Errorf("row = %+v", row)
	}
}

func TestSplitSourceResults(t *testing.T) {
	results := SplitSourceResults(
		[]PairRow{{SourceID: "a", RequestCount: 1}, {SourceID: "b", RequestCount: 2}},
		[]IPSummary{{SourceID: "a", ClientIP: "1.1.1.1"}},
		100, 10,
	)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for _, result := range results {
		if result.BytesScanned != 100 || result.RecordsScanned != 10 {
			t.Errorf("stats = %+v", result)
		}
	}
}
