package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// NewAWSProvider は AWS の Provider を返す。listProfiles には aws.ListProfiles を、
// listRegions には aws.ListRegions を渡す。services の並びは Web UI のサイドバーと同じにする。
func NewAWSProvider(
	listProfiles func() ([]aws.Profile, error),
	listRegions func(ctx context.Context, profile string) ([]aws.RegionResource, error),
	services []Service,
) Provider {
	return &provider{
		id:            ProviderAWS,
		title:         "AWS",
		accent:        awsAccent,
		mutedAccent:   awsMutedAccent,
		scopeLabel:    "profile",
		regionLabel:   "region",
		requiresScope: false,
		services:      services,
		loadScopes: func(context.Context) ([]Scope, error) {
			if listProfiles == nil {
				return nil, nil
			}
			profiles, err := listProfiles()
			if err != nil {
				return nil, err
			}
			scopes := make([]Scope, 0, len(profiles))
			for _, p := range profiles {
				detail := p.AccountID
				if p.AuthType == aws.AuthTypeSSO {
					detail = fmt.Sprintf("%s  %s  sso:%s", p.AccountID, p.SSORoleName, p.SSOStatus)
				}
				scopes = append(scopes, Scope{Label: p.Name, Detail: detail, Value: p.Name})
			}
			return scopes, nil
		},
		loadRegions: func(ctx context.Context, profile string) ([]Scope, error) {
			if listRegions == nil {
				return nil, nil
			}
			regions, err := listRegions(ctx, profile)
			if err != nil {
				return nil, err
			}
			scopes := make([]Scope, 0, len(regions))
			for _, r := range regions {
				scopes = append(scopes, Scope{Label: r.Code, Detail: r.Name, Value: r.Code})
			}
			return scopes, nil
		},
	}
}

// newLegacyAWSProvider は Deps.Services だけを指定された TUI のために、AWS プロバイダーを
// 組み立てる。既存のテストが Deps.Services と Deps.ListProfiles / Deps.ListRegions だけで
// Model を作れるようにするための経路で、本番は cli が NewAWSProvider で組み立てる。
func newLegacyAWSProvider(deps Deps) Provider {
	return NewAWSProvider(deps.ListProfiles, deps.ListRegions, deps.Services)
}
