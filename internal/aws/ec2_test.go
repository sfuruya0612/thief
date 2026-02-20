package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

var mockOutput = &ec2.DescribeInstancesOutput{
	Reservations: []types.Reservation{
		{
			Instances: []types.Instance{
				{
					InstanceId:        aws.String("i-1234567890abcdef0"),
					InstanceType:      types.InstanceTypeT2Micro,
					InstanceLifecycle: "OnDemand",
					PrivateIpAddress:  aws.String("192.168.1.1"),
					PublicIpAddress:   aws.String("203.0.113.1"),
					State:             &types.InstanceState{Name: types.InstanceStateNameRunning},
					KeyName:           aws.String("my-key"),
					Placement:         &types.Placement{AvailabilityZone: aws.String("ap-northeast-1a")},
					LaunchTime:        aws.Time(time.Now()),
					Tags: []types.Tag{
						{Key: aws.String("Name"), Value: aws.String("MyInstance")},
					},
				},
			},
		},
	},
}

type mockEc2Api struct {
	describeInstancesOutput *ec2.DescribeInstancesOutput
	describeInstancesErr    error
	describeRegionsOutput   *ec2.DescribeRegionsOutput
	describeRegionsErr      error
}

func (m *mockEc2Api) DescribeInstances(ctx context.Context, input *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return m.describeInstancesOutput, m.describeInstancesErr
}

func (m *mockEc2Api) DescribeRegions(ctx context.Context, input *ec2.DescribeRegionsInput, opts ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	return m.describeRegionsOutput, m.describeRegionsErr
}

func TestGenerateDescribeInstancesInput(t *testing.T) {
	tests := []struct {
		name    string
		opts    *Ec2Opts
		wantErr bool
	}{
		{
			name: "running instances",
			opts: &Ec2Opts{Running: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := GenerateDescribeInstancesInput(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if input == nil {
					t.Fatal("expected non-nil input, got nil")
				}
			}
		})
	}
}

func TestDescribeInstances(t *testing.T) {
	mockApi := &mockEc2Api{
		describeInstancesOutput: mockOutput,
		describeInstancesErr:    nil,
	}

	input := &ec2.DescribeInstancesInput{}
	result, err := DescribeInstances(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[0].Name != "MyInstance" {
		t.Errorf("expected Name 'MyInstance', got '%s'", result[0].Name)
	}
	if result[0].InstanceID != "i-1234567890abcdef0" {
		t.Errorf("expected InstanceID 'i-1234567890abcdef0', got '%s'", result[0].InstanceID)
	}
	if result[0].InstanceType != "t2.micro" {
		t.Errorf("expected InstanceType 't2.micro', got '%s'", result[0].InstanceType)
	}
	if result[0].Lifecycle != "OnDemand" {
		t.Errorf("expected Lifecycle 'OnDemand', got '%s'", result[0].Lifecycle)
	}
	if result[0].PrivateIP != "192.168.1.1" {
		t.Errorf("expected PrivateIP '192.168.1.1', got '%s'", result[0].PrivateIP)
	}
	if result[0].PublicIP != "203.0.113.1" {
		t.Errorf("expected PublicIP '203.0.113.1', got '%s'", result[0].PublicIP)
	}
	if result[0].State != "running" {
		t.Errorf("expected State 'running', got '%s'", result[0].State)
	}
	if result[0].KeyName != "my-key" {
		t.Errorf("expected KeyName 'my-key', got '%s'", result[0].KeyName)
	}
	if result[0].AZ != "ap-northeast-1a" {
		t.Errorf("expected AZ 'ap-northeast-1a', got '%s'", result[0].AZ)
	}
	if result[0].LaunchTime == "" {
		t.Error("expected non-empty LaunchTime")
	}
}

func TestDescribeInstances_Error(t *testing.T) {
	mockApi := &mockEc2Api{
		describeInstancesOutput: mockOutput,
		describeInstancesErr:    errors.New("error"),
	}

	input := &ec2.DescribeInstancesInput{}
	result, err := DescribeInstances(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}
