// Package ssoauth は AWS IAM Identity Center (SSO) のデバイス認可フロー (RFC 8628) を提供する。
//
// internal/cli と internal/api の両方から使う共有ロジックであり、フローを「デバイス認可の
// 開始」(Start) と「トークン待機」(Wait) の 2 つの公開関数に分割している。CLI は 2 つを
// 続けて呼び、API サーバは開始と完了を別のリクエストとして呼べる。verification_uri の提示や
// ブラウザの起動といった利用者への提示手段は呼び出し側の責務であり、本パッケージは扱わない。
package ssoauth

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

const (
	clientName = "thief"
	clientType = "public"
	grantType  = "urn:ietf:params:oauth:grant-type:device_code"
)

// TokenCache は ~/.aws/sso/cache に保存するトークンキャッシュの JSON 形状。
// AWS CLI (aws sso login) と互換のフォーマットを維持する。
type TokenCache struct {
	StartURL              string `json:"startUrl"`
	Region                string `json:"region"`
	AccessToken           string `json:"accessToken"`
	ExpiresAt             string `json:"expiresAt"`
	ClientID              string `json:"clientId"`
	ClientSecret          string `json:"clientSecret"`
	RegistrationExpiresAt string `json:"registrationExpiresAt"`
}

// Session は Start が返し Wait が受け取る、デバイス認可フローの中間状態。
// 利用者への提示に使う verification_uri / user_code / verification_uri_complete は
// DeviceAuth が持つ。
type Session struct {
	Region       string
	StartURL     string
	Registration *awsinternal.SSOClientRegistration
	DeviceAuth   *awsinternal.SSODeviceAuthorization
}

// Deps は Start と Wait が呼ぶ外部処理をまとめる。
// RegisterClient / StartDeviceAuth / WaitForToken は AWS への接続を、SaveCache は
// $HOME/.aws/sso/cache への書き込みを伴い、テストからは実行できないため差し替える。
// これらは元から自由関数であり、絞り込む対象の具象型が無い。internal/aws のように
// SDK クライアントをコンシューマ定義インターフェースで受けるのではなく、関数値を
// 持たせているのはそのためである。
type Deps struct {
	RegisterClient  func(ctx context.Context, region, clientName, clientType string) (*awsinternal.SSOClientRegistration, error)
	StartDeviceAuth func(ctx context.Context, region string, reg *awsinternal.SSOClientRegistration, startURL string) (*awsinternal.SSODeviceAuthorization, error)
	WaitForToken    func(ctx context.Context, region string, reg *awsinternal.SSOClientRegistration, deviceAuth *awsinternal.SSODeviceAuthorization, grantType string) (*awsinternal.SSOToken, error)
	SaveCache       func(cache *TokenCache) error
}

// DefaultDeps は本番で使う実装を返す。
func DefaultDeps() Deps {
	return Deps{
		RegisterClient:  awsinternal.RegisterSSOClient,
		StartDeviceAuth: awsinternal.StartSSODeviceAuthorization,
		WaitForToken:    awsinternal.WaitForSSOToken,
		SaveCache:       saveCacheFile,
	}
}

// Start はデバイス認可フローの開始段を実行する。OIDC クライアントを登録し、デバイス認可を
// 開始して (RFC 8628 §3.1 / §3.2)、トークン待機 (Wait) と利用者への提示に必要な中間状態を返す。
//
// 認可サーバが返した verification_uri / user_code / verification_uri_complete は
// Session.DeviceAuth がそのまま持つ。verification_uri_complete が空 (§3.2 で OPTIONAL) でも
// start URL から URI を組み立てて補うことはしない。組み立てた値には仕様上の裏付けが無く、
// サーバの指示として見せることになるためである (§3.3)。
//
// awsinternal の 2 つの呼び出しは、どの API で失敗したかを示す文言で既にラップされて返る。
// この層から足せる情報が無いため包み直さず、そのまま伝播させる。
func Start(ctx context.Context, region, startURL string, deps Deps) (*Session, error) {
	registration, err := deps.RegisterClient(ctx, region, clientName, clientType)
	if err != nil {
		return nil, err
	}

	deviceAuth, err := deps.StartDeviceAuth(ctx, region, registration, startURL)
	if err != nil {
		return nil, err
	}

	return &Session{
		Region:       region,
		StartURL:     startURL,
		Registration: registration,
		DeviceAuth:   deviceAuth,
	}, nil
}

