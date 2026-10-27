package tui

import (
	"context"
	"sort"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

// DatadogProviderOptions は Datadog プロバイダーの組み立てに使う設定。
type DatadogProviderOptions struct {
	Site       string
	APIKey     string
	AppKey     string
	StartMonth string
	EndMonth   string
	View       string
}

// NewDatadogProvider は Datadog の Provider を返す。組織の一覧は親組織の認証で取り、
// ダッシュボードとコストは選択中の組織の認証で取る。
func NewDatadogProvider(opts DatadogProviderOptions) Provider {
	auth := NewDatadogAuth(opts.Site, opts.APIKey, opts.AppKey)
	cfg := datadog.NewConfiguration(opts.Site)
	orgAPI := datadog.NewOrganizationsV2API(cfg)
	dashAPI := datadog.NewDashboardsV1API(cfg)
	costAPI := datadog.NewUsageMeteringV2API(cfg)

	services := []Service{
		NewDatadogDashboardsService(auth, dashAPI, opts.Site),
		NewDatadogCostService("datadog-cost-historical", "Cost (Historical)", auth, costAPI,
			datadog.GetHistoricalCost, opts.StartMonth, opts.EndMonth, opts.View),
		NewDatadogCostService("datadog-cost-estimated", "Cost (Estimated)", auth, costAPI,
			datadog.GetEstimatedCost, opts.StartMonth, opts.EndMonth, opts.View),
	}

	return newDatadogProvider(func(ctx context.Context) ([]datadog.OrgInfo, error) {
		return datadogCall(ctx, auth, "", func(ctx context.Context) ([]datadog.OrgInfo, error) {
			return datadog.ListOrgs(ctx, orgAPI)
		})
	}, services)
}

// newDatadogProvider は組織一覧の取得関数を差し替えられる Provider を返す。テストで使う。
func newDatadogProvider(
	listOrgs func(ctx context.Context) ([]datadog.OrgInfo, error),
	services []Service,
) Provider {
	return &provider{
		id:            ProviderDatadog,
		title:         "Datadog",
		accent:        datadogAccent,
		mutedAccent:   datadogMutedAccent,
		scopeLabel:    "organization",
		regionLabel:   "",
		requiresScope: false,
		services:      services,
		loadScopes: func(ctx context.Context) ([]Scope, error) {
			if listOrgs == nil {
				return []Scope{{Label: "parent", Detail: "parent organization", Value: ""}}, nil
			}
			orgs, err := listOrgs(ctx)
			scopes := make([]Scope, 0, len(orgs)+1)
			hasParent := false
			for _, org := range orgs {
				if !org.IsSelf && org.ID == "" {
					// ID の無い Sub Organization は API の呼び先を決められないため落とす。
					continue
				}
				value := org.ID
				detail := "sub organization  " + org.ID
				if org.IsSelf {
					value = ""
					detail = "parent organization"
					hasParent = true
				}
				label := org.Name
				if label == "" {
					label = org.ID
				}
				if label == "" {
					label = "parent"
				}
				scopes = append(scopes, Scope{Label: label, Detail: detail, Value: value})
			}
			if !hasParent {
				scopes = append(scopes, Scope{Label: "parent", Detail: "parent organization", Value: ""})
			}
			sort.SliceStable(scopes, func(i, j int) bool {
				// 親組織を先頭にし、その中では表示名の昇順にする。
				if (scopes[i].Value == "") != (scopes[j].Value == "") {
					return scopes[i].Value == ""
				}
				return scopes[i].Label < scopes[j].Label
			})
			return scopes, err
		},
	}
}
