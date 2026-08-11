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

// デバイス認可フローでのトークンポーリングの既定値。いずれも RFC 8628 に根拠がある。
const (
	// ssoTokenPollDefaultInterval は device authorization response が interval を
	// 指示しなかったときに使う間隔。RFC 8628 §3.2 は interval を OPTIONAL と定め、
	// "If no value is provided, clients MUST use 5 as the default." と述べる。
	ssoTokenPollDefaultInterval = 5 * time.Second

	// ssoTokenPollSlowDownIncrement は slow_down を受けたときの間隔の増分。
	// RFC 8628 §3.5 は "the interval MUST be increased by 5 seconds for this and all
	// subsequent requests" と定める。倍加ではなく固定の加算である。
	ssoTokenPollSlowDownIncrement = 5 * time.Second

	// ssoTokenPollDefaultTimeout は device authorization response が expires_in を
	// 指示しなかったときに使う打ち切りまでの猶予。RFC 8628 §3.2 は expires_in を
	// REQUIRED と定めるため、指示が無いのは仕様に従っていない応答である。それでも
	// 際限なくポーリングしないよう上限を置く。値は AWS が実際に返す 600 秒に合わせた。
	ssoTokenPollDefaultTimeout = 600 * time.Second
)

// デバイス認可フローのポーリングが返すセンチネルエラー。呼び出し側が文字列一致ではなく
// errors.Is で判別できるようにしてある。パッケージ外に出す必要が生じていないので非公開。
var (
	// errSSOTokenTimeout は device code の有効期限までに承認が完了しなかったことを表す。
	errSSOTokenTimeout = errors.New("timeout waiting for authentication")

	// errNilSSODeviceAuthorization はデバイス認可の応答を受け取らずにポーリングを
	// 要求されたことを表す。呼び出し側の誤りであり、参照外しで panic させずに返す。
	errNilSSODeviceAuthorization = errors.New("nil sso device authorization")

	// errInvalidSSOTokenPollPolicy はポーリング方針が不変条件を満たしていないことを表す。
	// 間隔と猶予はいずれも正でなければならない。
	errInvalidSSOTokenPollPolicy = errors.New("invalid sso token poll policy")
)

// SSOClientRegistration は SSO OIDC のクライアント登録結果を保持する。
type SSOClientRegistration struct {
	ClientID              string
	ClientSecret          string
	ClientSecretExpiresAt int64
}

// SSODeviceAuthorization はデバイス認可フローの開始結果を保持する。
type SSODeviceAuthorization struct {
	DeviceCode string
	UserCode   string

	// VerificationURI は認可サーバが指示した利用者向けの検証 URI。
	// RFC 8628 §3.2 の verification_uri に対応し、REQUIRED である。§3.3 は user_code と
	// あわせてこれを利用者へ提示することを求めており、提示する URI はここで受け取った
	// 値でなければならない。start URL から組み立てた推測値には仕様上の裏付けが無い。
	VerificationURI string

	// VerificationURIComplete は user_code を含む検証 URI。
	// RFC 8628 §3.2 の verification_uri_complete に対応し、OPTIONAL である。
	// §3.3.1 の非テキストでの提示 (ブラウザの自動起動) に使う。
	VerificationURIComplete string

	// Interval はサーバが指示したポーリング間隔 (秒)。RFC 8628 §3.2 の interval に対応する。
	// §3.2 での位置づけは OPTIONAL かつ SHOULD だが、§3.5 の authorization_pending の説明が
	// クライアントがこれ以上待つことを MUST と定める。OPTIONAL であるため、指示が無いとき
	// SDK は 0 を入れる。
	Interval int32

	// ExpiresIn は device code と user code が無効になるまでの秒数。
	// RFC 8628 §3.2 の expires_in に対応する。ポーリングの打ち切りはこれを基準に決まる。
	ExpiresIn int32
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
		VerificationURI:         ptrStr(o.VerificationUri),
		VerificationURIComplete: ptrStr(o.VerificationUriComplete),
		Interval:                o.Interval,
		ExpiresIn:               o.ExpiresIn,
	}, nil
}

