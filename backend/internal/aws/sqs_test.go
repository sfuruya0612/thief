package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// mockSQSQueueTagsClient は sqsQueueTagsClient の手書きモック。
type mockSQSQueueTagsClient struct {
	listQueueTags func(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error)
}

func (m *mockSQSQueueTagsClient) ListQueueTags(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
	return m.listQueueTags(ctx, params, optFns...)
}

// mockSQSQueueListClient は listSQSResources が要求する sqsQueueListClient を
// テスト用に実装する手書きモック。受け取った Input を記録し、用意したレスポンスを返す。
// キューごとの GetQueueAttributes と ListQueueTags は errgroup で並列に呼ばれるため、
// 記録は mutex で保護する。呼び出し順は不定なので、検証側で QueueUrl 順に整列してから比較する。
// ListQueues は SDK のページネータ経由で逐次呼ばれ、ページネータは呼び出しごとに Input を
// 値でコピーして NextToken だけ差し替える。GetQueueAttributesInput と ListQueueTagsInput は
// キューごとに新しく確保される。いずれも記録した後に内容が書き換わることはなく、
// ポインタのまま記録してよい。
type mockSQSQueueListClient struct {
	listPages []*sqs.ListQueuesOutput
	// attrs はキュー URL ごとに GetQueueAttributes が返す属性。
	attrs map[string]map[string]string

	mu         sync.Mutex
	listInputs []*sqs.ListQueuesInput
	attrInputs []*sqs.GetQueueAttributesInput
	tagInputs  []*sqs.ListQueueTagsInput
}

func (m *mockSQSQueueListClient) ListQueues(_ context.Context, params *sqs.ListQueuesInput, _ ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listInputs = append(m.listInputs, params)
	idx := len(m.listInputs) - 1
	if idx >= len(m.listPages) {
		return nil, fmt.Errorf("unexpected ListQueues call %d: only %d pages prepared", idx+1, len(m.listPages))
	}
	return m.listPages[idx], nil
}

func (m *mockSQSQueueListClient) GetQueueAttributes(_ context.Context, params *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	m.mu.Lock()
	m.attrInputs = append(m.attrInputs, params)
	m.mu.Unlock()
	return &sqs.GetQueueAttributesOutput{Attributes: m.attrs[ptrStr(params.QueueUrl)]}, nil
}

func (m *mockSQSQueueListClient) ListQueueTags(_ context.Context, params *sqs.ListQueueTagsInput, _ ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
	m.mu.Lock()
	m.tagInputs = append(m.tagInputs, params)
	m.mu.Unlock()
	return &sqs.ListQueueTagsOutput{}, nil
}

