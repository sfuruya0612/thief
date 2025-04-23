package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockRegisterClientOutput = &ssooidc.RegisterClientOutput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
	}

	mockStartDeviceAuthorizationOutput = &ssooidc.StartDeviceAuthorizationOutput{
		DeviceCode:              aws.String("device-code-abcdef1234567890"),
		UserCode:                aws.String("USER-CODE"),
		VerificationUri:         aws.String("https://device.sso.region.amazonaws.com/"),
		VerificationUriComplete: aws.String("https://device.sso.region.amazonaws.com/?user_code=USER-CODE"),
		ExpiresIn:               900,
		Interval:                5,
	}

	mockCreateTokenOutput = &ssooidc.CreateTokenOutput{
		AccessToken:  aws.String("access-token-abcdef1234567890"),
		RefreshToken: aws.String("refresh-token-abcdef1234567890"),
		IdToken:      aws.String("id-token-abcdef1234567890"),
		TokenType:    aws.String("Bearer"),
		ExpiresIn:    8 * 3600,
	}
)

type mockSsoOidcApi struct {
	mock.Mock
}

func (m *mockSsoOidcApi) RegisterClient(ctx context.Context, input *ssooidc.RegisterClientInput, opts ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ssooidc.RegisterClientOutput), args.Error(1)
}

func (m *mockSsoOidcApi) StartDeviceAuthorization(ctx context.Context, input *ssooidc.StartDeviceAuthorizationInput, opts ...func(*ssooidc.Options)) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ssooidc.StartDeviceAuthorizationOutput), args.Error(1)
}

func (m *mockSsoOidcApi) CreateToken(ctx context.Context, input *ssooidc.CreateTokenInput, opts ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*ssooidc.CreateTokenOutput), args.Error(1)
}

func TestGenerateRegisterClientInput(t *testing.T) {
	clientName := "thief-app"
	clientType := "public"
	opts := SSOOidcOpts{ClientName: clientName, ClientType: clientType}
	input := GenerateRegisterClientInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, clientName, *input.ClientName)
	assert.Equal(t, clientType, *input.ClientType)
}

func TestRegisterClient(t *testing.T) {
	mockApi := new(mockSsoOidcApi)
	mockApi.On("RegisterClient", mock.Anything, mock.Anything, mock.Anything).Return(mockRegisterClientOutput, nil)

	input := &ssooidc.RegisterClientInput{
		ClientName: aws.String("thief-app"),
		ClientType: aws.String("public"),
	}
	result, err := RegisterClient(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "client-abcdef1234567890", *result.ClientId)
	assert.Equal(t, "client-secret-abcdef1234567890", *result.ClientSecret)

	mockApi.AssertExpectations(t)
}

func TestRegisterClient_Error(t *testing.T) {
	mockApi := new(mockSsoOidcApi)
	mockApi.On("RegisterClient", mock.Anything, mock.Anything, mock.Anything).Return(mockRegisterClientOutput, errors.New("error"))

	input := &ssooidc.RegisterClientInput{
		ClientName: aws.String("thief-app"),
		ClientType: aws.String("public"),
	}
	result, err := RegisterClient(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateStartDeviceAuthorizationInput(t *testing.T) {
	clientId := "client-abcdef1234567890"
	clientSecret := "client-secret-abcdef1234567890"
	startUrl := "https://start.url.aws/start"
	opts := SSOOidcOpts{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		StartUrl:     startUrl,
	}
	input := GenerateStartDeviceAuthorizationInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, clientId, *input.ClientId)
	assert.Equal(t, clientSecret, *input.ClientSecret)
	assert.Equal(t, startUrl, *input.StartUrl)
}

func TestStartDeviceAuthorization(t *testing.T) {
	mockApi := new(mockSsoOidcApi)
	mockApi.On("StartDeviceAuthorization", mock.Anything, mock.Anything, mock.Anything).Return(mockStartDeviceAuthorizationOutput, nil)

	input := &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		StartUrl:     aws.String("https://start.url.aws/start"),
	}
	result, err := StartDeviceAuthorization(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "device-code-abcdef1234567890", *result.DeviceCode)
	assert.Equal(t, "USER-CODE", *result.UserCode)
	assert.Equal(t, "https://device.sso.region.amazonaws.com/", *result.VerificationUri)
	assert.Equal(t, int32(900), result.ExpiresIn)
	assert.Equal(t, int32(5), result.Interval)

	mockApi.AssertExpectations(t)
}

func TestStartDeviceAuthorization_Error(t *testing.T) {
	mockApi := new(mockSsoOidcApi)
	mockApi.On("StartDeviceAuthorization", mock.Anything, mock.Anything, mock.Anything).Return(mockStartDeviceAuthorizationOutput, errors.New("error"))

	input := &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		StartUrl:     aws.String("https://start.url.aws/start"),
	}
	result, err := StartDeviceAuthorization(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateCreateTokenInput(t *testing.T) {
	clientId := "client-abcdef1234567890"
	clientSecret := "client-secret-abcdef1234567890"
	deviceCode := "device-code-abcdef1234567890"
	grantType := "urn:ietf:params:oauth:grant-type:device_code"
	opts := SSOOidcOpts{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		DeviceCode:   deviceCode,
		GrantType:    grantType,
	}
	input := GenerateCreateTokenInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, clientId, *input.ClientId)
	assert.Equal(t, clientSecret, *input.ClientSecret)
	assert.Equal(t, deviceCode, *input.DeviceCode)
	assert.Equal(t, grantType, *input.GrantType)
}

func TestCreateToken(t *testing.T) {
	mockApi := new(mockSsoOidcApi)
	mockApi.On("CreateToken", mock.Anything, mock.Anything, mock.Anything).Return(mockCreateTokenOutput, nil)

	input := &ssooidc.CreateTokenInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		DeviceCode:   aws.String("device-code-abcdef1234567890"),
		GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
	}
	result, err := CreateToken(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "access-token-abcdef1234567890", *result.AccessToken)
	assert.Equal(t, "refresh-token-abcdef1234567890", *result.RefreshToken)
	assert.Equal(t, "id-token-abcdef1234567890", *result.IdToken)
	assert.Equal(t, "Bearer", *result.TokenType)
	assert.Equal(t, int32(8*3600), result.ExpiresIn)

	mockApi.AssertExpectations(t)
}

func TestCreateToken_Error(t *testing.T) {
	mockApi := new(mockSsoOidcApi)
	mockApi.On("CreateToken", mock.Anything, mock.Anything, mock.Anything).Return(mockCreateTokenOutput, errors.New("error"))

	input := &ssooidc.CreateTokenInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		DeviceCode:   aws.String("device-code-abcdef1234567890"),
		GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
	}
	result, err := CreateToken(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestWaitForToken_Success(t *testing.T) {
	mockApi := new(mockSsoOidcApi)
	mockApi.On("CreateToken", mock.Anything, mock.Anything, mock.Anything).Return(mockCreateTokenOutput, nil)

	input := &ssooidc.CreateTokenInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		DeviceCode:   aws.String("device-code-abcdef1234567890"),
		GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := WaitForToken(ctx, mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "access-token-abcdef1234567890", *result.AccessToken)

	mockApi.AssertExpectations(t)
}
