package datadog

import (
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

func TestGenerateDatadogContext(t *testing.T) {
	apiKey := "test-api-key"
	appKey := "test-app-key"

	ctx := GenerateDatadogContext(apiKey, appKey)

	// Verify context contains API keys
	apiKeys, ok := ctx.Value(datadog.ContextAPIKeys).(map[string]datadog.APIKey)
	if !ok {
		t.Fatal("Context should contain API keys")
	}
	if apiKeys["apiKeyAuth"].Key != apiKey {
		t.Errorf("expected apiKeyAuth %q, got %q", apiKey, apiKeys["apiKeyAuth"].Key)
	}
	if apiKeys["appKeyAuth"].Key != appKey {
		t.Errorf("expected appKeyAuth %q, got %q", appKey, apiKeys["appKeyAuth"].Key)
	}
}

func TestNewDatadogClient(t *testing.T) {
	site := "datadoghq.com"
	client := NewDatadogClient(site)

	if client == nil {
		t.Fatal("expected non-nil client, got nil")
	}
	if client.GetConfig().Host != "api.datadoghq.com" {
		t.Errorf("expected Host 'api.datadoghq.com', got '%s'", client.GetConfig().Host)
	}
}
