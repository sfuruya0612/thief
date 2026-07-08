package aws

import (
	"reflect"
	"testing"
	"time"
)

func TestNewIAMUserResource(t *testing.T) {
	lastUsed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name             string
		id               string
		userName         string
		arn              string
		mfaEnabled       bool
		passwordLastUsed *time.Time
		groups           []string
		policies         []string
		want             IAMResource
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newIAMUserResource(tt.id, tt.userName, tt.arn, tt.mfaEnabled, tt.passwordLastUsed, tt.groups, tt.policies)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestNewIAMRoleResource(t *testing.T) {
	lastUsed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name     string
		id       string
		roleName string
		arn      string
		lastUsed *time.Time
		policies []string
		want     IAMResource
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newIAMRoleResource(tt.id, tt.roleName, tt.arn, tt.lastUsed, tt.policies)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
