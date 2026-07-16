package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
)

// APIGatewayResource represents an API Gateway (REST/HTTP/WebSocket) API.
type APIGatewayResource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	State       string            `json:"state"`
	Type        string            `json:"type"`
	Stage       string            `json:"stage"`
	Endpoint    string            `json:"endpoint"`
	Tags        map[string]string `json:"tags"`
	CostMonthly float64           `json:"cost_monthly"`
}

func (r APIGatewayResource) ResourceID() string    { return r.ID }
func (r APIGatewayResource) ResourceName() string  { return r.Name }
func (r APIGatewayResource) ResourceState() string { return NormalizeState(r.State) }
func (r APIGatewayResource) ServiceName() string   { return "apigw" }

// ListAPIGatewayResources returns all API Gateway APIs (REST v1 + HTTP/WebSocket v2)
// for the given profile/region.
func ListAPIGatewayResources(ctx context.Context, profile, region string) ([]APIGatewayResource, error) {
	restClient, err := newAPIGatewayClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	v2Client, err := newAPIGatewayV2Client(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var resources []APIGatewayResource

	restPaginator := apigateway.NewGetRestApisPaginator(restClient, &apigateway.GetRestApisInput{})
	for restPaginator.HasMorePages() {
		page, err := restPaginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get rest apis: %w", err)
		}
		for _, api := range page.Items {
			resources = append(resources, apigwFromRestApi(api, region))
		}
	}

	// apigatewayv2 の GetApis はページネーター未提供のため NextToken で自前ループ
	var nextToken *string
	for {
		out, err := v2Client.GetApis(ctx, &apigatewayv2.GetApisInput{NextToken: nextToken})
		if err != nil {
			return nil, fmt.Errorf("get apis v2: %w", err)
		}
		for _, api := range out.Items {
			resources = append(resources, apigwFromV2Api(api))
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}

	return resources, nil
}

func apigwFromRestApi(a apigwtypes.RestApi, region string) APIGatewayResource {
	id := ptrStr(a.Id)
	endpoint := ""
	if id != "" && region != "" {
		endpoint = fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", id, region)
	}
	return APIGatewayResource{
		ID:       id,
		Name:     ptrStr(a.Name),
		State:    "active",
		Type:     "REST",
		Endpoint: endpoint,
		Tags:     a.Tags,
	}
}

func apigwFromV2Api(a apigwv2types.Api) APIGatewayResource {
	typ := string(a.ProtocolType)
	switch a.ProtocolType {
	case apigwv2types.ProtocolTypeHttp:
		typ = "HTTP"
	case apigwv2types.ProtocolTypeWebsocket:
		typ = "WebSocket"
	}
	return APIGatewayResource{
		ID:       ptrStr(a.ApiId),
		Name:     ptrStr(a.Name),
		State:    "active",
		Type:     typ,
		Endpoint: ptrStr(a.ApiEndpoint),
		Tags:     a.Tags,
	}
}

// newAPIGatewayClient は API Gateway (REST) API クライアントを生成する。
func newAPIGatewayClient(ctx context.Context, profile, region string) (*apigateway.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *apigateway.Client {
		return apigateway.NewFromConfig(cfg)
	})
}

// newAPIGatewayV2Client は API Gateway (HTTP/WebSocket) API クライアントを生成する。
func newAPIGatewayV2Client(ctx context.Context, profile, region string) (*apigatewayv2.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *apigatewayv2.Client {
		return apigatewayv2.NewFromConfig(cfg)
	})
}
