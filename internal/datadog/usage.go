package datadog

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// DatadogCostInfo holds display fields for a Datadog cost record.
type DatadogCostInfo struct {
	Month       string
	AccountName string
	OrgName     string
	ProductName string
	ChargeType  string
	Cost        string
}

// ToRow converts DatadogCostInfo to a string slice suitable for table formatting.
func (d DatadogCostInfo) ToRow() []string {
	return []string{d.Month, d.AccountName, d.OrgName, d.ProductName, d.ChargeType, d.Cost}
}

// NewDatadogUsageMeteringApi creates a new UsageMeteringApi wrapper.
func NewDatadogUsageMeteringApi(client *datadog.APIClient) *datadogV2.UsageMeteringApi {
	return datadogV2.NewUsageMeteringApi(client)
}

// GenerateGetHistoricalCostByOrgOptionalParameters builds the optional parameters
// for the GetHistoricalCostByOrg API call.
func GenerateGetHistoricalCostByOrgOptionalParameters(view string, endMonth time.Time) *datadogV2.GetHistoricalCostByOrgOptionalParameters {
	params := datadogV2.NewGetHistoricalCostByOrgOptionalParameters()

	params.View = &view

	if endMonth != (time.Time{}) {
		params.EndMonth = &endMonth
	}

	return params
}

// GetHistoricalCostByOrg fetches completed billing period costs from Datadog and
// returns the results as a typed slice of DatadogCostInfo.
func GetHistoricalCostByOrg(ctx context.Context, api *datadogV2.UsageMeteringApi, startMonth time.Time, params datadogV2.GetHistoricalCostByOrgOptionalParameters) ([]DatadogCostInfo, error) {
	resp, r, err := api.GetHistoricalCostByOrg(ctx, startMonth, params)
	if err != nil {
		return nil, fmt.Errorf("getting historical cost by org: %v, http response: %v", err, r)
	}

	var costs []DatadogCostInfo
	for _, c := range resp.Data {
		for _, p := range c.Attributes.Charges {
			costs = append(costs, DatadogCostInfo{
				Month:       c.Attributes.Date.Format("2006-01"),
				AccountName: *c.Attributes.AccountName,
				OrgName:     *c.Attributes.OrgName,
				ProductName: *p.ProductName,
				ChargeType:  *p.ChargeType,
				Cost:        strconv.FormatFloat(*p.Cost, 'f', -1, 64),
			})
		}
	}

	return costs, nil
}

// GenerateGetEstimatedCostByOrgOptionalParameters builds the optional parameters
// for the GetEstimatedCostByOrg API call.
func GenerateGetEstimatedCostByOrgOptionalParameters(view string, startMonth, endMonth time.Time) *datadogV2.GetEstimatedCostByOrgOptionalParameters {
	params := datadogV2.NewGetEstimatedCostByOrgOptionalParameters()

	params.View = &view
	params.StartMonth = &startMonth

	if endMonth != (time.Time{}) {
		params.EndMonth = &endMonth
	}

	return params
}

// GetEstimatedCostByOrg fetches current period estimated costs from Datadog and
// returns the results as a typed slice of DatadogCostInfo.
func GetEstimatedCostByOrg(ctx context.Context, api *datadogV2.UsageMeteringApi, params datadogV2.GetEstimatedCostByOrgOptionalParameters) ([]DatadogCostInfo, error) {
	resp, r, err := api.GetEstimatedCostByOrg(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("getting estimated cost by org: %v, http response: %v", err, r)
	}

	var costs []DatadogCostInfo
	for _, c := range resp.Data {
		for _, p := range c.Attributes.Charges {
			costs = append(costs, DatadogCostInfo{
				Month:       c.Attributes.Date.Format("2006-01"),
				AccountName: *c.Attributes.AccountName,
				OrgName:     *c.Attributes.OrgName,
				ProductName: *p.ProductName,
				ChargeType:  *p.ChargeType,
				Cost:        strconv.FormatFloat(*p.Cost, 'f', -1, 64),
			})
		}
	}

	return costs, nil
}
