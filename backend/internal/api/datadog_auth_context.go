package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

// ErrDatadogNoCredentials は Datadog を呼ぶ資格情報が 1 つも使えないことを表す。
// OAuth トークンが無い (または使えない) 状態で、静的キー
// (DATADOG_API_KEY / DATADOG_APP_KEY) も揃っていない場合に返る。
var ErrDatadogNoCredentials = errors.New("no usable Datadog credentials")

// datadogErrorBodyLogBytes は Datadog のエラー応答の本文をログへ載せる際の上限。
const datadogErrorBodyLogBytes = 512

// datadogParentOrg は親組織を表す org の値。Sub Organization は識別子 (パブリック ID)
// で表し、空文字は親組織を意味する。
const datadogParentOrg = ""

// datadogAuthDeps は Datadog の OAuth 認証状態の解決とログイン系エンドポイントが呼ぶ
// 外部処理をまとめる。いずれも Datadog への接続または config.Dir()/datadog 配下の
// 読み書きを伴い、テストからは実行できないため関数値で差し替える
// (internal/api の ssoLoginDeps と同じ理由)。now は期限判定の基準時刻を固定するために持つ。
type datadogAuthDeps struct {
	loadToken     func(site, org string) (*datadogauth.TokenSet, bool, error)
	saveToken     func(site, org string, tok *datadogauth.TokenSet) error
	loadClient    func(site, org string) (*datadogauth.ClientCredentials, bool, error)
	refreshToken  func(ctx context.Context, site, clientID, refreshToken string) (*datadogauth.TokenSet, error)
	prepareLogin  func(ctx context.Context, p datadogauth.PrepareParams) (*datadogauth.Login, error)
	completeLogin func(ctx context.Context, login *datadogauth.Login, state, code string) (*datadogauth.TokenSet, error)
	logout        func(site, org string) error
	now           func() time.Time
}

// defaultDatadogAuthDeps は本番で使う実装を返す。
func defaultDatadogAuthDeps() datadogAuthDeps {
	return datadogAuthDepsFrom(datadogauth.DefaultDeps())
}

// datadogAuthDepsFrom は datadogauth.Deps を本パッケージの形へ写す。
func datadogAuthDepsFrom(deps datadogauth.Deps) datadogAuthDeps {
	return datadogAuthDeps{
		loadToken:    deps.LoadToken,
		saveToken:    deps.SaveToken,
		loadClient:   deps.LoadClient,
		refreshToken: deps.RefreshToken,
		prepareLogin: func(ctx context.Context, p datadogauth.PrepareParams) (*datadogauth.Login, error) {
			return datadogauth.PrepareLogin(ctx, p, deps)
		},
		completeLogin: func(ctx context.Context, login *datadogauth.Login, state, code string) (*datadogauth.TokenSet, error) {
			return datadogauth.CompleteLogin(ctx, login, state, code, deps)
		},
		logout: func(site, org string) error { return datadogauth.Logout(site, org, deps) },
		now:    deps.Now,
	}
}

// datadogAuthContext は org (空文字は親組織) の Datadog API 呼び出しに使う認証
// コンテキストを組み立てる。
//
// 保存済みの OAuth トークン (config.Dir()/datadog/token_<site>[_<org>].json) を毎回
// 読み直す。起動時に固定した context を使い回さないのは、CLI
// (thief datadog auth login/logout) がサーバプロセスの外でトークンを更新するためで、
// ファイルを唯一の真実源とする (internal/pricecache が TTL を持たずファイルを都度読むのと
// 同じ考え方)。
//
// 状態と挙動の対応は次のとおり。静的キーへ倒すときは、通常運用 (未ログイン) を除いて
// 必ず slog.Warn を残す。黙って倒すと、認証が壊れていることに運用者が気付けなくなる。
// 静的キーへ倒せるのは親組織のときだけで、Sub Organization では倒さない
// (datadogFallbackContext を参照)。
//
//   - トークン有効: そのトークンで続行する (ログ不要)
//   - トークン無し + 静的キーあり: 静的キーで続行する (ログ不要)
//   - トークン無し + 静的キー無し: ErrDatadogNoCredentials を返す
//   - 期限切れ → リフレッシュ成功: 新しいトークンで続行する
//   - 期限切れ → リフレッシュ失敗: 警告してから静的キーへ倒す (無ければエラー)
//   - トークンファイル破損: 未ログインと区別して警告してから静的キーへ倒す (無ければエラー)
func (s *Server) datadogAuthContext(ctx context.Context, org string) (context.Context, error) {
	site := s.cfg.Datadog.Site

	tok, ok, err := s.ddAuth.loadToken(site, org)
	if err != nil {
		// 読めたが壊れている (不正 JSON、access_token 欠落など)。ファイルが無い
		// 「未ログイン」とは別の事態なので、静かに静的キーへ倒さず警告を残す。
		slog.Warn("stored datadog oauth token is unusable", "site", site, "org", org, "err", err)
		return s.datadogFallbackContext(ctx, org, "the stored Datadog OAuth token is broken; log in to Datadog again")
	}
	if !ok {
		return s.datadogFallbackContext(ctx, org, "no Datadog OAuth token is stored; "+datadogLoginHint(org))
	}

	if !tok.IsExpired(s.ddAuth.now()) {
		return ddclient.NewOAuthContext(ctx, tok.AccessTokenValue()), nil
	}

	fresh, err := s.refreshDatadogToken(ctx, org, tok)
	if err != nil {
		slog.Warn("refresh datadog oauth token failed", "site", site, "org", org, "err", err)
		return s.datadogFallbackContext(ctx, org, "the stored Datadog OAuth token is expired and could not be refreshed; log in to Datadog again")
	}
	return ddclient.NewOAuthContext(ctx, fresh.AccessTokenValue()), nil
}

