# SSO デバイス認可を開始・完了する API エンドポイントを追加する

Created: 2026-08-24
Model: Claude Fable 5
Completed: 2026-08-25

## 背景

docs/issues/TODO.md の次の項目に由来する。

> Frontend で SSO Login をして別タブで開いたリダイレクト先の AWS のページでの認証が通ったあと Frontend のタブに切り替わるようにしたい

この要望の実現は 3 つの issue に分割した。本 issue はその第 2 段階で、backend にデバイス認可の開始 / 完了エンドポイントを追加する。第 1 段階 (docs/issues/0147-refactor-extract-sso-device-auth-package.md) で CLI から抽出した共有パッケージを利用する。frontend の切り替えと旧エンドポイントの削除は第 3 段階 (docs/issues/0149-feat-sso-login-refocus-frontend-tab.md) で行い、本 issue の時点では現行のログイン経路を変更せず並存させる。

現状の SSO ログインの API は `POST /api/aws/profiles/{profile}/sso/login` (backend/internal/api/routes.go:38) のみで、`handleSSOLogin` (backend/internal/api/handlers_sso.go:15) が `aws sso login --profile <profile>` を `exec.CommandContext` で起動し、ブラウザでの認可完了まで待って 1 回だけ応答する (タイムアウト `ssoLoginTimeout` = 5 分)。この方式では認可ページのタブを開くのは AWS CLI のプロセスであり、frontend のスクリプトは認可 URL もタブへの参照 (`WindowProxy`) も得られない。frontend が認可タブを制御する (0149) には、認可 URL を認可の開始時点で frontend に返し、トークンの待機を別リクエストに分ける API が必要である。これが本 issue の対象である。

profile から SSO 設定を解決する経路には制約がある。`backend/internal/aws/profiles.go` は `~/.aws/config` から profile ごとの `sso_session` / `sso_start_url` / `sso_region` をパースしている (profiles.go:150-166, 244-263) が、公開型 `Profile` (`ListProfiles()` の戻り値、profiles.go:50-62) に SSO の start URL とリージョンは含まれず、非公開の中間型 (`profileSection` / `ssoSessionSection`) にのみ保持される。profile 名からこれらを取り出す経路を新たに公開する必要がある。

## 目的

frontend が認可 URL を自分のスクリプトで開けるように、デバイス認可の開始 (認可 URL の取得) と完了 (トークンの待機と保存) を分離した API が backend に追加される。本 issue の完了時点では利用者はまだおらず、現行のログイン経路はそのまま動き続ける。

## 設計判断

- `POST /api/aws/profiles/{profile}/sso/login/start`: backend が profile から SSO リージョンと start URL を解決し (背景に書いたとおり `internal/aws` に取り出し経路の公開が要る)、共有パッケージ (0147) の「デバイス認可の開始」を呼んで、`verification_uri_complete`、`verification_uri`、`user_code` と、デバイスコードを引くためのログインセッション ID を返す。デバイスコードとクライアントシークレットはレスポンスに含めず、backend がセッション ID に紐づけてメモリ上に保持する (露出させる必要が無い)。
- `POST /api/aws/profiles/{profile}/sso/login/complete`: リクエストボディでセッション ID を受け取り、共有パッケージの「トークン待機」を呼んで CreateToken をポーリングし (タイムアウトは現行と同じ 5 分)、成功時に AWS CLI 互換キャッシュへ保存して 204 を返す。
- ログインセッションの管理: セッション ID は `crypto/rand` 由来の推測不能な値とし、`map[string]session` を `sync.Mutex` で保護して保持する (AGENTS.md の並行処理の規約に従う)。セッションはデバイス認可の有効期限 (StartDeviceAuthorization レスポンスの `expiresIn`) で失効させ、失効分は `complete` 到達時または定期掃除で破棄する。`complete` は同一セッション ID に対して 1 回だけ有効 (取り出した時点でマップから削除する)。未知または失効したセッション ID (backend 再起動で消えた場合を含む) には 404 相当のエラーコードを返す。同時ログインはセッション ID 単位で独立に扱い、同一 profile での複数回の `start` はそれぞれ別セッションとして許容する (上書きしない)。
- セッションはメモリ保持のみとし、永続化しない。backend はローカルで常駐する単一プロセスであり、再起動でセッションが消えても frontend が 404 を受けてログインをやり直せば足りる。
- ユーザが認可を拒否 (deny) した場合、CreateToken は `access_denied` を返し、既存の待機ロジック (backend/internal/aws/sso_oidc.go の `waitForSSOToken`) はこれを再試行せず即時失敗として返す。`complete` は `access_denied` を他の失敗と区別できるエラーコードで返す (0149 で frontend が「認可タブを閉じてよい失敗」を判別するため)。
- profile に SSO 設定 (`sso_session` も `sso_start_url` も) が無い場合、`start` は 4xx でエラーコードを返す。
- 既存の `POST /api/aws/profiles/{profile}/sso/login` と `handleSSOLogin` は本 issue では変更しない。削除は frontend の切り替えと同時に 0149 で行う。系列のどの時点でも動作するログイン経路が常に 1 つ存在する状態を保つためである。

