package datadog

import (
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/stretchr/testify/assert"
)

func TestGenerateDatadogContext(t *testing.T) {
	apiKey := "test-api-key"
	appKey := "test-app-key"

	ctx := GenerateDatadogContext(apiKey, appKey)

	// Verify context contains API keys
	apiKeys, ok := ctx.Value(datadog.ContextAPIKeys).(map[string]datadog.APIKey)
	assert.True(t, ok, "Context should contain API keys")
	assert.Equal(t, apiKey, apiKeys["apiKeyAuth"].Key)
	assert.Equal(t, appKey, apiKeys["appKeyAuth"].Key)
}

func TestNewDatadogClient(t *testing.T) {
	site := "datadoghq.com"
	client := NewDatadogClient(site)

	assert.NotNil(t, client)
	assert.Equal(t, "api.datadoghq.com", client.GetConfig().Host)
}
