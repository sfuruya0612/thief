package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// CostResource represents a line item in Cost Explorer results.
type CostResource struct {
	TimePeriod         string  `json:"time_period"`
	Service            string  `json:"service"`
	UnblendedAmount    float64 `json:"unblended_amount"`
	NetAmortizedAmount float64 `json:"net_amortized_amount"`
	Unit               string  `json:"unit"`
}

func (r CostResource) ResourceID() string    { return r.TimePeriod + "/" + r.Service }
func (r CostResource) ResourceName() string  { return r.Service }
func (r CostResource) ResourceState() string { return "active" }
func (r CostResource) ServiceName() string   { return "cost" }

// ForecastResource represents a cost forecast entry.
type ForecastResource struct {
	TimePeriod string  `json:"time_period"`
	Amount     float64 `json:"amount"`
	Unit       string  `json:"unit"`
}

// GetCost returns daily cost by service for the given date range.
// If includeToday is false, the end date is yesterday.
func GetCost(ctx context.Context, profile, region string, includeToday bool) ([]CostResource, error) {
	// Cost Explorer is a global service; us-east-1 is the standard endpoint.
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *costexplorer.Client {
		return costexplorer.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	end := now.Format("2006-01-02")
	if !includeToday {
		end = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	start := now.AddDate(0, -1, 0).Format("2006-01-02")

	out, err := client.GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"UnblendedCost", "NetAmortizedCost"},
		GroupBy: []cetypes.GroupDefinition{
			{Type: cetypes.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get cost and usage: %w", err)
	}

	var resources []CostResource
	for _, result := range out.ResultsByTime {
		period := ""
		if result.TimePeriod != nil {
			period = ptrStr(result.TimePeriod.Start)
		}
		for _, group := range result.Groups {
			service := ""
			if len(group.Keys) > 0 {
				service = group.Keys[0]
			}
			unblended := 0.0
			netAmortized := 0.0
			unit := ""
			if m, ok := group.Metrics["UnblendedCost"]; ok {
				if m.Amount != nil {
					fmt.Sscanf(*m.Amount, "%f", &unblended)
				}
				unit = ptrStr(m.Unit)
			}
			if m, ok := group.Metrics["NetAmortizedCost"]; ok {
				if m.Amount != nil {
					fmt.Sscanf(*m.Amount, "%f", &netAmortized)
				}
				if unit == "" {
					unit = ptrStr(m.Unit)
				}
			}
			resources = append(resources, CostResource{
				TimePeriod:         period,
				Service:            service,
				UnblendedAmount:    unblended,
				NetAmortizedAmount: netAmortized,
				Unit:               unit,
			})
		}
	}
	return resources, nil
}

// GetForecast returns the cost forecast for the current month.
func GetForecast(ctx context.Context, profile, _ string) ([]ForecastResource, error) {
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *costexplorer.Client {
		return costexplorer.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	start := now.Format("2006-01-02")
	// End of current month.
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	end := nextMonth.Format("2006-01-02")

	out, err := client.GetCostForecast(ctx, &costexplorer.GetCostForecastInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: cetypes.GranularityMonthly,
		Metric:      cetypes.MetricBlendedCost,
	})
	if err != nil {
		return nil, fmt.Errorf("get cost forecast: %w", err)
	}

	var resources []ForecastResource
	for _, result := range out.ForecastResultsByTime {
		period := ""
		if result.TimePeriod != nil {
			period = ptrStr(result.TimePeriod.Start)
		}
		amount := 0.0
		unit := ""
		if result.MeanValue != nil {
			fmt.Sscanf(*result.MeanValue, "%f", &amount)
		}
		if out.Total != nil {
			unit = ptrStr(out.Total.Unit)
		}
		resources = append(resources, ForecastResource{
			TimePeriod: period,
			Amount:     amount,
			Unit:       unit,
		})
	}
	return resources, nil
}
