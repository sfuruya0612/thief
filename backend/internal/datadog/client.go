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

// NewOAuthContext returns a context carrying an OAuth 2.0 bearer access token.
// The SDK turns it into an "Authorization: Bearer <token>" header. It is
// deliberately exclusive with NewContext: a request must present either the
// static API/App keys or the OAuth token, never both.
func NewOAuthContext(ctx context.Context, accessToken string) context.Context {
	return context.WithValue(ctx, datadog.ContextAccessToken, accessToken)
}

// HasOAuthToken reports whether ctx carries an OAuth access token (i.e. it was
// built by NewOAuthContext rather than NewContext).
func HasOAuthToken(ctx context.Context) bool {
	tok, ok := ctx.Value(datadog.ContextAccessToken).(string)
	return ok && tok != ""
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

// OrganizationsV1API wraps the Datadog v1 organizations API. The organizations
// endpoint only exists in v1; there is no v2 equivalent.
type OrganizationsV1API struct {
	api *datadogV1.OrganizationsApi
}

// NewOrganizationsV1API creates a new OrganizationsV1API.
func NewOrganizationsV1API(cfg *datadog.Configuration) *OrganizationsV1API {
	client := datadog.NewAPIClient(cfg)
	return &OrganizationsV1API{api: datadogV1.NewOrganizationsApi(client)}
}
