package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
)

var (
	mockListAccountsOutput = &sso.ListAccountsOutput{
		AccountList: []types.AccountInfo{
			{
				AccountId:    aws.String("123456789012"),
				AccountName:  aws.String("MyTestAccount"),
				EmailAddress: aws.String("test@example.com"),
			},
		},
		NextToken: nil,
	}

	mockListAccountRolesOutput = &sso.ListAccountRolesOutput{
		RoleList: []types.RoleInfo{
			{
				AccountId: aws.String("123456789012"),
				RoleName:  aws.String("AdminRole"),
			},
		},
	}
)

type mockSsoApi struct {
	listAccountsOutput     *sso.ListAccountsOutput
	listAccountsErr        error
	listAccountRolesOutput *sso.ListAccountRolesOutput
	listAccountRolesErr    error
}

func (m *mockSsoApi) ListAccounts(ctx context.Context, input *sso.ListAccountsInput, opts ...func(*sso.Options)) (*sso.ListAccountsOutput, error) {
	return m.listAccountsOutput, m.listAccountsErr
}

func (m *mockSsoApi) ListAccountRoles(ctx context.Context, input *sso.ListAccountRolesInput, opts ...func(*sso.Options)) (*sso.ListAccountRolesOutput, error) {
	return m.listAccountRolesOutput, m.listAccountRolesErr
}

func TestGenerateListAccountsInput(t *testing.T) {
	accessToken := "test-access-token"
	opts := SSOOpts{AccessToken: accessToken}
	input := GenerateListAccountsInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if *input.AccessToken != accessToken {
		t.Errorf("expected AccessToken %q, got %q", accessToken, *input.AccessToken)
	}
}

func TestListAccounts(t *testing.T) {
	mockApi := &mockSsoApi{
		listAccountsOutput: mockListAccountsOutput,
		listAccountsErr:    nil,
	}

	input := &sso.ListAccountsInput{
		AccessToken: aws.String("test-access-token"),
	}
	result, err := ListAccounts(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.AccountList) != 1 {
		t.Fatalf("expected 1 account, got %d", len(result.AccountList))
	}
	if *result.AccountList[0].AccountId != "123456789012" {
		t.Errorf("expected AccountId '123456789012', got '%s'", *result.AccountList[0].AccountId)
	}
	if *result.AccountList[0].AccountName != "MyTestAccount" {
		t.Errorf("expected AccountName 'MyTestAccount', got '%s'", *result.AccountList[0].AccountName)
	}
	if *result.AccountList[0].EmailAddress != "test@example.com" {
		t.Errorf("expected EmailAddress 'test@example.com', got '%s'", *result.AccountList[0].EmailAddress)
	}
}

func TestListAccounts_Error(t *testing.T) {
	mockApi := &mockSsoApi{
		listAccountsOutput: mockListAccountsOutput,
		listAccountsErr:    errors.New("error"),
	}

	input := &sso.ListAccountsInput{
		AccessToken: aws.String("test-access-token"),
	}
	result, err := ListAccounts(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestGenerateListAccountRolesInput(t *testing.T) {
	accessToken := "test-access-token"
	accountId := "123456789012"
	opts := SSOOpts{AccessToken: accessToken, AccountId: accountId}
	input := GenerateListAccountRolesInput(opts)
	if input == nil {
		t.Fatal("expected non-nil input, got nil")
	}
	if *input.AccessToken != accessToken {
		t.Errorf("expected AccessToken %q, got %q", accessToken, *input.AccessToken)
	}
	if *input.AccountId != accountId {
		t.Errorf("expected AccountId %q, got %q", accountId, *input.AccountId)
	}
}

func TestListAccountRoles(t *testing.T) {
	mockApi := &mockSsoApi{
		listAccountRolesOutput: mockListAccountRolesOutput,
		listAccountRolesErr:    nil,
	}

	input := &sso.ListAccountRolesInput{
		AccessToken: aws.String("test-access-token"),
		AccountId:   aws.String("123456789012"),
	}
	result, err := ListAccountRoles(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.RoleList) != 1 {
		t.Fatalf("expected 1 role, got %d", len(result.RoleList))
	}
	if *result.RoleList[0].AccountId != "123456789012" {
		t.Errorf("expected AccountId '123456789012', got '%s'", *result.RoleList[0].AccountId)
	}
	if *result.RoleList[0].RoleName != "AdminRole" {
		t.Errorf("expected RoleName 'AdminRole', got '%s'", *result.RoleList[0].RoleName)
	}
}

func TestListAccountRoles_Error(t *testing.T) {
	mockApi := &mockSsoApi{
		listAccountRolesOutput: mockListAccountRolesOutput,
		listAccountRolesErr:    errors.New("error"),
	}

	input := &sso.ListAccountRolesInput{
		AccessToken: aws.String("test-access-token"),
		AccountId:   aws.String("123456789012"),
	}
	result, err := ListAccountRoles(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestNewSSOClient_NewMethod(t *testing.T) {
	t.Skip("Skipping NewSSOClient test as it requires AWS credentials")

	client, err := NewSSOClient("default", "us-west-2")
	if err == nil && client == nil {
		t.Error("expected non-nil client")
	}
}

func TestSSOOpts_Structure(t *testing.T) {
	opts := SSOOpts{
		AccessToken: "test-token-123",
		AccountId:   "987654321098",
	}

	if opts.AccessToken != "test-token-123" {
		t.Errorf("expected AccessToken 'test-token-123', got '%s'", opts.AccessToken)
	}
	if opts.AccountId != "987654321098" {
		t.Errorf("expected AccountId '987654321098', got '%s'", opts.AccountId)
	}
}
