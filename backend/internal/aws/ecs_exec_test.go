package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestECSServiceFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   ecstypes.Service
		want ECSServiceResource
	}{
		{
			name: "active uppercase to lowercase",
			in: ecstypes.Service{
				ServiceArn:     aws.String("arn:aws:ecs:svc/a"),
				ServiceName:    aws.String("svc-a"),
				Status:         aws.String("ACTIVE"),
				DesiredCount:   3,
				RunningCount:   3,
				PendingCount:   0,
				TaskDefinition: aws.String("td:1"),
				LaunchType:     ecstypes.LaunchTypeFargate,
			},
			want: ECSServiceResource{
				ARN:            "arn:aws:ecs:svc/a",
				Name:           "svc-a",
				Status:         "active",
				DesiredCount:   3,
				RunningCount:   3,
				PendingCount:   0,
				TaskDefinition: "td:1",
				LaunchType:     "FARGATE",
			},
		},
		{
			name: "draining",
			in: ecstypes.Service{
				ServiceArn: aws.String("arn:aws:ecs:svc/b"),
				Status:     aws.String("DRAINING"),
			},
			want: ECSServiceResource{
				ARN:    "arn:aws:ecs:svc/b",
				Status: "draining",
			},
		},
		{
			name: "empty status",
			in:   ecstypes.Service{},
			want: ECSServiceResource{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecsServiceFromSDK(tt.in)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestECSTaskFromSDK(t *testing.T) {
	startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	stoppedAt := time.Date(2026, 1, 2, 4, 5, 6, 0, time.UTC)
	exitCode := int32(1)

	tests := []struct {
		name string
		in   ecstypes.Task
		want ECSTaskResource
	}{
		{
			name: "running uppercase with containers",
			in: ecstypes.Task{
				TaskArn:              aws.String("arn:task/a"),
				Group:                aws.String("service:svc-a"),
				LastStatus:           aws.String("RUNNING"),
				DesiredStatus:        aws.String("RUNNING"),
				LaunchType:           ecstypes.LaunchTypeFargate,
				EnableExecuteCommand: true,
				Cpu:                  aws.String("256"),
				Memory:               aws.String("512"),
				StartedAt:            &startedAt,
				Containers: []ecstypes.Container{
					{
						Name:         aws.String("app"),
						Image:        aws.String("app:latest"),
						LastStatus:   aws.String("RUNNING"),
						HealthStatus: ecstypes.HealthStatusHealthy,
						RuntimeId:    aws.String("runtime-app"),
					},
					{Name: aws.String("sidecar")},
				},
			},
			want: ECSTaskResource{
				ARN:                  "arn:task/a",
				Group:                "service:svc-a",
				LastStatus:           "running",
				DesiredStatus:        "running",
				LaunchType:           "FARGATE",
				EnableExecuteCommand: true,
				ContainerNames:       []string{"app", "sidecar"},
				CPU:                  "256",
				Memory:               "512",
				StartedAt:            startedAt.Format(time.RFC3339),
				Containers: []ECSTaskContainerDetail{
					{
						Name:         "app",
						Image:        "app:latest",
						LastStatus:   "running",
						HealthStatus: "healthy",
						RuntimeID:    "runtime-app",
						ExecEnabled:  true,
					},
					{Name: "sidecar"},
				},
			},
		},
		{
			name: "stopped with reason and exit code",
			in: ecstypes.Task{
				TaskArn:       aws.String("arn:task/b"),
				LastStatus:    aws.String("STOPPED"),
				DesiredStatus: aws.String("STOPPED"),
				StoppedAt:     &stoppedAt,
				StoppedReason: aws.String("Essential container in task exited"),
				Containers: []ecstypes.Container{
					{Name: aws.String("app"), LastStatus: aws.String("STOPPED"), ExitCode: &exitCode, Reason: aws.String("nonzero exit"), RuntimeId: aws.String("runtime-app")},
				},
			},
			want: ECSTaskResource{
				ARN:            "arn:task/b",
				LastStatus:     "stopped",
				DesiredStatus:  "stopped",
				ContainerNames: []string{"app"},
				StoppedAt:      stoppedAt.Format(time.RFC3339),
				StoppedReason:  "Essential container in task exited",
				Containers: []ECSTaskContainerDetail{
					// EnableExecuteCommand が false のため、RuntimeID があっても ExecEnabled は false
					{Name: "app", LastStatus: "stopped", ExitCode: &exitCode, Reason: "nonzero exit", RuntimeID: "runtime-app", ExecEnabled: false},
				},
			},
		},
		{
			name: "empty statuses",
			in:   ecstypes.Task{},
			want: ECSTaskResource{ContainerNames: []string{}, Containers: []ECSTaskContainerDetail{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecsTaskFromSDK(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestListECSTasksSendsServiceName は service 引数の有無で ListTasksInput の ServiceName が
// 切り替わることを検証する。ServiceName の設定を落とすと呼び出しは成功したままサービスによる
// タスクの絞り込みが効かなくなり、クラスタ内の全タスクが返る。
func TestListECSTasksSendsServiceName(t *testing.T) {
	const cluster = "demo-cluster"

	tests := []struct {
		name    string
		service string
		// wantInputs は呼び出し順に期待する Input。要素数が期待する呼び出し回数を兼ねる。
		wantInputs []*ecs.ListTasksInput
	}{
		{
			name:    "service 未指定のとき ServiceName は nil",
			service: "",
			wantInputs: []*ecs.ListTasksInput{
				{Cluster: aws.String(cluster)},
				{Cluster: aws.String(cluster), NextToken: aws.String("page-2")},
			},
		},
		{
			name:    "service 指定時は ServiceName に載る",
			service: "web-service",
			wantInputs: []*ecs.ListTasksInput{
				{Cluster: aws.String(cluster), ServiceName: aws.String("web-service")},
				{Cluster: aws.String(cluster), ServiceName: aws.String("web-service"), NextToken: aws.String("page-2")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockECSTaskListClient{listPages: ecsListTasksPages()}
			if _, err := listECSTasks(context.Background(), client, cluster, tt.service); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(client.listInputs) != len(tt.wantInputs) {
				t.Fatalf("ListTasks called %d times, want %d", len(client.listInputs), len(tt.wantInputs))
			}
			// Input 全体を比較し、ServiceName に加えて Cluster と NextToken の引き継ぎも
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
				{Cluster: aws.String(cluster), Tasks: []string{ecsTaskArnPage1, ecsTaskArnPage2}},
			}
			describeOpts := cmpopts.IgnoreUnexported(ecs.DescribeTasksInput{})
			if diff := cmp.Diff(wantDescribe, client.describeInputs, describeOpts); diff != "" {
				t.Errorf("DescribeTasks inputs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
