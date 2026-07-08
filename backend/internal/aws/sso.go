package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
)

// SSOAccountResource represents an AWS SSO account assignment.
type SSOAccountResource struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	State        string   `json:"state"`
	EmailAddress string   `json:"email_address"`
	Roles        []string `json:"roles"`
}

func (r SSOAccountResource) ResourceID() string    { return r.ID }
func (r SSOAccountResource) ResourceName() string  { return r.Name }
func (r SSOAccountResource) ResourceState() string { return "active" }
func (r SSOAccountResource) ServiceName() string   { return "sso" }

// ListSSOAccounts returns all SSO-accessible accounts using the cached SSO token.
// The accessToken must be obtained from the local SSO token cache.
func ListSSOAccounts(ctx context.Context, profile, region string) ([]SSOAccountResource, error) {
	accessToken, err := loadSSOAccessToken(profile)
	if err != nil {
		return nil, fmt.Errorf("load sso token for profile %s: %w", profile, err)
	}

	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *sso.Client {
		return sso.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var accounts []SSOAccountResource
	paginator := sso.NewListAccountsPaginator(client, &sso.ListAccountsInput{
		AccessToken: aws.String(accessToken),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sso accounts: %w", err)
		}
		for _, a := range page.AccountList {
			roles, _ := listSSOAccountRoles(ctx, client, accessToken, a)
			accounts = append(accounts, ssoFromAccount(a, roles))
		}
	}
	return accounts, nil
}

func listSSOAccountRoles(ctx context.Context, client *sso.Client, accessToken string, account ssotypes.AccountInfo) ([]string, error) {
	var roles []string
	paginator := sso.NewListAccountRolesPaginator(client, &sso.ListAccountRolesInput{
		AccessToken: aws.String(accessToken),
		AccountId:   account.AccountId,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return roles, err
		}
		for _, r := range page.RoleList {
			roles = append(roles, ptrStr(r.RoleName))
		}
	}
	return roles, nil
}

func ssoFromAccount(a ssotypes.AccountInfo, roles []string) SSOAccountResource {
	return SSOAccountResource{
		ID:           ptrStr(a.AccountId),
		Name:         ptrStr(a.AccountName),
		EmailAddress: ptrStr(a.EmailAddress),
		Roles:        roles,
	}
}
