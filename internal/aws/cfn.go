// Package aws provides AWS service clients and utilities for interacting with AWS services.
package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

// cfnApi defines the interface for CloudFormation API operations.
// This interface helps with testing by allowing mock implementations.
type cfnApi interface {
	ListStacks(ctx context.Context, input *cloudformation.ListStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error)
	DescribeStacks(ctx context.Context, input *cloudformation.DescribeStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	DescribeChangeSet(ctx context.Context, input *cloudformation.DescribeChangeSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error)
}

// NewCfnClient creates a new CloudFormation client using the specified AWS profile and region.
func NewCfnClient(profile, region string) (cfnApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create cloudformation client: %w", err)
	}
	return cloudformation.NewFromConfig(cfg), nil
}

// CfnStackSummary holds the display fields for a CloudFormation stack summary.
type CfnStackSummary struct {
	StackName   string
	Status      string
	DriftStatus string
	CreatedTime string
	UpdatedTime string
	Description string
}

// ToRow converts CfnStackSummary to a string slice suitable for table formatting.
func (s CfnStackSummary) ToRow() []string {
	return []string{s.StackName, s.Status, s.DriftStatus, s.CreatedTime, s.UpdatedTime, s.Description}
}

// CfnParameter holds a CloudFormation stack parameter key/value pair.
type CfnParameter struct {
	Key           string
	Value         string
	ResolvedValue string
}

// ToRow converts CfnParameter to a string slice suitable for table formatting.
func (p CfnParameter) ToRow() []string {
	return []string{p.Key, p.Value, p.ResolvedValue}
}

// CfnOutput holds a CloudFormation stack output entry.
type CfnOutput struct {
	Key         string
	Value       string
	ExportName  string
	Description string
}

// ToRow converts CfnOutput to a string slice suitable for table formatting.
func (o CfnOutput) ToRow() []string {
	return []string{o.Key, o.Value, o.ExportName, o.Description}
}

// CfnTag holds a CloudFormation stack tag key/value pair.
type CfnTag struct {
	Key   string
	Value string
}

// ToRow converts CfnTag to a string slice suitable for table formatting.
func (t CfnTag) ToRow() []string {
	return []string{t.Key, t.Value}
}

// CfnStackDetail holds the full detail of a CloudFormation stack including
// its parameters, outputs, and tags.
type CfnStackDetail struct {
	StackName   string
	Status      string
	DriftStatus string
	CreatedTime string
	UpdatedTime string
	Description string
	Parameters  []CfnParameter
	Outputs     []CfnOutput
	Tags        []CfnTag
}

// CfnChangeDetail holds the display fields for a single change within a Change Set.
type CfnChangeDetail struct {
	Action       string
	LogicalID    string
	ResourceType string
	Replacement  string
}

// ToRow converts CfnChangeDetail to a string slice suitable for table formatting.
func (c CfnChangeDetail) ToRow() []string {
	return []string{c.Action, c.LogicalID, c.ResourceType, c.Replacement}
}

// ListCfnStacks retrieves all CloudFormation stacks excluding DELETE_COMPLETE ones.
// It handles pagination automatically.
func ListCfnStacks(api cfnApi) ([]CfnStackSummary, error) {
	// Exclude DELETE_COMPLETE stacks so the list stays relevant.
	statusFilters := []types.StackStatus{
		types.StackStatusCreateInProgress,
		types.StackStatusCreateFailed,
		types.StackStatusCreateComplete,
		types.StackStatusRollbackInProgress,
		types.StackStatusRollbackFailed,
		types.StackStatusRollbackComplete,
		types.StackStatusDeleteInProgress,
		types.StackStatusDeleteFailed,
		types.StackStatusUpdateInProgress,
		types.StackStatusUpdateCompleteCleanupInProgress,
		types.StackStatusUpdateComplete,
		types.StackStatusUpdateFailed,
		types.StackStatusUpdateRollbackInProgress,
		types.StackStatusUpdateRollbackFailed,
		types.StackStatusUpdateRollbackCompleteCleanupInProgress,
		types.StackStatusUpdateRollbackComplete,
		types.StackStatusReviewInProgress,
		types.StackStatusImportInProgress,
		types.StackStatusImportComplete,
		types.StackStatusImportRollbackInProgress,
		types.StackStatusImportRollbackFailed,
		types.StackStatusImportRollbackComplete,
	}

	input := &cloudformation.ListStacksInput{
		StackStatusFilter: statusFilters,
	}

	var summaries []CfnStackSummary
	for {
		o, err := api.ListStacks(context.Background(), input)
		if err != nil {
			return nil, err
		}

		for _, s := range o.StackSummaries {
			updatedTime := ""
			if s.LastUpdatedTime != nil {
				updatedTime = s.LastUpdatedTime.String()
			}

			driftStatus := "NOT_CHECKED"
			if s.DriftInformation != nil {
				driftStatus = string(s.DriftInformation.StackDriftStatus)
			}

			description := ""
			if s.TemplateDescription != nil {
				description = *s.TemplateDescription
			}

			summaries = append(summaries, CfnStackSummary{
				StackName:   aws.ToString(s.StackName),
				Status:      string(s.StackStatus),
				DriftStatus: driftStatus,
				CreatedTime: s.CreationTime.String(),
				UpdatedTime: updatedTime,
				Description: description,
			})
		}

		if o.NextToken == nil {
			break
		}
		input.NextToken = o.NextToken
	}

	return summaries, nil
}

