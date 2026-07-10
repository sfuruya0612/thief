package aws

import (
	"testing"
	"time"

	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

func TestCostGranularity(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want cetypes.Granularity
	}{
		{name: "monthly", in: "MONTHLY", want: cetypes.GranularityMonthly},
		{name: "daily", in: "DAILY", want: cetypes.GranularityDaily},
		{name: "empty defaults to daily", in: "", want: cetypes.GranularityDaily},
		{name: "unknown defaults to daily", in: "HOURLY", want: cetypes.GranularityDaily},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := costGranularity(tt.in)
			if got != tt.want {
				t.Errorf("costGranularity(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCostGroupByDimension(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "usage type", in: CostGroupByUsageType, want: CostGroupByUsageType},
		{name: "linked account", in: CostGroupByLinkedAccount, want: CostGroupByLinkedAccount},
		{name: "region", in: CostGroupByRegion, want: CostGroupByRegion},
		{name: "service", in: CostGroupByService, want: CostGroupByService},
		{name: "empty defaults to service", in: "", want: CostGroupByService},
		{name: "unknown defaults to service", in: "AZ", want: CostGroupByService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := costGroupByDimension(tt.in)
			if got != tt.want {
				t.Errorf("costGroupByDimension(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCostDateRange(t *testing.T) {
	t.Run("StartDate/EndDate 両方指定時はそれを優先する", func(t *testing.T) {
		start, end := costDateRange(CostQueryOptions{
			StartDate: "2026-01-01",
			EndDate:   "2026-01-31",
			Months:    6, // 優先されないことを確認する
		})
		if start != "2026-01-01" || end != "2026-01-31" {
			t.Errorf("costDateRange() = (%q, %q), want (2026-01-01, 2026-01-31)", start, end)
		}
	})

	t.Run("StartDate のみでは Months ベースにフォールバックする", func(t *testing.T) {
		_, end := costDateRange(CostQueryOptions{StartDate: "2026-01-01"})
		wantEnd := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		if end != wantEnd {
			t.Errorf("end = %q, want %q (StartDate だけでは無効)", end, wantEnd)
		}
	})

	t.Run("IncludeToday が false の場合 end は前日になる", func(t *testing.T) {
		_, end := costDateRange(CostQueryOptions{IncludeToday: false})
		want := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		if end != want {
			t.Errorf("end = %q, want %q", end, want)
		}
	})

	t.Run("IncludeToday が true の場合 end は今日になる", func(t *testing.T) {
		_, end := costDateRange(CostQueryOptions{IncludeToday: true})
		want := time.Now().UTC().Format("2006-01-02")
		if end != want {
			t.Errorf("end = %q, want %q", end, want)
		}
	})

	t.Run("Months が 0 以下ならデフォルト 1 ヶ月遡る", func(t *testing.T) {
		start, _ := costDateRange(CostQueryOptions{Months: -1})
		want := time.Now().UTC().AddDate(0, -1, 0).Format("2006-01-02")
		if start != want {
			t.Errorf("start = %q, want %q", start, want)
		}
	})

	t.Run("Months 指定分だけ遡る", func(t *testing.T) {
		start, _ := costDateRange(CostQueryOptions{Months: 3})
		want := time.Now().UTC().AddDate(0, -3, 0).Format("2006-01-02")
		if start != want {
			t.Errorf("start = %q, want %q", start, want)
		}
	})
}
