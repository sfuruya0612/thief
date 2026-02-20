package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
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
	registerClientOutput           *ssooidc.RegisterClientOutput
	registerClientErr              error
	startDeviceAuthorizationOutput *ssooidc.StartDeviceAuthorizationOutput
	startDeviceAuthorizationErr    error
	createTokenOutput              *ssooidc.CreateTokenOutput
	createTokenErr                 error
}

func (m *mockSsoOidcApi) RegisterClient(ctx context.Context, input *ssooidc.RegisterClientInput, opts ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error) {
	return m.registerClientOutput, m.registerClientErr
}

func (m *mockSsoOidcApi) StartDeviceAuthorization(ctx context.Context, input *ssooidc.StartDeviceAuthorizationInput, opts ...func(*ssooidc.Options)) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	return m.startDeviceAuthorizationOutput, m.startDeviceAuthorizationErr
}

func (m *mockSsoOidcApi) CreateToken(ctx context.Context, input *ssooidc.CreateTokenInput, opts ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	return m.createTokenOutput, m.createTokenErr
}

func TestGenerateRegisterClientInput(t *testing.T) {
	clientName := "thief-app"
	clientType := "public"
	opts := SSOOidcOpts{ClientName: clientName, ClientType: clientType}
	input := GenerateRegisterClientInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if *input.ClientName != clientName {
		t.Errorf("expected ClientName %q, got %q", clientName, *input.ClientName)
	}
	if *input.ClientType != clientType {
		t.Errorf("expected ClientType %q, got %q", clientType, *input.ClientType)
	}
}

func TestRegisterClient(t *testing.T) {
	mockApi := &mockSsoOidcApi{
		registerClientOutput: mockRegisterClientOutput,
		registerClientErr:    nil,
	}

	input := &ssooidc.RegisterClientInput{
		ClientName: aws.String("thief-app"),
		ClientType: aws.String("public"),
	}
	result, err := RegisterClient(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result.ClientId != "client-abcdef1234567890" {
		t.Errorf("expected ClientId 'client-abcdef1234567890', got '%s'", *result.ClientId)
	}
	if *result.ClientSecret != "client-secret-abcdef1234567890" {
		t.Errorf("expected ClientSecret 'client-secret-abcdef1234567890', got '%s'", *result.ClientSecret)
	}
}

func TestRegisterClient_Error(t *testing.T) {
	mockApi := &mockSsoOidcApi{
		registerClientOutput: mockRegisterClientOutput,
		registerClientErr:    errors.New("error"),
	}

	input := &ssooidc.RegisterClientInput{
		ClientName: aws.String("thief-app"),
		ClientType: aws.String("public"),
	}
	result, err := RegisterClient(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
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
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if *input.ClientId != clientId {
		t.Errorf("expected ClientId %q, got %q", clientId, *input.ClientId)
	}
	if *input.ClientSecret != clientSecret {
		t.Errorf("expected ClientSecret %q, got %q", clientSecret, *input.ClientSecret)
	}
	if *input.StartUrl != startUrl {
		t.Errorf("expected StartUrl %q, got %q", startUrl, *input.StartUrl)
	}
}

func TestStartDeviceAuthorization(t *testing.T) {
	mockApi := &mockSsoOidcApi{
		startDeviceAuthorizationOutput: mockStartDeviceAuthorizationOutput,
		startDeviceAuthorizationErr:    nil,
	}

	input := &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		StartUrl:     aws.String("https://start.url.aws/start"),
	}
	result, err := StartDeviceAuthorization(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result.DeviceCode != "device-code-abcdef1234567890" {
		t.Errorf("expected DeviceCode 'device-code-abcdef1234567890', got '%s'", *result.DeviceCode)
	}
	if *result.UserCode != "USER-CODE" {
		t.Errorf("expected UserCode 'USER-CODE', got '%s'", *result.UserCode)
	}
	if *result.VerificationUri != "https://device.sso.region.amazonaws.com/" {
		t.Errorf("expected VerificationUri 'https://device.sso.region.amazonaws.com/', got '%s'", *result.VerificationUri)
	}
	if result.ExpiresIn != int32(900) {
		t.Errorf("expected ExpiresIn 900, got %d", result.ExpiresIn)
	}
	if result.Interval != int32(5) {
		t.Errorf("expected Interval 5, got %d", result.Interval)
	}
}

func TestStartDeviceAuthorization_Error(t *testing.T) {
	mockApi := &mockSsoOidcApi{
		startDeviceAuthorizationOutput: mockStartDeviceAuthorizationOutput,
		startDeviceAuthorizationErr:    errors.New("error"),
	}

	input := &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		StartUrl:     aws.String("https://start.url.aws/start"),
	}
	result, err := StartDeviceAuthorization(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
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
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if *input.ClientId != clientId {
		t.Errorf("expected ClientId %q, got %q", clientId, *input.ClientId)
	}
	if *input.ClientSecret != clientSecret {
		t.Errorf("expected ClientSecret %q, got %q", clientSecret, *input.ClientSecret)
	}
	if *input.DeviceCode != deviceCode {
		t.Errorf("expected DeviceCode %q, got %q", deviceCode, *input.DeviceCode)
	}
	if *input.GrantType != grantType {
		t.Errorf("expected GrantType %q, got %q", grantType, *input.GrantType)
	}
}

func TestCreateToken(t *testing.T) {
	mockApi := &mockSsoOidcApi{
		createTokenOutput: mockCreateTokenOutput,
		createTokenErr:    nil,
	}

	input := &ssooidc.CreateTokenInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		DeviceCode:   aws.String("device-code-abcdef1234567890"),
		GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
	}
	result, err := CreateToken(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result.AccessToken != "access-token-abcdef1234567890" {
		t.Errorf("expected AccessToken 'access-token-abcdef1234567890', got '%s'", *result.AccessToken)
	}
	if *result.RefreshToken != "refresh-token-abcdef1234567890" {
		t.Errorf("expected RefreshToken 'refresh-token-abcdef1234567890', got '%s'", *result.RefreshToken)
	}
	if *result.IdToken != "id-token-abcdef1234567890" {
		t.Errorf("expected IdToken 'id-token-abcdef1234567890', got '%s'", *result.IdToken)
	}
	if *result.TokenType != "Bearer" {
		t.Errorf("expected TokenType 'Bearer', got '%s'", *result.TokenType)
	}
	if result.ExpiresIn != int32(8*3600) {
		t.Errorf("expected ExpiresIn %d, got %d", int32(8*3600), result.ExpiresIn)
	}
}

func TestCreateToken_Error(t *testing.T) {
	mockApi := &mockSsoOidcApi{
		createTokenOutput: mockCreateTokenOutput,
		createTokenErr:    errors.New("error"),
	}

	input := &ssooidc.CreateTokenInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		DeviceCode:   aws.String("device-code-abcdef1234567890"),
		GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
	}
	result, err := CreateToken(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestWaitForToken_Success(t *testing.T) {
	mockApi := &mockSsoOidcApi{
		createTokenOutput: mockCreateTokenOutput,
		createTokenErr:    nil,
	}

	input := &ssooidc.CreateTokenInput{
		ClientId:     aws.String("client-abcdef1234567890"),
		ClientSecret: aws.String("client-secret-abcdef1234567890"),
		DeviceCode:   aws.String("device-code-abcdef1234567890"),
		GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := WaitForToken(ctx, mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result.AccessToken != "access-token-abcdef1234567890" {
		t.Errorf("expected AccessToken 'access-token-abcdef1234567890', got '%s'", *result.AccessToken)
	}
}
