package datadogauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"time"
)

var (
	// ErrStateMismatch はコールバックで返った state が認可要求で送った値と一致しない
	// ことを表す (RFC 6749 §10.12 の CSRF 対策)。
	ErrStateMismatch = errors.New("datadog oauth state mismatch")
	// ErrRedirectURINotRegistered は、登録済みクライアントの redirect_uris に今回
	// 使う URI が含まれていないことを表す。
	ErrRedirectURINotRegistered = errors.New("redirect URI is not registered for the stored datadog oauth client")
	// ErrRefreshTokenMissing は、期限切れのトークンにリフレッシュトークンが無く
	// 更新できないことを表す。
	ErrRefreshTokenMissing = errors.New("datadog oauth token is expired and has no refresh token")
	// ErrLoginIncomplete は PrepareLogin を経ていない Login を渡されたことを表す。
	ErrLoginIncomplete = errors.New("datadog oauth login is incomplete: it must be built by PrepareLogin")
)

// Deps は PrepareLogin / CompleteLogin / EnsureFreshToken / Logout が呼ぶ外部処理を
// まとめる。RegisterClient / ExchangeCode / RefreshToken は Datadog への接続を、
// Load・Save・Delete 系は config.Dir()/datadog への読み書きを伴い、テストからは
// 実行できないため差し替える。Now は期限判定の基準時刻を固定するために差し替える。
// これらは元から自由関数であり、絞り込む対象の具象型が無いため、コンシューマ定義の
// インターフェースではなく関数値で持つ (internal/ssoauth の Deps と同じ形)。
type Deps struct {
	RegisterClient func(ctx context.Context, site, clientName string, redirectURIs []string) (*ClientCredentials, error)
	ExchangeCode   func(ctx context.Context, site, clientID, code, redirectURI, codeVerifier string) (*TokenSet, error)
	RefreshToken   func(ctx context.Context, site, clientID, refreshToken string) (*TokenSet, error)
	LoadClient     func(site string) (*ClientCredentials, bool, error)
	SaveClient     func(site string, creds *ClientCredentials) error
	LoadToken      func(site string) (*TokenSet, bool, error)
	SaveToken      func(site string, tok *TokenSet) error
	DeleteToken    func(site string) error
	DeleteClient   func(site string) error
	Now            func() time.Time
}

// DefaultDeps は本番で使う実装を返す。保存先のディレクトリは呼び出しのたびに Dir() で
// 解決する。$XDG_CONFIG_HOME の変更に追従させるためと、Dir() の失敗を DefaultDeps の
// 呼び出し時点ではなく実際の読み書きの時点のエラーとして返すためである。
func DefaultDeps() Deps {
	return Deps{
		RegisterClient: RegisterClient,
		ExchangeCode:   ExchangeCode,
		RefreshToken:   Refresh,
		LoadClient: func(site string) (*ClientCredentials, bool, error) {
			dir, err := Dir()
			if err != nil {
				return nil, false, err
			}
			return LoadClient(dir, site)
		},
		SaveClient: func(site string, creds *ClientCredentials) error {
			dir, err := Dir()
			if err != nil {
				return err
			}
			return SaveClient(dir, site, creds)
		},
		LoadToken: func(site string) (*TokenSet, bool, error) {
			dir, err := Dir()
			if err != nil {
				return nil, false, err
			}
			return LoadToken(dir, site)
		},
		SaveToken: func(site string, tok *TokenSet) error {
			dir, err := Dir()
			if err != nil {
				return err
			}
			return SaveToken(dir, site, tok)
		},
		DeleteToken: func(site string) error {
			dir, err := Dir()
			if err != nil {
				return err
			}
			return DeleteToken(dir, site)
		},
		DeleteClient: func(site string) error {
			dir, err := Dir()
			if err != nil {
				return err
			}
			return DeleteClient(dir, site)
		},
		Now: time.Now,
	}
}

