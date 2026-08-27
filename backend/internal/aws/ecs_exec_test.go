package aws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
			// Fargate のタスクは ContainerInstanceArn が nil で、空文字列になる
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
						Name:              aws.String("app"),
						Image:             aws.String("app:latest"),
						LastStatus:        aws.String("RUNNING"),
						HealthStatus:      ecstypes.HealthStatusHealthy,
						RuntimeId:         aws.String("runtime-app"),
						Cpu:               aws.String("128"),
						Memory:            aws.String("256"),
						MemoryReservation: aws.String("128"),
					},
					// Cpu / Memory / MemoryReservation が nil のコンテナは空文字列になる
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
						Name:              "app",
						Image:             "app:latest",
						LastStatus:        "running",
						HealthStatus:      "healthy",
						RuntimeID:         "runtime-app",
						CPU:               "128",
						Memory:            "256",
						MemoryReservation: "128",
						ExecEnabled:       true,
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
				// EC2 起動タイプのタスクはコンテナインスタンスの ARN を持つ
				ContainerInstanceArn: aws.String("arn:aws:ecs:ap-northeast-1:123456789012:container-instance/demo/ci-1"),
				Containers: []ecstypes.Container{
					{Name: aws.String("app"), LastStatus: aws.String("STOPPED"), ExitCode: &exitCode, Reason: aws.String("nonzero exit"), RuntimeId: aws.String("runtime-app")},
				},
			},
			want: ECSTaskResource{
				ARN:                  "arn:task/b",
				LastStatus:           "stopped",
				DesiredStatus:        "stopped",
				ContainerNames:       []string{"app"},
				StoppedAt:            stoppedAt.Format(time.RFC3339),
				StoppedReason:        "Essential container in task exited",
				ContainerInstanceArn: "arn:aws:ecs:ap-northeast-1:123456789012:container-instance/demo/ci-1",
				Containers: []ECSTaskContainerDetail{
					// EnableExecuteCommand が false のため、RuntimeID があっても ExecEnabled は false
					{Name: "app", LastStatus: "stopped", ExitCode: &exitCode, Reason: "nonzero exit", RuntimeID: "runtime-app", ExecEnabled: false},
				},
			},
		},
		{
			name: "container cpu zero and memory only are copied as is",
			in: ecstypes.Task{
				Containers: []ecstypes.Container{
					{Name: aws.String("app"), Cpu: aws.String("0"), Memory: aws.String("512")},
				},
			},
			want: ECSTaskResource{
				ContainerNames: []string{"app"},
				// "0" は未指定を表す SDK の値だが backend では変換せず frontend の表示で判定する
				Containers: []ECSTaskContainerDetail{{Name: "app", CPU: "0", Memory: "512"}},
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

// mockECSContainerInstanceListClient は listECSContainerInstances のテスト用クライアント。
// ListContainerInstances は listPages を呼び出し順に返し、DescribeContainerInstances は
// 受け取った ARN のうち instances にあるものを ARN の順に返し、無いものは実 API と同じく
// Failures (Reason MISSING) に載せる。listErr / describeErr を設定すると各 API がそのエラーを返す。
type mockECSContainerInstanceListClient struct {
	listPages      []*ecs.ListContainerInstancesOutput
	instances      map[string]ecstypes.ContainerInstance
	listErr        error
	describeErr    error
	listInputs     []*ecs.ListContainerInstancesInput
	describeInputs []*ecs.DescribeContainerInstancesInput
}

func (m *mockECSContainerInstanceListClient) ListContainerInstances(_ context.Context, params *ecs.ListContainerInstancesInput, _ ...func(*ecs.Options)) (*ecs.ListContainerInstancesOutput, error) {
	m.listInputs = append(m.listInputs, params)
	if m.listErr != nil {
		return nil, m.listErr
	}
	idx := len(m.listInputs) - 1
	if idx >= len(m.listPages) {
		return nil, fmt.Errorf("unexpected ListContainerInstances call %d: only %d pages prepared", idx+1, len(m.listPages))
	}
	return m.listPages[idx], nil
}

func (m *mockECSContainerInstanceListClient) DescribeContainerInstances(_ context.Context, params *ecs.DescribeContainerInstancesInput, _ ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error) {
	m.describeInputs = append(m.describeInputs, params)
	if m.describeErr != nil {
		return nil, m.describeErr
	}
	out := &ecs.DescribeContainerInstancesOutput{}
	for _, arn := range params.ContainerInstances {
		ci, ok := m.instances[arn]
		if !ok {
			out.Failures = append(out.Failures, ecstypes.Failure{Arn: aws.String(arn), Reason: aws.String("MISSING")})
			continue
		}
		out.ContainerInstances = append(out.ContainerInstances, ci)
	}
	return out, nil
}

// ecsContainerInstanceArn はテスト用のコンテナインスタンス ARN を連番で作る。
func ecsContainerInstanceArn(i int) string {
	return fmt.Sprintf("arn:aws:ecs:ap-northeast-1:123456789012:container-instance/demo/ci-%03d", i)
}

// ecsContainerInstances は n 台分の ARN と、それぞれ ACTIVE のコンテナインスタンスを返す。
func ecsContainerInstances(n int) ([]string, map[string]ecstypes.ContainerInstance) {
	arns := make([]string, 0, n)
	instances := make(map[string]ecstypes.ContainerInstance, n)
	for i := range n {
		arn := ecsContainerInstanceArn(i)
		arns = append(arns, arn)
		instances[arn] = ecstypes.ContainerInstance{ContainerInstanceArn: aws.String(arn), Status: aws.String("ACTIVE")}
	}
	return arns, instances
}

// captureDefaultLogs は既定の slog ロガーをバッファへ書くハンドラに差し替え、
// 出力を後から確認できるようにする。既定ロガーはプロセス全体で共有されるため、
// これを使うテストは t.Parallel を呼ばない。
func captureDefaultLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// assertWarnLogLines は TextHandler の出力から message を含む行を数え、want と一致することと、
// 各行が wantAttrs のいずれか 1 組をすべて含むことを確認する (1 レコード = 1 行)。
func assertWarnLogLines(t *testing.T, logs string, message string, wantAttrs [][]string) {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, message) {
			lines = append(lines, line)
		}
	}
	if len(lines) != len(wantAttrs) {
		t.Fatalf("warn log %q emitted %d times, want %d: %q", message, len(lines), len(wantAttrs), logs)
	}
	for i, line := range lines {
		for _, w := range append([]string{"level=WARN"}, wantAttrs[i]...) {
			if !strings.Contains(line, w) {
				t.Errorf("warn log line %q does not contain %q", line, w)
			}
		}
	}
}

