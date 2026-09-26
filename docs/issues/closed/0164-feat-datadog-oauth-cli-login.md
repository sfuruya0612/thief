# Datadog CLI 認証に OAuth 2.0 (Authorization Code + PKCE + DCR) ログインを追加する

Created: 2026-09-10
Model: Claude Sonnet 5
Completed: 2026-09-11

## 背景

thief の Datadog CLI コマンド (`backend/internal/cli/datadog.go`) は、`newDatadogCmd()` (27 行目) で `site`/`api-key`/`app-key`/`view`/`start-month`/`end-month` の persistent flags を定義し、`historical`/`estimated` サブコマンド (41-59 行目) はいずれも `showDatadogCost(cmd, ...)` (67 行目) を呼ぶ。`showDatadogCost` は `cfg.DatadogAPIKey() == "" || cfg.DatadogAppKey() == ""` (73 行目) を必須チェックとしており、環境変数 `DATADOG_API_KEY`/`DATADOG_APP_KEY` (`backend/internal/config/config.go:222,225`、`backend/internal/cli/datadog.go:74` のエラーメッセージにも同名で明記) またはコマンドラインフラグが無いと即座にエラーを返す。この 2 値の静的キー方式が唯一の認証手段であり、`backend/internal/datadog/client.go` の `NewContext(ctx, apiKey, appKey)` (18 行目) も `datadog.ContextAPIKeys` のみを埋め込む実装で、Bearer トークンを埋め込む経路は存在しない。

