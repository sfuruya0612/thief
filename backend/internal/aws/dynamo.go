package aws

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// dynamoItemQueryLimit は Item 検索 (Query/Scan) の取得件数の既定値。
// リクエストで Limit が指定されなかった場合に使う。
const dynamoItemQueryLimit = 10

// dynamoItemQueryMaxLimit は Item 検索 (Query/Scan) の取得件数の上限。
// これを超える Limit 指定は上限に切り詰め、負荷とコストを最小化する。
const dynamoItemQueryMaxLimit = 100

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
	client, err := newDynamoClient(ctx, profile, region)
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
		State:     DisplayState(string(t.TableStatus)),
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

// DynamoKeyAttribute はテーブル/GSI のキー属性名と型 (S/N/B) を表す。
type DynamoKeyAttribute struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// DynamoIndexSchema はテーブル自身または GSI のキースキーマを表す。
type DynamoIndexSchema struct {
	Name         string              `json:"name"`
	PartitionKey DynamoKeyAttribute  `json:"partition_key"`
	SortKey      *DynamoKeyAttribute `json:"sort_key,omitempty"`
}

// DynamoTableSchema は Item 検索フォームを構築するためのキースキーマ情報。
type DynamoTableSchema struct {
	TableName string              `json:"table_name"`
	Table     DynamoIndexSchema   `json:"table"`
	GSIs      []DynamoIndexSchema `json:"gsis"`
}

// DescribeDynamoTable はテーブルのキースキーマ (PK/SK 名と型) と GSI 一覧を返す。
// UI 側が Key-Value 検索フォームを組み立てるために使う。
func DescribeDynamoTable(ctx context.Context, profile, region, table string) (DynamoTableSchema, error) {
	client, err := newDynamoClient(ctx, profile, region)
	if err != nil {
		return DynamoTableSchema{}, err
	}

	desc, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(table),
	})
	if err != nil {
		return DynamoTableSchema{}, fmt.Errorf("describe dynamodb table %s: %w", table, err)
	}
	if desc.Table == nil {
		return DynamoTableSchema{}, fmt.Errorf("describe dynamodb table %s: empty table description", table)
	}

	attrTypes := dynamoAttributeTypes(desc.Table.AttributeDefinitions)
	schema := DynamoTableSchema{
		TableName: table,
		Table:     dynamoIndexSchemaFromKeySchema(table, desc.Table.KeySchema, attrTypes),
	}
	for _, gsi := range desc.Table.GlobalSecondaryIndexes {
		schema.GSIs = append(schema.GSIs, dynamoIndexSchemaFromKeySchema(ptrStr(gsi.IndexName), gsi.KeySchema, attrTypes))
	}
	return schema, nil
}

func dynamoAttributeTypes(defs []dynamodbtypes.AttributeDefinition) map[string]string {
	m := make(map[string]string, len(defs))
	for _, d := range defs {
		m[ptrStr(d.AttributeName)] = string(d.AttributeType)
	}
	return m
}

func dynamoIndexSchemaFromKeySchema(name string, keySchema []dynamodbtypes.KeySchemaElement, attrTypes map[string]string) DynamoIndexSchema {
	idx := DynamoIndexSchema{Name: name}
	for _, k := range keySchema {
		attr := DynamoKeyAttribute{Name: ptrStr(k.AttributeName), Type: attrTypes[ptrStr(k.AttributeName)]}
		switch k.KeyType {
		case dynamodbtypes.KeyTypeHash:
			idx.PartitionKey = attr
		case dynamodbtypes.KeyTypeRange:
			idx.SortKey = &attr
		}
	}
	return idx
}

// DynamoItemQuery は Item 検索の Key-Value 指定を表す。PK/SK いずれも未指定ならプレビューとして扱う。
// AttrName/AttrValue は PK/SK 以外の任意属性による絞り込み (FilterExpression) を表し、
// PK/SK と併用できる。AttrName 単独 (PK 未指定) の場合は Scan + FilterExpression になる。
// Limit が 0 以下の場合は dynamoItemQueryLimit (10) を既定値として使う。
type DynamoItemQuery struct {
	PKValue   string
	SKValue   string
	AttrName  string
	AttrValue string
	Limit     int32
}

// resolveDynamoItemLimit はリクエストの Limit を実際に使う値に解決する。
// 0 以下は既定値 (dynamoItemQueryLimit) に、上限 (dynamoItemQueryMaxLimit) 超過は上限に切り詰める。
func resolveDynamoItemLimit(requested int32) int32 {
	if requested <= 0 {
		return dynamoItemQueryLimit
	}
	if requested > dynamoItemQueryMaxLimit {
		return dynamoItemQueryMaxLimit
	}
	return requested
}