// DescribeCfnStack retrieves the full detail of a single CloudFormation stack
// by name, including its parameters, outputs, and tags.
func DescribeCfnStack(api cfnApi, stackName string) (*CfnStackDetail, error) {
	o, err := api.DescribeStacks(context.Background(), &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return nil, err
	}

	if len(o.Stacks) == 0 {
		return nil, fmt.Errorf("stack %q not found", stackName)
	}

	s := o.Stacks[0]

	updatedTime := ""
	if s.LastUpdatedTime != nil {
		updatedTime = s.LastUpdatedTime.String()
	}

	driftStatus := "NOT_CHECKED"
	if s.DriftInformation != nil {
		driftStatus = string(s.DriftInformation.StackDriftStatus)
	}

	description := ""
	if s.Description != nil {
		description = *s.Description
	}

	detail := &CfnStackDetail{
		StackName:   aws.ToString(s.StackName),
		Status:      string(s.StackStatus),
		DriftStatus: driftStatus,
		CreatedTime: s.CreationTime.String(),
		UpdatedTime: updatedTime,
		Description: description,
	}

	for _, p := range s.Parameters {
		resolvedValue := ""
		if p.ResolvedValue != nil {
			resolvedValue = *p.ResolvedValue
		}
		detail.Parameters = append(detail.Parameters, CfnParameter{
			Key:           aws.ToString(p.ParameterKey),
			Value:         aws.ToString(p.ParameterValue),
			ResolvedValue: resolvedValue,
		})
	}

	for _, out := range s.Outputs {
		exportName := ""
		if out.ExportName != nil {
			exportName = *out.ExportName
		}
		outDescription := ""
		if out.Description != nil {
			outDescription = *out.Description
		}
		detail.Outputs = append(detail.Outputs, CfnOutput{
			Key:         aws.ToString(out.OutputKey),
			Value:       aws.ToString(out.OutputValue),
			ExportName:  exportName,
			Description: outDescription,
		})
	}

	for _, tag := range s.Tags {
		detail.Tags = append(detail.Tags, CfnTag{
			Key:   aws.ToString(tag.Key),
			Value: aws.ToString(tag.Value),
		})
	}

	return detail, nil
}

// DescribeCfnChangeSet retrieves the changes within a specific Change Set.
func DescribeCfnChangeSet(api cfnApi, stackName, changeSetName string) ([]CfnChangeDetail, error) {
	o, err := api.DescribeChangeSet(context.Background(), &cloudformation.DescribeChangeSetInput{
		StackName:     aws.String(stackName),
		ChangeSetName: aws.String(changeSetName),
	})
	if err != nil {
		return nil, err
	}

	var changes []CfnChangeDetail
	for _, c := range o.Changes {
		if c.ResourceChange == nil {
			continue
		}
		rc := c.ResourceChange

		replacement := "N/A"
		if rc.Replacement != "" {
			replacement = string(rc.Replacement)
		}

		changes = append(changes, CfnChangeDetail{
			Action:       string(rc.Action),
			LogicalID:    aws.ToString(rc.LogicalResourceId),
			ResourceType: aws.ToString(rc.ResourceType),
			Replacement:  replacement,
		})
	}

	return changes, nil
}