// TestListSQSResourcesSendsAttributeNamesAll は GetQueueAttributes へ AttributeNames に
// QueueAttributeNameAll を載せて送ることを検証する。この指定が無いと SQS は属性を 1 つも返さず、
// sqsFromAttributes が参照する QueueArn / FifoQueue / MessageRetentionPeriod / メッセージ数が
// 呼び出しを成功させたまま静かに既定値 (ID は URL フォールバック、Type は常に Standard、
// カウント類は 0) に落ちる。
// 期待値の AttributeNames は実装の定数式ではなく AWS API の値で直接書き下している。
// 併せて ListQueues と ListQueueTags の Input も検証する。前者はページ送りが行われること、
// 後者は一覧経路がキューごとにタグ取得を呼ぶこと (省いてもキュー情報は返るため他の
// アサーションでは検出できない) を固定する。
func TestListSQSResourcesSendsAttributeNamesAll(t *testing.T) {
	const (
		urlA = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/alpha"
		urlB = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/bravo"
		arnA = "arn:aws:sqs:ap-northeast-1:123456789012:alpha"
		arnB = "arn:aws:sqs:ap-northeast-1:123456789012:bravo"
	)

	client := &mockSQSQueueListClient{
		// URL の列挙は 2 ページに分ける。両ページのキューについて属性取得が行われることを
		// 確かめる。
		listPages: []*sqs.ListQueuesOutput{
			{QueueUrls: []string{urlA}, NextToken: aws.String("page-2")},
			{QueueUrls: []string{urlB}},
		},
		attrs: map[string]map[string]string{
			urlA: {"QueueArn": arnA},
			urlB: {"QueueArn": arnB},
		},
	}

	got, err := listSQSResources(context.Background(), client, "test-profile", "ap-northeast-1")
	if err != nil {
		t.Fatalf("listSQSResources() error = %v", err)
	}

	// SQS の各 Input は unexported フィールド (noSmithyDocumentSerde) を持つため無視する。
	opts := cmpopts.IgnoreUnexported(
		sqs.ListQueuesInput{},
		sqs.GetQueueAttributesInput{},
		sqs.ListQueueTagsInput{},
	)

	// ページ送りが 2 回目の呼び出しへ NextToken として引き継がれることを固定する。
	// ページネータは Limit 未指定のため MaxResults を設定しない。
	wantListInputs := []*sqs.ListQueuesInput{
		{},
		{NextToken: aws.String("page-2")},
	}
	if diff := cmp.Diff(wantListInputs, client.listInputs, opts); diff != "" {
		t.Errorf("ListQueues inputs mismatch (-want +got):\n%s", diff)
	}

	// 呼び出し順は errgroup により不定のため QueueUrl 順に整列する。g.Wait() の後なので
	// 全 goroutine の記録が完了しており、ここでのロックは不要。
	slices.SortFunc(client.attrInputs, func(a, b *sqs.GetQueueAttributesInput) int {
		return strings.Compare(ptrStr(a.QueueUrl), ptrStr(b.QueueUrl))
	})
	// Input 全体を比較し、AttributeNames に加えて QueueUrl の対応付けも固定する。
	// スライス全体を比較することで呼び出し回数も同時に固定される。
	wantAttrInputs := []*sqs.GetQueueAttributesInput{
		{QueueUrl: aws.String(urlA), AttributeNames: []sqstypes.QueueAttributeName{"All"}},
		{QueueUrl: aws.String(urlB), AttributeNames: []sqstypes.QueueAttributeName{"All"}},
	}
	if diff := cmp.Diff(wantAttrInputs, client.attrInputs, opts); diff != "" {
		t.Errorf("GetQueueAttributes inputs mismatch (-want +got):\n%s", diff)
	}

	slices.SortFunc(client.tagInputs, func(a, b *sqs.ListQueueTagsInput) int {
		return strings.Compare(ptrStr(a.QueueUrl), ptrStr(b.QueueUrl))
	})
	wantTagInputs := []*sqs.ListQueueTagsInput{
		{QueueUrl: aws.String(urlA)},
		{QueueUrl: aws.String(urlB)},
	}
	if diff := cmp.Diff(wantTagInputs, client.tagInputs, opts); diff != "" {
		t.Errorf("ListQueueTags inputs mismatch (-want +got):\n%s", diff)
	}

	// 両ページの URL が集約され、取得した属性が結果へ反映されることも確認する。
	gotIDs := make([]string, 0, len(got))
	for _, r := range got {
		gotIDs = append(gotIDs, r.ID)
	}
	if diff := cmp.Diff([]string{arnA, arnB}, gotIDs); diff != "" {
		t.Errorf("queue IDs mismatch (-want +got):\n%s", diff)
	}
}

