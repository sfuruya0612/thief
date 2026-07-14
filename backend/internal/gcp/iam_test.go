package gcp

import (
	"reflect"
	"testing"

	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	iam "google.golang.org/api/iam/v1"
)

func TestIamBindingFromAPI(t *testing.T) {
	tests := []struct {
		name      string
		binding   *cloudresourcemanager.Binding
		member    string
		projectID string
		want      IAMBindingInfo
	}{
		{
			name:      "condition なし",
			binding:   &cloudresourcemanager.Binding{Role: "roles/owner"},
			member:    "user:alice@example.com",
			projectID: "proj-1",
			want: IAMBindingInfo{
				Member:    "user:alice@example.com",
				Role:      "roles/owner",
				ProjectID: "proj-1",
			},
		},
		{
			name: "condition あり",
			binding: &cloudresourcemanager.Binding{
				Role:      "roles/viewer",
				Condition: &cloudresourcemanager.Expr{Title: "expirable access"},
			},
			member:    "group:admins@example.com",
			projectID: "proj-2",
			want: IAMBindingInfo{
				Member:         "group:admins@example.com",
				Role:           "roles/viewer",
				ProjectID:      "proj-2",
				ConditionTitle: "expirable access",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := iamBindingFromAPI(tt.binding, tt.member, tt.projectID)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("iamBindingFromAPI() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestServiceAccountFromAPI(t *testing.T) {
	tests := []struct {
		name string
		in   *iam.ServiceAccount
		want ServiceAccountInfo
	}{
		{
			name: "nil",
			in:   nil,
			want: ServiceAccountInfo{},
		},
		{
			name: "全フィールド",
			in: &iam.ServiceAccount{
				Email:       "sa@proj.iam.gserviceaccount.com",
				DisplayName: "My SA",
				Description: "for testing",
				ProjectId:   "proj-1",
				UniqueId:    "12345",
				Disabled:    true,
			},
			want: ServiceAccountInfo{
				Email:       "sa@proj.iam.gserviceaccount.com",
				DisplayName: "My SA",
				Description: "for testing",
				ProjectID:   "proj-1",
				UniqueID:    "12345",
				Disabled:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serviceAccountFromAPI(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("serviceAccountFromAPI() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
