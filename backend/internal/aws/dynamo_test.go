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
