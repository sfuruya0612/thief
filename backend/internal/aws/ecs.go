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

// ecsClusterListClient はクラスタ一覧の取得に必要な API を抽象化する。
// ARN の列挙と詳細取得の 2 段構えのため、ページネータが要求する
// ecs.ListClustersAPIClient に DescribeClusters を加える。
// テストではモックを差し込み、実行時は *ecs.Client がこれを満たす。
type ecsClusterListClient interface {
	ecs.ListClustersAPIClient
	DescribeClusters(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
}

// ecsTaskListClient はタスク一覧の取得に必要な API を抽象化する。
// ARN の列挙と詳細取得の 2 段構えのため、ページネータが要求する
// ecs.ListTasksAPIClient に DescribeTasks を加える。
// ListECSTasks (ecs_exec.go) と ListECSTaskInfos (ecs_cli.go) が同じ形で使う。
type ecsTaskListClient interface {
	ecs.ListTasksAPIClient
	DescribeTasks(ctx context.Context, params *ecs.DescribeTasksInput, optFns ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

// ecsContainerInstanceListClient は ListECSContainerInstances が使う ECS API の部分集合。
type ecsContainerInstanceListClient interface {
	ecs.ListContainerInstancesAPIClient
	DescribeContainerInstances(ctx context.Context, params *ecs.DescribeContainerInstancesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error)
}

// ListECSResources returns all ECS clusters for the given profile/region.
func ListECSResources(ctx context.Context, profile, region string) ([]ECSResource, error) {
	client, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listECSResources(ctx, client)
}

// listECSResources は生成済みクライアントでクラスタ一覧を取得するコア。
// DescribeClustersInput に載せる Include を単体テストで固定できるよう、
// クライアントの生成と分離してある。
func listECSResources(ctx context.Context, client ecsClusterListClient) ([]ECSResource, error) {
	arns, err := listECSClusterArnsWith(ctx, client)
	if err != nil {
		return nil, err
	}
	if len(arns) == 0 {
		return nil, nil
	}

	var resources []ECSResource
	for i := 0; i < len(arns); i += ecsDescribeClustersBatchSize {
		end := min(i+ecsDescribeClustersBatchSize, len(arns))
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

// listECSClusterArnsWith は ECS クラスタの ARN 一覧をページネーションで取得する。
// ListECSResources と ListECSClusterArns の両方から使う共通コア。
// ListClusters しか使わないため、引数はページネータが要求する
// ecs.ListClustersAPIClient に絞ってある (ecsClusterListClient も満たす)。
func listECSClusterArnsWith(ctx context.Context, client ecs.ListClustersAPIClient) ([]string, error) {
	var arns []string
	paginator := ecs.NewListClustersPaginator(client, &ecs.ListClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs clusters: %w", err)
		}
		arns = append(arns, page.ClusterArns...)
	}
	return arns, nil
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
