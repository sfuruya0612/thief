package aws

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func TestCfnEventFromSDK(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name string
		in   cfntypes.StackEvent
		want CFNStackEvent
	}{
		{
			name: "failure event populated",
			in: cfntypes.StackEvent{
				Timestamp:            &ts,
				LogicalResourceId:    aws.String("MyResource"),
				ResourceType:         aws.String("AWS::S3::Bucket"),
				ResourceStatus:       cfntypes.ResourceStatusCreateFailed,
				ResourceStatusReason: aws.String("Bucket already exists"),
			},
			want: CFNStackEvent{
				Timestamp:            ts.Format(time.RFC3339),
				LogicalResourceID:    "MyResource",
				ResourceType:         "AWS::S3::Bucket",
				ResourceStatus:       "CREATE_FAILED",
				ResourceStatusReason: "Bucket already exists",
			},
		},
		{
			name: "no timestamp and no reason stays empty",
			in: cfntypes.StackEvent{
				LogicalResourceId: aws.String("Other"),
				ResourceType:      aws.String("AWS::IAM::Role"),
				ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
			},
			want: CFNStackEvent{
				LogicalResourceID: "Other",
				ResourceType:      "AWS::IAM::Role",
				ResourceStatus:    "CREATE_COMPLETE",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfnEventFromSDK(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestCfnResourceFromSDK(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name string
		in   cfntypes.StackResourceSummary
		want CFNStackResourceSummary
	}{
		{
			name: "populated resource",
			in: cfntypes.StackResourceSummary{
				LogicalResourceId:    aws.String("MyBucket"),
				PhysicalResourceId:   aws.String("my-bucket-abc123"),
				ResourceType:         aws.String("AWS::S3::Bucket"),
				ResourceStatus:       cfntypes.ResourceStatusUpdateComplete,
				LastUpdatedTimestamp: &ts,
			},
			want: CFNStackResourceSummary{
				LogicalResourceID:  "MyBucket",
				PhysicalResourceID: "my-bucket-abc123",
				ResourceType:       "AWS::S3::Bucket",
				ResourceStatus:     "UPDATE_COMPLETE",
				LastUpdatedTime:    ts.Format(time.RFC3339),
			},
		},
		{
			name: "no physical id (creation still in progress)",
			in: cfntypes.StackResourceSummary{
				LogicalResourceId:    aws.String("Pending"),
				ResourceType:         aws.String("AWS::Lambda::Function"),
				ResourceStatus:       cfntypes.ResourceStatusCreateInProgress,
				LastUpdatedTimestamp: &ts,
			},
			want: CFNStackResourceSummary{
				LogicalResourceID: "Pending",
				ResourceType:      "AWS::Lambda::Function",
				ResourceStatus:    "CREATE_IN_PROGRESS",
				LastUpdatedTime:   ts.Format(time.RFC3339),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfnResourceFromSDK(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestAppendCFNStackEventsPage(t *testing.T) {
	makeEvents := func(n int) []cfntypes.StackEvent {
		events := make([]cfntypes.StackEvent, n)
		for i := range events {
			events[i] = cfntypes.StackEvent{
				LogicalResourceId: aws.String("R"),
				ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
			}
		}
		return events
	}

	tests := []struct {
		name     string
		existing int
		page     int
		limit    int
		wantLen  int
	}{
		{name: "under limit appends all", existing: 0, page: 3, limit: 100, wantLen: 3},
		{name: "page exactly fills limit", existing: 90, page: 10, limit: 100, wantLen: 100},
		{name: "page exceeds limit truncates", existing: 95, page: 20, limit: 100, wantLen: 100},
		{name: "already at limit appends nothing", existing: 100, page: 5, limit: 100, wantLen: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := make([]CFNStackEvent, tt.existing)
			got := appendCFNStackEventsPage(existing, makeEvents(tt.page), tt.limit)
			if len(got) != tt.wantLen {
				t.Errorf("got len %d want %d", len(got), tt.wantLen)
			}
		})
	}
}
