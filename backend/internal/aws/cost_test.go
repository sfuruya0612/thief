package aws

import (
	"testing"

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
