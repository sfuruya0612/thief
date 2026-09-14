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

// OrganizationsV2API wraps the Datadog v2 organizations API.
//
// v1 の GET /api/v1/org は呼び出し元自身の組織 1 件しか返さず Sub Organization を
// 列挙できないため (issue 0171 で実機検証済み)、v2 の managed orgs
// エンドポイント (GET /api/v2/org) を使う。
type OrganizationsV2API struct {
	api *datadogV2.OrganizationsApi
}

// NewOrganizationsV2API creates a new OrganizationsV2API.
func NewOrganizationsV2API(cfg *datadog.Configuration) *OrganizationsV2API {
	client := datadog.NewAPIClient(cfg)
	return &OrganizationsV2API{api: datadogV2.NewOrganizationsApi(client)}
}

// DashboardsV1API wraps the Datadog v1 dashboards API. Dashboards only exist in
// v1; there is no v2 equivalent.
type DashboardsV1API struct {
	api *datadogV1.DashboardsApi
}

// NewDashboardsV1API creates a new DashboardsV1API.
func NewDashboardsV1API(cfg *datadog.Configuration) *DashboardsV1API {
	client := datadog.NewAPIClient(cfg)
	return &DashboardsV1API{api: datadogV1.NewDashboardsApi(client)}
}

// MetricsV1API wraps the Datadog v1 metrics API. The metrics query endpoint
// (GET /api/v1/query) only exists in v1; there is no v2 equivalent.
type MetricsV1API struct {
	api *datadogV1.MetricsApi
}

// NewMetricsV1API creates a new MetricsV1API.
func NewMetricsV1API(cfg *datadog.Configuration) *MetricsV1API {
	client := datadog.NewAPIClient(cfg)
	return &MetricsV1API{api: datadogV1.NewMetricsApi(client)}
}