// PrepareParams は PrepareLogin の入力。
type PrepareParams struct {
	// Site は Datadog の site (datadoghq.com など)。
	Site string
	// RedirectURI は今回のログインでコールバックを受け取る URI。
	RedirectURI string
	// RegisterRedirectURIs はクライアントを新規登録するときに登録する URI 一覧。
	// RedirectURI を必ず含める。登録は 1 回だけなので、この呼び出し以外で使う URI
	// (CLI とサーバの両方) をここでまとめて渡す。
	RegisterRedirectURIs []string
	// Scopes は認可要求で求めるスコープ。空の場合は DefaultScopes() を使う。
	Scopes []string
}

// Login は PrepareLogin が返す、利用者への提示と CompleteLogin に必要な中間状態。
// code_verifier は認可コードの引き換えでしか使わない秘密なので外へ出さない。
type Login struct {
	Site             string
	ClientID         string
	RedirectURI      string
	State            string
	AuthorizationURL string

	verifier secret
}

// PrepareLogin は認可の準備を行い、利用者に開かせる認可 URL を組み立てて返す。
//
// クライアント登録ファイルが存在すればそのまま再利用し、存在しない場合だけ
// p.RegisterRedirectURIs をまとめて 1 回登録する。Datadog の DCR には登録済みの
// redirect_uris を後から追加するエンドポイントが無く、追加登録は新しい client_id の
// 発行になって既存のトークンを実質的に無効化するためである。再利用したクライアントの
// redirect_uris に p.RedirectURI が含まれない場合は、再登録せず
// ErrRedirectURINotRegistered を返す (無効化を伴う再登録は利用者の明示的な
// ログアウトの後に行わせる)。
func PrepareLogin(ctx context.Context, p PrepareParams, deps Deps) (*Login, error) {
	if err := ValidateSite(p.Site); err != nil {
		return nil, err
	}
	if p.RedirectURI == "" {
		return nil, errors.New("prepare datadog oauth login: redirect URI is empty")
	}
	if !slices.Contains(p.RegisterRedirectURIs, p.RedirectURI) {
		return nil, fmt.Errorf("prepare datadog oauth login: redirect URI %q is not in the URIs to register", p.RedirectURI)
	}

	creds, ok, err := deps.LoadClient(p.Site)
	if err != nil {
		return nil, err
	}
	if !ok {
		creds, err = deps.RegisterClient(ctx, p.Site, ClientName, p.RegisterRedirectURIs)
		if err != nil {
			return nil, err
		}
		if err := creds.Validate(); err != nil {
			return nil, err
		}
		if err := deps.SaveClient(p.Site, creds); err != nil {
			return nil, err
		}
	}

	if !creds.HasRedirectURI(p.RedirectURI) {
		return nil, fmt.Errorf("%w: %q (registered: %v); run 'thief datadog auth logout' and log in again to register it", ErrRedirectURINotRegistered, p.RedirectURI, creds.RedirectURIs)
	}

	pkce, err := NewPKCE()
	if err != nil {
		return nil, err
	}
	state, err := newState()
	if err != nil {
		return nil, err
	}
	authURL, err := BuildAuthorizationURL(p.Site, creds.ClientID, p.RedirectURI, state, pkce, p.Scopes)
	if err != nil {
		return nil, err
	}

	return &Login{
		Site:             p.Site,
		ClientID:         creds.ClientID,
		RedirectURI:      p.RedirectURI,
		State:            state,
		AuthorizationURL: authURL,
		verifier:         pkce.Verifier,
	}, nil
}

