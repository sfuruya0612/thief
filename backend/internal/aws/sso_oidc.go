package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
)

// ssoTokenPollMaxAttempts / ssoTokenPollInterval はデバイス認可フローでの
// トークンポーリングの最大試行回数と初期間隔。
const (
	ssoTokenPollMaxAttempts = 60
	ssoTokenPollInterval    = 1 * time.Second
)

// SSOClientRegistration は SSO OIDC のクライアント登録結果を保持する。
type SSOClientRegistration struct {
	ClientID              string
	ClientSecret          string
	ClientSecretExpiresAt int64
}

// SSODeviceAuthorization はデバイス認可フローの開始結果を保持する。
type SSODeviceAuthorization struct {
	DeviceCode              string
	UserCode                string
	VerificationURIComplete string
}

// SSOToken はデバイス認可フローで取得したアクセストークンを保持する。
type SSOToken struct {
	AccessToken string
	ExpiresIn   int32
}

func newSSOOidcClient(ctx context.Context, region string) (*ssooidc.Client, error) {
	return NewClient(ctx, "", region, func(cfg awssdk.Config) *ssooidc.Client {
		return ssooidc.NewFromConfig(cfg)
	})
}

func newSSOClient(ctx context.Context, region string) (*sso.Client, error) {
	return NewClient(ctx, "", region, func(cfg awssdk.Config) *sso.Client {
		return sso.NewFromConfig(cfg)
	})
}

// RegisterSSOClient は SSO OIDC クライアントを登録し、クライアント ID とシークレットを返す。
func RegisterSSOClient(ctx context.Context, region, clientName, clientType string) (*SSOClientRegistration, error) {
	client, err := newSSOOidcClient(ctx, region)
	if err != nil {
		return nil, err
	}

	o, err := client.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: awssdk.String(clientName),
		ClientType: awssdk.String(clientType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register client: %w", err)
	}

	return &SSOClientRegistration{
		ClientID:              ptrStr(o.ClientId),
		ClientSecret:          ptrStr(o.ClientSecret),
		ClientSecretExpiresAt: o.ClientSecretExpiresAt,
	}, nil
}

// StartSSODeviceAuthorization はデバイス認可フローを開始し、
// ユーザーがブラウザで承認するための情報を返す。
func StartSSODeviceAuthorization(ctx context.Context, region string, reg *SSOClientRegistration, startURL string) (*SSODeviceAuthorization, error) {
	client, err := newSSOOidcClient(ctx, region)
	if err != nil {
		return nil, err
	}

	o, err := client.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     awssdk.String(reg.ClientID),
		ClientSecret: awssdk.String(reg.ClientSecret),
		StartUrl:     awssdk.String(startURL),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start device authorization: %w", err)
	}

	return &SSODeviceAuthorization{
		DeviceCode:              ptrStr(o.DeviceCode),
		UserCode:                ptrStr(o.UserCode),
		VerificationURIComplete: ptrStr(o.VerificationUriComplete),
	}, nil
}

// WaitForSSOToken はユーザーのブラウザ承認が完了するまで CreateToken をポーリングし、
// アクセストークンを返す。承認待ち (AuthorizationPending) は再試行し、
// レート制限 (SlowDown) では間隔を倍にして再試行する。それ以外のエラーは即時失敗する。
func WaitForSSOToken(ctx context.Context, region string, reg *SSOClientRegistration, deviceCode, grantType string) (*SSOToken, error) {
	client, err := newSSOOidcClient(ctx, region)
	if err != nil {
		return nil, err
	}

	input := &ssooidc.CreateTokenInput{
		ClientId:     awssdk.String(reg.ClientID),
		ClientSecret: awssdk.String(reg.ClientSecret),
		DeviceCode:   awssdk.String(deviceCode),
		GrantType:    awssdk.String(grantType),
	}

	interval := ssoTokenPollInterval
	for i := 0; i < ssoTokenPollMaxAttempts; i++ {
		o, err := client.CreateToken(ctx, input)
		if err == nil {
			return &SSOToken{
				AccessToken: ptrStr(o.AccessToken),
				ExpiresIn:   o.ExpiresIn,
			}, nil
		}

		var pending *ssooidctypes.AuthorizationPendingException
		var slowDown *ssooidctypes.SlowDownException
		switch {
		case errors.As(err, &slowDown):
			interval *= 2
		case errors.As(err, &pending):
			// ユーザーのブラウザ承認待ち。間隔は変えずに再試行する。
		default:
			return nil, fmt.Errorf("token creation failed: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}

	return nil, fmt.Errorf("timeout waiting for authentication")
}

// SSOAccountInfo は generate-config で使うアカウントの基本情報を保持する。
type SSOAccountInfo struct {
	AccountID   string
	AccountName string
}

// ListSSOAccountInfos は指定アクセストークンで参照可能な SSO アカウント一覧を返す。
func ListSSOAccountInfos(ctx context.Context, region, accessToken string) ([]SSOAccountInfo, error) {
	client, err := newSSOClient(ctx, region)
	if err != nil {
		return nil, err
	}

	var accounts []SSOAccountInfo
	paginator := sso.NewListAccountsPaginator(client, &sso.ListAccountsInput{
		AccessToken: awssdk.String(accessToken),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sso accounts: %w", err)
		}
		for _, a := range page.AccountList {
			accounts = append(accounts, SSOAccountInfo{
				AccountID:   ptrStr(a.AccountId),
				AccountName: ptrStr(a.AccountName),
			})
		}
	}
	return accounts, nil
}

// ListSSOAccountRoleNames は指定アカウントで利用可能なロール名一覧を返す。
func ListSSOAccountRoleNames(ctx context.Context, region, accessToken, accountID string) ([]string, error) {
	client, err := newSSOClient(ctx, region)
	if err != nil {
		return nil, err
	}

	var roles []string
	paginator := sso.NewListAccountRolesPaginator(client, &sso.ListAccountRolesInput{
		AccessToken: awssdk.String(accessToken),
		AccountId:   awssdk.String(accountID),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sso account roles: %w", err)
		}
		for _, r := range page.RoleList {
			roles = append(roles, ptrStr(r.RoleName))
		}
	}
	return roles, nil
}