// datadogFallbackContext は OAuth トークンが使えないときの退避先を返す。
//
// 親組織 (org が空) では従来どおり静的キーへ倒す。Sub Organization では倒さず、
// reason と org を添えたエラーを返す。静的キー (DATADOG_API_KEY / DATADOG_APP_KEY) は
// org 非依存のグローバルな設定であり、Sub Organization の文脈でこれを使うと、求められた
// Sub Organization ではなく親組織 (または静的キーが属する別の組織) のデータを黙って
// 返すことになる。Sub Organization 同士はデータが完全に分離されているため、誤りに
// 気付ける手掛かりが応答に残らない。
func (s *Server) datadogFallbackContext(ctx context.Context, org, reason string) (context.Context, error) {
	if org != datadogParentOrg {
		return nil, fmt.Errorf("%w for Datadog sub organization %q: %s", ErrDatadogNoCredentials, org, reason)
	}
	return s.datadogStaticKeyContext(ctx, reason)
}

// datadogLoginHint は再ログインに使うコマンドを文言にする。
func datadogLoginHint(org string) string {
	if org == datadogParentOrg {
		return "run 'thief datadog auth login'"
	}
	return fmt.Sprintf("run 'thief datadog auth login --org %s'", org)
}

// datadogStaticKeyContext は静的キー (DATADOG_API_KEY / DATADOG_APP_KEY) の認証
// コンテキストを返す。Usage Metering は API key と App key の両方を要求するため、
// 片方でも欠けていれば「静的キー無し」として reason を添えたエラーにする。
func (s *Server) datadogStaticKeyContext(ctx context.Context, reason string) (context.Context, error) {
	apiKey, appKey := s.cfg.DatadogAPIKey(), s.cfg.DatadogAppKey()
	if apiKey == "" || appKey == "" {
		return nil, fmt.Errorf("%w: %s, and DATADOG_API_KEY / DATADOG_APP_KEY are not both set", ErrDatadogNoCredentials, reason)
	}
	return ddclient.NewContext(ctx, apiKey, appKey), nil
}

