package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

// CFNStackResource represents a CloudFormation stack.
type CFNStackResource struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	State           string            `json:"state"`
	CreationTime    string            `json:"creation_time"`
	LastUpdatedTime string            `json:"last_updated_time"`
	DriftStatus     string            `json:"drift_status"`
	Tags            map[string]string `json:"tags"`
}

func (r CFNStackResource) ResourceID() string    { return r.ID }
func (r CFNStackResource) ResourceName() string  { return r.Name }
func (r CFNStackResource) ResourceState() string { return NormalizeState(r.State) }
func (r CFNStackResource) ServiceName() string   { return "cfn" }

// ListCFNStacks returns all non-deleted CloudFormation stacks for the given profile/region.
func ListCFNStacks(ctx context.Context, profile, region string) ([]CFNStackResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *cloudformation.Client {
		return cloudformation.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	// Exclude deleted stacks.
	statusFilter := []cfntypes.StackStatus{
		cfntypes.StackStatusCreateComplete,
		cfntypes.StackStatusUpdateComplete,
		cfntypes.StackStatusRollbackComplete,
		cfntypes.StackStatusUpdateRollbackComplete,
		cfntypes.StackStatusCreateInProgress,
		cfntypes.StackStatusUpdateInProgress,
		cfntypes.StackStatusDeleteInProgress,
		cfntypes.StackStatusRollbackInProgress,
		cfntypes.StackStatusCreateFailed,
		cfntypes.StackStatusUpdateFailed,
		cfntypes.StackStatusRollbackFailed,
		cfntypes.StackStatusUpdateRollbackFailed,
		cfntypes.StackStatusImportComplete,
		cfntypes.StackStatusImportInProgress,
		cfntypes.StackStatusImportRollbackComplete,
		cfntypes.StackStatusImportRollbackFailed,
		cfntypes.StackStatusImportRollbackInProgress,
		cfntypes.StackStatusReviewInProgress,
	}

	var resources []CFNStackResource
	paginator := cloudformation.NewListStacksPaginator(client, &cloudformation.ListStacksInput{
		StackStatusFilter: statusFilter,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list cfn stacks: %w", err)
		}
		for _, s := range page.StackSummaries {
			resources = append(resources, cfnFromSummary(s))
		}
	}
	return resources, nil
}

func cfnFromSummary(s cfntypes.StackSummary) CFNStackResource {
	createdAt := ""
	if s.CreationTime != nil {
		createdAt = s.CreationTime.Format(time.RFC3339)
	}
	updatedAt := ""
	if s.LastUpdatedTime != nil {
		updatedAt = s.LastUpdatedTime.Format(time.RFC3339)
	}
	driftStatus := "NOT_CHECKED"
	if s.DriftInformation != nil {
		driftStatus = string(s.DriftInformation.StackDriftStatus)
	}
	return CFNStackResource{
		ID:              ptrStr(s.StackId),
		Name:            ptrStr(s.StackName),
		State:           string(s.StackStatus),
		CreationTime:    createdAt,
		LastUpdatedTime: updatedAt,
		DriftStatus:     driftStatus,
	}
}

// CfnStackSummary はレガシー CLI 互換の CloudFormation スタック一覧表示用フィールドを保持する。
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

// CfnStackDetail はスタックの詳細 (パラメータ・出力・タグを含む) を保持する。
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

// CfnChangeDetail は Change Set 内の 1 変更の表示用フィールドを保持する。
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

func newCfnClient(ctx context.Context, profile, region string) (*cloudformation.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *cloudformation.Client {
		return cloudformation.NewFromConfig(cfg)
	})
}

