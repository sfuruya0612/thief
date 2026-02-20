package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
)

type SSOOpts struct {
	AccessToken string
	AccountId   string
}

// type stsApi interface {
// 	GetCallerIdentity(ctx context.Context, input *sts.GetCallerIdentityInput, opts ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
// }

// func NewSTSClient(profile, region string) stsApi {
// 	return sts.NewFromConfig(GetSession(profile, region))
// }

type ssoApi interface {
	ListAccounts(ctx context.Context, input *sso.ListAccountsInput, opts ...func(*sso.Options)) (*sso.ListAccountsOutput, error)
	ListAccountRoles(ctx context.Context, input *sso.ListAccountRolesInput, opts ...func(*sso.Options)) (*sso.ListAccountRolesOutput, error)
}

// NewSSOClient creates a new SSO client using the specified AWS profile and region.
func NewSSOClient(profile, region string) (ssoApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create sso client: %w", err)
	}
	return sso.NewFromConfig(cfg), nil
}

// func GenerateGetCallerIdentityInput() *sts.GetCallerIdentityInput {
// 	return &sts.GetCallerIdentityInput{}
// }

// func Login(api stsApi, input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
// 	o, err := api.GetCallerIdentity(context.Background(), input)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return o, nil
// }

func GenerateListAccountsInput(opts SSOOpts) *sso.ListAccountsInput {
	return &sso.ListAccountsInput{
		AccessToken: aws.String(opts.AccessToken),
	}
}

func ListAccounts(api ssoApi, input *sso.ListAccountsInput) (*sso.ListAccountsOutput, error) {
	var accounts []types.AccountInfo
	var nextToken *string

	for {
		o, err := api.ListAccounts(context.Background(), input)
		if err != nil {
			return nil, err
		}

		accounts = append(accounts, o.AccountList...)
		nextToken = o.NextToken

		if nextToken == nil {
			break
		}

		input.NextToken = nextToken
	}

	return &sso.ListAccountsOutput{
		AccountList: accounts,
	}, nil
}

func GenerateListAccountRolesInput(opts SSOOpts) *sso.ListAccountRolesInput {
	return &sso.ListAccountRolesInput{
		AccessToken: aws.String(opts.AccessToken),
		AccountId:   aws.String(opts.AccountId),
	}
}

func ListAccountRoles(api ssoApi, input *sso.ListAccountRolesInput) (*sso.ListAccountRolesOutput, error) {
	o, err := api.ListAccountRoles(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return o, nil
}
