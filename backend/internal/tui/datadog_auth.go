package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

// ErrDatadogNoCredentials は Datadog を呼ぶ資格情報が 1 つも使えないことを表す。
// Web UI の API サーバが返す DATADOG_NO_CREDENTIALS と同じ意味で、TUI は通知行に出す。
var ErrDatadogNoCredentials = errors.New("no usable Datadog credentials")

// DatadogAuth は Datadog の組織ごとの認証コンテキストを組み立てる。org が空文字なら
// 親組織を表す。TUI のサービスはこのインターフェース越しに Datadog API を呼ぶ。
type DatadogAuth interface {
	Context(ctx context.Context, org string) (context.Context, error)
	// StaticContext は静的キー (DATADOG_API_KEY / DATADOG_APP_KEY) の認証コンテキストを返す。
	// 両方が設定されていなければ ok=false を返す。
	StaticContext(ctx context.Context) (context.Context, bool)
}

// datadogAuth は保存済みの OAuth トークンと静的キーから認証コンテキストを組み立てる。
type datadogAuth struct {
	site   string
	apiKey string
	appKey string
	deps   datadogauth.Deps
	now    func() time.Time
}

// NewDatadogAuth は本番用の DatadogAuth を返す。site は datadoghq.com など。
func NewDatadogAuth(site, apiKey, appKey string) DatadogAuth {
	return newDatadogAuth(site, apiKey, appKey, datadogauth.DefaultDeps())
}

// newDatadogAuth は依存を差し替えられる DatadogAuth を返す。テストで使う。
func newDatadogAuth(site, apiKey, appKey string, deps datadogauth.Deps) *datadogAuth {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &datadogAuth{site: site, apiKey: apiKey, appKey: appKey, deps: deps, now: deps.Now}
}

// Context は org の認証コンテキストを返す。OAuth トークンが期限内ならそれを使い、
// 期限切れなら更新する。OAuth が使えない親組織では静的キーへ倒す。Sub Organization では
// 静的キーへ倒さない (静的キーは親組織のデータを返しうるため)。
func (a *datadogAuth) Context(ctx context.Context, org string) (context.Context, error) {
	tok, ok, err := a.deps.LoadToken(a.site, org)
	if err != nil {
		return a.fallback(ctx, org, "the stored Datadog OAuth token is broken; log in to Datadog again")
	}
	if !ok || tok == nil {
		return a.fallback(ctx, org, "no Datadog OAuth token is stored")
	}
	if !tok.IsExpired(a.now()) {
		return ddclient.NewOAuthContext(ctx, tok.AccessTokenValue()), nil
	}

	fresh, err := a.refresh(ctx, org, tok)
	if err != nil {
		return a.fallback(ctx, org, "the stored Datadog OAuth token is expired and could not be refreshed; log in to Datadog again")
	}
	return ddclient.NewOAuthContext(ctx, fresh.AccessTokenValue()), nil
}

// refresh は期限切れのトークンを更新して保存する。API サーバと同じく、応答が
// refresh_token を省いた場合は元の値を引き継ぎ、client_id も引き継ぐ。
func (a *datadogAuth) refresh(ctx context.Context, org string, tok *datadogauth.TokenSet) (*datadogauth.TokenSet, error) {
	clientID := tok.ClientID
	if clientID == "" {
		creds, ok, err := a.deps.LoadClient(a.site, org)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: no stored client registration for site %q org %q", datadogauth.ErrClientIncomplete, a.site, org)
		}
		clientID = creds.ClientID
	}
	fresh, err := a.deps.RefreshToken(ctx, a.site, clientID, tok.RefreshTokenValue())
	if err != nil {
		return nil, err
	}
	if err := fresh.Validate(); err != nil {
		return nil, err
	}
	if fresh.RefreshTokenValue() == "" {
		fresh.RefreshToken = tok.RefreshToken
	}
	if fresh.IssuedAt.IsZero() {
		fresh.IssuedAt = a.now().UTC()
	}
	if fresh.ClientID == "" {
		fresh.ClientID = clientID
	}
	if err := a.deps.SaveToken(a.site, org, fresh); err != nil {
		return nil, err
	}
	return fresh, nil
}

// StaticContext は静的キーの認証コンテキストを返す。両方のキーが揃っているときだけ
// ok=true にする。
func (a *datadogAuth) StaticContext(ctx context.Context) (context.Context, bool) {
	if a.apiKey == "" || a.appKey == "" {
		return nil, false
	}
	return ddclient.NewContext(ctx, a.apiKey, a.appKey), true
}

// datadogCall は auth で組み立てた認証コンテキストで call を実行する。OAuth トークンで
// 呼んで 403 になった場合は、親組織に限り静的キーで 1 回だけやり直す。Sub Organization
// では静的キーが別の組織のデータを返しうるため、やり直さない。
func datadogCall[T any](
	ctx context.Context,
	auth DatadogAuth,
	org string,
	call func(context.Context) (T, error),
) (T, error) {
	authCtx, err := auth.Context(ctx, org)
	if err != nil {
		var zero T
		return zero, err
	}
	v, err := call(authCtx)
	if err == nil || !ddclient.HasOAuthToken(authCtx) || !ddclient.IsForbidden(err) || org != "" {
		return v, err
	}
	staticCtx, ok := auth.StaticContext(ctx)
	if !ok {
		return v, err
	}
	return call(staticCtx)
}

// fallback は OAuth が使えないときの退避先を返す。親組織だけ静的キーを使える。
func (a *datadogAuth) fallback(ctx context.Context, org, reason string) (context.Context, error) {
	if org != "" {
		return nil, fmt.Errorf("%w for Datadog sub organization %q: %s; run 'thief datadog auth login --org %s'",
			ErrDatadogNoCredentials, org, reason, org)
	}
	if a.apiKey == "" || a.appKey == "" {
		return nil, fmt.Errorf("%w: %s, and DATADOG_API_KEY / DATADOG_APP_KEY are not both set; run 'thief datadog auth login'",
			ErrDatadogNoCredentials, reason)
	}
	return ddclient.NewContext(ctx, a.apiKey, a.appKey), nil
}