// ssoOidcCreateTokenAPI は CreateToken の呼び出しを抽象化する。
// テストではモックを差し込み、実行時は *ssooidc.Client がこれを満たす。
type ssoOidcCreateTokenAPI interface {
	CreateToken(ctx context.Context, params *ssooidc.CreateTokenInput, optFns ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error)
}

// ssoTokenPollPolicy はデバイス認可フローのポーリングの間隔と打ち切り条件をまとめる。
// 全フィールドが必須で、本番の値は newSSOTokenPollPolicy が組む。
//
// now と after はテストから時間を差し替えられるようにしてある。差し替えが必要な理由は
// internal/aws/sso_oidc_test.go 側に書いてある。
type ssoTokenPollPolicy struct {
	// interval は次の試行までの待ち時間の初期値。slow_down を受けるたびに増える。
	interval time.Duration

	// timeout はポーリングを打ち切るまでの猶予。device code の有効期限から決まる。
	timeout time.Duration

	// now は打ち切り判定に使う現在時刻を返す。
	now func() time.Time

	// after は次の試行までの待機を表すチャネルを返す。
	after func(d time.Duration) <-chan time.Time
}

// newSSOTokenPollPolicy は device authorization response の指示からポーリング方針を組む。
func newSSOTokenPollPolicy(deviceAuth *SSODeviceAuthorization) ssoTokenPollPolicy {
	// RFC 8628 §3.5 の authorization_pending の説明は "Before each new request, the client
	// MUST wait at least the number of seconds specified by the "interval" parameter of the
	// device authorization response (see Section 3.2), or 5 seconds if none was provided"
	// と定めるため、初期間隔はサーバの指示をそのまま採用する。指示が無いとき SDK は 0 を
	// 入れるので §3.2 の既定 5 秒に倒す。負の値も同じ扱いにする。
	// ここは無限ループの防波堤でもある。間隔が 0 以下だと打ち切り判定に使う時刻が進まず、
	// CreateToken を待機なしで呼び続けることになる。
	interval := ssoTokenPollDefaultInterval
	if deviceAuth.Interval > 0 {
		interval = time.Duration(deviceAuth.Interval) * time.Second
	}

	// 打ち切りは device code の有効期限に合わせる。試行回数で縛ると、間隔が変わるたびに
	// 実時間の上限が動いてしまい、コードがまだ有効なのに打ち切る (または期限を過ぎても
	// 叩き続ける) ことになる。
	timeout := ssoTokenPollDefaultTimeout
	if deviceAuth.ExpiresIn > 0 {
		timeout = time.Duration(deviceAuth.ExpiresIn) * time.Second
	}

	return ssoTokenPollPolicy{
		interval: interval,
		timeout:  timeout,
		now:      time.Now,
		after:    time.After,
	}
}

// WaitForSSOToken はユーザーのブラウザ承認が完了するまで CreateToken をポーリングし、
// アクセストークンを返す。承認待ち (AuthorizationPending) は間隔を変えずに再試行し、
// レート制限 (SlowDown) では RFC 8628 §3.5 に従って間隔を 5 秒増やして再試行する。
// それ以外のエラーは即時失敗する。device code の有効期限を過ぎたら打ち切る。
func WaitForSSOToken(ctx context.Context, region string, reg *SSOClientRegistration, deviceAuth *SSODeviceAuthorization, grantType string) (*SSOToken, error) {
	// deviceAuth は device code だけでなくポーリング方針の素にもなるため、この関数と
	// newSSOTokenPollPolicy の両方で参照外しする。nil で来たら panic させずに返す。
	// AGENTS.md は「リクエスト処理中の panic は禁止」と定めており、CLI のログイン処理も
	// これに当たる。呼び出し元は startDeviceAuth の戻り値をそのまま渡す実装だが、
	// これは関数値であり差し替えられ得るので、境界で自衛する。
	if deviceAuth == nil {
		return nil, errNilSSODeviceAuthorization
	}

	client, err := newSSOOidcClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return waitForSSOToken(ctx, client, reg, deviceAuth.DeviceCode, grantType, newSSOTokenPollPolicy(deviceAuth))
}

