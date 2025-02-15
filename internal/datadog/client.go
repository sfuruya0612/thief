package datadog

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

func GenerateDatadogContext(apiKey, appKey string) context.Context {
	return context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
			"appKeyAuth": {
				Key: appKey,
			},
		},
	)
}

func NewDatadogClient(site string) *datadog.APIClient {
	configuration := datadog.NewConfiguration()
	configuration.Host = fmt.Sprintf("api.%s", site)

	return datadog.NewAPIClient(configuration)
}
