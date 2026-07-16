package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesistypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
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

// ListKinesisResources returns all Kinesis Data Streams for the given profile/region.
func ListKinesisResources(ctx context.Context, profile, region string) ([]KinesisResource, error) {
	client, err := newKinesisClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var names []string
	paginator := kinesis.NewListStreamsPaginator(client, &kinesis.ListStreamsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list kinesis streams: %w", err)
		}
		names = append(names, page.StreamNames...)
	}

	var resources []KinesisResource
	for _, name := range names {
		out, err := client.DescribeStreamSummary(ctx, &kinesis.DescribeStreamSummaryInput{
			StreamName: aws.String(name),
		})
		if err != nil {
			return nil, fmt.Errorf("describe kinesis stream %s: %w", name, err)
		}
		resources = append(resources, kinesisFromSummary(out.StreamDescriptionSummary))
	}
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
