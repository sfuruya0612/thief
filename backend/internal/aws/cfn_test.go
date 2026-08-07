package aws

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// mockCfnListStacksClient は cfnListStacksClient の手書きモック。受け取った Input を
// 呼び出し順に記録し、pages に用意したレスポンスを 1 呼び出しにつき 1 ページ返す。
type mockCfnListStacksClient struct {
	pages  []*cloudformation.ListStacksOutput
	inputs []*cloudformation.ListStacksInput
}

func (m *mockCfnListStacksClient) ListStacks(_ context.Context, params *cloudformation.ListStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error) {
	m.inputs = append(m.inputs, params)
	idx := len(m.inputs) - 1
	if idx >= len(m.pages) {
		return nil, fmt.Errorf("unexpected ListStacks call %d: only %d pages prepared", idx+1, len(m.pages))
	}
	return m.pages[idx], nil
}

// cfnListStacksPages は NextToken で連結された 2 ページ構成のレスポンスを返す。
// ページネータが 2 ページ目の呼び出しでも元のパラメータを維持することを検証するために使う。
func cfnListStacksPages() []*cloudformation.ListStacksOutput {
	return []*cloudformation.ListStacksOutput{
		{
			StackSummaries: []cfntypes.StackSummary{{StackName: aws.String("stack-1")}},
			NextToken:      aws.String("page-2"),
		},
		{
			StackSummaries: []cfntypes.StackSummary{{StackName: aws.String("stack-2")}},
		},
	}
}

// assertCfnStackStatusFilterOnAllCalls は全 2 回の呼び出しの Input で StackStatusFilter が
// want と同じ集合であることと、2 回目の呼び出しに 1 ページ目の NextToken が引き継がれている
// (実際にページ送りが起きた) ことを検証する。
func assertCfnStackStatusFilterOnAllCalls(t *testing.T, inputs []*cloudformation.ListStacksInput, want []string) {
	t.Helper()
	if len(inputs) != 2 {
		t.Fatalf("ListStacks called %d times, want 2", len(inputs))
	}
	for i, in := range inputs {
		got := make([]string, 0, len(in.StackStatusFilter))
		for _, s := range in.StackStatusFilter {
			got = append(got, string(s))
		}
		// フィルタは集合として意味を持つため、列挙の順序には依存せずに比較する。
		sortStrings := cmpopts.SortSlices(func(a, b string) bool { return a < b })
		if diff := cmp.Diff(want, got, sortStrings); diff != "" {
			t.Errorf("call %d: StackStatusFilter mismatch (-want +got):\n%s", i+1, diff)
		}
	}
	if inputs[0].NextToken != nil {
		t.Errorf("call 1: NextToken = %q, want nil", aws.ToString(inputs[0].NextToken))
	}
	if got := aws.ToString(inputs[1].NextToken); got != "page-2" {
		t.Errorf("call 2: NextToken = %q, want %q", got, "page-2")
	}
}

// TestListStacksSendsStackStatusFilter は 2 つの一覧経路が、それぞれのステータス集合で
// ListStacks を呼ぶことを検証する。期待値は SDK の定数ではなく AWS API のステータス
// 文字列で書き、実装の列挙をそのまま写さずに集合を固定する。
// listCFNStacks は Web API 用の 18 種、listCfnStackSummaries は DELETE_COMPLETE のみを
// 除いたレガシー CLI 互換の 22 種である。前者が削除されていない 4 状態を落としている点は
// issues/0122 で扱うため、ここでは現状の集合をそのまま固定する。
func TestListStacksSendsStackStatusFilter(t *testing.T) {
	tests := []struct {
		name string
		list func(context.Context, cfnListStacksClient) (int, error)
		want []string
	}{
		{
			name: "listCFNStacks sends the 18 status web api set",
			list: func(ctx context.Context, c cfnListStacksClient) (int, error) {
				resources, err := listCFNStacks(ctx, c)
				return len(resources), err
			},
			want: []string{
				"CREATE_COMPLETE",
				"UPDATE_COMPLETE",
				"ROLLBACK_COMPLETE",
				"UPDATE_ROLLBACK_COMPLETE",
				"CREATE_IN_PROGRESS",
				"UPDATE_IN_PROGRESS",
				"DELETE_IN_PROGRESS",
				"ROLLBACK_IN_PROGRESS",
				"CREATE_FAILED",
				"UPDATE_FAILED",
				"ROLLBACK_FAILED",
				"UPDATE_ROLLBACK_FAILED",
				"IMPORT_COMPLETE",
				"IMPORT_IN_PROGRESS",
				"IMPORT_ROLLBACK_COMPLETE",
				"IMPORT_ROLLBACK_FAILED",
				"IMPORT_ROLLBACK_IN_PROGRESS",
				"REVIEW_IN_PROGRESS",
			},
		},
		{
			name: "listCfnStackSummaries sends the 22 status legacy cli set",
			list: func(ctx context.Context, c cfnListStacksClient) (int, error) {
				summaries, err := listCfnStackSummaries(ctx, c)
				return len(summaries), err
			},
			want: []string{
				"CREATE_IN_PROGRESS",
				"CREATE_FAILED",
				"CREATE_COMPLETE",
				"ROLLBACK_IN_PROGRESS",
				"ROLLBACK_FAILED",
				"ROLLBACK_COMPLETE",
				"DELETE_IN_PROGRESS",
				"DELETE_FAILED",
				"UPDATE_IN_PROGRESS",
				"UPDATE_COMPLETE_CLEANUP_IN_PROGRESS",
				"UPDATE_COMPLETE",
				"UPDATE_FAILED",
				"UPDATE_ROLLBACK_IN_PROGRESS",
				"UPDATE_ROLLBACK_FAILED",
				"UPDATE_ROLLBACK_COMPLETE_CLEANUP_IN_PROGRESS",
				"UPDATE_ROLLBACK_COMPLETE",
				"REVIEW_IN_PROGRESS",
				"IMPORT_IN_PROGRESS",
				"IMPORT_COMPLETE",
				"IMPORT_ROLLBACK_IN_PROGRESS",
				"IMPORT_ROLLBACK_FAILED",
				"IMPORT_ROLLBACK_COMPLETE",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockCfnListStacksClient{pages: cfnListStacksPages()}
			got, err := tt.list(context.Background(), client)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			// 2 ページ分を読み切っていること (ページ送りが実際に起きたこと) を先に固定する。
			if got != 2 {
				t.Fatalf("got %d items, want 2", got)
			}
			assertCfnStackStatusFilterOnAllCalls(t, client.inputs, tt.want)
		})
	}
}

