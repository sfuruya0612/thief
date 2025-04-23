package datadog

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/stretchr/testify/assert"
)

// This is a simpler approach to testing since we can't easily mock the DataDog API client
// These tests primarily check that parameters are constructed correctly

func TestNewDatadogUsageMeteringApi(t *testing.T) {
	client := &datadog.APIClient{}
	api := NewDatadogUsageMeteringApi(client)
	assert.NotNil(t, api)
}

func TestGenerateGetHistoricalCostByOrgOptionalParameters(t *testing.T) {
	view := "monthly"
	now := time.Now()
	endMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Test with endMonth
	params := GenerateGetHistoricalCostByOrgOptionalParameters(view, endMonth)
	assert.NotNil(t, params)
	assert.Equal(t, view, *params.View)
	assert.Equal(t, endMonth, *params.EndMonth)

	// Test without endMonth
	params = GenerateGetHistoricalCostByOrgOptionalParameters(view, time.Time{})
	assert.NotNil(t, params)
	assert.Equal(t, view, *params.View)
	assert.Nil(t, params.EndMonth)
}

func TestGenerateGetEstimatedCostByOrgOptionalParameters(t *testing.T) {
	view := "summary"
	now := time.Now()
	startMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	endMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Test with endMonth
	params := GenerateGetEstimatedCostByOrgOptionalParameters(view, startMonth, endMonth)
	assert.NotNil(t, params)
	assert.Equal(t, view, *params.View)
	assert.Equal(t, startMonth, *params.StartMonth)
	assert.Equal(t, endMonth, *params.EndMonth)

	// Test without endMonth
	params = GenerateGetEstimatedCostByOrgOptionalParameters(view, startMonth, time.Time{})
	assert.NotNil(t, params)
	assert.Equal(t, view, *params.View)
	assert.Equal(t, startMonth, *params.StartMonth)
	assert.Nil(t, params.EndMonth)
}

// Note: We can't easily test GetHistoricalCostByOrg and GetEstimatedCostByOrg
// in a unit test context without complex mocking of the DataDog API client.
// In a real-world situation, these would be integration tests.
