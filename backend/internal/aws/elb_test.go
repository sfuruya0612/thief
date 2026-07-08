package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

func TestElbFromLB(t *testing.T) {
	tests := []struct {
		name string
		in   elbv2types.LoadBalancer
		want ELBResource
	}{
		{
			name: "active",
			in: elbv2types.LoadBalancer{
				LoadBalancerArn:  aws.String("arn:1"),
				LoadBalancerName: aws.String("web"),
				State:            &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActive},
				Type:             elbv2types.LoadBalancerTypeEnumApplication,
				Scheme:           elbv2types.LoadBalancerSchemeEnumInternetFacing,
				DNSName:          aws.String("web.example.com"),
				VpcId:            aws.String("vpc-1"),
				AvailabilityZones: []elbv2types.AvailabilityZone{
					{ZoneName: aws.String("ap-northeast-1a")},
					{ZoneName: aws.String("ap-northeast-1c")},
				},
			},
			want: ELBResource{
				ID:      "arn:1",
				Name:    "web",
				State:   "active",
				Type:    "application",
				Scheme:  "internet-facing",
				DNSName: "web.example.com",
				VpcID:   "vpc-1",
				AZs:     []string{"ap-northeast-1a", "ap-northeast-1c"},
			},
		},
		{
			name: "active_impaired uses hyphenated display",
			in: elbv2types.LoadBalancer{
				LoadBalancerArn: aws.String("arn:2"),
				State:           &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActiveImpaired},
			},
			want: ELBResource{ID: "arn:2", State: "active-impaired"},
		},
		{
			name: "nil state defaults to unknown",
			in:   elbv2types.LoadBalancer{LoadBalancerArn: aws.String("arn:3")},
			want: ELBResource{ID: "arn:3", State: "unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := elbFromLB(tt.in)
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.State != tt.want.State ||
				got.Type != tt.want.Type || got.Scheme != tt.want.Scheme ||
				got.DNSName != tt.want.DNSName || got.VpcID != tt.want.VpcID ||
				!equalStrs(got.AZs, tt.want.AZs) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
