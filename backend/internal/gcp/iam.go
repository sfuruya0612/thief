package gcp

import (
	"context"
	"fmt"

	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	iam "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
)

// IAMBindingInfo は IAM ポリシーのバインディングをメンバー単位に展開した 1 行を表す。
// GCP の IAM はロール ↔ メンバーのバインディング形式であり、AWS の IAM User/Role のような
// 単体リソースを持たないため、プロジェクトの IAM ポリシーをメンバー単位で平坦化して返す。
type IAMBindingInfo struct {
	Member         string `json:"member"`
	Role           string `json:"role"`
	ProjectID      string `json:"project_id"`
	ConditionTitle string `json:"condition_title"`
}

// ListIAMBindings は指定プロジェクトの IAM ポリシーを取得し、メンバー単位に展開して返す。
func ListIAMBindings(ctx context.Context, projectID string) ([]IAMBindingInfo, error) {
	svc, err := cloudresourcemanager.NewService(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		return nil, fmt.Errorf("create cloudresourcemanager service: %w", err)
	}

	policy, err := svc.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get iam policy for %s: %w", projectID, err)
	}

	var bindings []IAMBindingInfo
	for _, b := range policy.Bindings {
		for _, member := range b.Members {
			bindings = append(bindings, iamBindingFromAPI(b, member, projectID))
		}
	}
	return bindings, nil
}

func iamBindingFromAPI(b *cloudresourcemanager.Binding, member, projectID string) IAMBindingInfo {
	info := IAMBindingInfo{
		Member:    member,
		Role:      b.Role,
		ProjectID: projectID,
	}
	if b.Condition != nil {
		info.ConditionTitle = b.Condition.Title
	}
	return info
}

// ServiceAccountInfo は Service Account の表示用メタデータ。
type ServiceAccountInfo struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	ProjectID   string `json:"project_id"`
	UniqueID    string `json:"unique_id"`
	Disabled    bool   `json:"disabled"`
}

// ListServiceAccounts は指定プロジェクトの Service Account を列挙する。
func ListServiceAccounts(ctx context.Context, projectID string) ([]ServiceAccountInfo, error) {
	svc, err := iam.NewService(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		return nil, fmt.Errorf("create iam service: %w", err)
	}

	var accounts []ServiceAccountInfo
	call := svc.Projects.ServiceAccounts.List("projects/" + projectID)
	if err := call.Pages(ctx, func(page *iam.ListServiceAccountsResponse) error {
		for _, a := range page.Accounts {
			accounts = append(accounts, serviceAccountFromAPI(a))
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("list service accounts for %s: %w", projectID, err)
	}
	return accounts, nil
}

func serviceAccountFromAPI(a *iam.ServiceAccount) ServiceAccountInfo {
	if a == nil {
		return ServiceAccountInfo{}
	}
	return ServiceAccountInfo{
		Email:       a.Email,
		DisplayName: a.DisplayName,
		Description: a.Description,
		ProjectID:   a.ProjectId,
		UniqueID:    a.UniqueId,
		Disabled:    a.Disabled,
	}
}
