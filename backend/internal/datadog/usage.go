package datadog

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// CostInfo represents a Datadog cost line item.
type CostInfo struct {
	Month       string  `json:"month"`
	AccountName string  `json:"account_name"`
	OrgName     string  `json:"org_name"`
	ProductName string  `json:"product_name"`
	ChargeType  string  `json:"charge_type"`
	Cost        float64 `json:"cost"`
}

// GetHistoricalCost returns historical monthly cost data.
func GetHistoricalCost(ctx context.Context, api *UsageMeteringV2API, startMonth, endMonth, view string) ([]CostInfo, error) {
	start, err := parseMonth(startMonth)
	if err != nil {
		return nil, fmt.Errorf("parse start_month: %w", err)
	}
	params := datadogV2.GetHistoricalCostByOrgOptionalParameters{}
	if endMonth != "" {
		end, err := parseMonth(endMonth)
		if err != nil {
			return nil, fmt.Errorf("parse end_month: %w", err)
		}
		params.EndMonth = &end
	}
	if view != "" {
		params.View = &view
	}

	resp, _, err := api.api.GetHistoricalCostByOrg(ctx, start, params)
	if err != nil {
		return nil, fmt.Errorf("get datadog historical cost: %w", err)
	}

	result := []CostInfo{}
	for _, item := range resp.GetData() {
		attrs := item.GetAttributes()
		for _, charge := range attrs.GetCharges() {
			result = append(result, CostInfo{
				Month:       attrs.GetDate().Format("2006-01"),
				AccountName: attrs.GetAccountName(),
				OrgName:     attrs.GetOrgName(),
				ProductName: charge.GetProductName(),
				ChargeType:  charge.GetChargeType(),
				Cost:        charge.GetCost(),
			})
		}
	}
	return result, nil
}

// GetEstimatedCost returns estimated cost data.
func GetEstimatedCost(ctx context.Context, api *UsageMeteringV1API, startMonth, endMonth string) ([]CostInfo, error) {
	start, err := parseMonth(startMonth)
	if err != nil {
		return nil, fmt.Errorf("parse start_month: %w", err)
	}
	params := datadogV1.GetUsageSummaryOptionalParameters{}
	if endMonth != "" {
		end, err := parseMonth(endMonth)
		if err != nil {
			return nil, fmt.Errorf("parse end_month: %w", err)
		}
		params.EndMonth = &end
	}

	resp, _, err := api.api.GetUsageSummary(ctx, start, params)
	if err != nil {
		return nil, fmt.Errorf("get datadog estimated cost: %w", err)
	}

	result := []CostInfo{}
	for _, usage := range resp.GetUsage() {
		result = append(result, CostInfo{
			Month: usage.GetDate().Format("2006-01"),
		})
	}
	return result, nil
}

func parseMonth(s string) (time.Time, error) {
	if s == "" {
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected format YYYY-MM, got %q: %w", s, err)
	}
	return t, nil
}
