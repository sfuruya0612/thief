package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
)

func TestGetSession(t *testing.T) {
	t.Skip("Skipping TestGetSession as it requires AWS credentials/config to be configured")
	tests := []struct {
		name    string
		profile string
		region  string
		wantErr bool
	}{
		{
			name:    "Valid profile and region",
			profile: "default",
			region:  "us-west-2",
			wantErr: false,
		},
		{
			name:    "Invalid profile",
			profile: "non-existent-profile",
			region:  "us-west-2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.LoadDefaultConfig(context.TODO(),
				config.WithSharedConfigProfile(tt.profile),
				config.WithRegion(tt.region),
			)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
