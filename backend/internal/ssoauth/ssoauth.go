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
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

const (
	clientName = "thief"
	clientType = "public"
	grantType  = "urn:ietf:params:oauth:grant-type:device_code"
)

// revokeTimeout はトークン 1 件ごとの sso:Logout の上限。ブラウザの操作を待たない
// 純粋なネットワーク往復なので、internal/api のデバイス認可開始と同じ 30 秒とする。
const revokeTimeout = 30 * time.Second

var (
	// errRegionUnknown は sso:Logout を送る region を決められないトークンの RevokeFailure.Err。
	errRegionUnknown = errors.New("region unknown")
	// errAccessTokenMissing は accessToken が空で sso:Logout を送れないトークンの RevokeFailure.Err。
	errAccessTokenMissing = errors.New("access token missing")
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

// Deps は Start / Wait / Logout / LogoutAll が呼ぶ外部処理をまとめる。
// RegisterClient / StartDeviceAuth / WaitForToken / RevokeToken は AWS への接続を、
// SaveCache / ListTokens / RemoveTokenCache / RemoveAllCache は $HOME/.aws/sso/cache への
// 読み書きを伴い、テストからは実行できないため差し替える。
// これらは元から自由関数であり、絞り込む対象の具象型が無い。internal/aws のように
// SDK クライアントをコンシューマ定義インターフェースで受けるのではなく、関数値を
// 持たせているのはそのためである。
type Deps struct {
	RegisterClient   func(ctx context.Context, region, clientName, clientType string) (*awsinternal.SSOClientRegistration, error)
	StartDeviceAuth  func(ctx context.Context, region string, reg *awsinternal.SSOClientRegistration, startURL string) (*awsinternal.SSODeviceAuthorization, error)
	WaitForToken     func(ctx context.Context, region string, reg *awsinternal.SSOClientRegistration, deviceAuth *awsinternal.SSODeviceAuthorization, grantType string) (*awsinternal.SSOToken, error)
	SaveCache        func(cache *TokenCache) error
	RevokeToken      func(ctx context.Context, region, accessToken string) error
	ListTokens       func(cacheDir, startURL string) ([]awsinternal.SSOCachedToken, error)
	RemoveTokenCache func(cacheDir, startURL string) error
	RemoveAllCache   func(cacheDir string) error
}

// DefaultDeps は本番で使う実装を返す。
func DefaultDeps() Deps {
	return Deps{
		RegisterClient:   awsinternal.RegisterSSOClient,
		StartDeviceAuth:  awsinternal.StartSSODeviceAuthorization,
		WaitForToken:     awsinternal.WaitForSSOToken,
		SaveCache:        saveCacheFile,
		RevokeToken:      awsinternal.RevokeSSOToken,
		ListTokens:       awsinternal.ListSSOTokenCache,
		RemoveTokenCache: awsinternal.RemoveSSOTokenCache,
		RemoveAllCache:   awsinternal.RemoveAllSSOCache,
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

// RevokeFailure は sso:Logout で失効させられなかった 1 つのサインインセッション
// (accessToken 単位)。FileNames はそのトークンを含む全キャッシュファイル名。
type RevokeFailure struct {
	FileNames []string
	Err       error
}

// LogoutResult は Logout / LogoutAll の失効の結果。Revoked と RevokeFailed はどちらも
// 重複排除後のユニークな accessToken (AWS 側のサインインセッション) 単位で数える。
// 期限切れで失効を省いたトークンはどちらにも数えない。
type LogoutResult struct {
	Revoked      int
	RevokeFailed []RevokeFailure
}

// Logout は startURL に対応するトークンキャッシュを列挙し、各トークンのサインイン
// セッションを sso:Logout で失効させてから、CacheDir() の一致するファイルを削除する
// (列挙 → 失効 → 削除)。同じ startURL を共有する他の profile も未ログインになる
// (AWS CLI の aws sso logout と同じ範囲)。
//
// 失効に使う region はキャッシュファイルの region を優先し、無ければ fallbackRegion
// (profile の sso_region) を使う。失効の失敗はログアウトを止めず、slog.Warn に記録して
// LogoutResult.RevokeFailed に計上し、削除を続行する。返り値の error は CacheDir() と
// 列挙の失敗、および削除の失敗だけを表す。
//
// 削除に失敗した場合も、失効は削除より前に終わっているため LogoutResult は有効な値を
// 返す (error が非 nil でも LogoutResult を使ってよい)。CacheDir() と列挙の失敗では
// 失効も削除も始まっていないため LogoutResult{} を返す。
func Logout(ctx context.Context, startURL, fallbackRegion string, deps Deps) (LogoutResult, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return LogoutResult{}, fmt.Errorf("failed to get cache directory: %w", err)
	}
	tokens, err := deps.ListTokens(cacheDir, startURL)
	if err != nil {
		return LogoutResult{}, err
	}
	result := revokeTokens(ctx, tokens, fallbackRegion, deps)
	return result, deps.RemoveTokenCache(cacheDir, startURL)
}

// LogoutAll は CacheDir() の全トークンを列挙し、各トークンのサインインセッションを
// sso:Logout で失効させてから、CacheDir() 直下の全ファイル (client registration を含む)
// を削除する (列挙 → 失効 → 削除)。CLI の thief sso logout が使う。
//
// トークンごとに region が異なりうるため fallbackRegion は取らず、region を持たない
// トークンは失効できないものとして RevokeFailed に計上する。失効の失敗の扱いと返り値の
// 契約 (error が非 nil でも LogoutResult は有効、ただし CacheDir() と列挙の失敗では
// LogoutResult{}) は Logout と同じ。
func LogoutAll(ctx context.Context, deps Deps) (LogoutResult, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return LogoutResult{}, fmt.Errorf("failed to get cache directory: %w", err)
	}
	tokens, err := deps.ListTokens(cacheDir, "")
	if err != nil {
		return LogoutResult{}, err
	}
	result := revokeTokens(ctx, tokens, "", deps)
	return result, deps.RemoveAllCache(cacheDir)
}

// revokeGroup は同じ accessToken を持つキャッシュファイルの組。ファイル名の辞書順で
// 最初のファイルの region と expiresAt を代表値とする。
type revokeGroup struct {
	accessToken string
	region      string
	expiresAt   string
	fileNames   []string
}

// groupTokensByAccessToken は tokens をファイル名の辞書順に並べ、accessToken ごとに
// まとめる。返り値の順序は各グループの最初のファイル名の辞書順。legacy 形式と sso-session
// 形式の併存で同じトークンが 2 ファイルに書かれている場合に、2 回目の sso:Logout が
// 失効済みで失敗して偽の失敗が計上されるのを避ける。
func groupTokensByAccessToken(tokens []awsinternal.SSOCachedToken) []revokeGroup {
	sorted := make([]awsinternal.SSOCachedToken, len(tokens))
	copy(sorted, tokens)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].FileName < sorted[j].FileName })

	var groups []revokeGroup
	index := map[string]int{}
	for _, tok := range sorted {
		if i, ok := index[tok.AccessToken]; ok {
			groups[i].fileNames = append(groups[i].fileNames, tok.FileName)
			continue
		}
		index[tok.AccessToken] = len(groups)
		groups = append(groups, revokeGroup{
			accessToken: tok.AccessToken,
			region:      tok.Region,
			expiresAt:   tok.ExpiresAt,
			fileNames:   []string{tok.FileName},
		})
	}
	return groups
}