// ListCfnStackSummaries は DELETE_COMPLETE を除く全スタックをレガシー CLI 互換フィールドで返す。
func ListCfnStackSummaries(ctx context.Context, profile, region string) ([]CfnStackSummary, error) {
	client, err := newCfnClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	// DELETE_COMPLETE を除外して一覧の関心を保つ。
	statusFilters := []cfntypes.StackStatus{
		cfntypes.StackStatusCreateInProgress,
		cfntypes.StackStatusCreateFailed,
		cfntypes.StackStatusCreateComplete,
		cfntypes.StackStatusRollbackInProgress,
		cfntypes.StackStatusRollbackFailed,
		cfntypes.StackStatusRollbackComplete,
		cfntypes.StackStatusDeleteInProgress,
		cfntypes.StackStatusDeleteFailed,
		cfntypes.StackStatusUpdateInProgress,
		cfntypes.StackStatusUpdateCompleteCleanupInProgress,
		cfntypes.StackStatusUpdateComplete,
		cfntypes.StackStatusUpdateFailed,
		cfntypes.StackStatusUpdateRollbackInProgress,
		cfntypes.StackStatusUpdateRollbackFailed,
		cfntypes.StackStatusUpdateRollbackCompleteCleanupInProgress,
		cfntypes.StackStatusUpdateRollbackComplete,
		cfntypes.StackStatusReviewInProgress,
		cfntypes.StackStatusImportInProgress,
		cfntypes.StackStatusImportComplete,
		cfntypes.StackStatusImportRollbackInProgress,
		cfntypes.StackStatusImportRollbackFailed,
		cfntypes.StackStatusImportRollbackComplete,
	}

	var summaries []CfnStackSummary
	paginator := cloudformation.NewListStacksPaginator(client, &cloudformation.ListStacksInput{
		StackStatusFilter: statusFilters,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list cfn stacks: %w", err)
		}
		for _, s := range page.StackSummaries {
			updatedTime := ""
			if s.LastUpdatedTime != nil {
				updatedTime = s.LastUpdatedTime.String()
			}

			driftStatus := "NOT_CHECKED"
			if s.DriftInformation != nil {
				driftStatus = string(s.DriftInformation.StackDriftStatus)
			}

			createdTime := ""
			if s.CreationTime != nil {
				createdTime = s.CreationTime.String()
			}

			summaries = append(summaries, CfnStackSummary{
				StackName:   ptrStr(s.StackName),
				Status:      string(s.StackStatus),
				DriftStatus: driftStatus,
				CreatedTime: createdTime,
				UpdatedTime: updatedTime,
				Description: ptrStr(s.TemplateDescription),
			})
		}
	}
	return summaries, nil
}

// DescribeCfnStack は単一スタックの詳細 (パラメータ・出力・タグを含む) を返す。
func DescribeCfnStack(ctx context.Context, profile, region, stackName string) (*CfnStackDetail, error) {
	client, err := newCfnClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	o, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
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

	createdTime := ""
	if s.CreationTime != nil {
		createdTime = s.CreationTime.String()
	}

	detail := &CfnStackDetail{
		StackName:   ptrStr(s.StackName),
		Status:      string(s.StackStatus),
		DriftStatus: driftStatus,
		CreatedTime: createdTime,
		UpdatedTime: updatedTime,
		Description: ptrStr(s.Description),
	}

	for _, p := range s.Parameters {
		detail.Parameters = append(detail.Parameters, CfnParameter{
			Key:           ptrStr(p.ParameterKey),
			Value:         ptrStr(p.ParameterValue),
			ResolvedValue: ptrStr(p.ResolvedValue),
		})
	}

	for _, out := range s.Outputs {
		detail.Outputs = append(detail.Outputs, CfnOutput{
			Key:         ptrStr(out.OutputKey),
			Value:       ptrStr(out.OutputValue),
			ExportName:  ptrStr(out.ExportName),
			Description: ptrStr(out.Description),
		})
	}

	for _, tag := range s.Tags {
		detail.Tags = append(detail.Tags, CfnTag{
			Key:   ptrStr(tag.Key),
			Value: ptrStr(tag.Value),
		})
	}

	return detail, nil
}

// DescribeCfnChangeSet は指定 Change Set に含まれるリソース変更一覧を返す。
func DescribeCfnChangeSet(ctx context.Context, profile, region, stackName, changeSetName string) ([]CfnChangeDetail, error) {
	client, err := newCfnClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	o, err := client.DescribeChangeSet(ctx, &cloudformation.DescribeChangeSetInput{
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
			LogicalID:    ptrStr(rc.LogicalResourceId),
			ResourceType: ptrStr(rc.ResourceType),
			Replacement:  replacement,
		})
	}

	return changes, nil
}