// QueryDynamoItems はテーブルの Item を検索する。
//
// コスト/負荷最小化のため:
//   - PK 未指定の場合は Scan を Limit 件で 1 回実行する (AttrName/AttrValue 指定時は FilterExpression を付与)。
//   - PK 指定時は必ず Query (KeyConditionExpression) を使い、Scan は使わない (AttrName/AttrValue 指定時は
//     FilterExpression を併用する)。
//
// 件数はプレビュー・明示検索とも req.Limit (未指定時は dynamoItemQueryLimit) 固定とし、ページングは行わない。
// FilterExpression は Query/Scan が返した Limit 件に対して適用されるため、フィルタ後の件数が
// Limit より少なくなることがある (DynamoDB の仕様通り)。
func QueryDynamoItems(ctx context.Context, profile, region, table string, req DynamoItemQuery) ([]map[string]any, error) {
	client, err := newDynamoClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	limit := resolveDynamoItemLimit(req.Limit)

	filterExpr, filterNames, filterValues := dynamoAttrFilterExpression(req.AttrName, req.AttrValue)

	if req.PKValue == "" {
		// プレビュー/属性フィルタのみの検索: キー未指定の場合は Scan を許容する (Limit 件, 1 回限り)。
		input := &dynamodb.ScanInput{
			TableName: aws.String(table),
			Limit:     aws.Int32(limit),
		}
		if filterExpr != "" {
			input.FilterExpression = aws.String(filterExpr)
			input.ExpressionAttributeNames = filterNames
			input.ExpressionAttributeValues = filterValues
		}
		out, err := client.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("scan dynamodb table %s: %w", table, err)
		}
		return dynamoUnmarshalItems(out.Items)
	}

	schema, err := DescribeDynamoTable(ctx, profile, region, table)
	if err != nil {
		return nil, err
	}

	pkName := schema.Table.PartitionKey.Name
	if pkName == "" {
		return nil, fmt.Errorf("describe dynamodb table %s: partition key not found", table)
	}

	keyCondition := "#pk = :pk"
	names := map[string]string{"#pk": pkName}
	values := map[string]dynamodbtypes.AttributeValue{
		":pk": dynamoAttributeValueFromString(req.PKValue, schema.Table.PartitionKey.Type),
	}

	if req.SKValue != "" && schema.Table.SortKey != nil {
		keyCondition += " AND #sk = :sk"
		names["#sk"] = schema.Table.SortKey.Name
		values[":sk"] = dynamoAttributeValueFromString(req.SKValue, schema.Table.SortKey.Type)
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(table),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		Limit:                     aws.Int32(limit),
	}
	if filterExpr != "" {
		input.FilterExpression = aws.String(filterExpr)
		for k, v := range filterNames {
			names[k] = v
		}
		for k, v := range filterValues {
			values[k] = v
		}
	}

	out, err := client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("query dynamodb table %s: %w", table, err)
	}
	return dynamoUnmarshalItems(out.Items)
}

// dynamoAttrFilterExpression は任意属性名/値から FilterExpression を組み立てる。
// AttrName が空の場合は絞り込みなし (空文字列を返す)。
func dynamoAttrFilterExpression(attrName, attrValue string) (string, map[string]string, map[string]dynamodbtypes.AttributeValue) {
	if attrName == "" {
		return "", nil, nil
	}
	return "#filterAttr = :filterVal",
		map[string]string{"#filterAttr": attrName},
		map[string]dynamodbtypes.AttributeValue{":filterVal": dynamoAttributeValueFromInput(attrValue)}
}

// dynamoAttributeValueFromString は文字列の検索入力値を、キー属性の型 (S/N/B) に応じた
// AttributeValue に変換する。B (バイナリ) は検索フォームでの入力を想定しないため S として扱う。
func dynamoAttributeValueFromString(value, attrType string) dynamodbtypes.AttributeValue {
	if attrType == string(dynamodbtypes.ScalarAttributeTypeN) {
		return &dynamodbtypes.AttributeValueMemberN{Value: value}
	}
	return &dynamodbtypes.AttributeValueMemberS{Value: value}
}

// dynamoAttributeValueFromInput は型情報のない任意属性の検索入力値を AttributeValue に変換する。
// 数値としてパース可能な場合は N、それ以外は S として扱う。
func dynamoAttributeValueFromInput(value string) dynamodbtypes.AttributeValue {
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return &dynamodbtypes.AttributeValueMemberN{Value: value}
	}
	return &dynamodbtypes.AttributeValueMemberS{Value: value}
}

func dynamoUnmarshalItems(items []map[string]dynamodbtypes.AttributeValue) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(items))
	if err := attributevalue.UnmarshalListOfMaps(items, &result); err != nil {
		return nil, fmt.Errorf("unmarshal dynamodb items: %w", err)
	}
	return result, nil
}

// newDynamoClient は DynamoDB API クライアントを生成する。
func newDynamoClient(ctx context.Context, profile, region string) (*dynamodb.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *dynamodb.Client {
		return dynamodb.NewFromConfig(cfg)
	})
}
