package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
)

type SSOOidcOpts struct {
	ClientName string
	ClientType string

	ClientId     string
	ClientSecret string
	StartUrl     string
	DeviceCode   string
	GrantType    string
}

type SlowDown struct {
	types.AuthorizationPendingException
}

type ssoOidcApi interface {
	RegisterClient(ctx context.Context, params *ssooidc.RegisterClientInput, optFns ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error)
	StartDeviceAuthorization(ctx context.Context, params *ssooidc.StartDeviceAuthorizationInput, optFns ...func(*ssooidc.Options)) (*ssooidc.StartDeviceAuthorizationOutput, error)
	CreateToken(ctx context.Context, params *ssooidc.CreateTokenInput, optFns ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error)
}

func NewSSOOidcClient(profile, region string) ssoOidcApi {
	return ssooidc.NewFromConfig(GetSession(profile, region))
}

func GenerateRegisterClientInput(opts SSOOidcOpts) *ssooidc.RegisterClientInput {
	return &ssooidc.RegisterClientInput{
		ClientName: &opts.ClientName,
		ClientType: &opts.ClientType,
	}
}

func RegisterClient(api ssoOidcApi, input *ssooidc.RegisterClientInput) (*ssooidc.RegisterClientOutput, error) {
	o, err := api.RegisterClient(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return o, nil
}

func GenerateStartDeviceAuthorizationInput(opts SSOOidcOpts) *ssooidc.StartDeviceAuthorizationInput {
	return &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     &opts.ClientId,
		ClientSecret: &opts.ClientSecret,
		StartUrl:     &opts.StartUrl,
	}
}

func StartDeviceAuthorization(api ssoOidcApi, input *ssooidc.StartDeviceAuthorizationInput) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	o, err := api.StartDeviceAuthorization(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return o, nil
}

func GenerateCreateTokenInput(opts SSOOidcOpts) *ssooidc.CreateTokenInput {
	return &ssooidc.CreateTokenInput{
		ClientId:     &opts.ClientId,
		ClientSecret: &opts.ClientSecret,
		DeviceCode:   &opts.DeviceCode,
		GrantType:    &opts.GrantType,
	}
}

func CreateToken(api ssoOidcApi, input *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
	o, err := api.CreateToken(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return o, nil
}

func WaitForToken(ctx context.Context, api ssoOidcApi, input *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
	maxAttempts := 10
	interval := 1 * time.Second

	for i := 0; i < maxAttempts; i++ {
		output, err := api.CreateToken(ctx, input)
		if err == nil {
			return output, nil
		}

		if errors.Is(err, &types.SlowDownException{}) && errors.Is(err, &types.AuthorizationPendingException{}) {
			return nil, fmt.Errorf("token creation failed: %v", err)
		}

		if errors.Is(err, &types.SlowDownException{}) {
			fmt.Println("Rate limited, waiting...")
			interval *= 2
		}

		time.Sleep(interval)
	}

	return nil, fmt.Errorf("timeout waiting for authentication")
}
