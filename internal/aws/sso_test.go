package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	mock.Mock
}

func (m *mockSsoApi) ListAccounts(ctx context.Context, input *sso.ListAccountsInput, opts ...func(*sso.Options)) (*sso.ListAccountsOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*sso.ListAccountsOutput), args.Error(1)
}

func (m *mockSsoApi) ListAccountRoles(ctx context.Context, input *sso.ListAccountRolesInput, opts ...func(*sso.Options)) (*sso.ListAccountRolesOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*sso.ListAccountRolesOutput), args.Error(1)
}

func TestGenerateListAccountsInput(t *testing.T) {
	accessToken := "test-access-token"
	opts := SSOOpts{AccessToken: accessToken}
	input := GenerateListAccountsInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, accessToken, *input.AccessToken)
}

func TestListAccounts(t *testing.T) {
	mockApi := new(mockSsoApi)
	mockApi.On("ListAccounts", mock.Anything, mock.Anything, mock.Anything).Return(mockListAccountsOutput, nil)

	input := &sso.ListAccountsInput{
		AccessToken: aws.String("test-access-token"),
	}
	result, err := ListAccounts(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.AccountList))
	assert.Equal(t, "123456789012", *result.AccountList[0].AccountId)
	assert.Equal(t, "MyTestAccount", *result.AccountList[0].AccountName)
	assert.Equal(t, "test@example.com", *result.AccountList[0].EmailAddress)

	mockApi.AssertExpectations(t)
}

func TestListAccounts_Error(t *testing.T) {
	mockApi := new(mockSsoApi)
	mockApi.On("ListAccounts", mock.Anything, mock.Anything, mock.Anything).Return(mockListAccountsOutput, errors.New("error"))

	input := &sso.ListAccountsInput{
		AccessToken: aws.String("test-access-token"),
	}
	result, err := ListAccounts(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}

func TestGenerateListAccountRolesInput(t *testing.T) {
	accessToken := "test-access-token"
	accountId := "123456789012"
	opts := SSOOpts{AccessToken: accessToken, AccountId: accountId}
	input := GenerateListAccountRolesInput(opts)
	assert.NotNil(t, input)
	assert.Equal(t, accessToken, *input.AccessToken)
	assert.Equal(t, accountId, *input.AccountId)
}

func TestListAccountRoles(t *testing.T) {
	mockApi := new(mockSsoApi)
	mockApi.On("ListAccountRoles", mock.Anything, mock.Anything, mock.Anything).Return(mockListAccountRolesOutput, nil)

	input := &sso.ListAccountRolesInput{
		AccessToken: aws.String("test-access-token"),
		AccountId:   aws.String("123456789012"),
	}
	result, err := ListAccountRoles(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.RoleList))
	assert.Equal(t, "123456789012", *result.RoleList[0].AccountId)
	assert.Equal(t, "AdminRole", *result.RoleList[0].RoleName)

	mockApi.AssertExpectations(t)
}

func TestListAccountRoles_Error(t *testing.T) {
	mockApi := new(mockSsoApi)
	mockApi.On("ListAccountRoles", mock.Anything, mock.Anything, mock.Anything).Return(mockListAccountRolesOutput, errors.New("error"))

	input := &sso.ListAccountRolesInput{
		AccessToken: aws.String("test-access-token"),
		AccountId:   aws.String("123456789012"),
	}
	result, err := ListAccountRoles(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}
