package aws

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestNatgwFromGateway(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name string
		in   ec2types.NatGateway
		want NATGatewayResource
	}{
		{
			name: "full",
			in: ec2types.NatGateway{
				NatGatewayId: aws.String("nat-1"),
				State:        ec2types.NatGatewayStateAvailable,
				VpcId:        aws.String("vpc-1"),
				SubnetId:     aws.String("subnet-1"),
				CreateTime:   aws.Time(created),
				NatGatewayAddresses: []ec2types.NatGatewayAddress{
					{PublicIp: aws.String("1.2.3.4")},
				},
				Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("prod-nat")}},
			},
			want: NATGatewayResource{
				ID:         "nat-1",
				Name:       "prod-nat",
				State:      "available",
				VpcID:      "vpc-1",
				SubnetID:   "subnet-1",
				ElasticIP:  "1.2.3.4",
				Tags:       map[string]string{"Name": "prod-nat"},
				LaunchTime: created,
			},
		},
		{
			name: "no addresses",
			in:   ec2types.NatGateway{NatGatewayId: aws.String("nat-2")},
			want: NATGatewayResource{ID: "nat-2", Tags: map[string]string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := natgwFromGateway(tt.in)
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.State != tt.want.State ||
				got.VpcID != tt.want.VpcID || got.SubnetID != tt.want.SubnetID ||
				got.ElasticIP != tt.want.ElasticIP || !got.LaunchTime.Equal(tt.want.LaunchTime) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
