package aws

import (
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/go-cmp/cmp"
)

func TestEc2InstanceInfoFromSDK(t *testing.T) {
	launchTime := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		inst ec2types.Instance
		want EC2InstanceInfo
	}{
		{
			name: "full fields",
			inst: ec2types.Instance{
				InstanceId:        awssdk.String("i-0123456789abcdef0"),
				InstanceType:      ec2types.InstanceTypeT3Micro,
				InstanceLifecycle: ec2types.InstanceLifecycleTypeSpot,
				PrivateIpAddress:  awssdk.String("10.0.0.1"),
				PublicIpAddress:   awssdk.String("203.0.113.1"),
				KeyName:           awssdk.String("my-key"),
				State:             &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
				Placement:         &ec2types.Placement{AvailabilityZone: awssdk.String("ap-northeast-1a")},
				LaunchTime:        awssdk.Time(launchTime),
				Tags: []ec2types.Tag{
					{Key: awssdk.String("Name"), Value: awssdk.String("web-server")},
				},
			},
			want: EC2InstanceInfo{
				Name:         "web-server",
				InstanceID:   "i-0123456789abcdef0",
				InstanceType: "t3.micro",
				Lifecycle:    "spot",
				PrivateIP:    "10.0.0.1",
				PublicIP:     "203.0.113.1",
				State:        "running",
				KeyName:      "my-key",
				AZ:           "ap-northeast-1a",
				LaunchTime:   launchTime.String(),
			},
		},
		{
			name: "missing optional fields use None defaults",
			inst: ec2types.Instance{
				InstanceId:   awssdk.String("i-0fedcba9876543210"),
				InstanceType: ec2types.InstanceTypeT3Small,
				State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped},
				Placement:    &ec2types.Placement{AvailabilityZone: awssdk.String("ap-northeast-1c")},
			},
			want: EC2InstanceInfo{
				Name:         "",
				InstanceID:   "i-0fedcba9876543210",
				InstanceType: "t3.small",
				Lifecycle:    "OnDemand",
				PrivateIP:    "None",
				PublicIP:     "None",
				State:        "stopped",
				KeyName:      "None",
				AZ:           "ap-northeast-1c",
				LaunchTime:   "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec2InstanceInfoFromSDK(tt.inst)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ec2InstanceInfoFromSDK mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
