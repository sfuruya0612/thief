package aws

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestEcsFromCluster(t *testing.T) {
	tests := []struct {
		name string
		in   ecstypes.Cluster
		want ECSResource
	}{
		{
			name: "active uppercase normalized",
			in: ecstypes.Cluster{
				ClusterArn:                        aws.String("arn:aws:ecs:ap-northeast-1:123:cluster/prod"),
				ClusterName:                       aws.String("prod"),
				Status:                            aws.String("ACTIVE"),
				ActiveServicesCount:               3,
				RunningTasksCount:                 5,
				PendingTasksCount:                 1,
				RegisteredContainerInstancesCount: 2,
				Tags: []ecstypes.Tag{
					{Key: aws.String("env"), Value: aws.String("prod")},
				},
			},
			want: ECSResource{
				ID:             "arn:aws:ecs:ap-northeast-1:123:cluster/prod",
				Name:           "prod",
				State:          "active",
				ActiveServices: 3,
				RunningTasks:   5,
				PendingTasks:   1,
				RegisteredEC2:  2,
				Tags:           map[string]string{"env": "prod"},
			},
		},
		{
			name: "empty status stays empty",
			in:   ecstypes.Cluster{ClusterName: aws.String("empty")},
			want: ECSResource{Name: "empty", Tags: map[string]string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecsFromCluster(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