// waitForSSOToken は生成済みクライアントでトークンをポーリングするコア。
// 分岐 (SlowDown での間隔の増加、AuthorizationPending での再試行、それ以外での即時失敗、
// 有効期限での打ち切り、ctx のキャンセル) を単体テストで検証できるよう、
// クライアントの生成とポーリング方針から分離してある。
//
// 引数が deviceCode だけで SSODeviceAuthorization を丸ごと受け取らないのは、この関数が
// 応答から読むのが device code だけだからである。interval と expires_in は
// newSSOTokenPollPolicy が policy へ畳み込む。
func waitForSSOToken(ctx context.Context, client ssoOidcCreateTokenAPI, reg *SSOClientRegistration, deviceCode, grantType string, policy ssoTokenPollPolicy) (*SSOToken, error) {
	// このループが有限で終わることは interval > 0 と timeout > 0 に依存している。間隔が
	// 0 以下だと待機で時刻が進まず、打ち切り判定が永久に成立しないまま CreateToken を
	// 連打し続ける。newSSOTokenPollPolicy はこの不変条件を必ず満たす方針を組むが、
	// policy は構造体リテラルでも組めるため、ここで自分の前提を検証する。
	if policy.interval <= 0 || policy.timeout <= 0 {
		return nil, fmt.Errorf("%w: interval %s and timeout %s must both be positive", errInvalidSSOTokenPollPolicy, policy.interval, policy.timeout)
	}

	input := &ssooidc.CreateTokenInput{
		ClientId:     awssdk.String(reg.ClientID),
		ClientSecret: awssdk.String(reg.ClientSecret),
		DeviceCode:   awssdk.String(deviceCode),
		GrantType:    awssdk.String(grantType),
	}

	interval := policy.interval

	// 期限はループに入る前に 1 回だけ確定させる。ループのたびに now() + timeout を
	// 計算し直すと締切が常に未来へ逃げ、打ち切りが効かなくなる。
	//
	// なお本番の policy.now は time.Now であり、返る Time は単調時計の読みを持つ。
	// time.Time.Add は単調成分を保持し、Before は両辺に単調成分があるとき壁時計を
	// 無視して単調成分だけで比較する。したがって NTP 補正や手動の時刻変更でこの
	// 判定が壊れることはない。単調時計はシステムのスリープ中に止まり得るが、その場合は
	// 期限切れをサーバが expired_token として返すので、クライアント側の打ち切りより
	// 明確な失敗になる。
	deadline := policy.now().Add(policy.timeout)

	for {
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
			// RFC 8628 §3.5: "the interval MUST be increased by 5 seconds for this and
			// all subsequent requests"。増分は固定であり、倍加ではない。
			interval += ssoTokenPollSlowDownIncrement
		case errors.As(err, &pending):
			// ユーザーのブラウザ承認待ち。間隔は変えずに再試行する。
		default:
			// RFC 8628 §3.5 は authorization_pending と slow_down 以外について
			// "For any other error, the client MUST stop polling" と定める。したがって
			// access_denied と expired_token も含め、ここは再試行せず即時失敗が正しい。
			// これらを再試行に回すと仕様違反になる。
			return nil, fmt.Errorf("create sso oidc token: %w", err)
		}

		// 待機を終える時刻が device code の有効期限に届くなら、待っても叩く先が無いので
		// ここで打ち切る。待ってから打ち切ると、既に無効なコードのためにユーザーが
		// 1 回分の間隔だけ余計に黙らされる。
		//
		// この判定は間隔の上限も兼ねている。interval が timeout 以上に伸びた時点で必ず
		// 成立するため、slow_down が続いても待機が猶予を超えて伸びることはない。
		if !policy.now().Add(interval).Before(deadline) {
			// この判定は ctx を見ないため、直前に ctx がキャンセルされていると打ち切りの
			// 方が先に返る。理由が利用者の中断なら、そちらを伝える方が実態に合う。
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, errSSOTokenTimeout
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-policy.after(interval):
		}
	}
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
