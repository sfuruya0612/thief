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
	DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// NewSsmClient creates a new SSM client using the specified AWS profile and region.
func NewSsmClient(profile, region string) (ssmApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create ssm client: %w", err)
	}
	return ssm.NewFromConfig(cfg), nil
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

// SSMParameterInfo holds metadata about an SSM parameter returned by DescribeParameters.
type SSMParameterInfo struct {
	Name             string
	Type             string
	LastModifiedDate string
	Version          int64
	DataType         string
}

// ToRow converts SSMParameterInfo to a string slice for table output.
func (p SSMParameterInfo) ToRow() []string {
	return []string{
		p.Name,
		p.Type,
		p.LastModifiedDate,
		fmt.Sprintf("%d", p.Version),
		p.DataType,
	}
}

// SSMParameterValue holds the value of an SSM parameter returned by GetParameter.
type SSMParameterValue struct {
	Name    string
	Type    string
	Value   string
	Version int64
	ARN     string
}

// ToRow converts SSMParameterValue to a string slice for table output.
func (p SSMParameterValue) ToRow() []string {
	return []string{
		p.Name,
		p.Type,
		p.Value,
		fmt.Sprintf("%d", p.Version),
		p.ARN,
	}
}

// GenerateDescribeParametersInput creates a DescribeParametersInput with an optional path prefix filter.
func GenerateDescribeParametersInput(path string) *ssm.DescribeParametersInput {
	input := &ssm.DescribeParametersInput{}

	if path != "" {
		input.ParameterFilters = []types.ParameterStringFilter{
			{
				Key:    aws.String("Name"),
				Option: aws.String("BeginsWith"),
				Values: []string{path},
			},
		}
	}

	return input
}

// DescribeParameters retrieves SSM parameter metadata with pagination support.
func DescribeParameters(client ssmApi, input *ssm.DescribeParametersInput) ([]SSMParameterInfo, error) {
	var params []SSMParameterInfo
	paginator := ssm.NewDescribeParametersPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, p := range output.Parameters {
			name := ""
			if p.Name != nil {
				name = *p.Name
			}
			lastModified := ""
			if p.LastModifiedDate != nil {
				lastModified = p.LastModifiedDate.Format("2006-01-02 15:04:05")
			}
			dataType := ""
			if p.DataType != nil {
				dataType = *p.DataType
			}

			params = append(params, SSMParameterInfo{
				Name:             name,
				Type:             string(p.Type),
				LastModifiedDate: lastModified,
				Version:          p.Version,
				DataType:         dataType,
			})
		}
	}

	return params, nil
}

// GetParameter retrieves the value of a single SSM parameter.
// Set withDecryption to true to decrypt SecureString parameters.
func GetParameter(client ssmApi, name string, withDecryption bool) (*SSMParameterValue, error) {
	input := &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(withDecryption),
	}

	output, err := client.GetParameter(context.Background(), input)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter %s: %w", name, err)
	}

	p := output.Parameter
	val := ""
	if p.Value != nil {
		val = *p.Value
	}
	arn := ""
	if p.ARN != nil {
		arn = *p.ARN
	}
	paramName := ""
	if p.Name != nil {
		paramName = *p.Name
	}

	return &SSMParameterValue{
		Name:    paramName,
		Type:    string(p.Type),
		Value:   val,
		Version: p.Version,
		ARN:     arn,
	}, nil
}
