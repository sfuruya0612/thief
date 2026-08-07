package aws

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestArnPart(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		idx  int
		want string
	}{
		{
			name: "cluster name",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
			idx:  1,
			want: "my-cluster",
		},
		{
			name: "task id",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:task/my-cluster/abc123",
			idx:  2,
			want: "abc123",
		},
		{
			name: "container runtime id",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:container/my-cluster/abc123/runtime-id",
			idx:  3,
			want: "runtime-id",
		},
		{
			name: "out of range falls back to full arn",
			arn:  "no-slash",
			idx:  1,
			want: "no-slash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arnPart(tt.arn, tt.idx); got != tt.want {
				t.Errorf("arnPart(%q, %d) = %q, want %q", tt.arn, tt.idx, got, tt.want)
			}
		})
	}
}

func TestECSExecSessionTarget(t *testing.T) {
	s := ECSExecSession{
		ClusterArn:   "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
		TaskArn:      "arn:aws:ecs:ap-northeast-1:123456789012:task/my-cluster/task-id",
		ContainerArn: "arn:aws:ecs:ap-northeast-1:123456789012:container/my-cluster/task-id/runtime-id",
	}
	want := "ecs:my-cluster_task-id_runtime-id"
	if got := s.Target(); got != want {
		t.Errorf("Target() = %q, want %q", got, want)
	}
}

func TestEcsTaskInfosFromSDK(t *testing.T) {
	startedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	task := ecstypes.Task{
		TaskDefinitionArn: awssdk.String("arn:aws:ecs:ap-northeast-1:123456789012:task-definition/my-app:12"),
		TaskArn:           awssdk.String("arn:aws:ecs:ap-northeast-1:123456789012:task/my-cluster/task-id"),
		LastStatus:        awssdk.String("RUNNING"),
		DesiredStatus:     awssdk.String("RUNNING"),
		LaunchType:        ecstypes.LaunchTypeFargate,
		PlatformFamily:    awssdk.String("Linux"),
		PlatformVersion:   awssdk.String("1.4.0"),
		StartedAt:         awssdk.Time(startedAt),
		Containers: []ecstypes.Container{
			{Name: awssdk.String("app"), HealthStatus: ecstypes.HealthStatusHealthy},
			{Name: awssdk.String("sidecar"), HealthStatus: ecstypes.HealthStatusUnknown},
		},
	}

	got := ecsTaskInfosFromSDK(task)

	want := []ECSTaskInfo{
		{
			TaskDefinition:  "my-app:12",
			TaskID:          "task-id",
			ContainerName:   "app",
			LastStatus:      "RUNNING",
			DesiredStatus:   "RUNNING",
			HealthStatus:    "HEALTHY",
			LaunchType:      "FARGATE",
			PlatformFamily:  "Linux",
			PlatformVersion: "1.4.0",
			StartedAt:       "2026-07-01T12:00:00Z",
		},
		{
			TaskDefinition:  "my-app:12",
			TaskID:          "task-id",
			ContainerName:   "sidecar",
			LastStatus:      "RUNNING",
			DesiredStatus:   "RUNNING",
			HealthStatus:    "UNKNOWN",
			LaunchType:      "FARGATE",
			PlatformFamily:  "Linux",
			PlatformVersion: "1.4.0",
			StartedAt:       "2026-07-01T12:00:00Z",
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ecsTaskInfosFromSDK mismatch (-want +got):\n%s", diff)
	}
}

func TestEcsTaskInfosFromSDK_Defaults(t *testing.T) {
	// PlatformFamily / PlatformVersion / StartedAt 未設定時は "None" を表示する。
	task := ecstypes.Task{
		TaskDefinitionArn: awssdk.String("arn:aws:ecs:ap-northeast-1:123456789012:task-definition/my-app:1"),
		TaskArn:           awssdk.String("arn:aws:ecs:ap-northeast-1:123456789012:task/my-cluster/task-id"),
		LastStatus:        awssdk.String("STOPPED"),
		DesiredStatus:     awssdk.String("STOPPED"),
		LaunchType:        ecstypes.LaunchTypeEc2,
		Containers: []ecstypes.Container{
			{Name: awssdk.String("app")},
		},
	}

	got := ecsTaskInfosFromSDK(task)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].PlatformFamily != "None" || got[0].PlatformVersion != "None" || got[0].StartedAt != "None" {
		t.Errorf("defaults = (%q, %q, %q), want (None, None, None)",
			got[0].PlatformFamily, got[0].PlatformVersion, got[0].StartedAt)
	}
}

// TestListECSTaskInfosSendsDesiredStatus は desiredStatus 引数の有無で ListTasksInput の
// DesiredStatus が切り替わることを検証する。DesiredStatus の設定を落とすと呼び出しは成功した
// まま希望ステータスによる絞り込みが効かなくなる。
// 期待値は実装を通さず AWS API の値 ("RUNNING") で直接書き下している。
func TestListECSTaskInfosSendsDesiredStatus(t *testing.T) {
	const cluster = "demo-cluster"

	tests := []struct {
		name          string
		desiredStatus string
		// wantInputs は呼び出し順に期待する Input。要素数が期待する呼び出し回数を兼ねる。
		wantInputs []*ecs.ListTasksInput
	}{
		{
			name:          "desiredStatus 未指定のとき DesiredStatus は空",
			desiredStatus: "",
			wantInputs: []*ecs.ListTasksInput{
				{Cluster: awssdk.String(cluster)},
				{Cluster: awssdk.String(cluster), NextToken: awssdk.String("page-2")},
			},
		},
		{
			name:          "desiredStatus 指定時は DesiredStatus に載る",
			desiredStatus: "RUNNING",
			wantInputs: []*ecs.ListTasksInput{
				{Cluster: awssdk.String(cluster), DesiredStatus: ecstypes.DesiredStatus("RUNNING")},
				{Cluster: awssdk.String(cluster), DesiredStatus: ecstypes.DesiredStatus("RUNNING"), NextToken: awssdk.String("page-2")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockECSTaskListClient{listPages: ecsListTasksPages()}
			if _, err := listECSTaskInfos(context.Background(), client, cluster, tt.desiredStatus); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(client.listInputs) != len(tt.wantInputs) {
				t.Fatalf("ListTasks called %d times, want %d", len(client.listInputs), len(tt.wantInputs))
			}
			// Input 全体を比較し、DesiredStatus に加えて Cluster と NextToken の引き継ぎも
			// 同時に固定する。ページ送り後の呼び出しでも絞り込みが維持される。
			opts := cmpopts.IgnoreUnexported(ecs.ListTasksInput{})
			for i, in := range client.listInputs {
				if diff := cmp.Diff(tt.wantInputs[i], in, opts); diff != "" {
					t.Errorf("call %d: input mismatch (-want +got):\n%s", i+1, diff)
				}
			}

			// 続く DescribeTasks へは両ページ分の ARN が 1 回でまとめて渡る。Cluster を落とすと
			// 実際の API では必須パラメータ不足で失敗し、Tasks を落とすとタスクが 1 件も返らないが、
			// どちらも ListTasksInput の比較だけでは検出できない。スライス全体を比較することで
			// 呼び出し回数も同時に固定する。
			wantDescribe := []*ecs.DescribeTasksInput{
				{Cluster: awssdk.String(cluster), Tasks: []string{ecsTaskArnPage1, ecsTaskArnPage2}},
			}
			describeOpts := cmpopts.IgnoreUnexported(ecs.DescribeTasksInput{})
			if diff := cmp.Diff(wantDescribe, client.describeInputs, describeOpts); diff != "" {
				t.Errorf("DescribeTasks inputs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