// assertECSContainerInstanceARNs は返却されたリソースの ARN 列が期待どおりかを確認する。
func assertECSContainerInstanceARNs(t *testing.T, got []ECSContainerInstanceResource, want []string) {
	t.Helper()
	gotARNs := make([]string, 0, len(got))
	for _, r := range got {
		gotARNs = append(gotARNs, r.ARN)
	}
	if diff := cmp.Diff(want, gotARNs); diff != "" {
		t.Errorf("returned arns mismatch (-want +got):\n%s", diff)
	}
}

func TestListECSContainerInstances(t *testing.T) {
	const cluster = "demo-cluster"

	t.Run("2 ページを NextToken で辿り全件を Describe に渡す", func(t *testing.T) {
		arns, instances := ecsContainerInstances(3)
		client := &mockECSContainerInstanceListClient{
			listPages: []*ecs.ListContainerInstancesOutput{
				{ContainerInstanceArns: arns[:2], NextToken: aws.String("page-2")},
				{ContainerInstanceArns: arns[2:]},
			},
			instances: instances,
		}
		got, err := listECSContainerInstances(context.Background(), client, cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantList := []*ecs.ListContainerInstancesInput{
			{Cluster: aws.String(cluster)},
			{Cluster: aws.String(cluster), NextToken: aws.String("page-2")},
		}
		if diff := cmp.Diff(wantList, client.listInputs, cmpopts.IgnoreUnexported(ecs.ListContainerInstancesInput{})); diff != "" {
			t.Errorf("ListContainerInstances inputs mismatch (-want +got):\n%s", diff)
		}
		wantDescribe := []*ecs.DescribeContainerInstancesInput{
			{Cluster: aws.String(cluster), ContainerInstances: arns},
		}
		if diff := cmp.Diff(wantDescribe, client.describeInputs, cmpopts.IgnoreUnexported(ecs.DescribeContainerInstancesInput{})); diff != "" {
			t.Errorf("DescribeContainerInstances inputs mismatch (-want +got):\n%s", diff)
		}
		if len(got) != 3 {
			t.Fatalf("got %d resources, want 3", len(got))
		}
		for i, r := range got {
			if r.ARN != arns[i] || r.Status != "active" {
				t.Errorf("resource %d = %+v, want arn %s status active", i, r, arns[i])
			}
		}
	})

	t.Run("101 件は 100 件と 1 件の 2 回に分けて Describe する", func(t *testing.T) {
		arns, instances := ecsContainerInstances(101)
		client := &mockECSContainerInstanceListClient{
			listPages: []*ecs.ListContainerInstancesOutput{{ContainerInstanceArns: arns}},
			instances: instances,
		}
		got, err := listECSContainerInstances(context.Background(), client, cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantDescribe := []*ecs.DescribeContainerInstancesInput{
			{Cluster: aws.String(cluster), ContainerInstances: arns[:100]},
			{Cluster: aws.String(cluster), ContainerInstances: arns[100:]},
		}
		if diff := cmp.Diff(wantDescribe, client.describeInputs, cmpopts.IgnoreUnexported(ecs.DescribeContainerInstancesInput{})); diff != "" {
			t.Errorf("DescribeContainerInstances inputs mismatch (-want +got):\n%s", diff)
		}
		assertECSContainerInstanceARNs(t, got, arns)
	})

	t.Run("ちょうど 100 件は 1 回の Describe で済ませ空のバッチを送らない", func(t *testing.T) {
		arns, instances := ecsContainerInstances(100)
		client := &mockECSContainerInstanceListClient{
			listPages: []*ecs.ListContainerInstancesOutput{{ContainerInstanceArns: arns}},
			instances: instances,
		}
		got, err := listECSContainerInstances(context.Background(), client, cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantDescribe := []*ecs.DescribeContainerInstancesInput{
			{Cluster: aws.String(cluster), ContainerInstances: arns},
		}
		if diff := cmp.Diff(wantDescribe, client.describeInputs, cmpopts.IgnoreUnexported(ecs.DescribeContainerInstancesInput{})); diff != "" {
			t.Errorf("DescribeContainerInstances inputs mismatch (-want +got):\n%s", diff)
		}
		assertECSContainerInstanceARNs(t, got, arns)
	})

	t.Run("0 件のときは Describe を呼ばず空配列を返す", func(t *testing.T) {
		client := &mockECSContainerInstanceListClient{
			listPages: []*ecs.ListContainerInstancesOutput{{}},
		}
		got, err := listECSContainerInstances(context.Background(), client, cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(client.describeInputs) != 0 {
			t.Errorf("DescribeContainerInstances called %d times, want 0", len(client.describeInputs))
		}
		// JSON で null ではなく [] になるよう nil でない空スライスを返す
		if got == nil || len(got) != 0 {
			t.Errorf("got %#v, want empty non-nil slice", got)
		}
	})

	t.Run("Failures が 1 件あっても残りの件を返しエラーにしない", func(t *testing.T) {
		arns, instances := ecsContainerInstances(3)
		// 2 台目は List と Describe の間に登録解除された想定で Describe の結果に含まれず Failures に載る
		delete(instances, arns[1])
		client := &mockECSContainerInstanceListClient{
			listPages: []*ecs.ListContainerInstancesOutput{{ContainerInstanceArns: arns}},
			instances: instances,
		}
		logs := captureDefaultLogs(t)
		got, err := listECSContainerInstances(context.Background(), client, cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertECSContainerInstanceARNs(t, got, []string{arns[0], arns[2]})
		// Failures は 1 件ずつ警告ログに残す
		assertWarnLogLines(t, logs.String(), "describe ecs container instance failed", [][]string{
			{"cluster=" + cluster, "arn=" + arns[1], "reason=MISSING"},
		})
	})

	t.Run("Failures が全件でもエラーにせず空配列を返す", func(t *testing.T) {
		arns, _ := ecsContainerInstances(2)
		// List の後に全台が登録解除され Describe の結果が Failures だけになった想定
		client := &mockECSContainerInstanceListClient{
			listPages: []*ecs.ListContainerInstancesOutput{{ContainerInstanceArns: arns}},
			instances: map[string]ecstypes.ContainerInstance{},
		}
		logs := captureDefaultLogs(t)
		got, err := listECSContainerInstances(context.Background(), client, cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(client.describeInputs) != 1 {
			t.Errorf("DescribeContainerInstances called %d times, want 1", len(client.describeInputs))
		}
		if got == nil || len(got) != 0 {
			t.Errorf("got %#v, want empty non-nil slice", got)
		}
		assertWarnLogLines(t, logs.String(), "describe ecs container instance failed", [][]string{
			{"cluster=" + cluster, "arn=" + arns[0], "reason=MISSING"},
			{"cluster=" + cluster, "arn=" + arns[1], "reason=MISSING"},
		})
	})

	t.Run("2 バッチ目の Failures は 1 バッチ目の結果と合算される", func(t *testing.T) {
		arns, instances := ecsContainerInstances(102)
		// 101 台目 (2 バッチ目の先頭) だけが Describe の間に登録解除された想定
		delete(instances, arns[100])
		client := &mockECSContainerInstanceListClient{
			listPages: []*ecs.ListContainerInstancesOutput{{ContainerInstanceArns: arns}},
			instances: instances,
		}
		logs := captureDefaultLogs(t)
		got, err := listECSContainerInstances(context.Background(), client, cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := append(append([]string{}, arns[:100]...), arns[101])
		assertECSContainerInstanceARNs(t, got, want)
		assertWarnLogLines(t, logs.String(), "describe ecs container instance failed", [][]string{
			{"cluster=" + cluster, "arn=" + arns[100], "reason=MISSING"},
		})
	})

	t.Run("ListContainerInstances のエラーはラップして返す", func(t *testing.T) {
		wantErr := errors.New("list boom")
		client := &mockECSContainerInstanceListClient{listErr: wantErr}
		_, err := listECSContainerInstances(context.Background(), client, cluster)
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want wrapping %v", err, wantErr)
		}
		if !strings.HasPrefix(err.Error(), "list ecs container instances: ") {
			t.Errorf("err = %q, want prefix %q", err.Error(), "list ecs container instances: ")
		}
	})

	t.Run("DescribeContainerInstances のエラーはラップして返す", func(t *testing.T) {
		arns, instances := ecsContainerInstances(1)
		wantErr := errors.New("describe boom")
		client := &mockECSContainerInstanceListClient{
			listPages:   []*ecs.ListContainerInstancesOutput{{ContainerInstanceArns: arns}},
			instances:   instances,
			describeErr: wantErr,
		}
		_, err := listECSContainerInstances(context.Background(), client, cluster)
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want wrapping %v", err, wantErr)
		}
		if !strings.HasPrefix(err.Error(), "describe ecs container instances: ") {
			t.Errorf("err = %q, want prefix %q", err.Error(), "describe ecs container instances: ")
		}
	})
}

func TestECSContainerInstanceFromSDK(t *testing.T) {
	integer := func(name string, v int32) ecstypes.Resource {
		return ecstypes.Resource{Name: aws.String(name), Type: aws.String("INTEGER"), IntegerValue: v}
	}
	int32Ptr := func(v int32) *int32 { return &v }

	tests := []struct {
		name string
		in   ecstypes.ContainerInstance
		want ECSContainerInstanceResource
	}{
		{
			name: "registered と remaining から CPU と MEMORY だけを取り出し PORTS は捨てる",
			in: ecstypes.ContainerInstance{
				ContainerInstanceArn: aws.String("arn:ci/1"),
				Ec2InstanceId:        aws.String("i-0123456789abcdef0"),
				Status:               aws.String("ACTIVE"),
				AgentConnected:       true,
				RunningTasksCount:    3,
				PendingTasksCount:    1,
				RegisteredResources: []ecstypes.Resource{
					integer("CPU", 2048),
					integer("MEMORY", 3900),
					{Name: aws.String("PORTS"), Type: aws.String("STRINGSET"), StringSetValue: []string{"22", "2376"}},
				},
				RemainingResources: []ecstypes.Resource{
					integer("CPU", 1024),
					integer("MEMORY", 1900),
					{Name: aws.String("PORTS"), Type: aws.String("STRINGSET"), StringSetValue: []string{"22"}},
				},
			},
			want: ECSContainerInstanceResource{
				ARN:               "arn:ci/1",
				EC2InstanceID:     "i-0123456789abcdef0",
				Status:            "active",
				AgentConnected:    true,
				RunningTasksCount: 3,
				PendingTasksCount: 1,
				RegisteredCPU:     int32Ptr(2048),
				RegisteredMemory:  int32Ptr(3900),
				RemainingCPU:      int32Ptr(1024),
				RemainingMemory:   int32Ptr(1900),
			},
		},
		{
			name: "registered に CPU が無いとき registered_cpu は nil",
			in: ecstypes.ContainerInstance{
				RegisteredResources: []ecstypes.Resource{integer("MEMORY", 3900)},
				RemainingResources:  []ecstypes.Resource{integer("CPU", 1024), integer("MEMORY", 1900)},
			},
			want: ECSContainerInstanceResource{
				RegisteredMemory: int32Ptr(3900),
				RemainingCPU:     int32Ptr(1024),
				RemainingMemory:  int32Ptr(1900),
			},
		},
		{
			name: "remaining に MEMORY が無いとき remaining_memory は nil",
			in: ecstypes.ContainerInstance{
				RegisteredResources: []ecstypes.Resource{integer("CPU", 2048), integer("MEMORY", 3900)},
				RemainingResources:  []ecstypes.Resource{integer("CPU", 1024)},
			},
			want: ECSContainerInstanceResource{
				RegisteredCPU:    int32Ptr(2048),
				RegisteredMemory: int32Ptr(3900),
				RemainingCPU:     int32Ptr(1024),
			},
		},
		{
			name: "registered の CPU が DOUBLE 型のとき registered_cpu は nil",
			in: ecstypes.ContainerInstance{
				RegisteredResources: []ecstypes.Resource{
					{Name: aws.String("CPU"), Type: aws.String("DOUBLE"), DoubleValue: 2048},
					integer("MEMORY", 3900),
				},
			},
			want: ECSContainerInstanceResource{RegisteredMemory: int32Ptr(3900)},
		},
		{
			name: "DRAINING は draining になる",
			in:   ecstypes.ContainerInstance{Status: aws.String("DRAINING")},
			want: ECSContainerInstanceResource{Status: "draining"},
		},
		{
			name: "REGISTERING は registering になる",
			in:   ecstypes.ContainerInstance{Status: aws.String("REGISTERING")},
			want: ECSContainerInstanceResource{Status: "registering"},
		},
		{
			name: "REGISTRATION_FAILED は registration-failed になる",
			in:   ecstypes.ContainerInstance{Status: aws.String("REGISTRATION_FAILED")},
			want: ECSContainerInstanceResource{Status: "registration-failed"},
		},
		{
			name: "DEREGISTERING は deregistering になる",
			in:   ecstypes.ContainerInstance{Status: aws.String("DEREGISTERING")},
			want: ECSContainerInstanceResource{Status: "deregistering"},
		},
		{
			name: "External の mi- 始まりの ID もそのまま入れる",
			in:   ecstypes.ContainerInstance{Ec2InstanceId: aws.String("mi-0123456789abcdef0")},
			want: ECSContainerInstanceResource{EC2InstanceID: "mi-0123456789abcdef0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecsContainerInstanceFromSDK(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestListECSContainerInstancesKeepsNonActive は DRAINING / REGISTERING / REGISTRATION_FAILED /
// DEREGISTERING のコンテナインスタンスが一覧から除外されないことを、一覧の経路で検証する。
func TestListECSContainerInstancesKeepsNonActive(t *testing.T) {
	statuses := []string{"ACTIVE", "DRAINING", "REGISTERING", "REGISTRATION_FAILED", "DEREGISTERING"}
	arns := make([]string, 0, len(statuses))
	instances := make(map[string]ecstypes.ContainerInstance, len(statuses))
	for i, st := range statuses {
		arn := ecsContainerInstanceArn(i)
		arns = append(arns, arn)
		instances[arn] = ecstypes.ContainerInstance{ContainerInstanceArn: aws.String(arn), Status: aws.String(st)}
	}
	client := &mockECSContainerInstanceListClient{
		listPages: []*ecs.ListContainerInstancesOutput{{ContainerInstanceArns: arns}},
		instances: instances,
	}
	got, err := listECSContainerInstances(context.Background(), client, "demo-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Status フィルタを渡していないことも固定する
	for i, in := range client.listInputs {
		if in.Status != "" {
			t.Errorf("ListContainerInstances call %d has Status filter %q, want none", i+1, in.Status)
		}
	}
	gotStatuses := make([]string, 0, len(got))
	for _, r := range got {
		gotStatuses = append(gotStatuses, r.Status)
	}
	want := []string{"active", "draining", "registering", "registration-failed", "deregistering"}
	if diff := cmp.Diff(want, gotStatuses); diff != "" {
		t.Errorf("statuses mismatch (-want +got):\n%s", diff)
	}
}