// revokeTokens は tokens を accessToken で重複排除し、ユニークなトークンごとに 1 回だけ
// deps.RevokeToken を順番に呼ぶ。goroutine は使わない (件数は通常 1 桁で、slog.Warn と
// RevokeFailed の順序を決定的に保つため)。
//
//   - expiresAt が RFC 3339 として解釈でき現在時刻より過去のトークンは失効を省き、
//     Revoked にも RevokeFailed にも数えない。無い、または解釈できない場合は試みる。
//   - region はキャッシュの値を優先し、無ければ fallbackRegion。どちらも空なら
//     errRegionUnknown で RevokeFailed に計上する。
//   - accessToken が空なら errAccessTokenMissing で RevokeFailed に計上する。
//   - 親の ctx が Done になった後は残りを呼ばず、ctx.Err() で RevokeFailed に計上する。
//
// 完了時は対象 0 件でも slog.Info を出す。
func revokeTokens(ctx context.Context, tokens []awsinternal.SSOCachedToken, fallbackRegion string, deps Deps) LogoutResult {
	var result LogoutResult
	now := time.Now()
	for _, g := range groupTokensByAccessToken(tokens) {
		if exp, err := time.Parse(time.RFC3339, g.expiresAt); err == nil && !exp.After(now) {
			// 期限切れトークンへの sso:Logout は失敗するだけで意味が無い。
			continue
		}
		if err := revokeGroupOnce(ctx, g, fallbackRegion, deps); err != nil {
			slog.Warn("sso token revoke failed", "files", g.fileNames, "err", err)
			result.RevokeFailed = append(result.RevokeFailed, RevokeFailure{FileNames: g.fileNames, Err: err})
			continue
		}
		result.Revoked++
	}
	slog.Info("sso logout completed", "revoked", result.Revoked, "revoke_failed", len(result.RevokeFailed))
	return result
}

// revokeGroupOnce は 1 つの accessToken の失効を試み、失効できない理由または
// deps.RevokeToken の失敗を返す。呼び出しごとに revokeTimeout の上限を掛ける。
func revokeGroupOnce(ctx context.Context, g revokeGroup, fallbackRegion string, deps Deps) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if g.accessToken == "" {
		return errAccessTokenMissing
	}
	region := g.region
	if region == "" {
		region = fallbackRegion
	}
	if region == "" {
		return errRegionUnknown
	}
	revokeCtx, cancel := context.WithTimeout(ctx, revokeTimeout)
	defer cancel()
	return deps.RevokeToken(revokeCtx, region, g.accessToken)
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
