package datadog

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// NewConfiguration creates a Datadog API configuration for the given site.
func NewConfiguration(site string) *datadog.Configuration {
	cfg := datadog.NewConfiguration()
	cfg.Host = "api." + site
	return cfg
}

// NewContext returns a context with Datadog API key and App key embedded.
func NewContext(ctx context.Context, apiKey, appKey string) context.Context {
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: apiKey},
		"appKeyAuth": {Key: appKey},
	})
	return ctx
}

// UsageMeteringV1API wraps the Datadog v1 usage metering API.
type UsageMeteringV1API struct {
	api *datadogV1.UsageMeteringApi
}

// NewUsageMeteringV1API creates a new UsageMeteringV1API.
func NewUsageMeteringV1API(cfg *datadog.Configuration) *UsageMeteringV1API {
	client := datadog.NewAPIClient(cfg)
	return &UsageMeteringV1API{api: datadogV1.NewUsageMeteringApi(client)}
}

// UsageMeteringV2API wraps the Datadog v2 usage metering API.
type UsageMeteringV2API struct {
	api *datadogV2.UsageMeteringApi
}

// NewUsageMeteringV2API creates a new UsageMeteringV2API.
func NewUsageMeteringV2API(cfg *datadog.Configuration) *UsageMeteringV2API {
	client := datadog.NewAPIClient(cfg)
	return &UsageMeteringV2API{api: datadogV2.NewUsageMeteringApi(client)}
}
