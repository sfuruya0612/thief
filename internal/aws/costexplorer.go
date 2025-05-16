package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

type CostExplorerClient struct {
	client *costexplorer.Client
}

// NewCostExplorerClient creates a new CostExplorer client using the specified AWS profile and region.
func NewCostExplorerClient(profile, region string) (*CostExplorerClient, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create CostExplorer client: %w", err)
	}
	return &CostExplorerClient{
		client: costexplorer.NewFromConfig(cfg),
	}, nil
}

type CostDetail struct {
	TimePeriod  string
	Amount      string
	Unit        string
	ServiceName string
	GroupKey    string
}

type CostMetric string

const (
	UnblendedCost         CostMetric = "UnblendedCost"
	BlendedCost           CostMetric = "BlendedCost"
	NetUnblendedCost      CostMetric = "NetUnblendedCost"
	NetAmortizedCost      CostMetric = "NetAmortizedCost"
	AmortizedCost         CostMetric = "AmortizedCost"
	UsageQuantity         CostMetric = "UsageQuantity"
	NormalizedUsageAmount CostMetric = "NormalizedUsageAmount"
)

func (c *CostExplorerClient) GetCostAndUsage(startDate, endDate string, granularity types.Granularity, groupBy []types.GroupDefinition, metric CostMetric) ([]CostDetail, error) {
	metricStr := string(metric)
	if metricStr == "" {
		metricStr = string(UnblendedCost)
	}

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(startDate),
			End:   aws.String(endDate),
		},
		Granularity: granularity,
		Metrics:     []string{metricStr},
	}

	if len(groupBy) > 0 {
		input.GroupBy = groupBy
	}

	resp, err := c.client.GetCostAndUsage(context.TODO(), input)
	if err != nil {
		return nil, err
	}

	var details []CostDetail

	for _, result := range resp.ResultsByTime {
		for _, group := range result.Groups {
			detail := CostDetail{
				TimePeriod: *result.TimePeriod.Start,
				Amount:     *group.Metrics[metricStr].Amount,
				Unit:       *group.Metrics[metricStr].Unit,
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

	return details, nil
}

func (c *CostExplorerClient) GetCostByService(startDate, endDate string, metric CostMetric) ([]CostDetail, error) {
	groupBy := []types.GroupDefinition{
		{
			Key:  aws.String("SERVICE"),
			Type: types.GroupDefinitionTypeDimension,
		},
	}

	return c.GetCostAndUsage(startDate, endDate, types.GranularityMonthly, groupBy, metric)
}

func (c *CostExplorerClient) GetCostByAccount(startDate, endDate string, metric CostMetric) ([]CostDetail, error) {
	groupBy := []types.GroupDefinition{
		{
			Key:  aws.String("LINKED_ACCOUNT"),
			Type: types.GroupDefinitionTypeDimension,
		},
	}

	return c.GetCostAndUsage(startDate, endDate, types.GranularityMonthly, groupBy, metric)
}

func (c *CostExplorerClient) GetCostForPeriod(startDate, endDate string, metric CostMetric) ([]CostDetail, error) {
	return c.GetCostAndUsage(startDate, endDate, types.GranularityMonthly, nil, metric)
}