func TestCfnEventFromSDK(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name string
		in   cfntypes.StackEvent
		want CFNStackEvent
	}{
		{
			name: "failure event populated",
			in: cfntypes.StackEvent{
				Timestamp:            &ts,
				LogicalResourceId:    aws.String("MyResource"),
				ResourceType:         aws.String("AWS::S3::Bucket"),
				ResourceStatus:       cfntypes.ResourceStatusCreateFailed,
				ResourceStatusReason: aws.String("Bucket already exists"),
			},
			want: CFNStackEvent{
				Timestamp:            ts.Format(time.RFC3339),
				LogicalResourceID:    "MyResource",
				ResourceType:         "AWS::S3::Bucket",
				ResourceStatus:       "CREATE_FAILED",
				ResourceStatusReason: "Bucket already exists",
			},
		},
		{
			name: "no timestamp and no reason stays empty",
			in: cfntypes.StackEvent{
				LogicalResourceId: aws.String("Other"),
				ResourceType:      aws.String("AWS::IAM::Role"),
				ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
			},
			want: CFNStackEvent{
				LogicalResourceID: "Other",
				ResourceType:      "AWS::IAM::Role",
				ResourceStatus:    "CREATE_COMPLETE",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfnEventFromSDK(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestCfnResourceFromSDK(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name string
		in   cfntypes.StackResourceSummary
		want CFNStackResourceSummary
	}{
		{
			name: "populated resource",
			in: cfntypes.StackResourceSummary{
				LogicalResourceId:    aws.String("MyBucket"),
				PhysicalResourceId:   aws.String("my-bucket-abc123"),
				ResourceType:         aws.String("AWS::S3::Bucket"),
				ResourceStatus:       cfntypes.ResourceStatusUpdateComplete,
				LastUpdatedTimestamp: &ts,
			},
			want: CFNStackResourceSummary{
				LogicalResourceID:  "MyBucket",
				PhysicalResourceID: "my-bucket-abc123",
				ResourceType:       "AWS::S3::Bucket",
				ResourceStatus:     "UPDATE_COMPLETE",
				LastUpdatedTime:    ts.Format(time.RFC3339),
			},
		},
		{
			name: "no physical id (creation still in progress)",
			in: cfntypes.StackResourceSummary{
				LogicalResourceId:    aws.String("Pending"),
				ResourceType:         aws.String("AWS::Lambda::Function"),
				ResourceStatus:       cfntypes.ResourceStatusCreateInProgress,
				LastUpdatedTimestamp: &ts,
			},
			want: CFNStackResourceSummary{
				LogicalResourceID: "Pending",
				ResourceType:      "AWS::Lambda::Function",
				ResourceStatus:    "CREATE_IN_PROGRESS",
				LastUpdatedTime:   ts.Format(time.RFC3339),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfnResourceFromSDK(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestAppendCFNStackEventsPage(t *testing.T) {
	makeEvents := func(n int) []cfntypes.StackEvent {
		events := make([]cfntypes.StackEvent, n)
		for i := range events {
			events[i] = cfntypes.StackEvent{
				LogicalResourceId: aws.String("R"),
				ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
			}
		}
		return events
	}

	tests := []struct {
		name     string
		existing int
		page     int
		limit    int
		wantLen  int
	}{
		{name: "under limit appends all", existing: 0, page: 3, limit: 100, wantLen: 3},
		{name: "page exactly fills limit", existing: 90, page: 10, limit: 100, wantLen: 100},
		{name: "page exceeds limit truncates", existing: 95, page: 20, limit: 100, wantLen: 100},
		{name: "already at limit appends nothing", existing: 100, page: 5, limit: 100, wantLen: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := make([]CFNStackEvent, tt.existing)
			got := appendCFNStackEventsPage(existing, makeEvents(tt.page), tt.limit)
			if len(got) != tt.wantLen {
				t.Errorf("got len %d want %d", len(got), tt.wantLen)
			}
		})
	}
}
