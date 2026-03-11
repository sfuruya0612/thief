package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

type mockIAMApi struct {
	listUsersOutput                *iam.ListUsersOutput
	listUsersErr                   error
	listGroupsForUserOutput        *iam.ListGroupsForUserOutput
	listGroupsForUserErr           error
	listAttachedUserPoliciesOutput *iam.ListAttachedUserPoliciesOutput
	listAttachedUserPoliciesErr    error
}

func (m *mockIAMApi) ListUsers(ctx context.Context, input *iam.ListUsersInput, opts ...func(*iam.Options)) (*iam.ListUsersOutput, error) {
	return m.listUsersOutput, m.listUsersErr
}

func (m *mockIAMApi) ListGroupsForUser(ctx context.Context, input *iam.ListGroupsForUserInput, opts ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error) {
	return m.listGroupsForUserOutput, m.listGroupsForUserErr
}

func (m *mockIAMApi) ListAttachedUserPolicies(ctx context.Context, input *iam.ListAttachedUserPoliciesInput, opts ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
	return m.listAttachedUserPoliciesOutput, m.listAttachedUserPoliciesErr
}

func TestListIAMUsers(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name         string
		mock         *mockIAMApi
		wantLen      int
		wantErr      bool
		wantGroups   string
		wantPolicies string
	}{
		{
			name: "returns users with groups and policies",
			mock: &mockIAMApi{
				listUsersOutput: &iam.ListUsersOutput{
					Users: []types.User{
						{
							UserName:   aws.String("alice"),
							UserId:     aws.String("AIDAAAAAAAAAAAAAAAAAA"),
							CreateDate: &now,
						},
					},
					IsTruncated: false,
				},
				listGroupsForUserOutput: &iam.ListGroupsForUserOutput{
					Groups: []types.Group{
						{GroupName: aws.String("developers"), GroupId: aws.String("g1"), Arn: aws.String("arn:aws:iam::123:group/developers"), Path: aws.String("/"), CreateDate: &now},
						{GroupName: aws.String("admins"), GroupId: aws.String("g2"), Arn: aws.String("arn:aws:iam::123:group/admins"), Path: aws.String("/"), CreateDate: &now},
					},
					IsTruncated: false,
				},
				listAttachedUserPoliciesOutput: &iam.ListAttachedUserPoliciesOutput{
					AttachedPolicies: []types.AttachedPolicy{
						{PolicyName: aws.String("ReadOnlyAccess"), PolicyArn: aws.String("arn:aws:iam::aws:policy/ReadOnlyAccess")},
					},
					IsTruncated: false,
				},
			},
			wantLen:      1,
			wantGroups:   "developers,admins",
			wantPolicies: "ReadOnlyAccess",
		},
		{
			name: "user with no groups or policies",
			mock: &mockIAMApi{
				listUsersOutput: &iam.ListUsersOutput{
					Users: []types.User{
						{UserName: aws.String("bob"), UserId: aws.String("AIDABBBBBBBBBBBBBBBBB"), CreateDate: &now},
					},
					IsTruncated: false,
				},
				listGroupsForUserOutput:        &iam.ListGroupsForUserOutput{IsTruncated: false},
				listAttachedUserPoliciesOutput: &iam.ListAttachedUserPoliciesOutput{IsTruncated: false},
			},
			wantLen:      1,
			wantGroups:   "",
			wantPolicies: "",
		},
		{
			name:    "list users api error",
			mock:    &mockIAMApi{listUsersErr: errors.New("api error")},
			wantErr: true,
		},
		{
			name: "list groups api error",
			mock: &mockIAMApi{
				listUsersOutput: &iam.ListUsersOutput{
					Users:       []types.User{{UserName: aws.String("carol"), UserId: aws.String("AIDACCCCCCCCCCCCCCCCC"), CreateDate: &now}},
					IsTruncated: false,
				},
				listGroupsForUserErr: errors.New("groups error"),
			},
			wantErr: true,
		},
		{
			name: "list policies api error",
			mock: &mockIAMApi{
				listUsersOutput: &iam.ListUsersOutput{
					Users:       []types.User{{UserName: aws.String("dave"), UserId: aws.String("AIDADDDDDDDDDDDDDDDDD"), CreateDate: &now}},
					IsTruncated: false,
				},
				listGroupsForUserOutput:     &iam.ListGroupsForUserOutput{IsTruncated: false},
				listAttachedUserPoliciesErr: errors.New("policies error"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ListIAMUsers(tt.mock)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("expected %d users, got %d", tt.wantLen, len(result))
			}
			if tt.wantLen > 0 {
				if result[0].Groups != tt.wantGroups {
					t.Errorf("expected Groups %q, got %q", tt.wantGroups, result[0].Groups)
				}
				if result[0].Policies != tt.wantPolicies {
					t.Errorf("expected Policies %q, got %q", tt.wantPolicies, result[0].Policies)
				}
			}
		})
	}
}
