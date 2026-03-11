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
	describeParametersOutput          *ssm.DescribeParametersOutput
	describeParametersErr             error
	getParameterOutput                *ssm.GetParameterOutput
	getParameterErr                   error
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

func (m *mockSsmClient) DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
	return m.describeParametersOutput, m.describeParametersErr
}

func (m *mockSsmClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return m.getParameterOutput, m.getParameterErr
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

func TestGenerateDescribeParametersInput(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectFilters  bool
		expectedPrefix string
	}{
		{"empty path", "", false, ""},
		{"with path prefix", "/myapp/", true, "/myapp/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := GenerateDescribeParametersInput(tt.path)

			if tt.expectFilters {
				if len(input.ParameterFilters) != 1 {
					t.Fatalf("expected 1 filter, got %d", len(input.ParameterFilters))
				}
				if *input.ParameterFilters[0].Key != "Name" {
					t.Errorf("expected filter key 'Name', got '%s'", *input.ParameterFilters[0].Key)
				}
				if *input.ParameterFilters[0].Option != "BeginsWith" {
					t.Errorf("expected filter option 'BeginsWith', got '%s'", *input.ParameterFilters[0].Option)
				}
				if input.ParameterFilters[0].Values[0] != tt.expectedPrefix {
					t.Errorf("expected filter value '%s', got '%s'", tt.expectedPrefix, input.ParameterFilters[0].Values[0])
				}
			} else {
				if len(input.ParameterFilters) != 0 {
					t.Errorf("expected no filters, got %d", len(input.ParameterFilters))
				}
			}
		})
	}
}

func TestDescribeParameters(t *testing.T) {
	mockClient := &mockSsmClient{
		describeParametersOutput: &ssm.DescribeParametersOutput{
			Parameters: []types.ParameterMetadata{
				{
					Name:     aws.String("/myapp/db/host"),
					Type:     types.ParameterTypeString,
					Version:  1,
					DataType: aws.String("text"),
				},
				{
					Name:     aws.String("/myapp/db/password"),
					Type:     types.ParameterTypeSecureString,
					Version:  3,
					DataType: aws.String("text"),
				},
			},
		},
	}

	input := &ssm.DescribeParametersInput{}
	params, err := DescribeParameters(mockClient, input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(params))
	}
	if params[0].Name != "/myapp/db/host" {
		t.Errorf("expected name '/myapp/db/host', got '%s'", params[0].Name)
	}
	if params[0].Type != "String" {
		t.Errorf("expected type 'String', got '%s'", params[0].Type)
	}
	if params[1].Name != "/myapp/db/password" {
		t.Errorf("expected name '/myapp/db/password', got '%s'", params[1].Name)
	}
	if params[1].Type != "SecureString" {
		t.Errorf("expected type 'SecureString', got '%s'", params[1].Type)
	}
	if params[1].Version != 3 {
		t.Errorf("expected version 3, got %d", params[1].Version)
	}
}

func TestDescribeParameters_Error(t *testing.T) {
	mockClient := &mockSsmClient{
		describeParametersOutput: &ssm.DescribeParametersOutput{},
		describeParametersErr:    errors.New("api error"),
	}

	input := &ssm.DescribeParametersInput{}
	params, err := DescribeParameters(mockClient, input)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if params != nil {
		t.Errorf("expected nil params, got %v", params)
	}
}

func TestGetParameter(t *testing.T) {
	tests := []struct {
		name           string
		paramName      string
		paramType      types.ParameterType
		paramValue     string
		withDecryption bool
	}{
		{"String parameter", "/myapp/db/host", types.ParameterTypeString, "localhost", false},
		{"SecureString parameter", "/myapp/secret", types.ParameterTypeSecureString, "decrypted-value", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockSsmClient{
				getParameterOutput: &ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Name:    aws.String(tt.paramName),
						Type:    tt.paramType,
						Value:   aws.String(tt.paramValue),
						Version: 1,
						ARN:     aws.String("arn:aws:ssm:ap-northeast-1:123456789012:parameter" + tt.paramName),
					},
				},
			}

			result, err := GetParameter(mockClient, tt.paramName, tt.withDecryption)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Name != tt.paramName {
				t.Errorf("expected name '%s', got '%s'", tt.paramName, result.Name)
			}
			if result.Value != tt.paramValue {
				t.Errorf("expected value '%s', got '%s'", tt.paramValue, result.Value)
			}
			if result.Type != string(tt.paramType) {
				t.Errorf("expected type '%s', got '%s'", string(tt.paramType), result.Type)
			}
		})
	}
}

func TestGetParameter_Error(t *testing.T) {
	mockClient := &mockSsmClient{
		getParameterErr: errors.New("parameter not found"),
	}

	result, err := GetParameter(mockClient, "/nonexistent", false)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
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
