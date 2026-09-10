package datadogauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ClientName は Dynamic Client Registration で登録するクライアント名。
const ClientName = "thief"

// registerPath は Dynamic Client Registration のエンドポイントのパス。
const registerPath = "/api/v2/oauth2/register"

// grantTypes は登録するクライアントが使う grant type。認可コードの引き換えと
// リフレッシュの両方を行うため 2 つとも登録する。
var grantTypes = []string{"authorization_code", "refresh_token"}

// registrationRequest は Dynamic Client Registration のリクエスト本文。
type registrationRequest struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types"`
}

// RegisterClient は POST https://api.{site}/api/v2/oauth2/register でクライアントを
// 登録し、発行された client_id と登録された redirect_uris を返す。
//
// 登録は 1 つのクライアントに対して 1 回だけ行い、以後は保存した登録を再利用する
// (PrepareLogin を参照)。redirectURIs には、そのクライアントが使いうる URI を
// すべて含めて渡すこと。Datadog の DCR には登録済みの redirect_uris を後から追加する
// エンドポイントが無く、追加登録は新しい client_id の発行になって、それ以前に発行された
// トークンを実質的に無効化するためである。
func RegisterClient(ctx context.Context, site, clientName string, redirectURIs []string) (*ClientCredentials, error) {
	return Client{}.RegisterClient(ctx, site, clientName, redirectURIs)
}

// RegisterClient は RegisterClient の本体。
func (c Client) RegisterClient(ctx context.Context, site, clientName string, redirectURIs []string) (*ClientCredentials, error) {
	base, err := apiBaseURL(site)
	if err != nil {
		return nil, err
	}
	if clientName == "" {
		return nil, errors.New("register datadog oauth client: client name is empty")
	}
	if len(redirectURIs) == 0 {
		return nil, errors.New("register datadog oauth client: redirect URIs are empty")
	}

	body := registrationRequest{
		ClientName:   clientName,
		RedirectURIs: redirectURIs,
		GrantTypes:   grantTypes,
	}

	// RFC 7591 §3.2.1 と Datadog の DCR は登録の成功を 201 Created で返す。
	var creds ClientCredentials
	if err := c.postJSON(ctx, base+registerPath, body, http.StatusCreated, &creds); err != nil {
		return nil, fmt.Errorf("register datadog oauth client: %w", err)
	}
	// 応答が redirect_uris / client_name を省いた場合は、登録を要求した値で補う。
	// 登録された値はこちらが送った値であり、応答の省略を理由に登録済みのクライアントを
	// 使えなくする必要は無い。
	if len(creds.RedirectURIs) == 0 {
		creds.RedirectURIs = redirectURIs
	}
	if creds.ClientName == "" {
		creds.ClientName = clientName
	}
	if err := creds.Validate(); err != nil {
		return nil, fmt.Errorf("register datadog oauth client: %w", err)
	}
	return &creds, nil
}
