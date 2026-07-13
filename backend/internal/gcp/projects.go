// Package gcp provides Google Cloud SDK wrappers used by the thief backend.
// All clients authenticate via Application Default Credentials (ADC): GCP does
// not have the notion of profiles or regions the way AWS does, so callers only
// need to supply a project ID.
package gcp

import (
	"context"
	"fmt"

	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
)

// ProjectInfo は Cloud Resource Manager で列挙可能なプロジェクトのメタデータを表す。
type ProjectInfo struct {
	ProjectID     string `json:"project_id"`
	Name          string `json:"name"`
	ProjectNumber int64  `json:"project_number"`
	State         string `json:"state"`
	CreateTime    string `json:"create_time"`
}

// ListProjects は ADC でアクセス可能な ACTIVE 状態のプロジェクトを列挙する。
func ListProjects(ctx context.Context) ([]ProjectInfo, error) {
	svc, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("create cloudresourcemanager service: %w", err)
	}

	var projects []ProjectInfo
	call := svc.Projects.List().Filter("lifecycleState:ACTIVE")
	if err := call.Pages(ctx, func(page *cloudresourcemanager.ListProjectsResponse) error {
		for _, p := range page.Projects {
			projects = append(projects, projectFromAPI(p))
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

func projectFromAPI(p *cloudresourcemanager.Project) ProjectInfo {
	if p == nil {
		return ProjectInfo{}
	}
	return ProjectInfo{
		ProjectID:     p.ProjectId,
		Name:          p.Name,
		ProjectNumber: p.ProjectNumber,
		State:         p.LifecycleState,
		CreateTime:    p.CreateTime,
	}
}
