package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// CostDetail はレガシー CLI 互換の Cost Explorer 明細を保持する。
// Amount は API のレスポンス文字列をそのまま保持する (丸めない)。
type CostDetail struct {
	TimePeriod  string
	Amount      string
	Unit        string
	ServiceName string
	GroupKey    string
}

// CostMetric は Cost Explorer のコストメトリクス名。
type CostMetric string

// Cost Explorer がサポートするコストメトリクス。
const (
	UnblendedCost         CostMetric = "UnblendedCost"
	BlendedCost           CostMetric = "BlendedCost"
	NetUnblendedCost      CostMetric = "NetUnblendedCost"
	NetAmortizedCost      CostMetric = "NetAmortizedCost"
	AmortizedCost         CostMetric = "AmortizedCost"
	UsageQuantity         CostMetric = "UsageQuantity"
	NormalizedUsageAmount CostMetric = "NormalizedUsageAmount"
)

// getCostDetails は Cost Explorer GetCostAndUsage を呼び、明細一覧を返す。
func getCostDetails(ctx context.Context, profile, region, startDate, endDate string, granularity cetypes.Granularity, groupBy []cetypes.GroupDefinition, metric CostMetric) ([]CostDetail, error) {
	client, err := newCostExplorerClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	metricStr := string(metric)
	if metricStr == "" {
		metricStr = string(UnblendedCost)
	}

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(startDate),
			End:   aws.String(endDate),
		},
		Granularity: granularity,
		Metrics:     []string{metricStr},
	}
	if len(groupBy) > 0 {
		input.GroupBy = groupBy
	}

	var details []CostDetail
	for {
		resp, err := client.GetCostAndUsage(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("get cost and usage: %w", err)
		}

		for _, result := range resp.ResultsByTime {
			period := ""
			if result.TimePeriod != nil {
				period = ptrStr(result.TimePeriod.Start)
			}
			// GroupBy 未指定 (overview) のときは Groups が空になり Total に金額が入る。
			if len(result.Groups) == 0 {
				if m, ok := result.Total[metricStr]; ok {
					details = append(details, CostDetail{
						TimePeriod: period,
						Amount:     ptrStr(m.Amount),
						Unit:       ptrStr(m.Unit),
					})
				}
				continue
			}
			for _, group := range result.Groups {
				m, ok := group.Metrics[metricStr]
				if !ok {
					continue
				}
				detail := CostDetail{
					TimePeriod: period,
					Amount:     ptrStr(m.Amount),
					Unit:       ptrStr(m.Unit),
				}
				if len(group.Keys) > 0 {
					detail.GroupKey = group.Keys[0]
					if len(group.Keys) > 1 {
						detail.ServiceName = group.Keys[1]
					} else {
						detail.ServiceName = group.Keys[0]
					}
				}
				details = append(details, detail)
			}
		}

		if resp.NextPageToken == nil {
			break
		}
		input.NextPageToken = resp.NextPageToken
	}

	return details, nil
}

func costGroupByDefinition(key string) []cetypes.GroupDefinition {
	return []cetypes.GroupDefinition{
		{
			Key:  aws.String(key),
			Type: cetypes.GroupDefinitionTypeDimension,
		},
	}
}

// GetCostByService はサービス単位で集計したコスト明細を返す。
func GetCostByService(ctx context.Context, profile, region, startDate, endDate string, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
	return getCostDetails(ctx, profile, region, startDate, endDate, granularity, costGroupByDefinition("SERVICE"), metric)
}

// GetCostByAccount はリンクアカウント単位で集計したコスト明細を返す。
func GetCostByAccount(ctx context.Context, profile, region, startDate, endDate string, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
	return getCostDetails(ctx, profile, region, startDate, endDate, granularity, costGroupByDefinition("LINKED_ACCOUNT"), metric)
}

// GetCostByUsageType は使用タイプ単位で集計したコスト明細を返す。
func GetCostByUsageType(ctx context.Context, profile, region, startDate, endDate string, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
	return getCostDetails(ctx, profile, region, startDate, endDate, granularity, costGroupByDefinition("USAGE_TYPE"), metric)
}

// GetCostForPeriod は集計軸なしの期間合計コスト明細を返す。
func GetCostForPeriod(ctx context.Context, profile, region, startDate, endDate string, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
	return getCostDetails(ctx, profile, region, startDate, endDate, granularity, nil, metric)
}
