package aws

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesistypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"golang.org/x/sync/errgroup"
)

// KinesisResource represents a Kinesis Data Stream.
type KinesisResource struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	State          string            `json:"state"`
	ShardCount     int32             `json:"shard_count"`
	RetentionHours int32             `json:"retention_hours"`
	EncryptionType string            `json:"encryption_type"`
	Tags           map[string]string `json:"tags"`
	CostMonthly    float64           `json:"cost_monthly"`
}

func (r KinesisResource) ResourceID() string    { return r.ID }
func (r KinesisResource) ResourceName() string  { return r.Name }
func (r KinesisResource) ResourceState() string { return NormalizeState(r.State) }
func (r KinesisResource) ServiceName() string   { return "kinesis" }

// kinesisStreamConcurrency はストリームごとの詳細取得 (DescribeStreamSummary) を同時実行する
// 上限数。無制限にすると Kinesis のリクエストレート上限に抵触しうるため上限を設ける
// (s3BucketConcurrency と同型)。
const kinesisStreamConcurrency = 30

// ListKinesisResources returns all Kinesis Data Streams for the given profile/region.
func ListKinesisResources(ctx context.Context, profile, region string) ([]KinesisResource, error) {
	// 各フェーズの所要時間を計測してログに残す (issue 0081: クライアント生成の区間は
	// issue 0083 の GetSession キャッシュ化調査の入力を兼ねる)。
	overallStart := time.Now()

	clientStart := time.Now()
	client, err := newKinesisClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	slog.Info("kinesis client created",
		"profile", profile, "region", region, "duration_ms", time.Since(clientStart).Milliseconds())

	listStart := time.Now()
	var names []string
	paginator := kinesis.NewListStreamsPaginator(client, &kinesis.ListStreamsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list kinesis streams: %w", err)
		}
		names = append(names, page.StreamNames...)
	}
	slog.Info("kinesis list streams done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(listStart).Milliseconds(), "count", len(names))

	// ストリームごとの詳細取得は互いに独立しているため並列実行する。各 goroutine は
	// 自分の index にのみ書き込むため結果スライスへの書き込みはロック不要で競合しない
	// (データオーナーシップを goroutine ごとに分離、ListS3Resources と同型)。
	detailStart := time.Now()
	resources := make([]KinesisResource, len(names))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(kinesisStreamConcurrency)
	for i, name := range names {
		g.Go(func() error {
			out, err := client.DescribeStreamSummary(gctx, &kinesis.DescribeStreamSummaryInput{
				StreamName: aws.String(name),
			})
			if err != nil {
				return fmt.Errorf("describe kinesis stream %s: %w", name, err)
			}
			resources[i] = kinesisFromSummary(out.StreamDescriptionSummary)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slog.Info("kinesis describe streams done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(detailStart).Milliseconds(),
		"concurrency", kinesisStreamConcurrency, "count", len(resources))

	slog.Info("kinesis list all done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(overallStart).Milliseconds(), "count", len(resources))
	return resources, nil
}

func kinesisFromSummary(s *kinesistypes.StreamDescriptionSummary) KinesisResource {
	if s == nil {
		return KinesisResource{}
	}
	return KinesisResource{
		ID:             ptrStr(s.StreamARN),
		Name:           ptrStr(s.StreamName),
		State:          DisplayState(string(s.StreamStatus)),
		ShardCount:     ptrInt32(s.OpenShardCount),
		RetentionHours: ptrInt32(s.RetentionPeriodHours),
		EncryptionType: string(s.EncryptionType),
	}
}

// newKinesisClient は Kinesis API クライアントを生成する。
func newKinesisClient(ctx context.Context, profile, region string) (*kinesis.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *kinesis.Client {
		return kinesis.NewFromConfig(cfg)
	})
}