// Wait はトークン待機段を実行する。CreateToken のポーリング (RFC 8628 §3.4 / §3.5) で
// アクセストークンを取得し、AWS CLI 互換の TokenCache を組み立てて deps.SaveCache で
// 保存し、保存済みのキャッシュを返す。
//
// sess.DeviceAuth は丸ごと WaitForToken へ渡す。WaitForToken は device code だけでなく、
// サーバが指示した interval と expires_in からポーリング間隔と打ち切り期限を決める
// (§3.2 / §3.5)。ここで DeviceCode だけ取り出すと、その指示が捨てられて既定値に落ちる。
//
// WaitForToken の失敗は、どの API で失敗したかを示す文言で既にラップされて返るため
// 包み直さない。SaveCache の失敗は、注入された実装が何をしようとしたかまで述べる保証が
// 無いため、この層で文脈を足して包む。
//
// sess が Start の戻り値ではない場合 (nil、または Registration / DeviceAuth が nil の
// 不完全な状態) は、外部処理へ進まずエラーを返す。
func Wait(ctx context.Context, sess *Session, deps Deps) (*TokenCache, error) {
	// Start を経ていない不完全な Session は nil 参照の panic ではなくエラーで拒否する。
	// 公開関数の分割により呼び出し順の契約 (Start → Wait) が破られうるため、境界で
	// 自衛する (AGENTS.md はリクエスト処理中の panic を禁止している)。
	if sess == nil || sess.Registration == nil || sess.DeviceAuth == nil {
		return nil, errors.New("sso auth session is incomplete: it must be built by Start")
	}

	token, err := deps.WaitForToken(ctx, sess.Region, sess.Registration, sess.DeviceAuth, grantType)
	if err != nil {
		return nil, err
	}

	expireAt := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)

	cache := &TokenCache{
		StartURL:              sess.StartURL,
		Region:                sess.Region,
		AccessToken:           token.AccessToken,
		ExpiresAt:             expireAt.Format(time.RFC3339),
		ClientID:              sess.Registration.ClientID,
		ClientSecret:          sess.Registration.ClientSecret,
		RegistrationExpiresAt: time.Unix(sess.Registration.ClientSecretExpiresAt, 0).UTC().Format(time.RFC3339),
	}

	if err := deps.SaveCache(cache); err != nil {
		return nil, fmt.Errorf("save cache file: %w", err)
	}

	return cache, nil
}

// CacheDir はトークンキャッシュの保存先ディレクトリ (~/.aws/sso/cache) を返す。
// AWS CLI と SDK が参照する場所と同一であり、CLI の sso logout はここを走査して削除する。
func CacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".aws", "sso", "cache"), nil
}

// Logout は startURL に対応するトークンキャッシュを CacheDir() から削除する
// (aws.RemoveSSOTokenCache に委ねる薄い関数)。削除はローカルのキャッシュに限り、
// AWS 側のサインインセッションは失効させない。同じ startURL を共有する他の
// profile も未ログインになる (AWS CLI の aws sso logout と同じ範囲)。
func Logout(startURL string) error {
	cacheDir, err := CacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}
	return awsinternal.RemoveSSOTokenCache(cacheDir, startURL)
}

// saveCacheFile はトークンキャッシュを ~/.aws/sso/cache/<sha1>.json に保存する。
// ファイル名とパーミッション (ディレクトリ 0700、ファイル 0600) は AWS CLI と互換にする。
func saveCacheFile(cache *TokenCache) error {
	cacheDir, err := CacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failed to create sso cache directory: %w", err)
	}

	cacheFile := filepath.Join(cacheDir, cacheKey(cache.StartURL)+".json")

	jsonData, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := os.WriteFile(cacheFile, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// cacheKey は startUrl から AWS CLI 互換のキャッシュファイル名 (SHA-1 hex) を生成する。
func cacheKey(startURL string) string {
	hasher := sha1.New()
	hasher.Write([]byte(startURL))
	return hex.EncodeToString(hasher.Sum(nil))
}
