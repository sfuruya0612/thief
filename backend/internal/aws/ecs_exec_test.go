package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/google/go-cmp/cmp"
)

func TestECSServiceFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   ecstypes.Service
		want ECSServiceResource
	}{
		{
			name: "active uppercase to lowercase",
			in: ecstypes.Service{
				ServiceArn:     aws.String("arn:aws:ecs:svc/a"),
				ServiceName:    aws.String("svc-a"),
				Status:         aws.String("ACTIVE"),
				DesiredCount:   3,
				RunningCount:   3,
				PendingCount:   0,
				TaskDefinition: aws.String("td:1"),
				LaunchType:     ecstypes.LaunchTypeFargate,
			},
			want: ECSServiceResource{
				ARN:            "arn:aws:ecs:svc/a",
				Name:           "svc-a",
				Status:         "active",
				DesiredCount:   3,
				RunningCount:   3,
				PendingCount:   0,
				TaskDefinition: "td:1",
				LaunchType:     "FARGATE",
			},
		},
		{
			name: "draining",
			in: ecstypes.Service{
				ServiceArn: aws.String("arn:aws:ecs:svc/b"),
				Status:     aws.String("DRAINING"),
			},
			want: ECSServiceResource{
				ARN:    "arn:aws:ecs:svc/b",
				Status: "draining",
			},
		},
		{
			name: "empty status",
			in:   ecstypes.Service{},
			want: ECSServiceResource{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecsServiceFromSDK(tt.in)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestECSTaskFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   ecstypes.Task
		want ECSTaskResource
	}{
		{
			name: "running uppercase with containers",
			in: ecstypes.Task{
				TaskArn:              aws.String("arn:task/a"),
				Group:                aws.String("service:svc-a"),
				LastStatus:           aws.String("RUNNING"),
				DesiredStatus:        aws.String("RUNNING"),
				LaunchType:           ecstypes.LaunchTypeFargate,
				EnableExecuteCommand: true,
				Containers: []ecstypes.Container{
					{Name: aws.String("app")},
					{Name: aws.String("sidecar")},
				},
			},
			want: ECSTaskResource{
				ARN:                  "arn:task/a",
				Group:                "service:svc-a",
				LastStatus:           "running",
				DesiredStatus:        "running",
				LaunchType:           "FARGATE",
				EnableExecuteCommand: true,
				ContainerNames:       []string{"app", "sidecar"},
			},
		},
		{
			name: "stopped",
			in: ecstypes.Task{
				TaskArn:       aws.String("arn:task/b"),
				LastStatus:    aws.String("STOPPED"),
				DesiredStatus: aws.String("STOPPED"),
			},
			want: ECSTaskResource{
				ARN:            "arn:task/b",
				LastStatus:     "stopped",
				DesiredStatus:  "stopped",
				ContainerNames: []string{},
			},
		},
		{
			name: "empty statuses",
			in:   ecstypes.Task{},
			want: ECSTaskResource{ContainerNames: []string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecsTaskFromSDK(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
