package datadog

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

// This is a simpler approach to testing since we can't easily mock the DataDog API client
// These tests primarily check that parameters are constructed correctly

func TestNewDatadogUsageMeteringApi(t *testing.T) {
	client := &datadog.APIClient{}
	api := NewDatadogUsageMeteringApi(client)
	if api == nil {
		t.Error("expected non-nil api, got nil")
	}
}

func TestGenerateGetHistoricalCostByOrgOptionalParameters(t *testing.T) {
	view := "monthly"
	now := time.Now()
	endMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Test with endMonth
	params := GenerateGetHistoricalCostByOrgOptionalParameters(view, endMonth)
	if params == nil {
		t.Fatal("expected non-nil params, got nil")
	}
	if *params.View != view {
		t.Errorf("expected View %q, got %q", view, *params.View)
	}
	if *params.EndMonth != endMonth {
		t.Errorf("expected EndMonth %v, got %v", endMonth, *params.EndMonth)
	}

	// Test without endMonth
	params = GenerateGetHistoricalCostByOrgOptionalParameters(view, time.Time{})
	if params == nil {
		t.Fatal("expected non-nil params, got nil")
	}
	if *params.View != view {
		t.Errorf("expected View %q, got %q", view, *params.View)
	}
	if params.EndMonth != nil {
		t.Errorf("expected nil EndMonth, got %v", params.EndMonth)
	}
}

func TestGenerateGetEstimatedCostByOrgOptionalParameters(t *testing.T) {
	view := "summary"
	now := time.Now()
	startMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	endMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Test with endMonth
	params := GenerateGetEstimatedCostByOrgOptionalParameters(view, startMonth, endMonth)
	if params == nil {
		t.Fatal("expected non-nil params, got nil")
	}
	if *params.View != view {
		t.Errorf("expected View %q, got %q", view, *params.View)
	}
	if *params.StartMonth != startMonth {
		t.Errorf("expected StartMonth %v, got %v", startMonth, *params.StartMonth)
	}
	if *params.EndMonth != endMonth {
		t.Errorf("expected EndMonth %v, got %v", endMonth, *params.EndMonth)
	}

	// Test without endMonth
	params = GenerateGetEstimatedCostByOrgOptionalParameters(view, startMonth, time.Time{})
	if params == nil {
		t.Fatal("expected non-nil params, got nil")
	}
	if *params.View != view {
		t.Errorf("expected View %q, got %q", view, *params.View)
	}
	if *params.StartMonth != startMonth {
		t.Errorf("expected StartMonth %v, got %v", startMonth, *params.StartMonth)
	}
	if params.EndMonth != nil {
		t.Errorf("expected nil EndMonth, got %v", params.EndMonth)
	}
}

// Note: We can't easily test GetHistoricalCostByOrg and GetEstimatedCostByOrg
// in a unit test context without complex mocking of the DataDog API client.
// In a real-world situation, these would be integration tests.
