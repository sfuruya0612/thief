# URL 系設定値の末尾スラッシュを設定ロード時にトリムする

Created: 2026-09-16
Model: Claude Opus 5
Completed: 2026-09-17

## 症状

`docs/issues/TODO.md` の次の項目に対応する。

> - [ ] Datadog OAuth の `THIEF_DATADOG_OAUTH_REDIRECT_BASE` (`Datadog.OAuthRedirectBase`) を含む URL 系設定値に末尾スラッシュのトリム処理を入れたい (issue 0165 のスコープ外として送り)
>     - 末尾にスラッシュを付けて設定すると redirect_uri が二重スラッシュになり、Datadog 側に登録済みの redirect_uris と文字列一致せず認可要求が拒否される
>     - 同種のトリム欠如は AWS SSO start URL 等、他の URL 系設定にも既存

`THIEF_DATADOG_OAUTH_REDIRECT_BASE` に `http://127.0.0.1:8089/` のように末尾スラッシュを付けた URL を設定すると、API サーバが組み立てる redirect_uri が `http://127.0.0.1:8089//api/datadog/auth/callback` になる。
パス部が `//api/...` と二重スラッシュになっており、末尾スラッシュを付けずに設定した場合の `http://127.0.0.1:8089/api/datadog/auth/callback` と異なる文字列になる。

`http://127.0.0.1:8089/` は URL として正しい表記であり、設定値としても拒否されない。
正しい表記の設定値から、意図しない redirect_uri が組み立てられる。

出所は `docs/issues/closed/0165-feat-datadog-oauth-api-server-fallback.md` のレビューで却下した指摘である。原文は次のとおり。

> **`Datadog.OAuthRedirectBase` の末尾スラッシュ未検証 (テスト堅牢性レビュー、優先度低)**: 運用者が末尾にスラッシュを付けて設定すると redirect_uri が二重スラッシュになり認可要求が拒否されうるという指摘。本 issue の完了条件が求める挙動ではなく、同種のトリム欠如は本リポジトリの他の URL 系設定にも既存であり 0165 固有の新規劣化ではないため、本 issue のスコープでは修正しない。`docs/issues/TODO.md` にスコープ外の問題として追記した。

なお、この redirect_uri が Datadog 側で実際に拒否されるかは本 issue の時点で未検証である。
OAuth 2.0 の仕様 (RFC 6749 3.1.2.3 と RFC 6819 5.2.3.5) は、認可サーバが登録済みの redirect_uri と要求された redirect_uri を単純な文字列比較で照合することを求めており、`//api/...` と `/api/...` は文字列として一致しない。
Datadog が仕様どおりに照合するなら認可要求は拒否される。これは仕様に基づく推論であり、実機での確認は行っていない。
本 issue は「redirect_uri が二重スラッシュになる」ことを修正対象とし、Datadog 側の拒否の有無を完了条件に含めない。

## 再現手順

1. リポジトリ直下で `THIEF_DATADOG_OAUTH_REDIRECT_BASE=http://127.0.0.1:8089/ mise run backend:run` を実行して API サーバを起動する。
2. Datadog の OAuth ログインを開始する API (`POST /api/datadog/auth/login/start`) を呼ぶ。
3. 応答に含まれる認可 URL のクエリパラメータ `redirect_uri` を見る。`http%3A%2F%2F127.0.0.1%3A8089%2F%2Fapi%2Fdatadog%2Fauth%2Fcallback` (デコードすると `http://127.0.0.1:8089//api/datadog/auth/callback`) となり、パスの先頭が二重スラッシュになる。

Datadog の資格情報を用意せずに再現を確認する場合は、`backend/internal/config` の `Load` が返す `Config` の `Datadog.OAuthRedirectBase` が `http://127.0.0.1:8089/` のままであること、および `backend/internal/api` の `(*Server).datadogServerRedirectURI` の戻り値が二重スラッシュを含むことを Go のテストで観測する。

## 原因

`backend/internal/config/config.go` の `applyEnv` は環境変数の値をそのまま代入する。

```go
if v := os.Getenv("THIEF_DATADOG_OAUTH_REDIRECT_BASE"); v != "" {
    cfg.Datadog.OAuthRedirectBase = v
}
```

`applyFile` の YAML 由来の上書きも同様に値をそのまま代入する。
`config` パッケージには設定値を検証する関数も正規化する関数も存在せず、`Load` はファイル I/O と YAML パースのエラーだけを返す。

一方、redirect_uri を組み立てる側は文字列連結である。
`backend/internal/api/handlers_datadog_auth.go` の `datadogServerRedirectURI` は次のとおり。

```go
func (s *Server) datadogServerRedirectURI() string {
    return s.cfg.Datadog.OAuthRedirectBase + config.DatadogOAuthCallbackPath
}
```

