package aws

import (
	"context"
	"fmt"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestEc2InstanceInfoFromSDK(t *testing.T) {
	launchTime := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		inst ec2types.Instance
		want EC2InstanceInfo
	}{
		{
			name: "full fields",
			inst: ec2types.Instance{
				InstanceId:        awssdk.String("i-0123456789abcdef0"),
				InstanceType:      ec2types.InstanceTypeT3Micro,
				InstanceLifecycle: ec2types.InstanceLifecycleTypeSpot,
				PrivateIpAddress:  awssdk.String("10.0.0.1"),
				PublicIpAddress:   awssdk.String("203.0.113.1"),
				KeyName:           awssdk.String("my-key"),
				State:             &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
				Placement:         &ec2types.Placement{AvailabilityZone: awssdk.String("ap-northeast-1a")},
				LaunchTime:        awssdk.Time(launchTime),
				Tags: []ec2types.Tag{
					{Key: awssdk.String("Name"), Value: awssdk.String("web-server")},
				},
			},
			want: EC2InstanceInfo{
				Name:         "web-server",
				InstanceID:   "i-0123456789abcdef0",
				InstanceType: "t3.micro",
				Lifecycle:    "spot",
				PrivateIP:    "10.0.0.1",
				PublicIP:     "203.0.113.1",
				State:        "running",
				KeyName:      "my-key",
				AZ:           "ap-northeast-1a",
				LaunchTime:   launchTime.String(),
			},
		},
		{
			name: "missing optional fields use None defaults",
			inst: ec2types.Instance{
				InstanceId:   awssdk.String("i-0fedcba9876543210"),
				InstanceType: ec2types.InstanceTypeT3Small,
				State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped},
				Placement:    &ec2types.Placement{AvailabilityZone: awssdk.String("ap-northeast-1c")},
			},
			want: EC2InstanceInfo{
				Name:         "",
				InstanceID:   "i-0fedcba9876543210",
				InstanceType: "t3.small",
				Lifecycle:    "OnDemand",
				PrivateIP:    "None",
				PublicIP:     "None",
				State:        "stopped",
				KeyName:      "None",
				AZ:           "ap-northeast-1c",
				LaunchTime:   "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec2InstanceInfoFromSDK(tt.inst)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ec2InstanceInfoFromSDK mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// mockEC2DescribeInstancesClient は listEC2Instances が要求する ec2DescribeInstancesClient を
// テスト用に実装する手書きモック。受け取った Input を呼び出し順に記録し、pages に用意した
// レスポンスを 1 呼び出しにつき 1 ページ返す。
// SDK のページネータは呼び出しごとに Input を値でコピーして NextToken だけ差し替えるため、
// 記録した後に内容が書き換わることはなく、ポインタのまま記録してよい。
type mockEC2DescribeInstancesClient struct {
	pages  []*ec2.DescribeInstancesOutput
	inputs []*ec2.DescribeInstancesInput
}

func (m *mockEC2DescribeInstancesClient) DescribeInstances(_ context.Context, params *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.inputs = append(m.inputs, params)
	idx := len(m.inputs) - 1
	if idx >= len(m.pages) {
		return nil, fmt.Errorf("unexpected DescribeInstances call %d: only %d pages prepared", idx+1, len(m.pages))
	}
	return m.pages[idx], nil
}

// ec2DescribeInstancesPages は NextToken で連結された 2 ページ構成のレスポンスを返す。
// ページネータが 2 ページ目の呼び出しでもフィルタを維持することを検証するために使う。
func ec2DescribeInstancesPages() []*ec2.DescribeInstancesOutput {
	return []*ec2.DescribeInstancesOutput{
		{
			Reservations: []ec2types.Reservation{
				{Instances: []ec2types.Instance{{InstanceId: awssdk.String("i-page1")}}},
			},
			NextToken: awssdk.String("page-2"),
		},
		{
			Reservations: []ec2types.Reservation{
				{Instances: []ec2types.Instance{{InstanceId: awssdk.String("i-page2")}}},
			},
		},
	}
}

// TestListEC2InstancesSendsFilters は EC2ListOptions の組み合わせごとに、
// DescribeInstances へ送られる Filters を検証する。
// Filters の構築を落とすと絞り込みが静かに効かなくなり全インスタンスが返るため、
// オプション未指定のときに Filters が nil のままであることも併せて固定する。
// フィルタの値はケースごとに変える。全ケースで同じ値にすると、オプションを無視して
// その値を固定で送る実装に変えてもテストが通ってしまう。
// 期待値の Name はいずれも実装の定数を参照せず AWS API の文字列で直接書き下している。
func TestListEC2InstancesSendsFilters(t *testing.T) {
	tests := []struct {
		name        string
		opts        EC2ListOptions
		wantFilters []ec2types.Filter
	}{
		{
			name:        "オプション未指定",
			opts:        EC2ListOptions{},
			wantFilters: nil,
		},
		{
			name: "Running のみ",
			opts: EC2ListOptions{Running: true},
			wantFilters: []ec2types.Filter{
				{Name: awssdk.String("instance-state-name"), Values: []string{"running"}},
			},
		},
		{
			// InstanceIDs は複数指定する。1 件だけにすると、先頭の 1 件しか
			// Values に載せない実装に変えても検出できない。
			name: "InstanceIDs のみ",
			opts: EC2ListOptions{InstanceIDs: []string{"i-0a1b2c3d4e5f6789", "i-9876543210fedcba"}},
			wantFilters: []ec2types.Filter{
				{Name: awssdk.String("instance-id"), Values: []string{"i-0a1b2c3d4e5f6789", "i-9876543210fedcba"}},
			},
		},
		{
			// 両方を指定したときは running 側が先、instance-id 側が後に並ぶ。
			name: "Running と InstanceIDs",
			opts: EC2ListOptions{Running: true, InstanceIDs: []string{"i-1122334455667788"}},
			wantFilters: []ec2types.Filter{
				{Name: awssdk.String("instance-state-name"), Values: []string{"running"}},
				{Name: awssdk.String("instance-id"), Values: []string{"i-1122334455667788"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockEC2DescribeInstancesClient{pages: ec2DescribeInstancesPages()}
			got, err := listEC2Instances(context.Background(), client, tt.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(client.inputs) != 2 {
				t.Fatalf("DescribeInstances called %d times, want 2", len(client.inputs))
			}
			// フィルタは 2 ページ目の呼び出しでも維持される。
			for i, in := range client.inputs {
				if diff := cmp.Diff(tt.wantFilters, in.Filters, cmpopts.IgnoreUnexported(ec2types.Filter{})); diff != "" {
					t.Errorf("call %d: Filters mismatch (-want +got):\n%s", i+1, diff)
				}
			}
			// 1 回目に NextToken が無く 2 回目に引き継がれていることで、実際にページ送りが
			// 起きた (2 回目が同一ページの再取得ではない) ことを確かめる。
			if client.inputs[0].NextToken != nil {
				t.Errorf("call 1: NextToken = %q, want nil", awssdk.ToString(client.inputs[0].NextToken))
			}
			if v := awssdk.ToString(client.inputs[1].NextToken); v != "page-2" {
				t.Errorf("call 2: NextToken = %q, want %q", v, "page-2")
			}

			// 両ページのインスタンスが集約されることも確認する。最後のページだけを使う
			// 実装や、Reservations の入れ子ループが崩れた実装をここで検出する。
			gotIDs := make([]string, 0, len(got))
			for _, inst := range got {
				gotIDs = append(gotIDs, inst.InstanceID)
			}
			if diff := cmp.Diff([]string{"i-page1", "i-page2"}, gotIDs); diff != "" {
				t.Errorf("instance ids mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
