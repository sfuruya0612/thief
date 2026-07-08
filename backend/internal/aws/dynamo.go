package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoResource represents a DynamoDB table.
type DynamoResource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	State       string            `json:"state"`
	Mode        string            `json:"mode"`
	ItemCount   int64             `json:"item_count"`
	SizeBytes   int64             `json:"size_bytes"`
	GSICount    int               `json:"gsi_count"`
	Tags        map[string]string `json:"tags"`
	CostMonthly float64           `json:"cost_monthly"`
}

func (r DynamoResource) ResourceID() string    { return r.ID }
func (r DynamoResource) ResourceName() string  { return r.Name }
func (r DynamoResource) ResourceState() string { return NormalizeState(r.State) }
func (r DynamoResource) ServiceName() string   { return "dynamo" }

// ListDynamoResources returns all DynamoDB tables for the given profile/region.
func ListDynamoResources(ctx context.Context, profile, region string) ([]DynamoResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *dynamodb.Client {
		return dynamodb.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var names []string
	paginator := dynamodb.NewListTablesPaginator(client, &dynamodb.ListTablesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list dynamodb tables: %w", err)
		}
		names = append(names, page.TableNames...)
	}

	var resources []DynamoResource
	for _, name := range names {
		desc, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(name),
		})
		if err != nil {
			return nil, fmt.Errorf("describe dynamodb table %s: %w", name, err)
		}
		// タグ取得は失敗してもテーブル情報は返す
		tags := map[string]string{}
		if desc.Table != nil && desc.Table.TableArn != nil {
			tagsOut, tagErr := client.ListTagsOfResource(ctx, &dynamodb.ListTagsOfResourceInput{
				ResourceArn: desc.Table.TableArn,
			})
			if tagErr == nil {
				tags = dynamoTagsToMap(tagsOut.Tags)
			}
		}
		r := dynamoFromDescription(desc.Table)
		r.Tags = tags
		resources = append(resources, r)
	}
	return resources, nil
}

func dynamoFromDescription(t *dynamodbtypes.TableDescription) DynamoResource {
	if t == nil {
		return DynamoResource{}
	}
	mode := ""
	if t.BillingModeSummary != nil {
		switch t.BillingModeSummary.BillingMode {
		case dynamodbtypes.BillingModePayPerRequest:
			mode = "on-demand"
		case dynamodbtypes.BillingModeProvisioned:
			mode = "provisioned"
		}
	} else {
		// BillingModeSummary が未設定の場合、既定はプロビジョンド
		mode = "provisioned"
	}
	return DynamoResource{
		ID:        ptrStr(t.TableArn),
		Name:      ptrStr(t.TableName),
		State:     string(t.TableStatus),
		Mode:      mode,
		ItemCount: ptrInt64(t.ItemCount),
		SizeBytes: ptrInt64(t.TableSizeBytes),
		GSICount:  len(t.GlobalSecondaryIndexes),
	}
}

func dynamoTagsToMap(tags []dynamodbtypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[ptrStr(t.Key)] = ptrStr(t.Value)
	}
	return m
}