func TestSQSFromAttributes(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		attrs           map[string]string
		tags            map[string]string
		tagsFetchFailed bool
		want            SQSResource
	}{
		{
			name: "standard",
			url:  "https://sqs.ap-northeast-1.amazonaws.com/123/my-queue",
			attrs: map[string]string{
				"QueueArn":                              "arn:aws:sqs:ap-northeast-1:123:my-queue",
				"ApproximateNumberOfMessages":           "5",
				"ApproximateNumberOfMessagesNotVisible": "2",
				"MessageRetentionPeriod":                "345600",
			},
			tags: map[string]string{"env": "prod"},
			want: SQSResource{
				ID:                "arn:aws:sqs:ap-northeast-1:123:my-queue",
				Name:              "my-queue",
				State:             "active",
				Type:              "Standard",
				AvailableMessages: 5,
				InFlight:          2,
				RetentionDays:     4,
				Tags:              map[string]string{"env": "prod"},
			},
		},
		{
			name: "fifo",
			url:  "https://sqs.ap-northeast-1.amazonaws.com/123/my-queue.fifo",
			attrs: map[string]string{
				"FifoQueue":              "true",
				"MessageRetentionPeriod": "86400",
			},
			tags: map[string]string{},
			want: SQSResource{
				ID:            "https://sqs.ap-northeast-1.amazonaws.com/123/my-queue.fifo",
				Name:          "my-queue.fifo",
				State:         "active",
				Type:          "FIFO",
				RetentionDays: 1,
				Tags:          map[string]string{},
			},
		},
		{
			name: "tags の取得失敗を反映する",
			url:  "https://sqs.ap-northeast-1.amazonaws.com/123/degraded-queue",
			attrs: map[string]string{
				"QueueArn": "arn:aws:sqs:ap-northeast-1:123:degraded-queue",
			},
			tags:            map[string]string{},
			tagsFetchFailed: true,
			want: SQSResource{
				ID:              "arn:aws:sqs:ap-northeast-1:123:degraded-queue",
				Name:            "degraded-queue",
				State:           "active",
				Type:            "Standard",
				Tags:            map[string]string{},
				TagsFetchFailed: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqsFromAttributes(tt.url, tt.attrs, tt.tags, tt.tagsFetchFailed)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestFetchSQSQueueTags(t *testing.T) {
	const url = "https://sqs.ap-northeast-1.amazonaws.com/123/my-queue"

	t.Run("成功時はタグを返す", func(t *testing.T) {
		client := &mockSQSQueueTagsClient{
			listQueueTags: func(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
				return &sqs.ListQueueTagsOutput{Tags: map[string]string{"env": "prod"}}, nil
			},
		}

		tags, tagsFetchFailed, err := fetchSQSQueueTags(context.Background(), client, url)
		if err != nil {
			t.Fatalf("fetchSQSQueueTags() error = %v", err)
		}
		if tagsFetchFailed {
			t.Errorf("tagsFetchFailed = true, want false")
		}
		if !reflect.DeepEqual(tags, map[string]string{"env": "prod"}) {
			t.Errorf("tags = %v, want {env: prod}", tags)
		}
	})

	t.Run("nil タグは非 nil の空 map に正規化する", func(t *testing.T) {
		client := &mockSQSQueueTagsClient{
			listQueueTags: func(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
				return &sqs.ListQueueTagsOutput{Tags: nil}, nil
			},
		}

		tags, tagsFetchFailed, err := fetchSQSQueueTags(context.Background(), client, url)
		if err != nil {
			t.Fatalf("fetchSQSQueueTags() error = %v", err)
		}
		if tagsFetchFailed {
			t.Errorf("tagsFetchFailed = true, want false")
		}
		if tags == nil {
			t.Fatal("tags = nil, want non-nil empty map")
		}
		if len(tags) != 0 {
			t.Errorf("tags = %v, want empty", tags)
		}
	})

	t.Run("取得失敗で tagsFetchFailed が立つ", func(t *testing.T) {
		client := &mockSQSQueueTagsClient{
			listQueueTags: func(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
				return nil, errors.New("throttled")
			},
		}

		tags, tagsFetchFailed, err := fetchSQSQueueTags(context.Background(), client, url)
		if err != nil {
			t.Fatalf("fetchSQSQueueTags() error = %v", err)
		}
		if !tagsFetchFailed {
			t.Errorf("tagsFetchFailed = false, want true")
		}
		if tags == nil {
			t.Fatal("tags = nil, want non-nil empty map")
		}
	})

	t.Run("出力が nil の場合も tagsFetchFailed が立つ", func(t *testing.T) {
		client := &mockSQSQueueTagsClient{
			listQueueTags: func(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
				return nil, nil
			},
		}

		tags, tagsFetchFailed, err := fetchSQSQueueTags(context.Background(), client, url)
		if err != nil {
			t.Fatalf("fetchSQSQueueTags() error = %v", err)
		}
		if !tagsFetchFailed {
			t.Errorf("tagsFetchFailed = false, want true")
		}
		if tags == nil {
			t.Fatal("tags = nil, want non-nil empty map")
		}
	})

	t.Run("キャンセルはエラーとして伝播する", func(t *testing.T) {
		client := &mockSQSQueueTagsClient{
			listQueueTags: func(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
				return nil, context.Canceled
			},
		}

		_, _, err := fetchSQSQueueTags(context.Background(), client, url)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("fetchSQSQueueTags() error = %v, want context.Canceled", err)
		}
	})
}

func TestSQSResourceJSONTagsNeverNull(t *testing.T) {
	tests := []struct {
		name            string
		tags            map[string]string
		tagsFetchFailed bool
		wantTagsJSON    string
	}{
		{name: "成功時は取得したタグがそのまま出力される", tags: map[string]string{"env": "prod"}, tagsFetchFailed: false, wantTagsJSON: `"tags":{"env":"prod"}`},
		{name: "失敗時は空 map が出力される", tags: map[string]string{}, tagsFetchFailed: true, wantTagsJSON: `"tags":{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := sqsFromAttributes("https://sqs.example/queue-1", map[string]string{}, tt.tags, tt.tagsFetchFailed)
			b, err := json.Marshal(r)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if strings.Contains(string(b), `"tags":null`) {
				t.Errorf("json = %s, tags は null になってはいけない", b)
			}
			if !strings.Contains(string(b), tt.wantTagsJSON) {
				t.Errorf("json = %s, want substring %s", b, tt.wantTagsJSON)
			}
		})
	}
}

func TestSQSResourceJSONFetchFailedOmitempty(t *testing.T) {
	tests := []struct {
		name            string
		tagsFetchFailed bool
		wantKey         bool
	}{
		{name: "false ならキーが省略される", tagsFetchFailed: false, wantKey: false},
		{name: "true ならキーが true で出力される", tagsFetchFailed: true, wantKey: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(SQSResource{
				ID:              "queue-1",
				Name:            "my-queue",
				TagsFetchFailed: tt.tagsFetchFailed,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			gotKey := strings.Contains(string(b), `"tags_fetch_failed":true`)
			if gotKey != tt.wantKey {
				t.Errorf("json = %s, tags_fetch_failed key present = %v, want %v", b, gotKey, tt.wantKey)
			}
		})
	}
}
