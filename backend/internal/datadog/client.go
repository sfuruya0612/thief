package datadog

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// NewConfiguration は指定した site 向けの Datadog API 設定を作る。
func NewConfiguration(site string) *datadog.Configuration {
	cfg := datadog.NewConfiguration()
	cfg.Host = "api." + site
	return cfg
}

// NewContext は Datadog の API キーと App キーを埋め込んだ context を返す。
func NewContext(ctx context.Context, apiKey, appKey string) context.Context {
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: apiKey},
		"appKeyAuth": {Key: appKey},
	})
	return ctx
}

// NewOAuthContext は OAuth 2.0 のベアラアクセストークンを載せた context を返す。
// SDK はこれを "Authorization: Bearer <token>" ヘッダに変換する。NewContext とは
// 意図的に排他であり、1 つのリクエストは静的な API / App キーか OAuth トークンの
// どちらか一方だけを提示する。
func NewOAuthContext(ctx context.Context, accessToken string) context.Context {
	return context.WithValue(ctx, datadog.ContextAccessToken, accessToken)
}

// HasOAuthToken は ctx が OAuth のアクセストークンを持つかどうか (すなわち
// NewContext ではなく NewOAuthContext で組み立てられたか) を返す。
func HasOAuthToken(ctx context.Context) bool {
	tok, ok := ctx.Value(datadog.ContextAccessToken).(string)
	return ok && tok != ""
}

// UsageMeteringV2API は Datadog の v2 usage metering API を包む。
type UsageMeteringV2API struct {
	api *datadogV2.UsageMeteringApi
}

// NewUsageMeteringV2API は UsageMeteringV2API を新しく作る。
func NewUsageMeteringV2API(cfg *datadog.Configuration) *UsageMeteringV2API {
	client := datadog.NewAPIClient(cfg)
	return &UsageMeteringV2API{api: datadogV2.NewUsageMeteringApi(client)}
}

// OrganizationsV2API は Datadog の v2 organizations API を包む。
//
// v1 の GET /api/v1/org は呼び出し元自身の組織 1 件しか返さず Sub Organization を
// 列挙できないため (issue 0171 で実機検証済み)、v2 の managed orgs
// エンドポイント (GET /api/v2/org) を使う。
type OrganizationsV2API struct {
	api *datadogV2.OrganizationsApi
}

// NewOrganizationsV2API は OrganizationsV2API を新しく作る。
func NewOrganizationsV2API(cfg *datadog.Configuration) *OrganizationsV2API {
	client := datadog.NewAPIClient(cfg)
	return &OrganizationsV2API{api: datadogV2.NewOrganizationsApi(client)}
}

// DashboardsV1API は Datadog の v1 dashboards API を包む。ダッシュボードは v1 にしか
// 存在せず、v2 に相当するものは無い。
type DashboardsV1API struct {
	api *datadogV1.DashboardsApi
}

// NewDashboardsV1API は DashboardsV1API を新しく作る。
func NewDashboardsV1API(cfg *datadog.Configuration) *DashboardsV1API {
	client := datadog.NewAPIClient(cfg)
	return &DashboardsV1API{api: datadogV1.NewDashboardsApi(client)}
}

// MetricsV1API は Datadog の v1 metrics API を包む。メトリクスのクエリの
// エンドポイント (GET /api/v1/query) は v1 にしか存在せず、v2 に相当するものは無い。
type MetricsV1API struct {
	api *datadogV1.MetricsApi
}

// NewMetricsV1API は MetricsV1API を新しく作る。
func NewMetricsV1API(cfg *datadog.Configuration) *MetricsV1API {
	client := datadog.NewAPIClient(cfg)
	return &MetricsV1API{api: datadogV1.NewMetricsApi(client)}
}
