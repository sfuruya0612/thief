package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
)

// SSOOidcOpts contains configuration options for SSO OIDC operations.
// It holds client registration details, authorization parameters, and token request information.
type SSOOidcOpts struct {
	ClientName string
	ClientType string

	ClientId     string
	ClientSecret string
	StartUrl     string
	DeviceCode   string
	GrantType    string
}

// SlowDown is a custom error type that embeds AuthorizationPendingException.
// It's used to indicate that the authorization process is still pending but should slow down requests.
type SlowDown struct {
	types.AuthorizationPendingException
}

// ssoOidcApi defines the interface for SSO OIDC operations.
// This interface allows for mocking the AWS SDK calls in tests.
type ssoOidcApi interface {
	RegisterClient(ctx context.Context, params *ssooidc.RegisterClientInput, optFns ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error)
	StartDeviceAuthorization(ctx context.Context, params *ssooidc.StartDeviceAuthorizationInput, optFns ...func(*ssooidc.Options)) (*ssooidc.StartDeviceAuthorizationOutput, error)
	CreateToken(ctx context.Context, params *ssooidc.CreateTokenInput, optFns ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error)
}

// NewSSOOidcClient creates a new SSO OIDC client using the specified AWS profile and region.
func NewSSOOidcClient(profile, region string) (ssoOidcApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create SSO OIDC client: %w", err)
	}
	return ssooidc.NewFromConfig(cfg), nil
}

// GenerateRegisterClientInput creates the input for the RegisterClient API call.
// It uses the provided options to set the client name and type.
func GenerateRegisterClientInput(opts SSOOidcOpts) *ssooidc.RegisterClientInput {
	return &ssooidc.RegisterClientInput{
		ClientName: &opts.ClientName,
		ClientType: &opts.ClientType,
	}
}

// RegisterClient registers an OIDC client with AWS SSO service.
// It returns the registration details including client ID and secret.
func RegisterClient(api ssoOidcApi, input *ssooidc.RegisterClientInput) (*ssooidc.RegisterClientOutput, error) {
	o, err := api.RegisterClient(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return o, nil
}

// GenerateStartDeviceAuthorizationInput creates the input for the StartDeviceAuthorization API call.
// It uses the client ID, client secret and start URL from the provided options.
func GenerateStartDeviceAuthorizationInput(opts SSOOidcOpts) *ssooidc.StartDeviceAuthorizationInput {
	return &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     &opts.ClientId,
		ClientSecret: &opts.ClientSecret,
		StartUrl:     &opts.StartUrl,
	}
}

// StartDeviceAuthorization initiates device authorization flow with AWS SSO.
// It returns information needed for the user to complete authorization, including verification URL and user code.
func StartDeviceAuthorization(api ssoOidcApi, input *ssooidc.StartDeviceAuthorizationInput) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	o, err := api.StartDeviceAuthorization(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return o, nil
}

// GenerateCreateTokenInput creates the input for the CreateToken API call.
// It uses the client ID, client secret, device code and grant type from the provided options.
func GenerateCreateTokenInput(opts SSOOidcOpts) *ssooidc.CreateTokenInput {
	return &ssooidc.CreateTokenInput{
		ClientId:     &opts.ClientId,
		ClientSecret: &opts.ClientSecret,
		DeviceCode:   &opts.DeviceCode,
		GrantType:    &opts.GrantType,
	}
}

// CreateToken requests an access token from AWS SSO service.
// This function makes a single attempt to create a token and returns the result.
func CreateToken(api ssoOidcApi, input *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
	o, err := api.CreateToken(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return o, nil
}

// WaitForToken repeatedly attempts to create a token until success or timeout.
// It handles rate limiting by implementing exponential backoff when SlowDownException is encountered.
// The function will attempt up to 60 times with increasing intervals between attempts.
func WaitForToken(ctx context.Context, api ssoOidcApi, input *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
	maxAttempts := 60
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
