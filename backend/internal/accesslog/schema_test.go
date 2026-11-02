package accesslog

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDetectProfile(t *testing.T) {
	tests := []struct {
		name    string
		columns []string
		want    SchemaProfile
		ok      bool
	}{
		{
			name:    "alb",
			columns: []string{"type", "time", "elb", "client_ip", "request_url", "elb_status_code", "actions_executed", "user_agent"},
			want:    SchemaALBLegacy,
			ok:      true,
		},
		{
			name:    "cloudfront legacy",
			columns: []string{"date", "time", "c_ip", "cs_uri_stem", "sc_status", "cs_user_agent", "cs_host"},
			want:    SchemaCloudFrontLegacy,
			ok:      true,
		},
		{
			name:    "cloudfront v2",
			columns: []string{"date", "time", "c-ip", "cs-uri-stem", "sc-status", "cs(User-Agent)", "x-host-header"},
			want:    SchemaCloudFrontV2JSON,
			ok:      true,
		},
		{
			name:    "waf",
			columns: []string{"timestamp", "webaclid", "terminatingruleid", "action", "httprequest"},
			want:    SchemaWAFJSON,
			ok:      true,
		},
		{
			name:    "unknown",
			columns: []string{"foo", "bar"},
			ok:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DetectProfile(tt.columns)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("profile = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMissingColumns(t *testing.T) {
	missing := MissingColumns(SchemaWAFJSON, []string{"timestamp", "action", "httprequest"})
	want := []string{"webaclid", "terminatingruleid"}
	if diff := cmp.Diff(want, missing); diff != "" {
		t.Errorf("missing columns mismatch (-want +got):\n%s", diff)
	}
}

func TestFindSourceKeyColumn(t *testing.T) {
	tests := []struct {
		name    string
		profile SchemaProfile
		columns []string
		want    string
	}{
		{name: "alb", profile: SchemaALBLegacy, columns: []string{"ELB"}, want: "ELB"},
		{name: "cloudfront legacy host", profile: SchemaCloudFrontLegacy, columns: []string{"cs_host"}, want: "cs_host"},
		{name: "cloudfront v2 distribution", profile: SchemaCloudFrontV2JSON, columns: []string{"distributionid", "c-ip"}, want: "distributionid"},
		{name: "waf", profile: SchemaWAFJSON, columns: []string{"webaclid"}, want: "webaclid"},
		{name: "none", profile: SchemaALBLegacy, columns: []string{"client_ip"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FindSourceKeyColumn(tt.profile, tt.columns); got != tt.want {
				t.Errorf("source key column = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectPartitionStyle(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want PartitionStyle
	}{
		{name: "alb", keys: []string{"day"}, want: PartitionDay},
		{name: "waf log time", keys: []string{"log_time"}, want: PartitionLogTime},
		{name: "cloudfront", keys: []string{"year", "month", "day", "hour"}, want: PartitionYearMonthDayHour},
		{name: "none", keys: nil, want: PartitionNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectPartitionStyle(tt.keys); got != tt.want {
				t.Errorf("partition style = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProfileForUnknown(t *testing.T) {
	_, err := ProfileFor("unknown")
	if !errors.Is(err, ErrUnsupportedProfile) {
		t.Fatalf("err = %v, want ErrUnsupportedProfile", err)
	}
}

func TestProfileForColumnsUsesActualNames(t *testing.T) {
	p, err := ProfileForColumns(SchemaCloudFrontV2JSON, []string{"C-IP", "CS-URI-STEM", "SC-STATUS", "DATE", "TIME", "CS(USER-AGENT)"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ClientIPExpr != `"C-IP"` {
		t.Errorf("client ip expr = %q", p.ClientIPExpr)
	}
	if p.StatusExpr != `CAST("SC-STATUS" AS int)` {
		t.Errorf("status expr = %q", p.StatusExpr)
	}
	if !slices.Contains([]string{"c-ip", "C-IP"}, findColumn([]string{"C-IP"}, "c-ip")) {
		t.Errorf("findColumn did not return the actual name")
	}
}
