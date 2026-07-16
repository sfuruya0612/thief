package datadog

import (
	"context"
	"fmt"
	"time"

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

	return costInfosFromResponse(resp), nil
}

// GetEstimatedCost returns estimated cost data for the current and/or previous
// month (Datadog only exposes estimated cost for those two months).
func GetEstimatedCost(ctx context.Context, api *UsageMeteringV2API, startMonth, endMonth, view string) ([]CostInfo, error) {
	start, err := parseMonth(startMonth)
	if err != nil {
		return nil, fmt.Errorf("parse start_month: %w", err)
	}
	params := datadogV2.GetEstimatedCostByOrgOptionalParameters{StartMonth: &start}
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

	resp, _, err := api.api.GetEstimatedCostByOrg(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("get datadog estimated cost: %w", err)
	}

	return costInfosFromResponse(resp), nil
}

// costInfosFromResponse は CostByOrgResponse の月次データを charge 単位の CostInfo に展開する。
// GetHistoricalCostByOrg と GetEstimatedCostByOrg は同じレスポンス型を返すため両方から使う。
func costInfosFromResponse(resp datadogV2.CostByOrgResponse) []CostInfo {
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
	return result
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
