package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type mockSsmClient struct {
	describeInstanceInformationOutput *ssm.DescribeInstanceInformationOutput
	describeInstanceInformationErr    error
	startSessionOutput                *ssm.StartSessionOutput
	startSessionErr                   error
	terminateSessionOutput            *ssm.TerminateSessionOutput
	terminateSessionErr               error
}

func (m *mockSsmClient) DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	return m.describeInstanceInformationOutput, m.describeInstanceInformationErr
}

func (m *mockSsmClient) StartSession(ctx context.Context, params *ssm.StartSessionInput, optFns ...func(*ssm.Options)) (*ssm.StartSessionOutput, error) {
	return m.startSessionOutput, m.startSessionErr
}

func (m *mockSsmClient) TerminateSession(ctx context.Context, params *ssm.TerminateSessionInput, optFns ...func(*ssm.Options)) (*ssm.TerminateSessionOutput, error) {
	return m.terminateSessionOutput, m.terminateSessionErr
}

func TestGenerateDescribeInstanceInformationInput(t *testing.T) {
	opts := &SsmOpts{
		PingStatus:   "Online",
		ResourceType: "EC2Instance",
	}

	input := GenerateDescribeInstanceInformationInput(opts)

	if len(input.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(input.Filters))
	}
	if *input.Filters[0].Key != "PingStatus" {
		t.Errorf("expected filter key 'PingStatus', got '%s'", *input.Filters[0].Key)
	}
	if len(input.Filters[0].Values) != 1 || input.Filters[0].Values[0] != "Online" {
		t.Errorf("expected filter values ['Online'], got %v", input.Filters[0].Values)
	}
	if *input.Filters[1].Key != "ResourceType" {
		t.Errorf("expected filter key 'ResourceType', got '%s'", *input.Filters[1].Key)
	}
	if len(input.Filters[1].Values) != 1 || input.Filters[1].Values[0] != "EC2Instance" {
		t.Errorf("expected filter values ['EC2Instance'], got %v", input.Filters[1].Values)
	}
}

func TestDescribeInstanceInformation(t *testing.T) {
	instanceID := "i-1234567890abcdef0"

	mockOutput := &ssm.DescribeInstanceInformationOutput{
		InstanceInformationList: []types.InstanceInformation{
			{
				InstanceId: aws.String(instanceID),
			},
		},
	}

	mockClient := &mockSsmClient{
		describeInstanceInformationOutput: mockOutput,
		describeInstanceInformationErr:    nil,
	}

	input := &ssm.DescribeInstanceInformationInput{}
	ids, err := DescribeInstanceInformation(mockClient, input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 id, got %d", len(ids))
	}
	if ids[0] != instanceID {
		t.Errorf("expected id '%s', got '%s'", instanceID, ids[0])
	}
}

func TestGenerateStartSessionInput(t *testing.T) {
	opts := &SsmOpts{
		InstanceId: "i-1234567890abcdef0",
	}

	input := GenerateStartSessionInput(opts)

	if *input.Target != opts.InstanceId {
		t.Errorf("expected Target '%s', got '%s'", opts.InstanceId, *input.Target)
	}
}

func TestStartSession(t *testing.T) {
	sessionID := "session-1234567890"

	mockClient := &mockSsmClient{
		startSessionOutput: &ssm.StartSessionOutput{
			SessionId: aws.String(sessionID),
		},
		startSessionErr: nil,
	}

	input := &ssm.StartSessionInput{
		Target: aws.String("i-1234567890abcdef0"),
	}

	output, err := StartSession(mockClient, input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *output.SessionId != sessionID {
		t.Errorf("expected SessionId '%s', got '%s'", sessionID, *output.SessionId)
	}
}

func TestGenerateTerminateSessionInput(t *testing.T) {
	opts := &SsmOpts{
		SessionId: "session-1234567890",
	}

	input := GenerateTerminateSessionInput(opts)

	if *input.SessionId != opts.SessionId {
		t.Errorf("expected SessionId '%s', got '%s'", opts.SessionId, *input.SessionId)
	}
}

func TestTerminateSession(t *testing.T) {
	sessionID := "session-1234567890"

	mockClient := &mockSsmClient{
		terminateSessionOutput: &ssm.TerminateSessionOutput{
			SessionId: aws.String(sessionID),
		},
		terminateSessionErr: nil,
	}

	input := &ssm.TerminateSessionInput{
		SessionId: aws.String(sessionID),
	}

	output, err := TerminateSession(mockClient, input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *output.SessionId != sessionID {
		t.Errorf("expected SessionId '%s', got '%s'", sessionID, *output.SessionId)
	}
}

func TestDescribeInstanceInformation_Error(t *testing.T) {
	mockClient := &mockSsmClient{
		describeInstanceInformationOutput: &ssm.DescribeInstanceInformationOutput{},
		describeInstanceInformationErr:    errors.New("api error"),
	}

	input := &ssm.DescribeInstanceInformationInput{}
	ids, err := DescribeInstanceInformation(mockClient, input)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if ids != nil {
		t.Errorf("expected nil ids, got %v", ids)
	}
}
