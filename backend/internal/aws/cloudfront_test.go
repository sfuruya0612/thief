package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
)

func TestCloudfrontFromSummary(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "deployed", status: "Deployed", want: "deployed"},
		{name: "in progress", status: "InProgress", want: "in-progress"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cftypes.DistributionSummary{
				Id:      aws.String("E123"),
				Comment: aws.String("test"),
				Status:  aws.String(tt.status),
			}
			got := cloudfrontFromSummary(d)
			if got.State != tt.want {
				t.Errorf("state = %q, want %q", got.State, tt.want)
			}
		})
	}
}
