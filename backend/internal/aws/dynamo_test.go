package aws

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

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
