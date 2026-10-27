package tui

import (
	"context"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

// NewGCPProvider は Google Cloud の Provider を返す。listProjects にはプロジェクト一覧の
// 取得関数 (gcp.ListProjects かローカルキャッシュを読む関数) を渡す。defaultProject は
// 設定 (GOOGLE_CLOUD_PROJECT など) で指定されたプロジェクトで、一覧に含まれない場合も
// 選択肢の先頭に足す。
func NewGCPProvider(
	listProjects func(ctx context.Context) ([]gcp.ProjectInfo, error),
	defaultProject string,
	services []Service,
) Provider {
	return &provider{
		id:            ProviderGCP,
		title:         "Google Cloud",
		accent:        gcpAccent,
		mutedAccent:   gcpMutedAccent,
		scopeLabel:    "project",
		regionLabel:   "",
		requiresScope: true,
		defaultScope:  defaultProject,
		services:      services,
		loadScopes: func(ctx context.Context) ([]Scope, error) {
			var projects []gcp.ProjectInfo
			var err error
			if listProjects != nil {
				projects, err = listProjects(ctx)
			}
			scopes := make([]Scope, 0, len(projects)+1)
			found := false
			for _, p := range projects {
				if p.ProjectID == "" {
					continue
				}
				if p.ProjectID == defaultProject {
					found = true
				}
				scopes = append(scopes, Scope{
					Label:  p.ProjectID,
					Detail: projectDetail(p),
					Value:  p.ProjectID,
				})
			}
			if defaultProject != "" && !found {
				scopes = append([]Scope{{
					Label:  defaultProject,
					Detail: "default project",
					Value:  defaultProject,
				}}, scopes...)
			}
			return scopes, err
		},
	}
}

// projectDetail はプロジェクトのピッカーに出す補足を返す。
func projectDetail(p gcp.ProjectInfo) string {
	detail := p.Name
	if p.State != "" {
		if detail != "" {
			detail += "  "
		}
		detail += p.State
	}
	return detail
}