// CompleteLogin はコールバックで受け取った state を検証してから認可コードをトークンへ
// 引き換え、保存したトークンを返す。
//
// state の比較は subtle.ConstantTimeCompare で行う。値の一致を時間差から推測させないため
// である。
func CompleteLogin(ctx context.Context, login *Login, state, code string, deps Deps) (*TokenSet, error) {
	if login == nil || login.State == "" || login.verifier.value() == "" {
		return nil, ErrLoginIncomplete
	}
	if subtle.ConstantTimeCompare([]byte(login.State), []byte(state)) != 1 {
		return nil, ErrStateMismatch
	}
	if code == "" {
		return nil, errors.New("complete datadog oauth login: authorization code is empty")
	}

	tok, err := deps.ExchangeCode(ctx, login.Site, login.ClientID, code, login.RedirectURI, login.verifier.value())
	if err != nil {
		return nil, err
	}
	if err := tok.Validate(); err != nil {
		return nil, err
	}
	stampToken(tok, login.ClientID, deps)

	if err := deps.SaveToken(login.Site, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// EnsureFreshToken は保存済みのトークンを、期限切れならリフレッシュしたうえで返す。
//
// 未ログイン (トークンファイルが無い) の場合は (nil, false, nil) を返す。呼び出し側は
// これを見て静的キー (DATADOG_API_KEY / DATADOG_APP_KEY) へフォールバックできる。
// 保存されたトークンが壊れている場合と、リフレッシュに失敗した場合はエラーを返す
// (どちらも利用者の対処が要るので、未ログインと同じ扱いにして黙って静的キーへ倒さない)。
func EnsureFreshToken(ctx context.Context, site string, deps Deps) (*TokenSet, bool, error) {
	if err := ValidateSite(site); err != nil {
		return nil, false, err
	}
	tok, ok, err := deps.LoadToken(site)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if !tok.IsExpired(deps.Now()) {
		return tok, true, nil
	}
	if tok.RefreshTokenValue() == "" {
		return nil, false, fmt.Errorf("%w; run 'thief datadog auth login' again", ErrRefreshTokenMissing)
	}

	clientID := tok.ClientID
	if clientID == "" {
		creds, ok, err := deps.LoadClient(site)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, fmt.Errorf("%w: no stored client registration for %q", ErrClientIncomplete, site)
		}
		clientID = creds.ClientID
	}

	fresh, err := deps.RefreshToken(ctx, site, clientID, tok.RefreshTokenValue())
	if err != nil {
		return nil, false, err
	}
	if err := fresh.Validate(); err != nil {
		return nil, false, err
	}
	// 応答が refresh_token を省いた場合 (RFC 6749 §6 で OPTIONAL) は元の値を引き継ぐ。
	if fresh.RefreshTokenValue() == "" {
		fresh.RefreshToken = tok.RefreshToken
	}
	stampToken(fresh, clientID, deps)

	if err := deps.SaveToken(site, fresh); err != nil {
		return nil, false, err
	}
	return fresh, true, nil
}

// Logout はローカルに保存したトークンとクライアント登録を削除する。Datadog 側の
// トークンを失効させる revoke エンドポイントは確認できていないため、削除はローカルに
// 限られる。
//
// 片方の削除に失敗しても、もう片方の削除は試みる。認証情報が中途半端に残る状態を
// 避けるためで、失敗した分は errors.Join でまとめて返す。
func Logout(site string, deps Deps) error {
	if err := ValidateSite(site); err != nil {
		return err
	}
	return errors.Join(deps.DeleteToken(site), deps.DeleteClient(site))
}

// stampToken は保存の前に、トークン側で欠けている値を補う。
//
// IssuedAt は本来トークンエンドポイントの応答を受け取った時刻 (requestToken が入れる)
// だが、Deps.ExchangeCode / Deps.RefreshToken は差し替え可能なので、欠けている場合に
// deps.Now() で補って期限の判定を成り立たせる。ClientID は保存後にリフレッシュで使う。
func stampToken(tok *TokenSet, clientID string, deps Deps) {
	if tok.IssuedAt.IsZero() {
		tok.IssuedAt = deps.Now().UTC()
	}
	if tok.ClientID == "" {
		tok.ClientID = clientID
	}
}
