package aws

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	kinesistypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

func TestKinesisFromSummary(t *testing.T) {
	tests := []struct {
		name string
		in   *kinesistypes.StreamDescriptionSummary
		want KinesisResource
	}{
		{
			name: "nil",
			in:   nil,
			want: KinesisResource{},
		},
		{
			name: "active",
			in: &kinesistypes.StreamDescriptionSummary{
				StreamARN:            aws.String("arn:aws:kinesis:ap-northeast-1:123:stream/foo"),
				StreamName:           aws.String("foo"),
				StreamStatus:         kinesistypes.StreamStatusActive,
				OpenShardCount:       aws.Int32(4),
				RetentionPeriodHours: aws.Int32(24),
				EncryptionType:       kinesistypes.EncryptionTypeKms,
			},
			want: KinesisResource{
				ID:             "arn:aws:kinesis:ap-northeast-1:123:stream/foo",
				Name:           "foo",
				State:          "active",
				ShardCount:     4,
				RetentionHours: 24,
				EncryptionType: "KMS",
			},
		},
		{
			name: "creating (updating-like raw preserved)",
			in: &kinesistypes.StreamDescriptionSummary{
				StreamName:   aws.String("bar"),
				StreamStatus: kinesistypes.StreamStatusCreating,
			},
			want: KinesisResource{Name: "bar", State: "creating"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kinesisFromSummary(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
