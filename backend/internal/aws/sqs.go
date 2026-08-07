package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"golang.org/x/sync/errgroup"
)

// SQSResource represents an SQS queue.
type SQSResource struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	State             string            `json:"state"`
	Type              string            `json:"type"`
	AvailableMessages int               `json:"available_messages"`
	InFlight          int               `json:"in_flight"`
	RetentionDays     int               `json:"retention_days"`
	Tags              map[string]string `json:"tags"`
	TagsFetchFailed   bool              `json:"tags_fetch_failed,omitempty"`
	CostMonthly       float64           `json:"cost_monthly"`
}

func (r SQSResource) ResourceID() string    { return r.ID }
func (r SQSResource) ResourceName() string  { return r.Name }
func (r SQSResource) ResourceState() string { return NormalizeState(r.State) }
func (r SQSResource) ServiceName() string   { return "sqs" }

// sqsQueueConcurrency はキューごとの詳細取得 (GetQueueAttributes / ListQueueTags) を同時実行
// する上限数。無制限にすると SQS のリクエストレート上限に抵触しうるため上限を設ける
// (s3BucketConcurrency と同型)。
const sqsQueueConcurrency = 30

// sqsQueueTagsClient は SQS キューのタグ取得に必要な API 呼び出しを抽象化する。
type sqsQueueTagsClient interface {
	ListQueueTags(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error)
}

// sqsQueueListClient はキュー一覧の取得に必要な API を抽象化する。
// URL の列挙とキューごとの属性・タグの取得という 2 段構えのため、ページネータが要求する
// sqs.ListQueuesAPIClient に GetQueueAttributes と、タグ取得の sqsQueueTagsClient を加える。
// テストではモックを差し込み、実行時は *sqs.Client がこれを満たす。
type sqsQueueListClient interface {
	sqs.ListQueuesAPIClient
	sqsQueueTagsClient
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

// ListSQSResources returns all SQS queues for the given profile/region.
func ListSQSResources(ctx context.Context, profile, region string) ([]SQSResource, error) {
	// 各フェーズの所要時間を計測してログに残す (issue 0081: クライアント生成の区間は
	// issue 0083 の GetSession キャッシュ化調査の入力を兼ねる)。
	overallStart := time.Now()

	clientStart := time.Now()
	client, err := newSQSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	slog.Info("sqs client created",
		"profile", profile, "region", region, "duration_ms", time.Since(clientStart).Milliseconds())

	resources, err := listSQSResources(ctx, client, profile, region)
	if err != nil {
		return nil, err
	}

	slog.Info("sqs list all done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(overallStart).Milliseconds(), "count", len(resources))
	return resources, nil
}

// listSQSResources は生成済みクライアントでキュー一覧と各キューの属性・タグを取得するコア。
// GetQueueAttributesInput に載せる AttributeNames を単体テストで固定できるよう、
// クライアントの生成と分離してある。
// profile と region はフェーズごとの所要時間ログの属性にのみ使う。
func listSQSResources(ctx context.Context, client sqsQueueListClient, profile, region string) ([]SQSResource, error) {
	listStart := time.Now()
	var urls []string
	paginator := sqs.NewListQueuesPaginator(client, &sqs.ListQueuesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sqs queues: %w", err)
		}
		urls = append(urls, page.QueueUrls...)
	}
	slog.Info("sqs list queues done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(listStart).Milliseconds(), "count", len(urls))

	// キューごとの詳細取得は互いに独立しているため並列実行する。各 goroutine は
	// 自分の index にのみ書き込むため結果スライスへの書き込みはロック不要で競合しない
	// (データオーナーシップを goroutine ごとに分離、ListS3Resources と同型)。
	detailStart := time.Now()
	resources := make([]SQSResource, len(urls))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(sqsQueueConcurrency)
	for i, url := range urls {
		g.Go(func() error {
			attrs, err := client.GetQueueAttributes(gctx, &sqs.GetQueueAttributesInput{
				QueueUrl:       aws.String(url),
				AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
			})
			if err != nil {
				return fmt.Errorf("get queue attributes %s: %w", url, err)
			}
			// タグ取得は失敗してもキュー情報は返す (キャンセル起因は全体エラーとして伝播)
			tags, tagsFetchFailed, err := fetchSQSQueueTags(gctx, client, url)
			if err != nil {
				return err
			}
			resources[i] = sqsFromAttributes(url, attrs.Attributes, tags, tagsFetchFailed)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slog.Info("sqs get queue attributes done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(detailStart).Milliseconds(),
		"concurrency", sqsQueueConcurrency, "count", len(resources))

	return resources, nil
}

// fetchSQSQueueTags はキューのタグを取得する。取得に失敗した場合は空 map と
// tagsFetchFailed=true を返す (キャンセル起因は全体エラーとして伝播)。
func fetchSQSQueueTags(ctx context.Context, client sqsQueueTagsClient, url string) (map[string]string, bool, error) {
	tags := map[string]string{}
	tagsOut, tagErr := client.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(url)})
	if tagErr == nil && tagsOut != nil {
		if tagsOut.Tags != nil {
			tags = tagsOut.Tags
		}
		return tags, false, nil
	}
	degraded, err := handleIgnoredErrFlag(tagErr, "list sqs queue tags failed (ignored)", "queue_url", url)
	if err != nil {
		return nil, false, err
	}
	tagsFetchFailed := degraded || tagErr == nil
	return tags, tagsFetchFailed, nil
}

func sqsFromAttributes(url string, attrs map[string]string, tags map[string]string, tagsFetchFailed bool) SQSResource {
	name := url
	if idx := strings.LastIndex(url, "/"); idx >= 0 && idx < len(url)-1 {
		name = url[idx+1:]
	}
	qtype := "Standard"
	if attrs["FifoQueue"] == "true" {
		qtype = "FIFO"
	}
	id := attrs["QueueArn"]
	if id == "" {
		id = url
	}
	retentionDays := 0
	if secs, err := strconv.Atoi(attrs["MessageRetentionPeriod"]); err == nil {
		retentionDays = secs / 86400
	}
	avail, _ := strconv.Atoi(attrs["ApproximateNumberOfMessages"])
	inflight, _ := strconv.Atoi(attrs["ApproximateNumberOfMessagesNotVisible"])
	return SQSResource{
		ID:                id,
		Name:              name,
		State:             "active",
		Type:              qtype,
		AvailableMessages: avail,
		InFlight:          inflight,
		RetentionDays:     retentionDays,
		Tags:              tags,
		TagsFetchFailed:   tagsFetchFailed,
	}
}

// newSQSClient は SQS API クライアントを生成する。
func newSQSClient(ctx context.Context, profile, region string) (*sqs.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *sqs.Client {
		return sqs.NewFromConfig(cfg)
	})
}
