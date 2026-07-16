package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ECSResource represents a single ECS cluster.
type ECSResource struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	State          string            `json:"state"`
	ActiveServices int32             `json:"active_services"`
	RunningTasks   int32             `json:"running_tasks"`
	PendingTasks   int32             `json:"pending_tasks"`
	RegisteredEC2  int32             `json:"registered_ec2"`
	Tags           map[string]string `json:"tags"`
	CostMonthly    float64           `json:"cost_monthly"`
}

func (r ECSResource) ResourceID() string    { return r.ID }
func (r ECSResource) ResourceName() string  { return r.Name }
func (r ECSResource) ResourceState() string { return NormalizeState(r.State) }
func (r ECSResource) ServiceName() string   { return "ecs" }

// ListECSResources returns all ECS clusters for the given profile/region.
func ListECSResources(ctx context.Context, profile, region string) ([]ECSResource, error) {
	client, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	// List cluster ARNs.
	var arns []string
	paginator := ecs.NewListClustersPaginator(client, &ecs.ListClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs clusters: %w", err)
		}
		arns = append(arns, page.ClusterArns...)
	}
	if len(arns) == 0 {
		return nil, nil
	}

	// Describe in batches of 100 (API limit).
	var resources []ECSResource
	for i := 0; i < len(arns); i += 100 {
		end := i + 100
		if end > len(arns) {
			end = len(arns)
		}
		out, err := client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: arns[i:end],
			Include:  []ecstypes.ClusterField{ecstypes.ClusterFieldTags},
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs clusters: %w", err)
		}
		for _, c := range out.Clusters {
			resources = append(resources, ecsFromCluster(c))
		}
	}
	return resources, nil
}

func ecsFromCluster(c ecstypes.Cluster) ECSResource {
	tags := ecsTagsToMap(c.Tags)
	return ECSResource{
		ID:             ptrStr(c.ClusterArn),
		Name:           ptrStr(c.ClusterName),
		State:          DisplayState(ptrStr(c.Status)),
		ActiveServices: c.ActiveServicesCount,
		RunningTasks:   c.RunningTasksCount,
		PendingTasks:   c.PendingTasksCount,
		RegisteredEC2:  c.RegisteredContainerInstancesCount,
		Tags:           tags,
	}
}

func ecsTagsToMap(tags []ecstypes.Tag) map[string]string {
	return tagsToMapFunc(tags, func(t ecstypes.Tag) (*string, *string) { return t.Key, t.Value })
}