`config.DatadogOAuthCallbackPath` は `"/api/datadog/auth/callback"` で先頭にスラッシュを含む。
ベース側の末尾スラッシュとパス側の先頭スラッシュが両方残るため、連結結果が二重スラッシュになる。

`backend/internal/cli/datadog_auth.go` の `datadogServerRedirectURI` も同じ連結を行うが、こちらは `config.DefaultDatadogOAuthRedirectBase` 定数を直接使っており設定値を読まないため、この症状は起きない。

## 修正方針

`config` パッケージに URL のベース値を正規化する非公開ヘルパーを 1 つ置き、`Load` が値を確定した後に `Datadog.OAuthRedirectBase` へ適用する。
正規化は前後の空白を落としたうえで末尾のスラッシュをすべて除去する。`backend/internal/aws/sso_cache.go` の `normalizeStartURL` と同じ `strings.TrimRight(strings.TrimSpace(u), "/")` の形にする。

適用箇所は `applyEnv` や `applyFile` の中ではなく、既定値と YAML と環境変数をすべて反映し終えた後の 1 か所にする。
3 か所に散らすと、片方だけを通る経路が生まれて抜けが出る。

### 対象とする設定値と、対象としない設定値

`config` パッケージと frontend の設定値をすべて数え上げ、本 issue で正規化するものを次のとおり決める。

**正規化する**

- `Datadog.OAuthRedirectBase` (`THIEF_DATADOG_OAUTH_REDIRECT_BASE`、既定値 `http://127.0.0.1:8089`)。パスと連結してから外部の認可サーバへ渡すため、末尾スラッシュが結果を変える。

**正規化しない**

- AWS SSO の start URL。`backend/internal/aws` が AWS profile の `sso_start_url` から読み、`backend/internal/aws/sso_oidc.go` の `StartSSODeviceAuthorization` (実体は `startSSODeviceAuthorization`) が `ssooidc.StartDeviceAuthorizationInput` の `StartUrl` へそのまま載せる不透明な識別子であり、パスと連結しない。`backend/internal/aws/sso_cache.go` の `normalizeStartURL` はキャッシュの突き合わせ専用に正規化するもので、SDK へ渡す値は非正規化のままである。値そのものを書き換えると SDK へ渡す識別子の意味が変わるため、対象から外す。
- `Datadog.Site` (既定値 `datadoghq.com`)。スキームもパスも持たないホスト名で、`backend/internal/datadogauth/client.go` の `"https://api." + site` や `backend/internal/datadog/client.go` の `"api." + site` のように前方へ連結する。末尾スラッシュを付けるとホスト名が壊れるが、これは URL のベース値ではなくホスト名の誤りであり、末尾スラッシュのトリムでは救えない (`datadoghq.com/` を `datadoghq.com` に直すことはできるが、`https://datadoghq.com` のような誤りは残る)。ホスト名の検証は別の論点として本 issue では扱わない。
- `ListenAddr` (`THIEF_LISTEN_ADDR`、既定値 `127.0.0.1:8089`)。`http.Server.Addr` に渡す `host:port` であり URL ではない。
- `WebOrigins` (`THIEF_WEB_ORIGINS`、既定値 `["localhost:8088", "127.0.0.1:8088"]`)。WebSocket の `OriginPatterns` に渡すホストのパターンで、パスと連結しない。要素ごとに `strings.TrimSpace` 済みである。
- frontend の `VITE_API_BASE` (`frontend/src/api/client.ts`、既定値 `http://127.0.0.1:8089`)。`buildUrl` が `new URL(path, BASE_URL)` の形で使い、`path` は `frontend/src/api/endpoints.ts` から常に先頭スラッシュ付きの絶対パスで渡る。URL の仕様上、絶対パスはベースのパスを置き換えるため、ベースの末尾スラッシュは結果に影響しない。

### 採らなかった案

- **連結する側でトリムする**。`datadogServerRedirectURI` で `strings.TrimRight` する案。連結箇所が増えるたびに同じ処理を書くことになり、書き忘れが再発する。設定値を 1 か所で正規化すれば、以降の利用箇所は正規化済みの前提で書ける。
- **`net/url` でパースして再構築する**。`url.Parse` してから `URL.JoinPath` で組み立てる案。スキームやホストの妥当性も併せて検証できるが、不正な値をどう扱うか (起動を止めるか既定値に戻すか) という新しい判断が要り、本 issue の範囲を超える。末尾スラッシュだけを落とす最小の変更にとどめる。
- **末尾スラッシュ付きの値を設定エラーとして起動を止める**。利用者に直させる案。`config` パッケージに検証の仕組みが無く、検証の枠組みごと新設することになる。また、末尾スラッシュ付きの URL は表記として正しく、拒否する理由が弱い。