採らなかった案とその却下理由。

- `aws sso login --no-browser` を exec し、stdout から認可 URL をパースして frontend へ渡す案。stdout の文言は AWS CLI の安定した契約ではなくバージョンで変わりうる。また URL はコマンド実行の途中で出力されるため、「完了まで待って 1 回だけ応答する」現行ハンドラの形では返せず、ストリーミング応答 (SSE 等) の導入が必要になる。
- 旧エンドポイントを本 issue で削除する案。frontend が旧エンドポイントを呼んでいる間に削除するとログインが壊れる。削除は利用箇所の切り替え (0149) と同一 issue で行う。
- セッションを 1 profile につき 1 つに制限して上書きする案。複数のブラウザタブから同時にログインを開始したとき、後発の `start` が先発のセッションを無効化し、先発タブの `complete` が理由の分かりにくい失敗になる。独立したセッションとして扱えば干渉しない。

追加の AWS API 呼び出しは、SSO OIDC の RegisterClient / StartDeviceAuthorization / CreateToken である。いずれも既存の CLI ログイン (`thief sso login`) が呼んでいるものと同一で、CLI 側は IAM 認証情報なしで呼べている。AWS の仕様上も認証前の公開エンドポイントとされるが、一次資料 (AWS 公式ドキュメント) は未確認である。

認証フロー (SSO) の挙動変更は AGENTS.md が事前質問の対象とするが、本 issue はユーザの TODO 要望そのものであり、この起票がその確認にあたる。

## 完了条件

- `POST /api/aws/profiles/{profile}/sso/login/start` が追加され、`verification_uri_complete` (無い場合は `verification_uri` と `user_code`。RFC 8628 §3.2 で `verification_uri_complete` は OPTIONAL) とセッション ID を返す。デバイスコードとクライアントシークレットはレスポンスに含まれない。
- `POST /api/aws/profiles/{profile}/sso/login/complete` が追加され、成功時に `~/.aws/sso/cache/` へ AWS CLI 互換のトークンキャッシュを書き込んで 204 を返す。
- `complete` の成功後、プロファイル一覧の SSO ステータスが `valid` (backend/internal/aws/profiles.go:42 の `SSOStatusValid`) を返す。
- `internal/aws` に profile 名から SSO リージョンと start URL を取り出す公開の経路が追加され、単体テストを持つ。
- 未知または失効したセッション ID への `complete` が 404 相当のエラーコードを返す。`access_denied` は他の失敗と区別できるエラーコードで返る。
- 既存の `POST /api/aws/profiles/{profile}/sso/login` と `handleSSOLogin` が変更されず残り、現行の frontend からのログインが引き続き動く。
- 新設エンドポイントのハンドラに `net/http/httptest` と依存注入による単体テスト (成功、SSO 設定なし profile、ポーリングタイムアウト、`access_denied`、未知または失効したセッション ID、キャッシュ保存失敗、同一 profile での並行セッション) が追加される。
- 扱わない範囲: frontend の変更 (0149 で行う)、ログインセッションの永続化 (設計判断のとおり行わない)。
- `mise run check` が通る。

