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
// GroupKey は GroupByDimension で指定した次元の値 (デフォルトはサービス名) を保持する。
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

// CostGroupByDimension は GetCost の GroupBy 次元として許可する値。
// AWS Cost Explorer が対応する Dimension のうち、コスト可視化で使う頻度が高いものに限定する (YAGNI)。
const (
	CostGroupByService       = "SERVICE"
	CostGroupByUsageType     = "USAGE_TYPE"
	CostGroupByLinkedAccount = "LINKED_ACCOUNT"
	CostGroupByRegion        = "REGION"
)

// CostQueryOptions は GetCost の検索条件を表す。ゼロ値は以下のデフォルトとして扱う。
//   - Granularity: 空文字は DAILY
//   - GroupByDimension: 空文字は SERVICE
//   - ServiceFilter: 空文字は絞り込みなし (Dimension SERVICE の EQUALS フィルタ)
//   - StartDate/EndDate: 両方指定時のみ有効な期間として使う (YYYY-MM-DD)。指定時は Months を無視する。
//   - Months: StartDate/EndDate 未指定時のみ使う。0 以下は 1 (取得期間を遡る月数)
type CostQueryOptions struct {
	IncludeToday     bool
	Granularity      string
	GroupByDimension string
	ServiceFilter    string
	StartDate        string
	EndDate          string
	Months           int
}

func costGranularity(g string) cetypes.Granularity {
	if g == "MONTHLY" {
		return cetypes.GranularityMonthly
	}
	return cetypes.GranularityDaily
}

func costGroupByDimension(dim string) string {
	switch dim {
	case CostGroupByUsageType, CostGroupByLinkedAccount, CostGroupByRegion:
		return dim
	default:
		return CostGroupByService
	}
}

// costDateRange は CostQueryOptions から Cost Explorer に渡す期間 (YYYY-MM-DD) を決める。
// StartDate/EndDate が両方指定されていればそれを使い、そうでなければ Months (現在からの
// 相対期間、デフォルト 1 ヶ月) から算出する。
func costDateRange(opts CostQueryOptions) (start, end string) {
	if opts.StartDate != "" && opts.EndDate != "" {
		return opts.StartDate, opts.EndDate
	}

	now := time.Now().UTC()
	end = now.Format("2006-01-02")
	if !opts.IncludeToday {
		end = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	months := opts.Months
	if months <= 0 {
		months = 1
	}
	start = now.AddDate(0, -months, 0).Format("2006-01-02")
	return start, end
}

// GetCost returns cost grouped by the given dimension for the given date range.
// If includeToday is false, the end date is yesterday.
func GetCost(ctx context.Context, profile, region string, opts CostQueryOptions) ([]CostResource, error) {
	// Cost Explorer is a global service; us-east-1 is the standard endpoint.
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *costexplorer.Client {
		return costexplorer.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	start, end := costDateRange(opts)

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: costGranularity(opts.Granularity),
		Metrics:     []string{"UnblendedCost", "NetAmortizedCost"},
		GroupBy: []cetypes.GroupDefinition{
			{Type: cetypes.GroupDefinitionTypeDimension, Key: aws.String(costGroupByDimension(opts.GroupByDimension))},
		},
	}
	if opts.ServiceFilter != "" {
		input.Filter = &cetypes.Expression{
			Dimensions: &cetypes.DimensionValues{
				Key:          cetypes.DimensionService,
				Values:       []string{opts.ServiceFilter},
				MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
			},
		}
	}

	out, err := client.GetCostAndUsage(ctx, input)
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
