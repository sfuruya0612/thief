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
				Mode:           "provisioned",
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
			want: KinesisResource{Name: "bar", State: "creating", Mode: "provisioned"},
		},
		{
			name: "provisioned default (no stream mode details)",
			in: &kinesistypes.StreamDescriptionSummary{
				StreamName:   aws.String("baz"),
				StreamStatus: kinesistypes.StreamStatusActive,
			},
			want: KinesisResource{Name: "baz", State: "active", Mode: "provisioned"},
		},
		{
			name: "on-demand",
			in: &kinesistypes.StreamDescriptionSummary{
				StreamName:   aws.String("qux"),
				StreamStatus: kinesistypes.StreamStatusActive,
				StreamModeDetails: &kinesistypes.StreamModeDetails{
					StreamMode: kinesistypes.StreamModeOnDemand,
				},
			},
			want: KinesisResource{Name: "qux", State: "active", Mode: "on-demand"},
		},
		{
			name: "provisioned (explicit)",
			in: &kinesistypes.StreamDescriptionSummary{
				StreamName:   aws.String("quux"),
				StreamStatus: kinesistypes.StreamStatusActive,
				StreamModeDetails: &kinesistypes.StreamModeDetails{
					StreamMode: kinesistypes.StreamModeProvisioned,
				},
			},
			want: KinesisResource{Name: "quux", State: "active", Mode: "provisioned"},
		},
		{
			name: "unknown stream mode falls back to provisioned",
			in: &kinesistypes.StreamDescriptionSummary{
				StreamName:   aws.String("corge"),
				StreamStatus: kinesistypes.StreamStatusActive,
				StreamModeDetails: &kinesistypes.StreamModeDetails{
					StreamMode: kinesistypes.StreamMode("UNKNOWN"),
				},
			},
			want: KinesisResource{Name: "corge", State: "active", Mode: "provisioned"},
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

func TestKinesisResourceToRow(t *testing.T) {
	tests := []struct {
		name string
		in   KinesisResource
		want []string
	}{
		{
			name: "on-demand",
			in: KinesisResource{
				Name:           "foo",
				State:          "active",
				Mode:           "on-demand",
				ShardCount:     4,
				RetentionHours: 24,
				EncryptionType: "KMS",
			},
			want: []string{"foo", "active", "on-demand", "4", "24", "KMS"},
		},
		{
			name: "provisioned",
			in: KinesisResource{
				Name:           "bar",
				State:          "creating",
				Mode:           "provisioned",
				ShardCount:     1,
				RetentionHours: 168,
				EncryptionType: "NONE",
			},
			want: []string{"bar", "creating", "provisioned", "1", "168", "NONE"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.ToRow()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