## 関連

- docs/issues/0147-refactor-extract-sso-device-auth-package.md: 同じ TODO 項目の第 1 段階。本 issue は 0147 が抽出する共有パッケージに依存するため、0147 の完了後に着手する。
- docs/issues/0149-feat-sso-login-refocus-frontend-tab.md: 同じ TODO 項目の第 3 段階。本 issue が追加するエンドポイントを frontend から利用し、旧エンドポイントを削除する。
- docs/issues/closed/0030-bug-aws-sso-expiry-not-refreshed-after-login.md: SSO ログイン後に期限表示が更新されないバグの修正。`complete` 成功後の SSO ステータス反映はこの対応で確立した経路に載る。

## 解決方法

設計判断のとおり、デバイス認可の開始と完了を分離した 2 つのエンドポイントを追加した。既存の `POST /api/aws/profiles/{profile}/sso/login` と `handleSSOLogin` は変更していない。

### 実装

- `backend/internal/aws/sso_config.go` (新規): 公開関数 `ResolveSSOConfig(profileName)` を追加した。`~/.aws/config` を `parseAWSConfig` でパースし、profile 名から `SSOConfig{Region, StartURL}` を解決する。sso_session 形式と inline 形式の併存時は sso-session 側を優先する (`applySSOStatus` と同じ方針)。profile が config に無い場合は `~/.aws/credentials` を `parseCredentials` で参照し、credentials のみで定義された profile は `ErrProfileNotFound` ではなく `ErrSSONotConfigured` に分類する (`listProfiles` が credentials-only の profile を一覧に含めることとの整合)。credentials の読み取り失敗 (存在しない以外) は `listProfiles` と同じく `slog.Warn` を出して分類の補助を諦める。
- `backend/internal/aws/profiles.go`: `profileSection` に `SSORegion` フィールドを追加し、`parseAWSConfig` が `sso_region` キーを保持するようにした (従来は inline 形式の `sso_region` を読み捨てていた)。
- `backend/internal/aws/errors.go`: `ErrSSONotConfigured` を追加した。
- `backend/internal/aws/sso_oidc.go`: `access_denied` 判定 (`ErrSSOTokenTimeout`) を handler 層から `errors.Is` で判別できるよう公開した。
- `backend/internal/api/sso_login_sessions.go` (新規): `ssoLoginSessionStore` を追加した。セッション ID は `crypto/rand` 由来の 32 バイト hex、`map[string]ssoLoginSession` を `sync.Mutex` で保護する。失効は `StartDeviceAuthorization` の `expiresIn` (欠落時は 600 秒) を期限とし、`put` / `take` 到達時に一括掃除する。`take(profile, id)` は一致時のみ削除する一回限りの取り出しで、profile 不一致では消費しない (誤 profile への complete が正規の complete を潰さないため)。乱数生成は `randRead` フィールドで注入可能にし、失敗経路をテストした。
- `backend/internal/api/handlers_sso.go`: `handleSSOLoginStart` と `handleSSOLoginComplete` を追加した。依存は `ssoLoginDeps` (resolveConfig / start / wait) として注入する。start は `r.Context()` に `ssoLoginStartTimeout` (30 秒) を掛ける (認可開始は同期処理でブラウザ待機を含まないため。start のタイムアウトはネットワーク起因の失敗であり、認可待機の 504 とは異なるので 500 `SSO_LOGIN_FAILED` に分類する)。complete は `context.Background()` 起点で現行と同じ 5 分の `ssoLoginTimeout` を使う。complete の profile 名は `ValidateProfileName` で検証する。
- `backend/internal/api/routes.go` / `server.go` / `server_test.go`: ルート 2 本と `Server` のフィールド (`ssoLoginSessions` / `ssoLogin`) を追加した。`newTestServer` にはエラーを返すダミー deps (`unconfiguredSSOLoginDeps`) を配線し、テストが誤って実 AWS へ接続しないようにした。