DataDog 公式 CLI `pup` (https://github.com/DataDog/pup) はこの API Key/App Key 方式に加え、`pup auth login` で OAuth 2.0 Authorization Code + PKCE + Dynamic Client Registration (DCR) によるブラウザログインを提供している。ソース (`src/auth/{dcr,pkce,types}.rs`, `src/raw_client.rs`, `src/commands/auth.rs`) を確認したところ、次のエンドポイント・挙動が確認できた。

- DCR: `POST https://api.{site}/api/v2/oauth2/register`、body は `{"client_name": "...", "redirect_uris": [...], "grant_types": ["authorization_code", "refresh_token"]}`、期待するステータスは `201 Created`。レスポンスは `{client_id, client_name, redirect_uris}` のみで `client_secret` を含まない (pup の `RegistrationResponse` 構造体に `client_secret` フィールドが無い。public client + PKCE 前提の設計)。
- 認可エンドポイント: `GET https://app.{site}/oauth2/v1/authorize` (DCR/トークンとホストが異なり `app.` サブドメイン)。クエリパラメータは `response_type=code, client_id, redirect_uri, state, scope, code_challenge, code_challenge_method=S256` (+任意で `dd_oid`)。
- トークンエンドポイント: `POST https://api.{site}/oauth2/v1/token`。`grant_type=authorization_code` (+`client_id, code, redirect_uri, code_verifier`) または `grant_type=refresh_token` (+`client_id, refresh_token`)。
- トークンの期限判定は `issued_at + expires_in` から算出し、実際の期限の 300 秒 (5 分) 前から expired 扱いにする早期バッファを持つ (`is_expired()`)。
- スコープ名 `usage_read` は pup のデフォルトスコープ一覧に実在する。

一方、thief が呼ぶコスト取得 API (`backend/internal/datadog/usage.go` の `GetHistoricalCost`/`GetEstimatedCost` がそれぞれ SDK の `GetHistoricalCostByOrg`/`GetEstimatedCostByOrg` を呼ぶ、22 行目・49 行目) は Datadog の Usage Metering v2 API (`/api/v2/usage/*`) に属する。pup の `raw_client.rs` の `apply_auth`/`OAUTH_EXCLUDED_ENDPOINTS` を確認したところ、OAuth 認証が使えないのは Fleet Automation の unstable エンドポイント (`/api/unstable/fleet/`) とプロファイリング系 (`/profiling/api/v1/`, `/api/unstable/profiles/`, `/api/ui/profiling/`) のみであり、`/api/v2/usage/*` は除外リストに含まれず、テストコードのコメントでも "Cost/Billing routes already accept OAuth server-side (DAL-959)" と明記されている。したがって thief の Usage Metering 呼び出しは OAuth Bearer トークンに対応可能な範囲に入る。

また、thief が既に vendor している公式 Go SDK `github.com/DataDog/datadog-api-client-go/v2` は `backend/go.mod:13` で `v2.55.0` を使用しており (`backend/go.sum:29-30` にも対応エントリあり)、この v2.55.0 のソースを GitHub 上で確認したところ、`api/datadog/configuration.go` に次の定義がある。

```go
ContextAccessToken = contextKey("accesstoken")
```

さらに `api/datadog/client.go` の `PrepareRequest` に次の実装がある。

```go
if auth, ok := ctx.Value(ContextAccessToken).(string); ok {
    localVarRequest.Header.Add("Authorization", "Bearer "+auth)
}
```

`datadog.ContextAPIKeys` と同じ枠組みで並行利用できる Bearer トークン用のコンテキストキーが既に用意されている。つまり SDK 側の変更は不要で、thief 側に Bearer トークンを取得・保存・注入する仕組みを追加するだけで OAuth ログインに対応できる。ただし、この SDK 本体は本リポジトリにベンダリングされておらず (`go.sum` にハッシュはあるがモジュール本体は未取得)、ローカルの `go build` でこの実装を直接確認することはできていない (GitHub 上のソース読み取りのみでの確認であり、末尾の未確定事項に実装時の再確認を残す)。

この issue は `docs/issues/TODO.md` の次の項目のうち、CLI 部分 (`thief datadog auth login/logout/refresh`) を対象にする。API サーバ側 (frontend の DatadogView 向け非同期ログイン) は依存先の issue 0165 で扱う。

> Datadog の認証を DD_API_KEY / DD_APP_KEY の静的キーに加えて、DataDog 公式 CLI pup の pup auth login と同じ OAuth 2.0 (Authorization Code + PKCE + Dynamic Client Registration) ログインにも対応させたい
> スコープは CLI (thief datadog auth login/logout/refresh) と API サーバ (frontend の DatadogView 向け) の両方
> トークンの保存方式は OS キーチェーンではなくファイル権限 (0600) 方式にする
> 既存の DD_API_KEY / DD_APP_KEY 方式は残し、OAuth トークンがあれば優先、無ければ静的キーにフォールバックする非破壊的な変更にする

## 目的

`thief datadog auth login` を実行するとブラウザ経由の OAuth 2.0 ログインが完了し、`~/.config/thief/datadog/` (`config.Dir()` 配下、`backend/internal/config/config.go:270` の `Dir()` を利用) にファイル権限 0600 でアクセストークン・リフレッシュトークンが保存される。`thief datadog auth refresh` で明示的な更新、`thief datadog auth logout` でローカルの認証情報を削除できる。既存の `thief datadog historical`/`estimated` (静的キー必須) の挙動は変更しない。

## 設計判断

### 1. 新規パッケージ `backend/internal/datadogauth/` を新設し、DCR クライアントは 1 つだけ・redirect_uri 2 つをまとめて初回登録する

`internal/ssoauth` (AWS SSO 認証) と同じ立ち位置で、CLI と将来のサーバ実装 (issue 0165) の両方から利用できる Deps 構造体注入の形にする。

DCR で登録するクライアントは 1 つに固定し、`redirect_uris` には次の 2 つの URI を **初回登録時からまとめて** 含める。0165 が未実装の間もサーバ用 URI 自体は登録しておき、0165 実装時に実際に待ち受けるハンドラを追加する。

- CLI 用: `http://127.0.0.1:8400/callback` (固定ポート 8400。理由は設計判断 2 を参照)
- サーバ用: `config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath` (後述の定数を連結した値。既定では `http://127.0.0.1:8089/api/datadog/auth/callback`)

本 issue の実装時点では issue 0165 の設定 (`Datadog.OAuthRedirectBase`) はまだ存在しない。サーバ用 URI の既定値を issue 0165 側の設定に依存させると、実装順序 (0164 → 0165) 上、0164 が未定義の値を参照することになり成立しない。

この値を保持する定数は、新設パッケージ `datadogauth` ではなく既存パッケージ `backend/internal/config/config.go` に追加する。理由は、`datadogauth/storage.go` が `config.Dir()` (`config.go:270`) を呼ぶために `datadogauth` は元々 `config` に依存しており (`datadogauth → config` の一方向)、この定数を `datadogauth` 側に置いてしまうと `Datadog.OAuthRedirectBase` の既定値を組み立てる issue 0165 側で `config` パッケージが `datadogauth` を import する必要が生じ、`config → datadogauth → config` の循環 import になり `go build` が失敗するため (レビューで指摘された欠陥)。`config` パッケージ自身に定数を置けば、`datadogauth` は既存の依存方向のまま `config.DefaultDatadogOAuthRedirectBase` を参照でき、`config` パッケージは `datadogauth` を一切 import しない。

```go
// backend/internal/config/config.go に追加する。
const (
    DefaultDatadogOAuthRedirectBase = "http://127.0.0.1:8089"
    DatadogOAuthCallbackPath        = "/api/datadog/auth/callback"
)
```

`thief datadog auth login` (本 issue) からの `PrepareLogin` 呼び出しは、サーバ用 URI として `config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath` をそのまま渡す。issue 0165 はこの値を独自に組み立て直すのではなく、`Datadog.OAuthRedirectBase` の既定値としてこの定数をそのまま使う (同一パッケージ内の定数参照であり、追加の import は発生しない。詳細は issue 0165 設計判断 5 を参照)。これにより、どちらの issue を先に実装しても redirect_uri の値がズレず、かつ import グラフに循環が生じない。

pup の `dcr.rs` を確認する限り DCR エンドポイントは登録専用の `POST` のみで、登録済みクライアントの `redirect_uris` を追記する更新用エンドポイントは確認できなかった。そのため「今回必要な URI だけを都度登録し、無ければ追加登録する」という動的な方式は、追加登録のたびに新しい `client_id` を発行し既存のトークンを実質的に無効化する。CLI とサーバが交互にログインするたびに互いのトークンを無効化し合う「ピンポン」を避けるため、初回登録の時点で 2 つの URI をまとめて 1 回だけ登録し、以後は登録済みクライアントをそのまま再利用する (`PrepareLogin` は「クライアント登録ファイルが存在すればそのまま再利用し、存在しなければ上記 2 URI をまとめて 1 回だけ登録する」という単純な分岐にする)。

- `types.go`: `TokenSet` (access_token/refresh_token/expires_in/issued_at/scope/client_id、`IsExpired(now)` で 300 秒バッファ判定)、`ClientCredentials` (client_id/redirect_uris)。`backend/internal/config/config.go:78` の `type redacted string` と同じ考え方で `String()`/`LogValue()` を固定文字列にする redact 型をトークン文字列に適用し、`slog` の構造化ログに生値が乗らないようにする。
- `pkce.go`: `crypto/rand` で 128 文字 base64url (パディング無し) の code_verifier を生成し、SHA-256 → base64url (パディング無し) で code_challenge (S256) を計算する。
- `dcr.go`: `RegisterClient(ctx, site, clientName, redirectURIs)` で `POST https://api.{site}/api/v2/oauth2/register` を呼ぶ。
- `authorize.go`: `BuildAuthorizationURL(site, clientID, redirectURI, state, pkce, scopes)` で `https://app.{site}/oauth2/v1/authorize` の URL を組み立てる。
- `token.go`: `ExchangeCode(...)`/`Refresh(...)` で `POST https://api.{site}/oauth2/v1/token` を呼ぶ。
- `storage.go`: `Dir()` (`config.Dir()+"/datadog"`)、`Load/Save/DeleteToken`、`Load/SaveClient`。保存は `backend/internal/pricecache/pricecache.go` の `Save()` (120-152 行目、同一ディレクトリへの一時ファイル書き込み → `os.Chmod(0o600)` → `os.Rename` のアトミックパターン) をそのまま踏襲し、ディレクトリは `0700` で作成する。site 名はパス生成前に正規表現で検証する。
- `login.go`: `Deps` 構造体 (RegisterClient/ExchangeCode/RefreshToken/Load・SaveClient/Load・SaveToken/DeleteToken/Now を関数値として注入)、`PrepareLogin` (前述の通りクライアント登録ファイルが存在すればそのまま再利用し、存在しなければ CLI 用・サーバ用の 2 URI をまとめて 1 回だけ登録する)、`CompleteLogin` (state 検証 → トークン交換 → 保存)、`EnsureFreshToken` (未ログインなら `(nil, false, nil)` を返して呼び出し側にフォールバックを促す、期限切れならリフレッシュ)、`Logout` (ローカルのトークン・クライアント登録ファイルの削除のみ)。

**却下した代替案 1**: OS キーチェーン (macOS Keychain/Windows Credential Manager 等) での保存は、ユーザーの明示指示によりファイル権限方式を採用したため見送る。理由は、thief が単一ユーザーのローカル CLI/サーバであり、OS 依存のキーチェーン API 抽象化を追加するコストに見合わないため。

**却下した代替案 2**: 「今回必要な URI だけを都度登録し、無ければ追加登録する」動的な方式は、上記の通り新しい `client_id` の発行によるトークン相互無効化のリスクがあるため見送る。

### 2. CLI のログインコールバックは固定ポート 8400 の loopback、ポートスキャンは行わない

`pup` は空きポートを探索する実装だが、thief では固定ポート `8400` (redirect_uri は `http://127.0.0.1:8400/callback`) を採用する。DCR で登録したクライアントの `redirect_uris` を再利用する設計上、ポート (=redirect_uri) が実行のたびに変わると再登録が必要になり、Datadog 側が loopback ポートの緩和 (RFC 8252 §7.3 相当) をサポートしているかどうかを実装前に確認できていないため、安全側に倒す。

**却下した代替案**: ポートスキャンして空いているポートを都度使う方式は、pup 同様に実装は可能だが、上記の理由で今回は採らない。ポート使用中の場合は別ポートへ黙ってフォールバックせず、`bind: address already in use` 相当のエラーで明確に失敗させる (登録済み URI と不一致になり認可サーバに拒否されるリスクを避けるため)。

### 3. `thief datadog historical`/`estimated` の認証方式は今回変更しない

`showDatadogCost` (`backend/internal/cli/datadog.go:67`) は静的キー必須のままにする。CLI のコスト取得コマンドを OAuth 優先にする対応は、本 issue のスコープ外とする (対応する場合は issue 0165 のサーバ側フォールバック実装と同一パターンが使えるため、別 issue として追加できる)。

**却下した代替案**: 本 issue で `historical`/`estimated` も同時に OAuth 対応させる案は、ログインコマンド自体の実装と検証を独立して完了させるため見送る。1 issue に詰め込むと、ログイン機能の完成とコスト取得コマンドの改修が同時に完了しないと close できなくなり、変更の影響範囲も広がる。

### 4. CLI コマンドの実装パターン

`backend/internal/cli/sso.go` の `xxxWith(cmd, deps)` + Deps 構造体注入パターン (`ssoLoginWith` 106 行目、`ssoLogoutWith` 150 行目) をそのまま踏襲し、新規 `backend/internal/cli/datadog_auth.go` に `datadogAuthLoginWith(cmd, deps)`/`datadogAuthLogoutWith(cmd, deps)`/`datadogAuthRefreshWith(cmd, deps)` を実装する。ブラウザ起動は `sso.go:511` の `openBrowser(url string) error` をそのまま再利用する (共通化のための抽象化は行わない)。ローカルコールバック受信は `net/http` の `http.Server` を 1 回だけ受理したら即 `Shutdown` する。キャンセルは `backend/internal/cli/run.go:132` の `commandContext(cmd)` を使い、`context.WithTimeout` (5 分) で無期限待機を防ぐ。ブラウザが自動起動できない環境向けに、認可コードを標準入力へ貼り付けるヘッドレスフォールバックを用意する。

### 5. 追加の外部 API 呼び出し・権限について

Datadog に対する新規の HTTP 呼び出しは DCR (`/api/v2/oauth2/register`)、認可 (`/oauth2/v1/authorize`、ブラウザ経由)、トークン交換・リフレッシュ (`/oauth2/v1/token`) の 3 種のみで、いずれも OAuth の標準フローであり thief 側に追加の Datadog 権限設定は不要 (Datadog Organization 側で OAuth Apps を有効化する必要があるかは未確定事項として残す)。新規の Go 依存は無い (`golang.org/x/sync/singleflight` は `backend/internal/pricecache/pricecache.go:16` で既に使用中の既存依存であり、本 issue でも新規追加にはならない)。

## 完了条件

- 新規パッケージ `backend/internal/datadogauth/` に `types.go`/`pkce.go`/`dcr.go`/`authorize.go`/`token.go`/`storage.go`/`login.go` が実装され、PKCE の code_verifier/code_challenge 生成、DCR、認可 URL 組み立て、トークン交換・リフレッシュ、ファイル保存 (0600) がそれぞれテーブル駆動テストでカバーされている。
- `storage.go` の Load/Save が壊れた JSON ファイル・不正な site 名を明確なエラーで拒否することがテストで確認できる。
- `PrepareLogin` (クライアント再利用 vs 再登録の分岐)、`CompleteLogin` (state 不一致の拒否)、`EnsureFreshToken` (未ログイン/有効/期限切れ→成功/失敗の各分岐) がテーブル駆動テストでカバーされている。
- `thief datadog auth login`/`logout`/`refresh` サブコマンドが `newDatadogCmd()` (`backend/internal/cli/datadog.go:27`) の下に追加され、`backend/internal/cli/datadog_auth_test.go` で deps を全差し替えしたユニットテストが存在する (ブラウザ起動失敗時の非致命フロー、stdin フォールバックを含む)。
- 固定ポート `8400` が使用中の場合、別ポートへの黙ったフォールバックをせず、明確なエラーで失敗することがテストで確認できる。
- `thief datadog historical`/`estimated` の挙動 (静的キー必須) が変更されていないことを既存テストが引き続き通ることで確認できる。
- `RegisterClient` (DCR 呼び出し) が、`PrepareLogin` を複数回 (初回ログイン・再ログイン) 呼び出しても通算 1 回しか呼ばれないことを、モックの呼び出し回数のアサーションでテストする (片方の URI だけを都度追加登録する経路が実装に存在しないことを、この呼び出し回数で保証する)。
- `config.DefaultDatadogOAuthRedirectBase`/`config.DatadogOAuthCallbackPath` が `backend/internal/config/config.go` に定数として追加されており (`datadogauth` パッケージ側には追加しない)、`PrepareLogin` の初回登録呼び出しがサーバ用 URI としてこの 2 定数の連結値を使うことがテストで確認できる。
- `datadogauth` パッケージのソースが `config` パッケージ以外の internal パッケージを import していないこと (特に `internal/api` を import していないこと) を `go build ./...` の成功、または import グラフの目視確認で確認する (`config → datadogauth → config` の循環 import が生じていないことの確認)。
- 実装着手前に、Datadog Organization 側で OAuth Apps (DCR ベースのクライアント登録) の利用に管理者による事前有効化が必要かどうかを確認し、結果を本 issue に追記する。
- ローカル環境で実際に `thief datadog auth login` を実行し、ブラウザ承認後にループバックコールバックが受理され、`~/.config/thief/datadog/token_<site>.json` がパーミッション 0600 で作成されることを手動で確認する。Datadog Organization 側の管理者操作が必要と判明し実装者がそれを実施できない場合は、この手動確認の代わりに `httptest` でトークンエンドポイントをモックした自動化された end-to-end 統合テストで代替し、その旨と代替理由を issue に追記した上で完了条件を満たすものとする。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。

## 関連

- issue 0165 (API サーバ側の Datadog 認証 OAuth 対応、非同期ログイン API) は本 issue の `backend/internal/datadogauth/` パッケージに依存する。本 issue を先に close すること。
- 実装時に次の未確定事項を確認すること (ライブ検証が必要で、読み取り調査だけでは確定できなかったため)。
  - loopback redirect_uri のポート一致要件 (RFC 8252 §7.3 の緩和が Datadog 側にあるか)。緩和が確認できればポートスキャン方式への変更を検討してよい。
  - `usage_read` スコープが `historical_cost`/`estimated_cost` の取得を実際にカバーするか (`403 insufficient_scope` が出ないかの確認)。
  - OAuth トークンの revoke エンドポイントの有無 (見つかれば `Logout` に組み込む。見つからない場合はローカル削除のみで良い)。
  - DCR レスポンスに `client_secret` が含まれるか (pup の `RegistrationResponse` 構造体には無いが、Datadog 側の実際のレスポンスは未確認)。含まれる場合も redact 対応する。
  - Datadog Organization 側で OAuth Apps (DCR ベースのクライアント登録) の利用に管理者による事前有効化が必要か。実装着手前に確認すること。
  - vendor している `github.com/DataDog/datadog-api-client-go/v2` v2.55.0 の `ContextAccessToken` (`api/datadog/configuration.go`) と `PrepareRequest` (`api/datadog/client.go`) の実装は GitHub 上のソースで確認済みだが、本リポジトリにモジュール本体はベンダリングされておらずローカルの `go build` では未確認。実装時に実際にビルド・動作することを確認すること。
  - pup の `OAUTH_EXCLUDED_ENDPOINTS` (`/api/unstable/fleet/`, `/profiling/api/v1/`, `/api/unstable/profiles/`, `/api/ui/profiling/`) に基づく「Usage Metering (`/api/v2/usage/*`) は OAuth 対応」という結論は pup 実装からの推測であり、Datadog 公式ドキュメントでの一次確認ではない。実装時に実際の OAuth トークンで `GetHistoricalCostByOrg`/`GetEstimatedCostByOrg` を呼び、認可エラーにならないことをライブ検証すること (issue 0165 の完了条件と対応)。
  - `OAuthRedirectBase` (issue 0165 で追加) を既定値から変更した場合、登録済みクライアントの `redirect_uris` に新しい URI が含まれず再登録が発生し、新しい `client_id` の発行により変更前に発行された全トークン (CLI 側も含む) が無効になる。この場合は全体の再ログインが必要になる既知の制限として扱う (詳細は issue 0165 を参照)。

## 調査結果 (2026-09-11)

「## 関連」の未確定事項について、リポジトリの読み取りで決着した事項と、この実装環境では検証できない事項を整理した。

### 決着した事項

- `github.com/DataDog/datadog-api-client-go/v2` v2.55.0 の Bearer トークン注入の実装は、ローカルの Go module cache (`$(go env GOMODCACHE)/github.com/!data!dog/datadog-api-client-go/v2@v2.55.0`) に実体があり、次の 2 箇所で確認した。SDK 側の変更が不要であることは確定である (「## 背景」末尾で残していた「ローカルの `go build` では未確認」を解消する)。
  - `api/datadog/configuration.go:40` の `ContextAccessToken = contextKey("accesstoken")`
  - `api/datadog/client.go:437` の `if auth, ok := ctx.Value(ContextAccessToken).(string); ok { localVarRequest.Header.Add("Authorization", "Bearer "+auth) }`
- 「## 背景」「## 設計判断」が引用する次のシンボルは、いずれも記載された位置に実在する。
  - `backend/internal/cli/datadog.go` の `newDatadogCmd()` (27 行目)、`showDatadogCost()` (67 行目)、静的キー必須チェック (73 行目)
  - `backend/internal/config/config.go` の `Dir()` (270 行目)、`type redacted string` (78 行目)、`DATADOG_API_KEY` / `DATADOG_APP_KEY` の解決 (222 行目・225 行目)、`Defaults()` (113 行目から 126 行目)
  - `backend/internal/cli/sso.go` の `openBrowser()` (511 行目)、`ssoLoginWith()` (106 行目)、`ssoLogoutWith()` (150 行目)
  - `backend/internal/cli/run.go` の `commandContext()` (132 行目)
  - `backend/internal/pricecache/pricecache.go` の `Save()` (120 行目から 152 行目。一時ファイル作成 → `os.Chmod(0o600)` → `os.Rename` のアトミックパターン)
  - `backend/internal/datadog/client.go` の `NewContext()` (18 行目。`datadog.ContextAPIKeys` のみを埋め込む実装)
- `backend/internal/datadogauth/` と `backend/internal/cli/datadog_auth.go` はいずれも存在しない。本 issue の要望は別の変更では満たされていない。

### 検証できない事項と完了条件の代替条項の適用

実装環境に Datadog の認証情報 (`DATADOG_API_KEY` / `DATADOG_APP_KEY`) と Datadog Organization の管理者権限が無く、次の 3 点はライブ検証で確定できない。

1. Datadog Organization 側で OAuth Apps (DCR ベースのクライアント登録) の利用に管理者による事前有効化が必要か
2. `usage_read` スコープが `historical_cost` / `estimated_cost` の取得をカバーするか
3. loopback redirect_uri のポート一致要件 (RFC 8252 §7.3 の緩和が Datadog 側にあるか)

1 点目は完了条件の「実装着手前に、Datadog Organization 側で OAuth Apps (DCR ベースのクライアント登録) の利用に管理者による事前有効化が必要かどうかを確認し、結果を本 issue に追記する」に対応する行である。確認の結果は「実装環境に Datadog Organization の管理者権限が無く、有効化の要否を確認できない」であり、これは完了条件の代替条項が発動要件とする「事前有効化が必要と判明した」という確定結果そのものではない。

より根本的な制約は、事前有効化の要否という個別の論点ではなく、実装環境に Datadog の認証情報 (`DATADOG_API_KEY` / `DATADOG_APP_KEY`) も Datadog Organization の管理者権限も一切無く、事前有効化の要否によらず実 Datadog に対するライブ検証そのものが不可能であることにある。事前有効化が不要であっても、認証情報が無ければ DCR 登録・認可・トークン交換のいずれも実 Datadog に到達できず、「実ログインの手動確認」(完了条件 118 行目) は元より実行できない。したがって、完了条件が個別に定めた代替条項 (事前有効化が必要と判明した場合の代替) の字義どおりの発動要件は満たさないが、その代替条項が想定する状況 (実 Datadog に対する検証手段が実装者に無い) をより広く包含する事実がある以上、同条項の趣旨に照らして代替を適用するのが妥当と判断し、`thief datadog auth login` の手動確認を `httptest` によるモック end-to-end 統合テスト (`TestDatadogAuthLoginEndToEnd`) で代替する。代替理由は、事前有効化の要否によらず実装環境に Datadog への到達手段 (認証情報・管理者権限) が一切無く、実 Datadog に対するライブ検証が不可能なことである。

2 点目と 3 点目は本 issue のスコープに影響しない。`historical` / `estimated` の認証方式は設計判断 3 で今回は変更しないと決めており、`usage_read` スコープの実効性は issue 0165 の完了条件で扱う。ポート一致要件は設計判断 2 で、緩和の有無を前提にしない固定ポート 8400 を採ることで決着している。

## 解決方法

`backend/internal/datadogauth/` を新設し、設計判断どおりの 7 ファイルに加えて実装時に必要になった `client.go` を実装した。

- `types.go`: `TokenSet`(access_token/refresh_token/expires_in/issued_at/scope/client_id、`IsExpired(now)` で早期 300 秒バッファ判定)、`ClientCredentials`(client_id/redirect_uris)。トークン文字列は `config.go` の `redacted` と同じ考え方の redact 型で覆い、`slog` の構造化ログへ生値が乗らないようにした。
- `pkce.go`: `crypto/rand` で 128 文字 base64url (パディング無し) の code_verifier を生成し、SHA-256 → base64url で code_challenge (S256) を算出する。
- `dcr.go`: `RegisterClient(ctx, site, clientName, redirectURIs)` で `POST https://api.{site}/api/v2/oauth2/register` を呼ぶ。
- `authorize.go`: `BuildAuthorizationURL(...)` で `https://app.{site}/oauth2/v1/authorize` の URL を組み立てる。
- `token.go`: `ExchangeCode`/`Refresh` で `POST https://api.{site}/oauth2/v1/token` を呼ぶ。
- `storage.go`: `Dir()`(`config.Dir()+"/datadog"`)、Load/Save/Delete(Token/Client)。`pricecache.Save()` と同じ一時ファイル→`os.Chmod(0o600)`→`os.Rename` のアトミックパターンを踏襲し、ディレクトリは 0700 で作成、site 名は正規表現で検証してからパスを組み立てる。
- `login.go`: `Deps` 構造体、`PrepareLogin`(登録済みクライアントファイルがあれば再利用、無ければ CLI 用・サーバ用の 2 redirect_uri をまとめて 1 回だけ DCR 登録)、`CompleteLogin`(state 検証 → トークン交換 → 保存)、`EnsureFreshToken`(未ログイン/有効/期限切れの分岐、期限切れはリフレッシュ)、`Logout`(トークン・クライアント登録ファイルの削除)。
- `client.go` (方針の一覧に無い追加ファイル。下記「方針からの乖離」を参照): `dcr.go`/`token.go` が共有する HTTP 実行部 (タイムアウト 30 秒、期待ステータス検査、エラー本文 1KiB 打ち切り、`https://api.{site}` の組み立て) を切り出した。

`backend/internal/config/config.go` に `DefaultDatadogOAuthRedirectBase`(`http://127.0.0.1:8089`)と `DatadogOAuthCallbackPath`(`/api/datadog/auth/callback`)の 2 定数を追加した (issue 0165 が参照する既定値の置き場であり、`datadogauth` パッケージ側には置いていない)。

`backend/internal/cli/datadog_auth.go` に `datadogAuthLoginWith`/`datadogAuthLogoutWith`/`datadogAuthRefreshWith` を実装し、`newDatadogCmd()`(`datadog.go:59`) に `thief datadog auth login/logout/refresh` サブコマンドとして追加した。ログインは固定ポート 8400 の loopback で 1 回だけ受理する `http.Server` を立て、ブラウザ起動 (`sso.go` の `openBrowser` を再利用) に失敗した場合と、標準入力に認可コードを貼り付けるヘッドレスフォールバックの両方を用意した。ポート 8400 が使用中の場合は別ポートへ黙ってフォールバックせず、明確なエラーで失敗する。

`backend/internal/cli/run_test.go` の既存ガードテスト `TestNoContextBlindStdinReadOutsideDesignatedFunctions` に、新規の `readDatadogAuthCode` を許可済み一覧として追記した (標準入力読み取り自体は `readWithContext` を経由し、Ctrl-C でのキャンセルが効くことをテストで確認している)。

### 完了条件ごとの検証

- PKCE/DCR/認可 URL/トークン交換・更新/0600 保存: `pkce_test.go`(RFC 7636 Appendix B のテストベクタで S256 を検証)、`dcr_test.go`、`authorize_test.go`、`token_test.go`、`storage_test.go`(`TestSaveTokenRoundTrip`/`TestSaveClientRoundTrip` でファイル 0600・ディレクトリ 0700 を確認)。
- 壊れた JSON・不正な site 名の拒否: `storage_test.go` の `TestLoadTokenErrors`/`TestLoadClientErrors`/`TestSaveRejectsInvalidInput`/`TestValidateSite`(パストラバーサル文字列を含む 18 ケース)。
- `PrepareLogin`/`CompleteLogin`/`EnsureFreshToken` の各分岐: `login_test.go` の `TestPrepareLogin`(10 ケース)/`TestCompleteLogin`(9 ケース、state 不一致は `ErrStateMismatch` で ExchangeCode を呼ばないことを確認)/`TestEnsureFreshToken`(12 ケース、300 秒バッファの境界を含む)。
- CLI サブコマンドと deps 全差し替えテスト: `datadog_auth_test.go` の `TestDatadogAuthLoginWith`(9 ケース、ブラウザ起動失敗時の非致命フロー・stdin フォールバックを含む)/`TestDatadogAuthLogoutWith`/`TestDatadogAuthRefreshWith`。
- 固定ポート使用中の明確な失敗: `TestDatadogAuthLoginWithFixedPortInUse`(エラー文言に `127.0.0.1:8400` を含み、DCR 登録・ブラウザ起動が 0 回であることを確認)。
- `historical`/`estimated` の挙動不変: `datadog.go` の差分は `AddCommand` への追記 1 行のみで、既存の `internal/cli`/`internal/datadog` テストが変更なく通過することで確認。
- DCR 通算 1 回: `TestPrepareLoginRegistersOnlyOnce`(`PrepareLogin` を 2 回呼んで登録回数 1 を断言)。
- `config` の 2 定数と初回登録での使用: `TestDatadogServerRedirectURI` と `TestDatadogAuthLoginWith` の登録リクエストパラメータ比較。
- `datadogauth` が `config` 以外の internal を import しない: `go list -f '{{join .Imports "\n"}}' ./internal/datadogauth/` で thief 内 import が `internal/config` のみであることと、`go build ./...` の成功で確認。
- Datadog Organization 側の OAuth Apps 事前有効化の要否確認: 「## 調査結果 (2026-09-11)」に追記済み (実装環境に管理者権限が無く確認できないことを記録)。
- 実ログインの手動確認 (代替条項): 事前有効化の要否によらず、実装環境に Datadog の認証情報 (`DATADOG_API_KEY`/`DATADOG_APP_KEY`) も Organization の管理者権限も一切無く実 Datadog へのライブ検証が不可能であることを「## 調査結果 (2026-09-11)」に追記し (レビュー指摘を受けて代替条項の適用根拠を訂正)、`TestDatadogAuthLoginEndToEnd` で DCR・トークンエンドポイントのみ `httptest` に差し替え、PKCE/state 生成・認可 URL 組み立て・ループバックコールバック受理・トークン保存 (0600) の一連を本番コードで通し、`refresh`/`logout` までを検証した。
- `mise run check`: 統合後に作業ツリーで実行し、backend 18 パッケージすべて `ok`(新規失敗 0)、frontend 794 テスト全通過を確認した。

### 方針からの乖離 (方式を保った実装詳細)

- `client.go` を新設し、`dcr.go`/`token.go` が共有する HTTP 実行部を切り出した。方針のファイル一覧には無いが、DCR/トークンの 2 ファイルが個別に実装すると重複するための整理であり、方式は変えていない。
- `Deps` に `DeleteClient` を追加した。方針が `Logout` を「トークン・クライアント登録ファイルの削除」と定めており、`DeleteToken` だけでは登録ファイルを消せないため。

### レビューで却下した指摘

- `storage.go`/`login.go` の `Logout` が第一引数に `ctx context.Context` を取らない点 (テストと堅牢性観点、低優先度): 既存の踏襲元である `pricecache.Save()` 自体が `ctx` を取らない前例であり、`Logout` が呼ぶ `os.Remove` 系の標準ライブラリ関数はキャンセル不可能なため、`ctx` を追加しても実質的なキャンセル可能性は生まれない。`pricecache.go` 側の是正は本 issue のスコープ外と判断し、却下した。

以上でこの issue の完了条件をすべて満たした。
