package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
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
	CostMonthly       float64           `json:"cost_monthly"`
}

func (r SQSResource) ResourceID() string    { return r.ID }
func (r SQSResource) ResourceName() string  { return r.Name }
func (r SQSResource) ResourceState() string { return NormalizeState(r.State) }
func (r SQSResource) ServiceName() string   { return "sqs" }

// ListSQSResources returns all SQS queues for the given profile/region.
func ListSQSResources(ctx context.Context, profile, region string) ([]SQSResource, error) {
	client, err := newSQSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var urls []string
	paginator := sqs.NewListQueuesPaginator(client, &sqs.ListQueuesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sqs queues: %w", err)
		}
		urls = append(urls, page.QueueUrls...)
	}

	var resources []SQSResource
	for _, url := range urls {
		attrs, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
			QueueUrl:       aws.String(url),
			AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
		})
		if err != nil {
			return nil, fmt.Errorf("get queue attributes %s: %w", url, err)
		}
		tags := map[string]string{}
		tagsOut, tagErr := client.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(url)})
		if tagErr == nil && tagsOut != nil {
			tags = tagsOut.Tags
		}
		resources = append(resources, sqsFromAttributes(url, attrs.Attributes, tags))
	}
	return resources, nil
}

func sqsFromAttributes(url string, attrs map[string]string, tags map[string]string) SQSResource {
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
	}
}

// newSQSClient は SQS API クライアントを生成する。
func newSQSClient(ctx context.Context, profile, region string) (*sqs.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *sqs.Client {
		return sqs.NewFromConfig(cfg)
	})
}
