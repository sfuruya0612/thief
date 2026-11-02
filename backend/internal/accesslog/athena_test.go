package accesslog

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testTimeRange() TimeRange {
	return TimeRange{
		From: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC),
	}
}

func TestBuildPairQueryALB(t *testing.T) {
	plan := AthenaPlan{
		Database:        "logs",
		Table:           "alb_access_logs",
		Profile:         SchemaALBLegacy,
		Columns:         []string{"type", "time", "elb", "client_ip", "request_url", "elb_status_code", "actions_executed", "user_agent"},
		PartitionStyle:  PartitionDay,
		PartitionKeys:   []string{"day"},
		SourceKeyColumn: "elb",
		Sources: []AthenaSource{
			{ID: "alb-a", KeyValues: []string{"app/prod-a/1"}},
			{ID: "alb-b", KeyValues: []string{"app/prod-b/2"}},
		},
		TimeRange: testTimeRange(),
		Filter: Filter{
			IP:            "203.0.113.9",
			URIPrefix:     "/admin",
			StatusClasses: []string{"4xx", "5xx"},
		},
		Limit: 500,
	}
	got, err := BuildPairQuery(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"logs"."alb_access_logs"`,
		`"day" BETWEEN ? AND ?`,
		`CAST("elb" AS varchar) IN (?, ?)`,
		`strpos(regexp_extract("request_url"`,
		`"elb_status_code" BETWEEN 400 AND 499`,
		`"elb_status_code" >= 500`,
		`LIMIT 200`,
		`max_by(user_agent, ts_ms)`,
	} {
		if !strings.Contains(got.SQL, want) {
			t.Errorf("SQL does not contain %q:\n%s", want, got.SQL)
		}
	}
	wantParams := []string{
		"2026/11/01", "2026/11/03",
		strconv.FormatInt(testTimeRange().From.UnixMilli(), 10),
		strconv.FormatInt(testTimeRange().To.UnixMilli(), 10),
		"app/prod-a/1", "app/prod-b/2",
		"203.0.113.9", "/admin",
	}
	if len(got.Parameters) != len(wantParams) {
		t.Fatalf("params = %v, want %v", got.Parameters, wantParams)
	}
	for i := range wantParams {
		if got.Parameters[i] != wantParams[i] {
			t.Errorf("param[%d] = %q, want %q", i, got.Parameters[i], wantParams[i])
		}
	}
	if got.FullScan {
		t.Error("full scan = true, want false")
	}
	if got.SourceKeyToID["app/prod-a/1"] != "alb-a" || got.SourceKeyToID["app/prod-b/2"] != "alb-b" {
		t.Errorf("source map = %v", got.SourceKeyToID)
	}
}

func TestBuildIPQueryWAF(t *testing.T) {
	plan := AthenaPlan{
		Database:        "waf",
		Table:           "waf_logs",
		Profile:         SchemaWAFJSON,
		Columns:         []string{"timestamp", "webaclid", "terminatingruleid", "action", "httprequest"},
		PartitionStyle:  PartitionLogTime,
		PartitionKeys:   []string{"log_time"},
		SourceKeyColumn: "webaclid",
		Sources:         []AthenaSource{{ID: "waf-a", KeyValues: []string{"arn:aws:wafv2:us-east-1:123:global/webacl/a"}}},
		TimeRange:       testTimeRange(),
		Filter:          Filter{OnlyBlocked: true},
	}
	got, err := BuildIPQuery(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`httprequest.clientip`,
		`upper(action) = 'BLOCK'`,
		`count(DISTINCT uri_path) AS distinct_uris`,
	} {
		if !strings.Contains(got.SQL, want) {
			t.Errorf("SQL does not contain %q:\n%s", want, got.SQL)
		}
	}
	if !strings.Contains(got.SQL, `"log_time" BETWEEN ? AND ?`) {
		t.Errorf("SQL does not contain log_time predicate:\n%s", got.SQL)
	}
	if !strings.Contains(got.SQL, `CAST("webaclid" AS varchar) IN (?)`) {
		t.Errorf("SQL does not contain webaclid filter:\n%s", got.SQL)
	}
}

func TestBuildPairQueryCloudFrontV2(t *testing.T) {
	plan := AthenaPlan{
		Database:        "cf",
		Table:           "cloudfront_logs",
		Profile:         SchemaCloudFrontV2JSON,
		Columns:         []string{"date", "time", "c-ip", "cs-uri-stem", "sc-status", "cs(User-Agent)", "distributionid"},
		PartitionStyle:  PartitionYearMonthDayHour,
		PartitionKeys:   []string{"year", "month", "day", "hour"},
		SourceKeyColumn: "distributionid",
		Sources:         []AthenaSource{{ID: "cf-a", KeyValues: []string{"E123"}}},
		TimeRange:       testTimeRange(),
	}
	got, err := BuildPairQuery(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"c-ip"`,
		`"cs-uri-stem"`,
		`CAST("sc-status" AS int)`,
		`CAST("distributionid" AS varchar) IN (?)`,
		`"year" = 2026 AND "month" = 11 AND "day" = 1`,
		`"year" = 2026 AND "month" = 11 AND "day" = 3`,
	} {
		if !strings.Contains(got.SQL, want) {
			t.Errorf("SQL does not contain %q:\n%s", want, got.SQL)
		}
	}
}

func TestBuildPairQuerySharedALB(t *testing.T) {
	plan := AthenaPlan{
		Database:        "logs",
		Table:           "shared_alb_logs",
		Profile:         SchemaALBLegacy,
		Columns:         []string{"elb", "client_ip", "request_url", "elb_status_code", "actions_executed", "user_agent", "time"},
		PartitionStyle:  PartitionDay,
		PartitionKeys:   []string{"day"},
		SourceKeyColumn: "elb",
		Sources: []AthenaSource{
			{ID: "alb-a", KeyValues: []string{"app/a/1"}},
			{ID: "alb-b", KeyValues: []string{"app/b/2"}},
		},
		TimeRange: testTimeRange(),
	}
	got, err := BuildPairQuery(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.SQL, `CAST("elb" AS varchar) IN (?, ?)`) {
		t.Errorf("source filter is missing:\n%s", got.SQL)
	}
	if !strings.Contains(got.SQL, `GROUP BY source_key, client_ip, uri_path`) {
		t.Errorf("source key is not grouped:\n%s", got.SQL)
	}
	if got.SourceKeyToID["app/a/1"] != "alb-a" || got.SourceKeyToID["app/b/2"] != "alb-b" {
		t.Errorf("source map = %v", got.SourceKeyToID)
	}
}

func TestBuildPairQueryWithoutSourceKey(t *testing.T) {
	plan := AthenaPlan{
		Database:  "logs",
		Table:     "single_alb",
		Profile:   SchemaALBLegacy,
		Columns:   []string{"elb", "client_ip", "request_url", "elb_status_code", "time", "actions_executed", "user_agent"},
		Sources:   []AthenaSource{{ID: "alb-a"}},
		TimeRange: testTimeRange(),
	}
	got, err := BuildPairQuery(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.SQL, "'' AS source_key") {
		t.Errorf("SQL does not use an empty source key:\n%s", got.SQL)
	}
	if !got.FullScan {
		t.Error("full scan = false, want true")
	}
}

func TestBuildPairQueryAttributionRequired(t *testing.T) {
	plan := AthenaPlan{
		Database:        "logs",
		Table:           "shared",
		Profile:         SchemaALBLegacy,
		Columns:         []string{"elb", "client_ip", "request_url", "elb_status_code", "time", "actions_executed", "user_agent"},
		SourceKeyColumn: "elb",
		Sources:         []AthenaSource{{ID: "alb-a"}},
		TimeRange:       testTimeRange(),
	}
	_, err := BuildPairQuery(plan)
	if !errors.Is(err, ErrAttributionRequired) {
		t.Fatalf("err = %v, want ErrAttributionRequired", err)
	}
}

func TestBuildPairQueryInvalidTable(t *testing.T) {
	plan := AthenaPlan{
		Database:  "logs; DROP TABLE x",
		Table:     "alb",
		Profile:   SchemaALBLegacy,
		TimeRange: testTimeRange(),
	}
	_, err := BuildPairQuery(plan)
	if !errors.Is(err, ErrInvalidTable) {
		t.Fatalf("err = %v, want ErrInvalidTable", err)
	}
}

func TestBuildPairQueryInvalidTimeRange(t *testing.T) {
	plan := AthenaPlan{
		Database:  "logs",
		Table:     "alb",
		Profile:   SchemaALBLegacy,
		TimeRange: TimeRange{From: testTimeRange().To, To: testTimeRange().From},
	}
	_, err := BuildPairQuery(plan)
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("err = %v, want ErrInvalidTimeRange", err)
	}
}

func TestBuildWAFInsightsQuery(t *testing.T) {
	got, err := BuildWAFInsightsQuery(InsightsPlan{
		LogGroups: []string{"aws-waf-logs-prod"},
		Sources:   []AthenaSource{{ID: "waf-a", KeyValues: []string{"arn:a"}}},
		TimeRange: testTimeRange(),
		Filter:    Filter{IP: `203.0.113.9"`, URIPrefix: `\admin`},
		Limit:     500,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`httpRequest.clientIp as ip`,
		`httpRequest.uri as uri`,
		`webaclId`,
		`stats count(*)`,
		`limit 200`,
		`httpRequest.clientIp = "203.0.113.9\""`,
		`strcontains(httpRequest.uri, "\\admin")`,
		`webaclId = "arn:a"`,
	} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query does not contain %q:\n%s", want, got.Query)
		}
	}
	if got.SourceKeyToID["arn:a"] != "waf-a" {
		t.Errorf("source map = %v", got.SourceKeyToID)
	}
}

func TestBuildWAFInsightsQuerySharedLogGroup(t *testing.T) {
	got, err := BuildWAFInsightsQuery(InsightsPlan{
		LogGroups: []string{"aws-waf-logs-shared"},
		Sources: []AthenaSource{
			{ID: "waf-a", KeyValues: []string{"arn:a"}},
			{ID: "waf-b", KeyValues: []string{"arn:b"}},
		},
		TimeRange: testTimeRange(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Query, `webaclId = "arn:a" or webaclId = "arn:b"`) {
		t.Errorf("source filter is missing:\n%s", got.Query)
	}
	if !strings.Contains(got.Query, `by ip, uri, action, webaclId`) {
		t.Errorf("webaclId is not grouped:\n%s", got.Query)
	}
}

func TestBuildWAFInsightsQueryNoLogGroup(t *testing.T) {
	_, err := BuildWAFInsightsQuery(InsightsPlan{TimeRange: testTimeRange()})
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("err = %v, want ErrInvalidSource", err)
	}
}
