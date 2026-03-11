package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

// mockCfnApi implements cfnApi for testing.
type mockCfnApi struct {
	listStacksOutput        *cloudformation.ListStacksOutput
	listStacksErr           error
	describeStacksOutput    *cloudformation.DescribeStacksOutput
	describeStacksErr       error
	describeChangeSetOutput *cloudformation.DescribeChangeSetOutput
	describeChangeSetErr    error
}

func (m *mockCfnApi) ListStacks(ctx context.Context, input *cloudformation.ListStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error) {
	return m.listStacksOutput, m.listStacksErr
}

func (m *mockCfnApi) DescribeStacks(ctx context.Context, input *cloudformation.DescribeStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	return m.describeStacksOutput, m.describeStacksErr
}

func (m *mockCfnApi) DescribeChangeSet(ctx context.Context, input *cloudformation.DescribeChangeSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error) {
	return m.describeChangeSetOutput, m.describeChangeSetErr
}

func TestListCfnStacks(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		mock      *mockCfnApi
		wantLen   int
		wantErr   bool
		wantName  string
		wantDrift string
	}{
		{
			name: "returns stacks",
			mock: &mockCfnApi{
				listStacksOutput: &cloudformation.ListStacksOutput{
					StackSummaries: []types.StackSummary{
						{
							StackName:           aws.String("my-stack"),
							StackStatus:         types.StackStatusCreateComplete,
							CreationTime:        &now,
							LastUpdatedTime:     &now,
							TemplateDescription: aws.String("My Stack"),
							DriftInformation: &types.StackDriftInformationSummary{
								StackDriftStatus: types.StackDriftStatusDrifted,
							},
						},
					},
				},
			},
			wantLen:   1,
			wantName:  "my-stack",
			wantDrift: "DRIFTED",
		},
		{
			name: "no drift information defaults to NOT_CHECKED",
			mock: &mockCfnApi{
				listStacksOutput: &cloudformation.ListStacksOutput{
					StackSummaries: []types.StackSummary{
						{
							StackName:    aws.String("bare-stack"),
							StackStatus:  types.StackStatusCreateComplete,
							CreationTime: &now,
						},
					},
				},
			},
			wantLen:   1,
			wantDrift: "NOT_CHECKED",
		},
		{
			name:    "api error",
			mock:    &mockCfnApi{listStacksErr: errors.New("api error")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ListCfnStacks(tt.mock)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("expected %d stacks, got %d", tt.wantLen, len(result))
			}
			if tt.wantName != "" && result[0].StackName != tt.wantName {
				t.Errorf("expected StackName %q, got %q", tt.wantName, result[0].StackName)
			}
			if tt.wantDrift != "" && result[0].DriftStatus != tt.wantDrift {
				t.Errorf("expected DriftStatus %q, got %q", tt.wantDrift, result[0].DriftStatus)
			}
		})
	}
}

func TestDescribeCfnStack(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		mock      *mockCfnApi
		stackName string
		wantErr   bool
		wantDesc  string
	}{
		{
			name:      "returns stack detail",
			stackName: "my-stack",
			mock: &mockCfnApi{
				describeStacksOutput: &cloudformation.DescribeStacksOutput{
					Stacks: []types.Stack{
						{
							StackName:    aws.String("my-stack"),
							StackStatus:  types.StackStatusCreateComplete,
							CreationTime: &now,
							Description:  aws.String("A test stack"),
							Parameters: []types.Parameter{
								{ParameterKey: aws.String("Env"), ParameterValue: aws.String("prod")},
							},
							Outputs: []types.Output{
								{OutputKey: aws.String("BucketName"), OutputValue: aws.String("my-bucket")},
							},
							Tags: []types.Tag{
								{Key: aws.String("Project"), Value: aws.String("thief")},
							},
							DriftInformation: &types.StackDriftInformation{
								StackDriftStatus: types.StackDriftStatusInSync,
							},
						},
					},
				},
			},
			wantDesc: "A test stack",
		},
		{
			name:      "stack not found",
			stackName: "missing",
			mock: &mockCfnApi{
				describeStacksOutput: &cloudformation.DescribeStacksOutput{
					Stacks: []types.Stack{},
				},
			},
			wantErr: true,
		},
		{
			name:      "api error",
			stackName: "err-stack",
			mock:      &mockCfnApi{describeStacksErr: errors.New("api error")},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DescribeCfnStack(tt.mock, tt.stackName)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Description != tt.wantDesc {
				t.Errorf("expected Description %q, got %q", tt.wantDesc, result.Description)
			}
			if len(result.Parameters) != 1 {
				t.Errorf("expected 1 parameter, got %d", len(result.Parameters))
			}
			if len(result.Outputs) != 1 {
				t.Errorf("expected 1 output, got %d", len(result.Outputs))
			}
			if len(result.Tags) != 1 {
				t.Errorf("expected 1 tag, got %d", len(result.Tags))
			}
		})
	}
}

func TestDescribeCfnChangeSet(t *testing.T) {
	tests := []struct {
		name            string
		mock            *mockCfnApi
		wantLen         int
		wantErr         bool
		wantAction      string
		wantReplacement string
	}{
		{
			name: "returns changes",
			mock: &mockCfnApi{
				describeChangeSetOutput: &cloudformation.DescribeChangeSetOutput{
					Changes: []types.Change{
						{
							ResourceChange: &types.ResourceChange{
								Action:            types.ChangeActionAdd,
								LogicalResourceId: aws.String("MyBucket"),
								ResourceType:      aws.String("AWS::S3::Bucket"),
								Replacement:       types.ReplacementFalse,
							},
						},
					},
				},
			},
			wantLen:         1,
			wantAction:      "Add",
			wantReplacement: "False",
		},
		{
			name: "skips nil ResourceChange",
			mock: &mockCfnApi{
				describeChangeSetOutput: &cloudformation.DescribeChangeSetOutput{
					Changes: []types.Change{
						{ResourceChange: nil},
					},
				},
			},
			wantLen: 0,
		},
		{
			name:    "api error",
			mock:    &mockCfnApi{describeChangeSetErr: errors.New("api error")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DescribeCfnChangeSet(tt.mock, "stack", "changeset")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("expected %d changes, got %d", tt.wantLen, len(result))
			}
			if tt.wantLen > 0 {
				if result[0].Action != tt.wantAction {
					t.Errorf("expected Action %q, got %q", tt.wantAction, result[0].Action)
				}
				if result[0].Replacement != tt.wantReplacement {
					t.Errorf("expected Replacement %q, got %q", tt.wantReplacement, result[0].Replacement)
				}
			}
		})
	}
}
