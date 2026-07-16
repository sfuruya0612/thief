package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

// LambdaResource represents a single Lambda function.
type LambdaResource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	State       string            `json:"state"`
	Runtime     string            `json:"runtime"`
	MemoryMB    int32             `json:"memory_mb"`
	TimeoutSec  int32             `json:"timeout_sec"`
	Handler     string            `json:"handler"`
	Role        string            `json:"role"`
	Tags        map[string]string `json:"tags"`
	CostMonthly float64           `json:"cost_monthly"`
}

func (r LambdaResource) ResourceID() string    { return r.ID }
func (r LambdaResource) ResourceName() string  { return r.Name }
func (r LambdaResource) ResourceState() string { return NormalizeState(r.State) }
func (r LambdaResource) ServiceName() string   { return "lambda" }

// ListLambdaResources returns all Lambda functions for the given profile/region.
func ListLambdaResources(ctx context.Context, profile, region string) ([]LambdaResource, error) {
	client, err := newLambdaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var resources []LambdaResource
	paginator := lambda.NewListFunctionsPaginator(client, &lambda.ListFunctionsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list lambda functions: %w", err)
		}
		for _, fn := range page.Functions {
			resources = append(resources, lambdaFromFunction(fn))
		}
	}
	return resources, nil
}

func lambdaFromFunction(fn lambdatypes.FunctionConfiguration) LambdaResource {
	// State 空 (古いランタイム等で未取得) は active 扱い
	state := "active"
	switch fn.State {
	case lambdatypes.StatePending:
		state = "pending"
	case lambdatypes.StateInactive:
		state = "inactive"
	case lambdatypes.StateFailed:
		state = "failed"
	case lambdatypes.StateActive:
		state = "active"
	}
	return LambdaResource{
		ID:         ptrStr(fn.FunctionArn),
		Name:       ptrStr(fn.FunctionName),
		State:      state,
		Runtime:    string(fn.Runtime),
		MemoryMB:   ptrInt32(fn.MemorySize),
		TimeoutSec: ptrInt32(fn.Timeout),
		Handler:    ptrStr(fn.Handler),
		Role:       ptrStr(fn.Role),
	}
}

// newLambdaClient は Lambda API クライアントを生成する。
func newLambdaClient(ctx context.Context, profile, region string) (*lambda.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *lambda.Client {
		return lambda.NewFromConfig(cfg)
	})
}
