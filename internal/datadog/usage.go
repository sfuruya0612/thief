package datadog

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

func NewDatadogUsageMeteringApi(client *datadog.APIClient) *datadogV2.UsageMeteringApi {
	return datadogV2.NewUsageMeteringApi(client)
}

func GenerateGetHistoricalCostByOrgOptionalParameters(view string, endMonth time.Time) *datadogV2.GetHistoricalCostByOrgOptionalParameters {
	params := datadogV2.NewGetHistoricalCostByOrgOptionalParameters()

	params.View = &view

	if endMonth != (time.Time{}) {
		params.EndMonth = &endMonth
	}

	return params
}

func GetHistoricalCostByOrg(ctx context.Context, api *datadogV2.UsageMeteringApi, startMonth time.Time, params datadogV2.GetHistoricalCostByOrgOptionalParameters) ([][]string, error) {
	resp, r, err := api.GetHistoricalCostByOrg(ctx, startMonth, params)
	if err != nil {
		return nil, fmt.Errorf("Error getting historical cost by org: %v, http response: %v", err, r)
	}

	var cost [][]string
	for _, c := range resp.Data {
		for _, p := range c.Attributes.Charges {
			cost = append(cost, []string{
				c.Attributes.Date.Format("2006-01"),
				*c.Attributes.AccountName,
				*c.Attributes.OrgName,
				*p.ProductName,
				*p.ChargeType,
				strconv.FormatFloat(*p.Cost, 'f', -1, 64),
			})
		}
	}

	return cost, nil
}

func GenerateGetEstimatedCostByOrgOptionalParameters(view string, startMonth, endMonth time.Time) *datadogV2.GetEstimatedCostByOrgOptionalParameters {
	params := datadogV2.NewGetEstimatedCostByOrgOptionalParameters()

	params.View = &view
	params.StartMonth = &startMonth

	if endMonth != (time.Time{}) {
		params.EndMonth = &endMonth
	}

	return params
}

func GetEstimatedCostByOrg(ctx context.Context, api *datadogV2.UsageMeteringApi, params datadogV2.GetEstimatedCostByOrgOptionalParameters) ([][]string, error) {
	resp, r, err := api.GetEstimatedCostByOrg(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("Error getting estimated cost by org: %v, http response: %v", err, r)
	}

	var cost [][]string
	for _, c := range resp.Data {
		for _, p := range c.Attributes.Charges {
			cost = append(cost, []string{
				c.Attributes.Date.Format("2006-01"),
				*c.Attributes.AccountName,
				*c.Attributes.OrgName,
				*p.ProductName,
				*p.ChargeType,
				strconv.FormatFloat(*p.Cost, 'f', -1, 64),
			})
		}
	}

	return cost, nil
}
