package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type SsmOpts struct {
	InstanceId   string
	PingStatus   string
	ResourceType string
	SessionId    string
}

type ssmApi interface {
	DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error)
	StartSession(ctx context.Context, params *ssm.StartSessionInput, optFns ...func(*ssm.Options)) (*ssm.StartSessionOutput, error)
	TerminateSession(ctx context.Context, params *ssm.TerminateSessionInput, optFns ...func(*ssm.Options)) (*ssm.TerminateSessionOutput, error)
}

func NewSsmClient(profile, region string) ssmApi {
	return ssm.NewFromConfig(GetSession(profile, region))
}

func GenerateDescribeInstanceInformationInput(opts *SsmOpts) *ssm.DescribeInstanceInformationInput {
	i := &ssm.DescribeInstanceInformationInput{}

	i.Filters = []types.InstanceInformationStringFilter{
		{
			Key:    aws.String("PingStatus"),
			Values: []string{opts.PingStatus},
		},
		{
			Key:    aws.String("ResourceType"),
			Values: []string{opts.ResourceType},
		}}

	return i
}

func DescribeInstanceInformation(client ssmApi, input *ssm.DescribeInstanceInformationInput) ([]string, error) {
	var ids []string
	paginator := ssm.NewDescribeInstanceInformationPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to get next page: %w", err)
		}

		for _, instance := range output.InstanceInformationList {
			if instance.InstanceId != nil {
				ids = append(ids, *instance.InstanceId)
			}
		}
	}

	return ids, nil
}

func GenerateStartSessionInput(opts *SsmOpts) *ssm.StartSessionInput {
	return &ssm.StartSessionInput{
		Target: aws.String(opts.InstanceId),
	}
}

func StartSession(client ssmApi, input *ssm.StartSessionInput) (*ssm.StartSessionOutput, error) {
	return client.StartSession(context.Background(), input)
}

func GenerateTerminateSessionInput(opts *SsmOpts) *ssm.TerminateSessionInput {
	return &ssm.TerminateSessionInput{
		SessionId: aws.String(opts.SessionId),
	}
}

func TerminateSession(client ssmApi, input *ssm.TerminateSessionInput) (*ssm.TerminateSessionOutput, error) {
	return client.TerminateSession(context.Background(), input)
}
