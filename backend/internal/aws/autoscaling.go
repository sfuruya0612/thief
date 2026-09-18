package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
)

// newAutoScalingClient は Auto Scaling API クライアントを生成する。
func newAutoScalingClient(ctx context.Context, profile, region string) (*autoscaling.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *autoscaling.Client {
		return autoscaling.NewFromConfig(cfg)
	})
}

// ListAutoScalingGroupNames はリージョン内の Auto Scaling グループ名を返す。
func ListAutoScalingGroupNames(ctx context.Context, profile, region string) ([]string, error) {
	client, err := newAutoScalingClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listAutoScalingGroupNamesWith(ctx, client)
}

// listAutoScalingGroupNamesWith は生成済みクライアントでグループ名を列挙するコア。
// クライアントの生成と分離し、単体テストで固定できるようにしてある。
func listAutoScalingGroupNamesWith(ctx context.Context, client autoscaling.DescribeAutoScalingGroupsAPIClient) ([]string, error) {
	names := []string{}
	paginator := autoscaling.NewDescribeAutoScalingGroupsPaginator(client, &autoscaling.DescribeAutoScalingGroupsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe auto scaling groups: %w", err)
		}
		for _, group := range page.AutoScalingGroups {
			if name := ptrStr(group.AutoScalingGroupName); name != "" {
				names = append(names, name)
			}
		}
	}
	return names, nil
}
