// Package aws provides AWS service clients and utilities for interacting with AWS services.
package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// iamApi defines the interface for IAM API operations used by this package.
type iamApi interface {
	ListUsers(ctx context.Context, input *iam.ListUsersInput, opts ...func(*iam.Options)) (*iam.ListUsersOutput, error)
	ListGroupsForUser(ctx context.Context, input *iam.ListGroupsForUserInput, opts ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error)
	ListAttachedUserPolicies(ctx context.Context, input *iam.ListAttachedUserPoliciesInput, opts ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error)
}

// NewIAMClient creates a new IAM client using the specified AWS profile and region.
func NewIAMClient(profile, region string) (iamApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create iam client: %w", err)
	}
	return iam.NewFromConfig(cfg), nil
}

// IAMUserInfo holds the display fields for an IAM user.
type IAMUserInfo struct {
	UserName   string
	UserID     string
	Groups     string // comma-separated group names
	Policies   string // comma-separated attached managed policy names
	CreateDate string
}

// ToRow converts IAMUserInfo to a string slice suitable for table formatting.
func (u IAMUserInfo) ToRow() []string {
	return []string{u.UserName, u.UserID, u.Groups, u.Policies, u.CreateDate}
}

// ListIAMUsers retrieves all IAM users along with their attached groups and
// managed policies. Groups and policies are returned as comma-separated strings.
func ListIAMUsers(api iamApi) ([]IAMUserInfo, error) {
	// Collect all users with pagination.
	var users []IAMUserInfo

	var nextMarker *string
	for {
		o, err := api.ListUsers(context.Background(), &iam.ListUsersInput{
			Marker: nextMarker,
		})
		if err != nil {
			return nil, err
		}

		for _, u := range o.Users {
			userName := aws.ToString(u.UserName)

			groups, err := listGroupNamesForUser(api, userName)
			if err != nil {
				return nil, fmt.Errorf("list groups for user %s: %w", userName, err)
			}

			policies, err := listAttachedPolicyNamesForUser(api, userName)
			if err != nil {
				return nil, fmt.Errorf("list policies for user %s: %w", userName, err)
			}

			users = append(users, IAMUserInfo{
				UserName:   userName,
				UserID:     aws.ToString(u.UserId),
				Groups:     strings.Join(groups, ","),
				Policies:   strings.Join(policies, ","),
				CreateDate: u.CreateDate.String(),
			})
		}

		if !o.IsTruncated {
			break
		}
		nextMarker = o.Marker
	}

	return users, nil
}

// listGroupNamesForUser returns the names of all groups the user belongs to.
func listGroupNamesForUser(api iamApi, userName string) ([]string, error) {
	var names []string
	var nextMarker *string
	for {
		o, err := api.ListGroupsForUser(context.Background(), &iam.ListGroupsForUserInput{
			UserName: aws.String(userName),
			Marker:   nextMarker,
		})
		if err != nil {
			return nil, err
		}
		for _, g := range o.Groups {
			names = append(names, aws.ToString(g.GroupName))
		}
		if !o.IsTruncated {
			break
		}
		nextMarker = o.Marker
	}
	return names, nil
}

// listAttachedPolicyNamesForUser returns the names of all managed policies
// directly attached to the user.
func listAttachedPolicyNamesForUser(api iamApi, userName string) ([]string, error) {
	var names []string
	var nextMarker *string
	for {
		o, err := api.ListAttachedUserPolicies(context.Background(), &iam.ListAttachedUserPoliciesInput{
			UserName: aws.String(userName),
			Marker:   nextMarker,
		})
		if err != nil {
			return nil, err
		}
		for _, p := range o.AttachedPolicies {
			names = append(names, aws.ToString(p.PolicyName))
		}
		if !o.IsTruncated {
			break
		}
		nextMarker = o.Marker
	}
	return names, nil
}