エラーコードの対応は次のとおり。

- start: 404 `PROFILE_NOT_FOUND`、400 `SSO_NOT_CONFIGURED`、400 `BAD_REQUEST` (不正 profile 名)、500 `SSO_LOGIN_FAILED` (開始失敗、タイムアウト含む)。
- complete: 400 `BAD_REQUEST` (不正 profile 名、不正ボディ、空 session_id)、404 `SSO_LOGIN_SESSION_NOT_FOUND` (未知、失効、使用済み、他 profile 宛)、403 `SSO_LOGIN_ACCESS_DENIED` (認可拒否)、504 `SSO_LOGIN_TIMEOUT` (認可待機タイムアウト)、500 `SSO_LOGIN_FAILED` (キャッシュ保存失敗を含むその他)。

### 検証

完了条件のハンドラ単体テスト (成功、SSO 設定なし profile、ポーリングタイムアウト、access_denied、未知または失効したセッション ID、キャッシュ保存失敗、同一 profile での並行セッション) は `handlers_sso_test.go` に追加した。`ResolveSSOConfig` の単体テストは `sso_config_test.go` に追加した (公開経路は `t.Setenv("HOME")` で検証)。セッションストアの失効、一回限りの取り出し、profile 不一致、乱数失敗、並行アクセスは `sso_login_sessions_test.go` で検証した。

完了条件 3 (complete 成功後に SSO ステータスが `valid` になる) は、直接の結合テストを持たない。0147 の `ssoauth.WaitForToken` が書き込むキャッシュパスが `os.UserHomeDir` 固定でハンドラテストから差し替えられないためで、次の 2 点の組で満たされていると判断した: (1) complete 成功はキャッシュ保存の成功を意味する (保存失敗は 500 になることをテスト済み)、(2) キャッシュファイルから `SSOStatusValid` を導く経路は docs/issues/closed/0030 で確立済みで、0147 が保存形式を AWS CLI 互換のまま抽出したことは 0147 のテストが担保する。

完了条件のテスト通過は `mise run check` の全通過で確認した (frontend 73 ファイル 732 テスト、backend 全パッケージ、govulncheck 0 件。ベースラインからの新たな失敗なし)。

### 0147 からの持ち越し論点の決着

- ポーリングの起点: セッションが `expiresIn` で失効するため、start から時間が経った complete は 404 になり、期限切れデバイスコードでのポーリングは始まらない。追加の対策は不要と判断した。
- `ErrIncompleteSession` 相当のセンチネル: complete が受けるセッションはストアから取り出した完全な値のみなので、不完全セッションの分類は不要だった。導入していない。

### レビューでの主な変更

多観点レビュー (5 観点 × 1 ラウンド + 変更点限定の追加 1 ラウンド) で次を反映した。

- `take` を `take(profile, id)` に変更し、profile 不一致でセッションを消費しないようにした (観点 3)。
- start に 30 秒のタイムアウトを追加した (観点 3)。
- complete に `ValidateProfileName` を追加した (観点 3)。
- credentials-only profile の分類 (`ErrSSONotConfigured`) と読み取り失敗時の `slog.Warn` を追加した (観点 5、追加ラウンド観点 3)。
- `randRead` の注入と乱数失敗、start タイムアウト、並行アクセスのテストを追加した (観点 2、追加ラウンド観点 2)。
- `newTestServer` にダミー deps を配線した (追加ラウンド観点 3)。

却下した指摘は 4 件: `t.Parallel()` の付与 (共有状態は無いがパッケージの既存テストが使っておらず慣習に合わせた)、ハンドラ個別のエラーログ追加 (docs/issues/0151 のミドルウェアで一括対応する)、close 前の作業ツリーに CHANGES.md エントリが無いこと (Step 8 で書き込む運用のため作業ツリーに無いのが正しい状態)、take の profile 不一致への監査ログ追加 (ローカル常駐の個人ツールに監査要件は無く、404 応答はアクセスログと 0151 で追跡できる)。
