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
				State:     "ACTIVE",
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
