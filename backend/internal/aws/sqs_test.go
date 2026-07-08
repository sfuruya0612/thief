package aws

import (
	"reflect"
	"testing"
)

func TestSqsFromAttributes(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		attrs map[string]string
		tags  map[string]string
		want  SQSResource
	}{
		{
			name: "standard",
			url:  "https://sqs.ap-northeast-1.amazonaws.com/123/my-queue",
			attrs: map[string]string{
				"QueueArn":                              "arn:aws:sqs:ap-northeast-1:123:my-queue",
				"ApproximateNumberOfMessages":           "5",
				"ApproximateNumberOfMessagesNotVisible": "2",
				"MessageRetentionPeriod":                "345600",
			},
			tags: map[string]string{"env": "prod"},
			want: SQSResource{
				ID:                "arn:aws:sqs:ap-northeast-1:123:my-queue",
				Name:              "my-queue",
				State:             "active",
				Type:              "Standard",
				AvailableMessages: 5,
				InFlight:          2,
				RetentionDays:     4,
				Tags:              map[string]string{"env": "prod"},
			},
		},
		{
			name: "fifo",
			url:  "https://sqs.ap-northeast-1.amazonaws.com/123/my-queue.fifo",
			attrs: map[string]string{
				"FifoQueue":              "true",
				"MessageRetentionPeriod": "86400",
			},
			tags: map[string]string{},
			want: SQSResource{
				ID:            "https://sqs.ap-northeast-1.amazonaws.com/123/my-queue.fifo",
				Name:          "my-queue.fifo",
				State:         "active",
				Type:          "FIFO",
				RetentionDays: 1,
				Tags:          map[string]string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqsFromAttributes(tt.url, tt.attrs, tt.tags)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
