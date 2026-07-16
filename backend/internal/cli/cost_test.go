package cli

import (
	"testing"
	"time"

	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/google/go-cmp/cmp"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
)

func TestParseGranularity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    cetypes.Granularity
		wantErr bool
	}{
		{name: "monthly", input: "MONTHLY", want: cetypes.GranularityMonthly},
		{name: "daily", input: "DAILY", want: cetypes.GranularityDaily},
		{name: "lowercase monthly", input: "monthly", want: cetypes.GranularityMonthly},
		{name: "lowercase daily", input: "daily", want: cetypes.GranularityDaily},
		{name: "unsupported", input: "HOURLY", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGranularity(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseGranularity(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseGranularity(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveDates(t *testing.T) {
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		startDate   string
		endDate     string
		granularity cetypes.Granularity
		wantStart   string
		wantEnd     string
	}{
		{
			name:        "both specified",
			startDate:   "2026-01-01",
			endDate:     "2026-02-01",
			granularity: cetypes.GranularityMonthly,
			wantStart:   "2026-01-01",
			wantEnd:     "2026-02-01",
		},
		{
			name:        "monthly defaults to 3 months back",
			granularity: cetypes.GranularityMonthly,
			wantStart:   "2026-05-01",
			wantEnd:     "2026-07-16",
		},
		{
			name:        "daily defaults to first day of current month",
			granularity: cetypes.GranularityDaily,
			wantStart:   "2026-07-01",
			wantEnd:     "2026-07-16",
		},
		{
			name:        "only end specified",
			endDate:     "2026-07-10",
			granularity: cetypes.GranularityDaily,
			wantStart:   "2026-07-01",
			wantEnd:     "2026-07-10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd := resolveDates(tt.startDate, tt.endDate, tt.granularity, now)
			if gotStart != tt.wantStart || gotEnd != tt.wantEnd {
				t.Errorf("resolveDates() = (%q, %q), want (%q, %q)", gotStart, gotEnd, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestBuildCostMatrix(t *testing.T) {
	costs := []awsinternal.CostDetail{
		{TimePeriod: "2026-06-01", Amount: "10.5", Unit: "USD", ServiceName: "Amazon EC2"},
		{TimePeriod: "2026-07-01", Amount: "20.5", Unit: "USD", ServiceName: "Amazon EC2"},
		{TimePeriod: "2026-07-01", Amount: "100", Unit: "USD", ServiceName: "Amazon RDS"},
	}

	columns, rows := buildCostMatrix(costs, "Service", func(c awsinternal.CostDetail) string { return c.ServiceName })

	wantColumns := []util.Column{
		{Header: "Service"},
		{Header: "Unit"},
		{Header: "2026-06-01"},
		{Header: "2026-07-01"},
	}
	if diff := cmp.Diff(wantColumns, columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}

	// 合計金額の降順: RDS (100) > EC2 (31)。期間欠損は "0" で埋める。
	wantRows := [][]string{
		{"Amazon RDS", "USD", "0", "100"},
		{"Amazon EC2", "USD", "10.5", "20.5"},
	}
	if diff := cmp.Diff(wantRows, rows); diff != "" {
		t.Errorf("rows mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildCostMatrix_Empty(t *testing.T) {
	columns, rows := buildCostMatrix(nil, "Overview", func(c awsinternal.CostDetail) string { return "Total" })

	wantColumns := []util.Column{{Header: "Overview"}, {Header: "Unit"}}
	if diff := cmp.Diff(wantColumns, columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want empty", rows)
	}
}

func TestParseGranularityErrorMessage(t *testing.T) {
	_, err := parseGranularity("WEEKLY")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := `unsupported granularity "WEEKLY": use MONTHLY or DAILY`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
