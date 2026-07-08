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
	return CFNStackResource{
		ID:              ptrStr(s.StackId),
		Name:            ptrStr(s.StackName),
		State:           string(s.StackStatus),
		CreationTime:    createdAt,
		LastUpdatedTime: updatedAt,
		DriftStatus:     string(s.DriftInformation.StackDriftStatus),
	}
}
