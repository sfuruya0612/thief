package aws

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// mockIAMUserDetailClient は iamUserDetailClient の手書きモック。
type mockIAMUserDetailClient struct {
	listMFADevices           func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error)
	listGroupsForUser        func(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error)
	listAttachedUserPolicies func(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error)
}

func (m *mockIAMUserDetailClient) ListMFADevices(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
	return m.listMFADevices(ctx, params, optFns...)
}

func (m *mockIAMUserDetailClient) ListGroupsForUser(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error) {
	return m.listGroupsForUser(ctx, params, optFns...)
}

func (m *mockIAMUserDetailClient) ListAttachedUserPolicies(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
	return m.listAttachedUserPolicies(ctx, params, optFns...)
}

// mockIAMRoleDetailClient は iamRoleDetailClient の手書きモック。
type mockIAMRoleDetailClient struct {
	listAttachedRolePolicies func(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
}

func (m *mockIAMRoleDetailClient) ListAttachedRolePolicies(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	return m.listAttachedRolePolicies(ctx, params, optFns...)
}

func TestNewIAMUserResource(t *testing.T) {
	lastUsed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name                  string
		id                    string
		userName              string
		arn                   string
		mfaEnabled            bool
		mfaEnabledFetchFailed bool
		passwordLastUsed      *time.Time
		groups                []string
		groupsFetchFailed     bool
		policies              []string
		policiesFetchFailed   bool
		want                  IAMResource
	}{
		{
			name:             "mfa enabled with groups and policies",
			id:               "AIDA1",
			userName:         "alice",
			arn:              "arn:aws:iam::123456789012:user/alice",
			mfaEnabled:       true,
			passwordLastUsed: &lastUsed,
			groups:           []string{"admins"},
			policies:         []string{"AdministratorAccess"},
			want: IAMResource{
				ID:           "AIDA1",
				Name:         "alice",
				ARN:          "arn:aws:iam::123456789012:user/alice",
				Kind:         "user",
				MFAEnabled:   true,
				LastActivity: lastUsed.Format(time.RFC3339),
				Groups:       []string{"admins"},
				Policies:     []string{"AdministratorAccess"},
			},
		},
		{
			name:             "no mfa no password last used",
			id:               "AIDA2",
			userName:         "bob",
			arn:              "arn:aws:iam::123456789012:user/bob",
			mfaEnabled:       false,
			passwordLastUsed: nil,
			groups:           nil,
			policies:         nil,
			want: IAMResource{
				ID:           "AIDA2",
				Name:         "bob",
				ARN:          "arn:aws:iam::123456789012:user/bob",
				Kind:         "user",
				MFAEnabled:   false,
				LastActivity: "",
				Groups:       nil,
				Policies:     nil,
			},
		},
		{
			name:                  "mfa と groups と policies の取得失敗を反映する",
			id:                    "AIDA3",
			userName:              "degraded-user",
			arn:                   "arn:aws:iam::123456789012:user/degraded-user",
			mfaEnabled:            false,
			mfaEnabledFetchFailed: true,
			passwordLastUsed:      nil,
			groups:                nil,
			groupsFetchFailed:     true,
			policies:              nil,
			policiesFetchFailed:   true,
			want: IAMResource{
				ID:                    "AIDA3",
				Name:                  "degraded-user",
				ARN:                   "arn:aws:iam::123456789012:user/degraded-user",
				Kind:                  "user",
				MFAEnabled:            false,
				MFAEnabledFetchFailed: true,
				LastActivity:          "",
				Groups:                nil,
				GroupsFetchFailed:     true,
				Policies:              nil,
				PoliciesFetchFailed:   true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newIAMUserResource(tt.id, tt.userName, tt.arn, tt.mfaEnabled, tt.mfaEnabledFetchFailed, tt.passwordLastUsed, tt.groups, tt.groupsFetchFailed, tt.policies, tt.policiesFetchFailed)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestNewIAMRoleResource(t *testing.T) {
	lastUsed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name                string
		id                  string
		roleName            string
		arn                 string
		lastUsed            *time.Time
		policies            []string
		policiesFetchFailed bool
		want                IAMResource
	}{
		{
			name:     "role with last used and policies",
			id:       "AROA1",
			roleName: "deploy-role",
			arn:      "arn:aws:iam::123456789012:role/deploy-role",
			lastUsed: &lastUsed,
			policies: []string{"AmazonS3FullAccess"},
			want: IAMResource{
				ID:           "AROA1",
				Name:         "deploy-role",
				ARN:          "arn:aws:iam::123456789012:role/deploy-role",
				Kind:         "role",
				LastActivity: lastUsed.Format(time.RFC3339),
				Policies:     []string{"AmazonS3FullAccess"},
			},
		},
		{
			name:     "role never used no policies",
			id:       "AROA2",
			roleName: "unused-role",
			arn:      "arn:aws:iam::123456789012:role/unused-role",
			lastUsed: nil,
			policies: nil,
			want: IAMResource{
				ID:           "AROA2",
				Name:         "unused-role",
				ARN:          "arn:aws:iam::123456789012:role/unused-role",
				Kind:         "role",
				LastActivity: "",
				Policies:     nil,
			},
		},
		{
			name:                "policies の取得失敗を反映し mfa と groups は常に false",
			id:                  "AROA3",
			roleName:            "degraded-role",
			arn:                 "arn:aws:iam::123456789012:role/degraded-role",
			lastUsed:            nil,
			policies:            nil,
			policiesFetchFailed: true,
			want: IAMResource{
				ID:                    "AROA3",
				Name:                  "degraded-role",
				ARN:                   "arn:aws:iam::123456789012:role/degraded-role",
				Kind:                  "role",
				MFAEnabledFetchFailed: false,
				LastActivity:          "",
				Groups:                nil,
				GroupsFetchFailed:     false,
				Policies:              nil,
				PoliciesFetchFailed:   true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newIAMRoleResource(tt.id, tt.roleName, tt.arn, tt.lastUsed, tt.policies, tt.policiesFetchFailed)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestIAMFromUser(t *testing.T) {
	user := iamtypes.User{
		UserId:   aws.String("AIDA1"),
		UserName: aws.String("alice"),
		Arn:      aws.String("arn:aws:iam::123456789012:user/alice"),
	}

	t.Run("mfa と groups と policies の取得が全て成功する", func(t *testing.T) {
		client := &mockIAMUserDetailClient{
			listMFADevices: func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
				return &iam.ListMFADevicesOutput{MFADevices: []iamtypes.MFADevice{{}}}, nil
			},
			listGroupsForUser: func(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error) {
				return &iam.ListGroupsForUserOutput{Groups: []iamtypes.Group{{GroupName: aws.String("admins")}}}, nil
			},
			listAttachedUserPolicies: func(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
				return &iam.ListAttachedUserPoliciesOutput{AttachedPolicies: []iamtypes.AttachedPolicy{{PolicyName: aws.String("AdministratorAccess")}}}, nil
			},
		}

		got, err := iamFromUser(context.Background(), client, user)
		if err != nil {
			t.Fatalf("iamFromUser() error = %v", err)
		}
		if got.MFAEnabled != true || got.MFAEnabledFetchFailed {
			t.Errorf("mfa = %v/%v, want true/false", got.MFAEnabled, got.MFAEnabledFetchFailed)
		}
		if !reflect.DeepEqual(got.Groups, []string{"admins"}) || got.GroupsFetchFailed {
			t.Errorf("groups = %v/%v, want [admins]/false", got.Groups, got.GroupsFetchFailed)
		}
		if !reflect.DeepEqual(got.Policies, []string{"AdministratorAccess"}) || got.PoliciesFetchFailed {
			t.Errorf("policies = %v/%v, want [AdministratorAccess]/false", got.Policies, got.PoliciesFetchFailed)
		}
	})

	t.Run("mfa の取得失敗で MFAEnabledFetchFailed が立つ", func(t *testing.T) {
		client := &mockIAMUserDetailClient{
			listMFADevices: func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
				return nil, errors.New("throttled")
			},
			listGroupsForUser: func(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error) {
				return &iam.ListGroupsForUserOutput{}, nil
			},
			listAttachedUserPolicies: func(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
				return &iam.ListAttachedUserPoliciesOutput{}, nil
			},
		}

		got, err := iamFromUser(context.Background(), client, user)
		if err != nil {
			t.Fatalf("iamFromUser() error = %v", err)
		}
		if got.MFAEnabled || !got.MFAEnabledFetchFailed {
			t.Errorf("mfa = %v/%v, want false/true", got.MFAEnabled, got.MFAEnabledFetchFailed)
		}
	})

	t.Run("groups の取得失敗で GroupsFetchFailed が立つ", func(t *testing.T) {
		client := &mockIAMUserDetailClient{
			listMFADevices: func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
				return &iam.ListMFADevicesOutput{}, nil
			},
			listGroupsForUser: func(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error) {
				return nil, errors.New("throttled")
			},
			listAttachedUserPolicies: func(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
				return &iam.ListAttachedUserPoliciesOutput{}, nil
			},
		}

		got, err := iamFromUser(context.Background(), client, user)
		if err != nil {
			t.Fatalf("iamFromUser() error = %v", err)
		}
		if got.Groups != nil || !got.GroupsFetchFailed {
			t.Errorf("groups = %v/%v, want nil/true", got.Groups, got.GroupsFetchFailed)
		}
	})

	t.Run("policies の取得失敗で PoliciesFetchFailed が立つ", func(t *testing.T) {
		client := &mockIAMUserDetailClient{
			listMFADevices: func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
				return &iam.ListMFADevicesOutput{}, nil
			},
			listGroupsForUser: func(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error) {
				return &iam.ListGroupsForUserOutput{}, nil
			},
			listAttachedUserPolicies: func(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
				return nil, errors.New("throttled")
			},
		}

		got, err := iamFromUser(context.Background(), client, user)
		if err != nil {
			t.Fatalf("iamFromUser() error = %v", err)
		}
		if got.Policies != nil || !got.PoliciesFetchFailed {
			t.Errorf("policies = %v/%v, want nil/true", got.Policies, got.PoliciesFetchFailed)
		}
	})

	t.Run("mfa 取得のキャンセルはエラーとして伝播する", func(t *testing.T) {
		client := &mockIAMUserDetailClient{
			listMFADevices: func(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
				return nil, context.Canceled
			},
			listGroupsForUser: func(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error) {
				t.Fatal("ListGroupsForUser は呼ばれないはず")
				return nil, nil
			},
			listAttachedUserPolicies: func(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
				t.Fatal("ListAttachedUserPolicies は呼ばれないはず")
				return nil, nil
			},
		}

		_, err := iamFromUser(context.Background(), client, user)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("iamFromUser() error = %v, want context.Canceled", err)
		}
	})
}

func TestIAMFromRole(t *testing.T) {
	role := iamtypes.Role{
		RoleId:   aws.String("AROA1"),
		RoleName: aws.String("deploy-role"),
		Arn:      aws.String("arn:aws:iam::123456789012:role/deploy-role"),
	}

	t.Run("policies の取得に成功する", func(t *testing.T) {
		client := &mockIAMRoleDetailClient{
			listAttachedRolePolicies: func(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
				return &iam.ListAttachedRolePoliciesOutput{AttachedPolicies: []iamtypes.AttachedPolicy{{PolicyName: aws.String("AmazonS3FullAccess")}}}, nil
			},
		}

		got, err := iamFromRole(context.Background(), client, role)
		if err != nil {
			t.Fatalf("iamFromRole() error = %v", err)
		}
		if !reflect.DeepEqual(got.Policies, []string{"AmazonS3FullAccess"}) || got.PoliciesFetchFailed {
			t.Errorf("policies = %v/%v, want [AmazonS3FullAccess]/false", got.Policies, got.PoliciesFetchFailed)
		}
		if got.MFAEnabledFetchFailed || got.GroupsFetchFailed {
			t.Errorf("mfa/groups fetch failed = %v/%v, want false/false", got.MFAEnabledFetchFailed, got.GroupsFetchFailed)
		}
	})

	t.Run("policies の取得失敗で PoliciesFetchFailed が立つ", func(t *testing.T) {
		client := &mockIAMRoleDetailClient{
			listAttachedRolePolicies: func(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
				return nil, errors.New("throttled")
			},
		}

		got, err := iamFromRole(context.Background(), client, role)
		if err != nil {
			t.Fatalf("iamFromRole() error = %v", err)
		}
		if got.Policies != nil || !got.PoliciesFetchFailed {
			t.Errorf("policies = %v/%v, want nil/true", got.Policies, got.PoliciesFetchFailed)
		}
	})

	t.Run("policies 取得のキャンセルはエラーとして伝播する", func(t *testing.T) {
		client := &mockIAMRoleDetailClient{
			listAttachedRolePolicies: func(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
				return nil, context.Canceled
			},
		}

		_, err := iamFromRole(context.Background(), client, role)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("iamFromRole() error = %v, want context.Canceled", err)
		}
	})
}

func TestIAMResourceJSONFetchFailedOmitempty(t *testing.T) {
	tests := []struct {
		name                  string
		mfaEnabledFetchFailed bool
		groupsFetchFailed     bool
		policiesFetchFailed   bool
		wantMFAKey            bool
		wantGroupsKey         bool
		wantPoliciesKey       bool
	}{
		{
			name:                  "全て false ならキーが省略される",
			mfaEnabledFetchFailed: false,
			groupsFetchFailed:     false,
			policiesFetchFailed:   false,
			wantMFAKey:            false,
			wantGroupsKey:         false,
			wantPoliciesKey:       false,
		},
		{
			name:                  "全て true ならキーが true で出力される",
			mfaEnabledFetchFailed: true,
			groupsFetchFailed:     true,
			policiesFetchFailed:   true,
			wantMFAKey:            true,
			wantGroupsKey:         true,
			wantPoliciesKey:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(IAMResource{
				ID:                    "user-1",
				Name:                  "alice",
				MFAEnabledFetchFailed: tt.mfaEnabledFetchFailed,
				GroupsFetchFailed:     tt.groupsFetchFailed,
				PoliciesFetchFailed:   tt.policiesFetchFailed,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			gotMFAKey := strings.Contains(string(b), `"mfa_enabled_fetch_failed":true`)
			if gotMFAKey != tt.wantMFAKey {
				t.Errorf("json = %s, mfa_enabled_fetch_failed key present = %v, want %v", b, gotMFAKey, tt.wantMFAKey)
			}
			gotGroupsKey := strings.Contains(string(b), `"groups_fetch_failed":true`)
			if gotGroupsKey != tt.wantGroupsKey {
				t.Errorf("json = %s, groups_fetch_failed key present = %v, want %v", b, gotGroupsKey, tt.wantGroupsKey)
			}
			gotPoliciesKey := strings.Contains(string(b), `"policies_fetch_failed":true`)
			if gotPoliciesKey != tt.wantPoliciesKey {
				t.Errorf("json = %s, policies_fetch_failed key present = %v, want %v", b, gotPoliciesKey, tt.wantPoliciesKey)
			}
		})
	}
}
