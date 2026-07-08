package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// IAMResource represents an IAM user.
type IAMResource struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	State        string   `json:"state"`
	ARN          string   `json:"arn"`
	Kind         string   `json:"kind"` // user
	MFAEnabled   bool     `json:"mfa_enabled"`
	LastActivity string   `json:"last_activity"`
	Groups       []string `json:"groups"`
	Policies     []string `json:"policies"`
}

func (r IAMResource) ResourceID() string    { return r.ID }
func (r IAMResource) ResourceName() string  { return r.Name }
func (r IAMResource) ResourceState() string { return "active" }
func (r IAMResource) ServiceName() string   { return "iam" }

// ListIAMResources returns all IAM users and roles for the given profile.
// IAM is a global service; region is ignored.
func ListIAMResources(ctx context.Context, profile, _ string) ([]IAMResource, error) {
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *iam.Client {
		return iam.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []IAMResource
	userPaginator := iam.NewListUsersPaginator(client, &iam.ListUsersInput{})
	for userPaginator.HasMorePages() {
		page, err := userPaginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list iam users: %w", err)
		}
		for _, u := range page.Users {
			r, err := iamFromUser(ctx, client, u)
			if err != nil {
				return nil, err
			}
			resources = append(resources, r)
		}
	}

	rolePaginator := iam.NewListRolesPaginator(client, &iam.ListRolesInput{})
	for rolePaginator.HasMorePages() {
		page, err := rolePaginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list iam roles: %w", err)
		}
		for _, role := range page.Roles {
			r, err := iamFromRole(ctx, client, role)
			if err != nil {
				return nil, err
			}
			resources = append(resources, r)
		}
	}
	return resources, nil
}

func iamFromUser(ctx context.Context, client *iam.Client, u iamtypes.User) (IAMResource, error) {
	name := ptrStr(u.UserName)

	// MFA devices
	mfaOut, err := client.ListMFADevices(ctx, &iam.ListMFADevicesInput{UserName: aws.String(name)})
	mfaEnabled := false
	if err == nil {
		mfaEnabled = len(mfaOut.MFADevices) > 0
	}

	// Groups
	groupsOut, err := client.ListGroupsForUser(ctx, &iam.ListGroupsForUserInput{UserName: aws.String(name)})
	var groups []string
	if err == nil {
		for _, g := range groupsOut.Groups {
			groups = append(groups, ptrStr(g.GroupName))
		}
	}

	// Attached policies
	policiesOut, err := client.ListAttachedUserPolicies(ctx, &iam.ListAttachedUserPoliciesInput{UserName: aws.String(name)})
	var policies []string
	if err == nil {
		for _, p := range policiesOut.AttachedPolicies {
			policies = append(policies, ptrStr(p.PolicyName))
		}
	}

	var passwordLastUsed *time.Time
	if u.PasswordLastUsed != nil {
		passwordLastUsed = u.PasswordLastUsed
	}

	return newIAMUserResource(ptrStr(u.UserId), name, ptrStr(u.Arn), mfaEnabled, passwordLastUsed, groups, policies), nil
}

func newIAMUserResource(id, name, arn string, mfaEnabled bool, passwordLastUsed *time.Time, groups, policies []string) IAMResource {
	lastActivity := ""
	if passwordLastUsed != nil {
		lastActivity = passwordLastUsed.Format(time.RFC3339)
	}
	return IAMResource{
		ID:           id,
		Name:         name,
		ARN:          arn,
		Kind:         "user",
		MFAEnabled:   mfaEnabled,
		LastActivity: lastActivity,
		Groups:       groups,
		Policies:     policies,
	}
}

func iamFromRole(ctx context.Context, client *iam.Client, role iamtypes.Role) (IAMResource, error) {
	name := ptrStr(role.RoleName)

	policiesOut, err := client.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: aws.String(name)})
	var policies []string
	if err == nil {
		for _, p := range policiesOut.AttachedPolicies {
			policies = append(policies, ptrStr(p.PolicyName))
		}
	}

	var lastUsed *time.Time
	if role.RoleLastUsed != nil && role.RoleLastUsed.LastUsedDate != nil {
		lastUsed = role.RoleLastUsed.LastUsedDate
	}

	return newIAMRoleResource(ptrStr(role.RoleId), name, ptrStr(role.Arn), lastUsed, policies), nil
}

func newIAMRoleResource(id, name, arn string, lastUsed *time.Time, policies []string) IAMResource {
	lastActivity := ""
	if lastUsed != nil {
		lastActivity = lastUsed.Format(time.RFC3339)
	}
	return IAMResource{
		ID:           id,
		Name:         name,
		ARN:          arn,
		Kind:         "role",
		LastActivity: lastActivity,
		Policies:     policies,
	}
}
