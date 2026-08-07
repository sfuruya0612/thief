package aws

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestEcsFromCluster(t *testing.T) {
	tests := []struct {
		name string
		in   ecstypes.Cluster
		want ECSResource
	}{
		{
			name: "active uppercase normalized",
			in: ecstypes.Cluster{
				ClusterArn:                        aws.String("arn:aws:ecs:ap-northeast-1:123:cluster/prod"),
				ClusterName:                       aws.String("prod"),
				Status:                            aws.String("ACTIVE"),
				ActiveServicesCount:               3,
				RunningTasksCount:                 5,
				PendingTasksCount:                 1,
				RegisteredContainerInstancesCount: 2,
				Tags: []ecstypes.Tag{
					{Key: aws.String("env"), Value: aws.String("prod")},
				},
			},
			want: ECSResource{
				ID:             "arn:aws:ecs:ap-northeast-1:123:cluster/prod",
				Name:           "prod",
				State:          "active",
				ActiveServices: 3,
				RunningTasks:   5,
				PendingTasks:   1,
				RegisteredEC2:  2,
				Tags:           map[string]string{"env": "prod"},
			},
		},
		{
			name: "empty status stays empty",
			in:   ecstypes.Cluster{ClusterName: aws.String("empty")},
			want: ECSResource{Name: "empty", Tags: map[string]string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecsFromCluster(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

// mockECSClusterListClient は listECSResources が要求する ecsClusterListClient を
// テスト用に実装する手書きモック。受け取った Input を呼び出し順に記録し、用意した
// レスポンスを 1 呼び出しにつき 1 ページ返す。
// ListClusters は SDK のページネータ経由で呼ばれる。ページネータは呼び出しごとに Input を
// 値でコピーして NextToken だけ差し替えるため、ポインタのまま記録してよい。
type mockECSClusterListClient struct {
	listPages      []*ecs.ListClustersOutput
	describePages  []*ecs.DescribeClustersOutput
	listInputs     []*ecs.ListClustersInput
	describeInputs []*ecs.DescribeClustersInput
}

func (m *mockECSClusterListClient) ListClusters(_ context.Context, params *ecs.ListClustersInput, _ ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	m.listInputs = append(m.listInputs, params)
	idx := len(m.listInputs) - 1
	if idx >= len(m.listPages) {
		return nil, fmt.Errorf("unexpected ListClusters call %d: only %d pages prepared", idx+1, len(m.listPages))
	}
	return m.listPages[idx], nil
}

func (m *mockECSClusterListClient) DescribeClusters(_ context.Context, params *ecs.DescribeClustersInput, _ ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
	m.describeInputs = append(m.describeInputs, params)
	idx := len(m.describeInputs) - 1
	if idx >= len(m.describePages) {
		return nil, fmt.Errorf("unexpected DescribeClusters call %d: only %d pages prepared", idx+1, len(m.describePages))
	}
	return m.describePages[idx], nil
}

// mockECSTaskListClient は listECSTasks / listECSTaskInfos が要求する ecsTaskListClient を
// テスト用に実装する手書きモック。ListTasks は SDK のページネータ経由で呼ばれ、
// DescribeTasks はバッチごとに Input を新しく確保して呼ばれるため、どちらも受け取った
// Input をポインタのまま記録してよい。
// DescribeTasks のレスポンスはタスク ARN の分だけ空のタスクを返す。本テスト群の検証対象は
// Input の構築であり、タスクの変換結果は既存の変換関数のテストが担当する。
type mockECSTaskListClient struct {
	listPages      []*ecs.ListTasksOutput
	listInputs     []*ecs.ListTasksInput
	describeInputs []*ecs.DescribeTasksInput
}

func (m *mockECSTaskListClient) ListTasks(_ context.Context, params *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	m.listInputs = append(m.listInputs, params)
	idx := len(m.listInputs) - 1
	if idx >= len(m.listPages) {
		return nil, fmt.Errorf("unexpected ListTasks call %d: only %d pages prepared", idx+1, len(m.listPages))
	}
	return m.listPages[idx], nil
}

func (m *mockECSTaskListClient) DescribeTasks(_ context.Context, params *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	m.describeInputs = append(m.describeInputs, params)
	tasks := make([]ecstypes.Task, 0, len(params.Tasks))
	for _, arn := range params.Tasks {
		tasks = append(tasks, ecstypes.Task{TaskArn: aws.String(arn)})
	}
	return &ecs.DescribeTasksOutput{Tasks: tasks}, nil
}

// ecsTaskArnPage1 / ecsTaskArnPage2 は ecsListTasksPages が各ページで返すタスク ARN。
// DescribeTasks へ両ページ分がまとめて渡ることの検証でも期待値として使う。
const (
	ecsTaskArnPage1 = "arn:aws:ecs:ap-northeast-1:123456789012:task/demo/page1"
	ecsTaskArnPage2 = "arn:aws:ecs:ap-northeast-1:123456789012:task/demo/page2"
)

// ecsListTasksPages は NextToken で連結された 2 ページ構成のタスク ARN 一覧を返す。
// ListTasksInput の各フィールドがページ送り後の呼び出しでも維持されることの検証に使う。
func ecsListTasksPages() []*ecs.ListTasksOutput {
	return []*ecs.ListTasksOutput{
		{
			TaskArns:  []string{ecsTaskArnPage1},
			NextToken: aws.String("page-2"),
		},
		{
			TaskArns: []string{ecsTaskArnPage2},
		},
	}
}

// TestListECSResourcesSendsIncludeTags は DescribeClusters へ Include に ClusterFieldTags を
// 載せて送ることを検証する。この指定が無いとレスポンスに Tags が返らず、クラスタのタグ表示が
// 呼び出しを成功させたまま静かに空になる。
// 期待値の Include は実装の定数式ではなく AWS API の値で直接書き下している。
func TestListECSResourcesSendsIncludeTags(t *testing.T) {
	const (
		arnA = "arn:aws:ecs:ap-northeast-1:123456789012:cluster/alpha"
		arnB = "arn:aws:ecs:ap-northeast-1:123456789012:cluster/bravo"
	)

	client := &mockECSClusterListClient{
		// ARN の列挙は 2 ページに分ける。両ページの ARN が 1 回の DescribeClusters に
		// まとめて渡ることを確かめる。
		listPages: []*ecs.ListClustersOutput{
			{ClusterArns: []string{arnA}, NextToken: aws.String("page-2")},
			{ClusterArns: []string{arnB}},
		},
		describePages: []*ecs.DescribeClustersOutput{
			{Clusters: []ecstypes.Cluster{
				{ClusterArn: aws.String(arnA), ClusterName: aws.String("alpha")},
				{ClusterArn: aws.String(arnB), ClusterName: aws.String("bravo")},
			}},
		},
	}

	got, err := listECSResources(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.describeInputs) != 1 {
		t.Fatalf("DescribeClusters called %d times, want 1", len(client.describeInputs))
	}
	want := &ecs.DescribeClustersInput{
		Clusters: []string{arnA, arnB},
		Include:  []ecstypes.ClusterField{"TAGS"},
	}
	opts := cmpopts.IgnoreUnexported(ecs.DescribeClustersInput{})
	if diff := cmp.Diff(want, client.describeInputs[0], opts); diff != "" {
		t.Errorf("DescribeClusters input mismatch (-want +got):\n%s", diff)
	}

	// 両ページの ARN が集約されたうえでクラスタが返ることも確認する。
	gotNames := make([]string, 0, len(got))
	for _, r := range got {
		gotNames = append(gotNames, r.Name)
	}
	if diff := cmp.Diff([]string{"alpha", "bravo"}, gotNames); diff != "" {
		t.Errorf("cluster names mismatch (-want +got):\n%s", diff)
	}
}
