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

// ssoOidcRegisterClientAPI は RegisterClient の呼び出しを抽象化する。
// テストではモックを差し込み、実行時は *ssooidc.Client がこれを満たす。
type ssoOidcRegisterClientAPI interface {
	RegisterClient(ctx context.Context, params *ssooidc.RegisterClientInput, optFns ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error)
}

// RegisterSSOClient は SSO OIDC クライアントを登録し、クライアント ID とシークレットを返す。
func RegisterSSOClient(ctx context.Context, region, clientName, clientType string) (*SSOClientRegistration, error) {
	client, err := newSSOOidcClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return registerSSOClient(ctx, client, clientName, clientType)
}

// registerSSOClient は生成済みクライアントでクライアント登録を行うコア。
// RegisterClientInput に載せる ClientName と ClientType を単体テストで固定できるよう、
// クライアントの生成と分離してある。
func registerSSOClient(ctx context.Context, client ssoOidcRegisterClientAPI, clientName, clientType string) (*SSOClientRegistration, error) {
	o, err := client.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: awssdk.String(clientName),
		ClientType: awssdk.String(clientType),
	})
	if err != nil {
		return nil, fmt.Errorf("register sso oidc client: %w", err)
	}

	return &SSOClientRegistration{
		ClientID:              ptrStr(o.ClientId),
		ClientSecret:          ptrStr(o.ClientSecret),
		ClientSecretExpiresAt: o.ClientSecretExpiresAt,
	}, nil
}

// ssoOidcStartDeviceAuthorizationAPI は StartDeviceAuthorization の呼び出しを抽象化する。
// テストではモックを差し込み、実行時は *ssooidc.Client がこれを満たす。
type ssoOidcStartDeviceAuthorizationAPI interface {
	StartDeviceAuthorization(ctx context.Context, params *ssooidc.StartDeviceAuthorizationInput, optFns ...func(*ssooidc.Options)) (*ssooidc.StartDeviceAuthorizationOutput, error)
}

// StartSSODeviceAuthorization はデバイス認可フローを開始し、
// ユーザーがブラウザで承認するための情報を返す。
func StartSSODeviceAuthorization(ctx context.Context, region string, reg *SSOClientRegistration, startURL string) (*SSODeviceAuthorization, error) {
	client, err := newSSOOidcClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return startSSODeviceAuthorization(ctx, client, reg, startURL)
}

// startSSODeviceAuthorization は生成済みクライアントでデバイス認可を開始するコア。
// StartDeviceAuthorizationInput に載せる ClientId / ClientSecret / StartUrl を
// 単体テストで固定できるよう、クライアントの生成と分離してある。
func startSSODeviceAuthorization(ctx context.Context, client ssoOidcStartDeviceAuthorizationAPI, reg *SSOClientRegistration, startURL string) (*SSODeviceAuthorization, error) {
	o, err := client.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     awssdk.String(reg.ClientID),
		ClientSecret: awssdk.String(reg.ClientSecret),
		StartUrl:     awssdk.String(startURL),
	})
	if err != nil {
		return nil, fmt.Errorf("start sso oidc device authorization: %w", err)
	}

	return &SSODeviceAuthorization{
		DeviceCode:              ptrStr(o.DeviceCode),
		UserCode:                ptrStr(o.UserCode),
		VerificationURIComplete: ptrStr(o.VerificationUriComplete),
	}, nil
}

// ssoOidcCreateTokenAPI は CreateToken の呼び出しを抽象化する。
// テストではモックを差し込み、実行時は *ssooidc.Client がこれを満たす。
type ssoOidcCreateTokenAPI interface {
	CreateToken(ctx context.Context, params *ssooidc.CreateTokenInput, optFns ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error)
}

// ssoTokenPollPolicy はデバイス認可フローのポーリングの初期間隔と打ち切り条件をまとめる。
// 全フィールドが必須で、本番の値は productionSSOTokenPollPolicy が組む。
//
// after は次の試行までの待機を表すチャネルを返す。テストから待機を差し替えられるように
// してある。差し替えが必要な理由は internal/aws/sso_oidc_test.go 側に書いてある。
type ssoTokenPollPolicy struct {
	interval    time.Duration
	maxAttempts int
	after       func(d time.Duration) <-chan time.Time
}

// productionSSOTokenPollPolicy は本番で使うポーリング方針を返す。
func productionSSOTokenPollPolicy() ssoTokenPollPolicy {
	return ssoTokenPollPolicy{
		interval:    ssoTokenPollInterval,
		maxAttempts: ssoTokenPollMaxAttempts,
		after:       time.After,
	}
}

// WaitForSSOToken はユーザーのブラウザ承認が完了するまで CreateToken をポーリングし、
// アクセストークンを返す。承認待ち (AuthorizationPending) は再試行し、
// レート制限 (SlowDown) では間隔を倍にして再試行する。それ以外のエラーは即時失敗する。
func WaitForSSOToken(ctx context.Context, region string, reg *SSOClientRegistration, deviceCode, grantType string) (*SSOToken, error) {
	client, err := newSSOOidcClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return waitForSSOToken(ctx, client, reg, deviceCode, grantType, productionSSOTokenPollPolicy())
}

// waitForSSOToken は生成済みクライアントでトークンをポーリングするコア。
// 分岐 (SlowDown での間隔倍加、AuthorizationPending での再試行、それ以外での即時失敗、
// 最大試行回数の超過、ctx のキャンセル) を単体テストで検証できるよう、
// クライアントの生成とポーリング方針から分離してある。
func waitForSSOToken(ctx context.Context, client ssoOidcCreateTokenAPI, reg *SSOClientRegistration, deviceCode, grantType string, policy ssoTokenPollPolicy) (*SSOToken, error) {
	input := &ssooidc.CreateTokenInput{
		ClientId:     awssdk.String(reg.ClientID),
		ClientSecret: awssdk.String(reg.ClientSecret),
		DeviceCode:   awssdk.String(deviceCode),
		GrantType:    awssdk.String(grantType),
	}

	interval := policy.interval
	for i := 0; i < policy.maxAttempts; i++ {
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
			// RFC 8628 §3.5 は slow_down に対して間隔を固定 5 秒増やすことを MUST と
			// 定めており、倍加は仕様外である。上限も無いため SlowDown が続くと待機が
			// 指数的に伸び、34 回で time.Duration が int64 を超えて負になる。
			// 挙動を変えない範囲では直せないため issue 0129 で扱う。
			// 単体テストは連続 2 回までしか固定していない。
			interval *= 2
		case errors.As(err, &pending):
			// ユーザーのブラウザ承認待ち。間隔は変えずに再試行する。
		default:
			return nil, fmt.Errorf("create sso oidc token: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-policy.after(interval):
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
	client, err := newSSOClient(ctx, "", region)
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
	client, err := newSSOClient(ctx, "", region)
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