## 完了条件

- `backend/internal/config` に、前後の空白を落として末尾のスラッシュをすべて除去する正規化ヘルパーが 1 つ追加されている。
- `Load` が返す `Config` の `Datadog.OAuthRedirectBase` が、既定値の経路、YAML の経路、環境変数の経路のいずれでも末尾スラッシュを含まない。
- `backend/internal/config` のテストに次のケースが追加されている。いずれも期待値は `http://127.0.0.1:8089` とする。
  - `THIEF_DATADOG_OAUTH_REDIRECT_BASE=http://127.0.0.1:8089/`
  - `THIEF_DATADOG_OAUTH_REDIRECT_BASE=http://127.0.0.1:8089///`
  - `THIEF_DATADOG_OAUTH_REDIRECT_BASE=" http://127.0.0.1:8089/ "` (前後に空白を含む)
  - YAML の `datadog.oauth-redirect-base` に `http://127.0.0.1:8089/` を設定した場合
- `THIEF_DATADOG_OAUTH_REDIRECT_BASE=/` や `THIEF_DATADOG_OAUTH_REDIRECT_BASE=///` のようにスラッシュだけの値を設定した場合の結果が、テストで固定されている。正規化後に空文字になるため、この場合は既定値 `config.DefaultDatadogOAuthRedirectBase` に戻す。空文字のまま残すと redirect_uri がスキームもホストも持たない相対 URI になり、認可要求が確実に壊れる。
- `backend/internal/api` のテストに、`Datadog.OAuthRedirectBase` を末尾スラッシュ付きで与えた `Server` の `datadogServerRedirectURI` が `http://127.0.0.1:8089/api/datadog/auth/callback` を返すケースが追加されている。
- `backend/internal/cli/datadog_auth.go` の `datadogServerRedirectURI` は変更しない (定数を直接使っており設定値を読まないため)。
- 上記「正規化しない」に挙げた設定値のコードは変更しない。
- `mise run check` が通る。

## 関連

- `docs/issues/closed/0165-feat-datadog-oauth-api-server-fallback.md` が `Datadog.OAuthRedirectBase` を追加した issue で、本 issue の指摘の出所である。

## 解決方法

「## 修正方針」の方式どおりに実装した。

- `backend/internal/config/config.go` に非公開ヘルパー `normalizeURLBase(u string) string` を追加した。前後の空白を `strings.TrimSpace` で落としたうえで末尾のスラッシュを `strings.TrimRight(..., "/")` ですべて除去する。`backend/internal/aws/sso_cache.go` の `normalizeStartURL` と同じ式である。
- `Load()` は `Defaults()` → `applyFile` → `applyEnv` をすべて適用し終えた後、1 か所で `cfg.Datadog.OAuthRedirectBase` に `normalizeURLBase` を適用する。正規化後に空文字になった場合 (値がスラッシュや空白だけだった場合) は `DefaultDatadogOAuthRedirectBase` にフォールバックする。`applyEnv`/`applyFile` 自体は変更していない。
- `backend/internal/config/config_test.go` に `TestLoadNormalizesDatadogOAuthRedirectBase` を追加し、`default`/`env`/`yaml` の 3 経路で末尾スラッシュ・複数スラッシュ・前後空白・スラッシュのみ (既定値へのフォールバック) を検証する。テスト間の隔離用に `isolateConfigFiles` ヘルパーを追加した。
- `backend/internal/api/handlers_datadog_auth_test.go` に `TestDatadogServerRedirectURITrimsTrailingSlash` を追加し、`config.Load()` を経由して構築した `Server` の `datadogServerRedirectURI()` が二重スラッシュを含まない URL を返すことを確認する。
- 「対象としない設定値」(AWS SSO start URL、`Datadog.Site`、`ListenAddr`、`WebOrigins`、frontend の `VITE_API_BASE`) と `backend/internal/cli/datadog_auth.go` の `datadogServerRedirectURI` は変更していない。
- Step 7 の多観点レビューで、テストヘルパー `isolateConfigFiles` のコメントが実際の隔離範囲 (`XDG_CONFIG_HOME`/`HOME` 配下の `config.yaml` と `THIEF_DATADOG_OAUTH_REDIRECT_BASE`) より広く保証しているように読める指摘 (優先度低) を受けた。`configFilePaths` が最優先で見るカレントディレクトリ相対の `config.yaml` はこのヘルパーの隔離対象外であり、パッケージのソースディレクトリに同名ファイルが存在しないことで別途担保している旨を明記するようコメントを修正し、追加レビューで指摘の解消を確認した。
- `mise run check` の通過を確認した (frontend 993/993 テスト、backend 全パッケージ ok、fmt/lint に新規の指摘なし)。