// refreshDatadogToken は期限切れの tok を更新して保存し、新しいトークンを返す。
//
// 同一プロセス内で重なった更新は singleflight で 1 回に集約する。ただし
// singleflight はプロセスをまたげず、CLI (thief datadog auth refresh) とサーバが
// 同時に同じトークンを更新しようとする競合は防げない。リフレッシュトークンを
// ローテーションする認可サーバでは後着が失敗し、実際には他プロセスが更新に成功して
// いるのに「再ログインが必要」と誤診断しうる。そのため次の 2 回、ディスクを読み直す。
//
//  1. トークンエンドポイントを叩く直前。既に他プロセスが更新済みなら HTTP を出さない。
//  2. 失敗を確定させる直前。他プロセスの成功が自分の失敗の原因だった場合を拾う。
func (s *Server) refreshDatadogToken(ctx context.Context, org string, tok *datadogauth.TokenSet) (*datadogauth.TokenSet, error) {
	site := s.cfg.Datadog.Site
	// org ごとに別のトークンを更新するため、集約の単位も org で分ける。site には
	// アンダースコアが現れない (datadogauth.ValidateSite) ので区切りに使える。
	v, err, _ := s.ddRefresh.Do(site+"_"+org, func() (any, error) {
		if fresh, ok := s.freshDatadogTokenOnDisk(site, org); ok {
			return fresh, nil
		}
		fresh, err := s.refreshDatadogTokenOnce(ctx, site, org, tok)
		if err != nil {
			if recovered, ok := s.freshDatadogTokenOnDisk(site, org); ok {
				return recovered, nil
			}
			return nil, err
		}
		return fresh, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*datadogauth.TokenSet), nil
}

// freshDatadogTokenOnDisk はディスク上のトークンを読み直し、期限内であれば返す。
// 読めない場合と期限切れの場合は ok=false を返す (呼び出し側は通常の更新へ進む)。
// ここでの読み取りエラーは握り潰してよい。直前に datadogAuthContext が同じファイルを
// 読んで警告済みか、これから走る更新の成否で結果が決まるためである。
func (s *Server) freshDatadogTokenOnDisk(site, org string) (*datadogauth.TokenSet, bool) {
	tok, ok, err := s.ddAuth.loadToken(site, org)
	if err != nil || !ok || tok.IsExpired(s.ddAuth.now()) {
		return nil, false
	}
	return tok, true
}

// refreshDatadogTokenOnce はトークンエンドポイントを 1 回呼び、結果を保存する。
func (s *Server) refreshDatadogTokenOnce(ctx context.Context, site, org string, tok *datadogauth.TokenSet) (*datadogauth.TokenSet, error) {
	if tok.RefreshTokenValue() == "" {
		return nil, fmt.Errorf("%w; %s again", datadogauth.ErrRefreshTokenMissing, datadogLoginHint(org))
	}

	clientID := tok.ClientID
	if clientID == "" {
		creds, ok, err := s.ddAuth.loadClient(site, org)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: no stored client registration for site %q org %q", datadogauth.ErrClientIncomplete, site, org)
		}
		clientID = creds.ClientID
	}

	fresh, err := s.ddAuth.refreshToken(ctx, site, clientID, tok.RefreshTokenValue())
	if err != nil {
		return nil, err
	}
	if err := fresh.Validate(); err != nil {
		return nil, err
	}
	// 応答が refresh_token を省いた場合 (RFC 6749 §6 で OPTIONAL) は元の値を引き継ぐ。
	if fresh.RefreshTokenValue() == "" {
		fresh.RefreshToken = tok.RefreshToken
	}
	if fresh.IssuedAt.IsZero() {
		fresh.IssuedAt = s.ddAuth.now().UTC()
	}
	if fresh.ClientID == "" {
		fresh.ClientID = clientID
	}

	if err := s.ddAuth.saveToken(site, org, fresh); err != nil {
		return nil, err
	}
	return fresh, nil
}

// datadogCall は datadogAuthContext が組み立てた authCtx で call を実行する。
// OAuth トークンで呼んで権限不足 (403) になった場合に限り、警告を残してから静的キーで
// 1 回だけやり直す。OAuth のスコープが対象 API を覆っていない構成でも、静的キーが
// あれば従来どおり動くようにするためである。org が親組織 (空文字) でない場合は
// やり直さない (datadogFallbackContext と同じ理由)。
//
// 静的キーの認証コンテキストは authCtx ではなく ctx (認証情報を載せる前のリクエストの
// context) から組み立てる。authCtx を親にすると Bearer トークンと API キーの両方が
// 同じリクエストに載ってしまう。
//
// 新しい Datadog API を呼び足す場合は、その API が OAuth に対応しているかを確認すること。
// 現在呼んでいる Usage Metering (/api/v2/usage/*) は OAuth 対応であり、非対応 API の
// 除外リストは持たない。
func (s *Server) datadogCall(ctx, authCtx context.Context, org string, call func(context.Context) (any, error)) (any, error) {
	v, err := call(authCtx)
	if err == nil || !ddclient.HasOAuthToken(authCtx) || !ddclient.IsForbidden(err) {
		return v, err
	}
	if org != datadogParentOrg {
		slog.Warn("datadog rejected the oauth token with 403 for a sub organization; the static API keys are not used as a fallback",
			"site", s.cfg.Datadog.Site, "org", org, "body", ddclient.ErrorBody(err, datadogErrorBodyLogBytes))
		return v, err
	}

	staticCtx, staticErr := s.datadogStaticKeyContext(ctx, "the Datadog OAuth token was rejected with 403")
	if staticErr != nil {
		slog.Warn("datadog rejected the oauth token with 403 and no static keys are available",
			"site", s.cfg.Datadog.Site, "body", ddclient.ErrorBody(err, datadogErrorBodyLogBytes))
		return v, err
	}
	slog.Warn("datadog rejected the oauth token with 403; falling back to the static API keys",
		"site", s.cfg.Datadog.Site, "body", ddclient.ErrorBody(err, datadogErrorBodyLogBytes))
	return call(staticCtx)
}
