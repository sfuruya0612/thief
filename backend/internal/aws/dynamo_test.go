package aws

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// mockDynamoTableTagsClient は dynamoTableTagsClient の手書きモック。
type mockDynamoTableTagsClient struct {
	listTagsOfResource func(ctx context.Context, params *dynamodb.ListTagsOfResourceInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error)
}

func (m *mockDynamoTableTagsClient) ListTagsOfResource(ctx context.Context, params *dynamodb.ListTagsOfResourceInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error) {
	return m.listTagsOfResource(ctx, params, optFns...)
}

func TestDynamoFromDescription(t *testing.T) {
	arn := "arn:aws:dynamodb:ap-northeast-1:123:table/foo"
	tests := []struct {
		name string
		in   *dynamodbtypes.TableDescription
		want DynamoResource
	}{
		{
			name: "nil",
			in:   nil,
			want: DynamoResource{},
		},
		{
			name: "on-demand",
			in: &dynamodbtypes.TableDescription{
				TableArn:    aws.String(arn),
				TableName:   aws.String("foo"),
				TableStatus: dynamodbtypes.TableStatusActive,
				BillingModeSummary: &dynamodbtypes.BillingModeSummary{
					BillingMode: dynamodbtypes.BillingModePayPerRequest,
				},
				ItemCount:      aws.Int64(10),
				TableSizeBytes: aws.Int64(1024),
				GlobalSecondaryIndexes: []dynamodbtypes.GlobalSecondaryIndexDescription{
					{}, {},
				},
			},
			want: DynamoResource{
				ID:        arn,
				Name:      "foo",
				State:     "active",
				Mode:      "on-demand",
				ItemCount: 10,
				SizeBytes: 1024,
				GSICount:  2,
			},
		},
		{
			name: "provisioned via summary",
			in: &dynamodbtypes.TableDescription{
				TableName: aws.String("bar"),
				BillingModeSummary: &dynamodbtypes.BillingModeSummary{
					BillingMode: dynamodbtypes.BillingModeProvisioned,
				},
			},
			want: DynamoResource{Name: "bar", Mode: "provisioned"},
		},
		{
			name: "unknown billing mode falls back to provisioned",
			in: &dynamodbtypes.TableDescription{
				TableName: aws.String("qux"),
				BillingModeSummary: &dynamodbtypes.BillingModeSummary{
					BillingMode: dynamodbtypes.BillingMode("UNKNOWN"),
				},
			},
			want: DynamoResource{Name: "qux", Mode: "provisioned"},
		},
		{
			name: "provisioned default (no summary)",
			in:   &dynamodbtypes.TableDescription{TableName: aws.String("baz")},
			want: DynamoResource{Name: "baz", Mode: "provisioned"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dynamoFromDescription(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestDynamoTagsToMap(t *testing.T) {
	tags := []dynamodbtypes.Tag{
		{Key: aws.String("k1"), Value: aws.String("v1")},
		{Key: aws.String("k2"), Value: aws.String("v2")},
	}
	got := dynamoTagsToMap(tags)
	want := map[string]string{"k1": "v1", "k2": "v2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestFetchDynamoTableTags(t *testing.T) {
	arn := "arn:aws:dynamodb:ap-northeast-1:123:table/foo"

	t.Run("tableArn が nil の場合は API を呼ばない", func(t *testing.T) {
		client := &mockDynamoTableTagsClient{
			listTagsOfResource: func(ctx context.Context, params *dynamodb.ListTagsOfResourceInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error) {
				t.Fatal("ListTagsOfResource should not be called when tableArn is nil")
				return nil, nil
			},
		}

		tags, tagsFetchFailed, err := fetchDynamoTableTags(context.Background(), client, nil)
		if err != nil {
			t.Fatalf("fetchDynamoTableTags() error = %v", err)
		}
		if tagsFetchFailed {
			t.Errorf("tagsFetchFailed = true, want false")
		}
		if !reflect.DeepEqual(tags, map[string]string{}) {
			t.Errorf("tags = %v, want empty map", tags)
		}
	})

	t.Run("成功時はタグを返す", func(t *testing.T) {
		client := &mockDynamoTableTagsClient{
			listTagsOfResource: func(ctx context.Context, params *dynamodb.ListTagsOfResourceInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error) {
				return &dynamodb.ListTagsOfResourceOutput{
					Tags: []dynamodbtypes.Tag{
						{Key: aws.String("env"), Value: aws.String("prod")},
					},
				}, nil
			},
		}

		tags, tagsFetchFailed, err := fetchDynamoTableTags(context.Background(), client, aws.String(arn))
		if err != nil {
			t.Fatalf("fetchDynamoTableTags() error = %v", err)
		}
		if tagsFetchFailed {
			t.Errorf("tagsFetchFailed = true, want false")
		}
		if !reflect.DeepEqual(tags, map[string]string{"env": "prod"}) {
			t.Errorf("tags = %v, want {env: prod}", tags)
		}
	})

	t.Run("取得失敗で tagsFetchFailed が立つ", func(t *testing.T) {
		client := &mockDynamoTableTagsClient{
			listTagsOfResource: func(ctx context.Context, params *dynamodb.ListTagsOfResourceInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error) {
				return nil, errors.New("throttled")
			},
		}

		tags, tagsFetchFailed, err := fetchDynamoTableTags(context.Background(), client, aws.String(arn))
		if err != nil {
			t.Fatalf("fetchDynamoTableTags() error = %v", err)
		}
		if !tagsFetchFailed {
			t.Errorf("tagsFetchFailed = false, want true")
		}
		if !reflect.DeepEqual(tags, map[string]string{}) {
			t.Errorf("tags = %v, want empty map", tags)
		}
	})

	t.Run("キャンセルはエラーとして伝播する", func(t *testing.T) {
		client := &mockDynamoTableTagsClient{
			listTagsOfResource: func(ctx context.Context, params *dynamodb.ListTagsOfResourceInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error) {
				return nil, context.Canceled
			},
		}

		_, _, err := fetchDynamoTableTags(context.Background(), client, aws.String(arn))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("fetchDynamoTableTags() error = %v, want context.Canceled", err)
		}
	})
}

func TestDynamoAttributeTypes(t *testing.T) {
	defs := []dynamodbtypes.AttributeDefinition{
		{AttributeName: aws.String("pk"), AttributeType: dynamodbtypes.ScalarAttributeTypeS},
		{AttributeName: aws.String("sk"), AttributeType: dynamodbtypes.ScalarAttributeTypeN},
	}
	got := dynamoAttributeTypes(defs)
	want := map[string]string{"pk": "S", "sk": "N"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestDynamoIndexSchemaFromKeySchema(t *testing.T) {
	attrTypes := map[string]string{"pk": "S", "sk": "N"}
	tests := []struct {
		name      string
		idxName   string
		keySchema []dynamodbtypes.KeySchemaElement
		want      DynamoIndexSchema
	}{
		{
			name:    "partition key only",
			idxName: "table",
			keySchema: []dynamodbtypes.KeySchemaElement{
				{AttributeName: aws.String("pk"), KeyType: dynamodbtypes.KeyTypeHash},
			},
			want: DynamoIndexSchema{
				Name:         "table",
				PartitionKey: DynamoKeyAttribute{Name: "pk", Type: "S"},
			},
		},
		{
			name:    "partition and sort key",
			idxName: "gsi1",
			keySchema: []dynamodbtypes.KeySchemaElement{
				{AttributeName: aws.String("pk"), KeyType: dynamodbtypes.KeyTypeHash},
				{AttributeName: aws.String("sk"), KeyType: dynamodbtypes.KeyTypeRange},
			},
			want: DynamoIndexSchema{
				Name:         "gsi1",
				PartitionKey: DynamoKeyAttribute{Name: "pk", Type: "S"},
				SortKey:      &DynamoKeyAttribute{Name: "sk", Type: "N"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dynamoIndexSchemaFromKeySchema(tt.idxName, tt.keySchema, attrTypes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestDynamoAttributeValueFromString(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		attrType string
		want     dynamodbtypes.AttributeValue
	}{
		{
			name:     "string type",
			value:    "abc",
			attrType: "S",
			want:     &dynamodbtypes.AttributeValueMemberS{Value: "abc"},
		},
		{
			name:     "number type",
			value:    "123",
			attrType: "N",
			want:     &dynamodbtypes.AttributeValueMemberN{Value: "123"},
		},
		{
			name:     "unknown type defaults to string",
			value:    "abc",
			attrType: "B",
			want:     &dynamodbtypes.AttributeValueMemberS{Value: "abc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dynamoAttributeValueFromString(tt.value, tt.attrType)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestDynamoAttrFilterExpression(t *testing.T) {
	tests := []struct {
		name       string
		attrName   string
		attrValue  string
		wantExpr   string
		wantNames  map[string]string
		wantValues map[string]dynamodbtypes.AttributeValue
	}{
		{
			name:      "attrName が空なら絞り込みなし",
			attrName:  "",
			attrValue: "anything",
			wantExpr:  "",
		},
		{
			name:      "文字列値",
			attrName:  "status",
			attrValue: "active",
			wantExpr:  "#filterAttr = :filterVal",
			wantNames: map[string]string{"#filterAttr": "status"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":filterVal": &dynamodbtypes.AttributeValueMemberS{Value: "active"},
			},
		},
		{
			name:      "数値値",
			attrName:  "age",
			attrValue: "42",
			wantExpr:  "#filterAttr = :filterVal",
			wantNames: map[string]string{"#filterAttr": "age"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":filterVal": &dynamodbtypes.AttributeValueMemberN{Value: "42"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExpr, gotNames, gotValues := dynamoAttrFilterExpression(tt.attrName, tt.attrValue)
			if gotExpr != tt.wantExpr {
				t.Errorf("expr = %q want %q", gotExpr, tt.wantExpr)
			}
			if !reflect.DeepEqual(gotNames, tt.wantNames) {
				t.Errorf("names = %#v want %#v", gotNames, tt.wantNames)
			}
			if !reflect.DeepEqual(gotValues, tt.wantValues) {
				t.Errorf("values = %#v want %#v", gotValues, tt.wantValues)
			}
		})
	}
}

func TestDynamoAttributeValueFromInput(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  dynamodbtypes.AttributeValue
	}{
		{
			name:  "整数はN型になる",
			value: "42",
			want:  &dynamodbtypes.AttributeValueMemberN{Value: "42"},
		},
		{
			name:  "小数もN型になる",
			value: "3.14",
			want:  &dynamodbtypes.AttributeValueMemberN{Value: "3.14"},
		},
		{
			name:  "数値でない文字列はS型になる",
			value: "abc",
			want:  &dynamodbtypes.AttributeValueMemberS{Value: "abc"},
		},
		{
			name:  "空文字列はS型になる",
			value: "",
			want:  &dynamodbtypes.AttributeValueMemberS{Value: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dynamoAttributeValueFromInput(tt.value)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveDynamoItemLimit(t *testing.T) {
	tests := []struct {
		name      string
		requested int32
		want      int32
	}{
		{name: "0以下は既定値", requested: 0, want: dynamoItemQueryLimit},
		{name: "負数は既定値", requested: -1, want: dynamoItemQueryLimit},
		{name: "指定値をそのまま使う", requested: 50, want: 50},
		{name: "上限超過は上限に切り詰め", requested: 1000, want: dynamoItemQueryMaxLimit},
		{name: "上限と同値はそのまま", requested: dynamoItemQueryMaxLimit, want: dynamoItemQueryMaxLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDynamoItemLimit(tt.requested)
			if got != tt.want {
				t.Errorf("got %d want %d", got, tt.want)
			}
		})
	}
}

func TestDynamoUnmarshalItems(t *testing.T) {
	items := []map[string]dynamodbtypes.AttributeValue{
		{
			"pk":   &dynamodbtypes.AttributeValueMemberS{Value: "user#1"},
			"name": &dynamodbtypes.AttributeValueMemberS{Value: "alice"},
			"age":  &dynamodbtypes.AttributeValueMemberN{Value: "30"},
		},
	}
	got, err := dynamoUnmarshalItems(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	if got[0]["pk"] != "user#1" || got[0]["name"] != "alice" {
		t.Errorf("got %#v", got[0])
	}
}

func TestDynamoResourceJSONFetchFailedOmitempty(t *testing.T) {
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
			b, err := json.Marshal(DynamoResource{
				ID:              "table-1",
				Name:            "my-table",
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

// mockDynamoItemQueryClient は dynamoItemQueryClient の手書きモック。
// 受け取った ScanInput / QueryInput をポインタのまま呼び出し順に記録する。queryDynamoItems は
// Scan か Query のどちらかを 1 回だけ呼んで返り、ページングで Input を使い回さないため、
// 記録した後に内容が書き換わることはない。
// DescribeTable は問い合わせられたテーブル名を記録したうえで、pkName / skName から組み立てた
// キースキーマを返す。skName が空のテーブルはソートキーを持たない。
type mockDynamoItemQueryClient struct {
	pkName string
	pkType string
	skName string
	skType string

	scanInputs         []*dynamodb.ScanInput
	queryInputs        []*dynamodb.QueryInput
	describeTableNames []string
}

func (m *mockDynamoItemQueryClient) Scan(_ context.Context, params *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	m.scanInputs = append(m.scanInputs, params)
	return &dynamodb.ScanOutput{}, nil
}

func (m *mockDynamoItemQueryClient) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.queryInputs = append(m.queryInputs, params)
	return &dynamodb.QueryOutput{}, nil
}

func (m *mockDynamoItemQueryClient) DescribeTable(_ context.Context, params *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	m.describeTableNames = append(m.describeTableNames, ptrStr(params.TableName))
	desc := &dynamodbtypes.TableDescription{
		AttributeDefinitions: []dynamodbtypes.AttributeDefinition{
			{AttributeName: aws.String(m.pkName), AttributeType: dynamodbtypes.ScalarAttributeType(m.pkType)},
		},
		KeySchema: []dynamodbtypes.KeySchemaElement{
			{AttributeName: aws.String(m.pkName), KeyType: dynamodbtypes.KeyTypeHash},
		},
	}
	if m.skName != "" {
		desc.AttributeDefinitions = append(desc.AttributeDefinitions,
			dynamodbtypes.AttributeDefinition{AttributeName: aws.String(m.skName), AttributeType: dynamodbtypes.ScalarAttributeType(m.skType)})
		desc.KeySchema = append(desc.KeySchema,
			dynamodbtypes.KeySchemaElement{AttributeName: aws.String(m.skName), KeyType: dynamodbtypes.KeyTypeRange})
	}
	return &dynamodb.DescribeTableOutput{Table: desc}, nil
}

// assertDynamoFilterExpression は FilterExpression が期待どおりかを検査する。
// want が空文字のときは、フィルタ未指定として nil のままであることを求める。
func assertDynamoFilterExpression(t *testing.T, got *string, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Errorf("FilterExpression = %q, want nil", *got)
		}
		return
	}
	if got == nil {
		t.Errorf("FilterExpression = nil, want %q", want)
		return
	}
	if *got != want {
		t.Errorf("FilterExpression = %q, want %q", *got, want)
	}
}

// TestQueryDynamoItemsScanInput は PK 未指定の経路が ScanInput に載せる値を検証する。
// Limit を落とすと AWS 既定のページサイズで返るようになり、フィルタ式を落とすと絞り込みが
// 効かず全件が返るが、どちらもコンパイルと API 呼び出しは成功してしまう。
// Limit は経路と条件ごとに別の値を渡す。全ケースで同じ値にすると、引数を無視してその値を
// 固定で送る実装に変えてもテストが通ってしまう。既定値のケースだけは resolveDynamoItemLimit が
// 呼ばれていること (req.Limit をそのまま載せていないこと) の確認を兼ねる。
func TestQueryDynamoItemsScanInput(t *testing.T) {
	const table = "items"

	tests := []struct {
		name       string
		req        DynamoItemQuery
		wantLimit  int32
		wantFilter string
		wantNames  map[string]string
		wantValues map[string]dynamodbtypes.AttributeValue
	}{
		{
			name:      "フィルタ未指定",
			req:       DynamoItemQuery{Limit: 7},
			wantLimit: 7,
		},
		{
			name:      "Limit 未指定は既定値",
			req:       DynamoItemQuery{},
			wantLimit: dynamoItemQueryLimit,
		},
		{
			name:       "属性フィルタ指定",
			req:        DynamoItemQuery{AttrName: "status", AttrValue: "active", Limit: 23},
			wantLimit:  23,
			wantFilter: "#filterAttr = :filterVal",
			wantNames:  map[string]string{"#filterAttr": "status"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":filterVal": &dynamodbtypes.AttributeValueMemberS{Value: "active"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockDynamoItemQueryClient{pkName: "id", pkType: "S"}
			if _, err := queryDynamoItems(context.Background(), client, table, tt.req); err != nil {
				t.Fatalf("queryDynamoItems: %v", err)
			}
			if len(client.scanInputs) != 1 {
				t.Fatalf("Scan called %d times, want 1", len(client.scanInputs))
			}
			// PK 未指定なので Query 経路へ落ちてはならない。キースキーマも不要なので
			// DescribeTable も呼ばない。
			if len(client.queryInputs) != 0 {
				t.Fatalf("Query called %d times, want 0", len(client.queryInputs))
			}
			if len(client.describeTableNames) != 0 {
				t.Errorf("DescribeTable called for %v, want no call", client.describeTableNames)
			}
			in := client.scanInputs[0]
			if got := ptrStr(in.TableName); got != table {
				t.Errorf("TableName = %q, want %q", got, table)
			}
			if in.Limit == nil {
				t.Errorf("Limit = nil, want %d", tt.wantLimit)
			} else if *in.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", *in.Limit, tt.wantLimit)
			}
			assertDynamoFilterExpression(t, in.FilterExpression, tt.wantFilter)
			if !reflect.DeepEqual(in.ExpressionAttributeNames, tt.wantNames) {
				t.Errorf("ExpressionAttributeNames = %#v, want %#v", in.ExpressionAttributeNames, tt.wantNames)
			}
			if !reflect.DeepEqual(in.ExpressionAttributeValues, tt.wantValues) {
				t.Errorf("ExpressionAttributeValues = %#v, want %#v", in.ExpressionAttributeValues, tt.wantValues)
			}
		})
	}
}

// TestQueryDynamoItemsQueryInput は PK 指定の経路が QueryInput に載せる値を検証する。
// 名前と値のマップはキー条件由来とフィルタ由来をマージするため、マージ後の最終形を固定する。
// Limit の値を条件ごとに変える意図は TestQueryDynamoItemsScanInput と同じ。
func TestQueryDynamoItemsQueryInput(t *testing.T) {
	const table = "items"

	tests := []struct {
		name       string
		client     *mockDynamoItemQueryClient
		req        DynamoItemQuery
		wantLimit  int32
		wantKeyCob string
		wantFilter string
		wantNames  map[string]string
		wantValues map[string]dynamodbtypes.AttributeValue
	}{
		{
			name:       "PK のみ",
			client:     &mockDynamoItemQueryClient{pkName: "id", pkType: "S"},
			req:        DynamoItemQuery{PKValue: "pk-1", Limit: 31},
			wantLimit:  31,
			wantKeyCob: "#pk = :pk",
			wantNames:  map[string]string{"#pk": "id"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":pk": &dynamodbtypes.AttributeValueMemberS{Value: "pk-1"},
			},
		},
		{
			// PK と SK でキー属性の型を変える。両方 S にすると、SK の値の変換に
			// SortKey.Type ではなく PartitionKey.Type を渡す取り違えを検出できない。
			name:       "PK と SK",
			client:     &mockDynamoItemQueryClient{pkName: "id", pkType: "S", skName: "seq", skType: "N"},
			req:        DynamoItemQuery{PKValue: "pk-2", SKValue: "20260807", Limit: 42},
			wantLimit:  42,
			wantKeyCob: "#pk = :pk AND #sk = :sk",
			wantNames:  map[string]string{"#pk": "id", "#sk": "seq"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":pk": &dynamodbtypes.AttributeValueMemberS{Value: "pk-2"},
				":sk": &dynamodbtypes.AttributeValueMemberN{Value: "20260807"},
			},
		},
		{
			name:       "Limit 未指定は既定値",
			client:     &mockDynamoItemQueryClient{pkName: "id", pkType: "S"},
			req:        DynamoItemQuery{PKValue: "pk-5"},
			wantLimit:  dynamoItemQueryLimit,
			wantKeyCob: "#pk = :pk",
			wantNames:  map[string]string{"#pk": "id"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":pk": &dynamodbtypes.AttributeValueMemberS{Value: "pk-5"},
			},
		},
		{
			name:       "SK 指定だがテーブルに SK が無い",
			client:     &mockDynamoItemQueryClient{pkName: "id", pkType: "S"},
			req:        DynamoItemQuery{PKValue: "pk-3", SKValue: "2026-08-07", Limit: 5},
			wantLimit:  5,
			wantKeyCob: "#pk = :pk",
			wantNames:  map[string]string{"#pk": "id"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":pk": &dynamodbtypes.AttributeValueMemberS{Value: "pk-3"},
			},
		},
		{
			name:       "PK と属性フィルタ",
			client:     &mockDynamoItemQueryClient{pkName: "id", pkType: "S"},
			req:        DynamoItemQuery{PKValue: "pk-4", AttrName: "status", AttrValue: "active", Limit: 88},
			wantLimit:  88,
			wantKeyCob: "#pk = :pk",
			wantFilter: "#filterAttr = :filterVal",
			wantNames:  map[string]string{"#pk": "id", "#filterAttr": "status"},
			wantValues: map[string]dynamodbtypes.AttributeValue{
				":pk":        &dynamodbtypes.AttributeValueMemberS{Value: "pk-4"},
				":filterVal": &dynamodbtypes.AttributeValueMemberS{Value: "active"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := queryDynamoItems(context.Background(), tt.client, table, tt.req); err != nil {
				t.Fatalf("queryDynamoItems: %v", err)
			}
			if len(tt.client.queryInputs) != 1 {
				t.Fatalf("Query called %d times, want 1", len(tt.client.queryInputs))
			}
			// PK 指定時は Scan を使わない (コスト/負荷の前提)。
			if len(tt.client.scanInputs) != 0 {
				t.Fatalf("Scan called %d times, want 0", len(tt.client.scanInputs))
			}
			// キー名の解決は検索対象と同じテーブルに対して行う。
			if !reflect.DeepEqual(tt.client.describeTableNames, []string{table}) {
				t.Errorf("DescribeTable called for %v, want %v", tt.client.describeTableNames, []string{table})
			}
			in := tt.client.queryInputs[0]
			if got := ptrStr(in.TableName); got != table {
				t.Errorf("TableName = %q, want %q", got, table)
			}
			if got := ptrStr(in.KeyConditionExpression); got != tt.wantKeyCob {
				t.Errorf("KeyConditionExpression = %q, want %q", got, tt.wantKeyCob)
			}
			if in.Limit == nil {
				t.Errorf("Limit = nil, want %d", tt.wantLimit)
			} else if *in.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", *in.Limit, tt.wantLimit)
			}
			assertDynamoFilterExpression(t, in.FilterExpression, tt.wantFilter)
			if !reflect.DeepEqual(in.ExpressionAttributeNames, tt.wantNames) {
				t.Errorf("ExpressionAttributeNames = %#v, want %#v", in.ExpressionAttributeNames, tt.wantNames)
			}
			if !reflect.DeepEqual(in.ExpressionAttributeValues, tt.wantValues) {
				t.Errorf("ExpressionAttributeValues = %#v, want %#v", in.ExpressionAttributeValues, tt.wantValues)
			}
		})
	}
}
