package aws

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// mockSQSQueueTagsClient は sqsQueueTagsClient の手書きモック。
type mockSQSQueueTagsClient struct {
	listQueueTags func(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error)
}

func (m *mockSQSQueueTagsClient) ListQueueTags(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
	return m.listQueueTags(ctx, params, optFns...)
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
