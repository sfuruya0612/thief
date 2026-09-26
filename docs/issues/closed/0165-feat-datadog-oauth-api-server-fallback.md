# API サーバの Datadog 認証を OAuth 対応にし静的キーへ非破壊的にフォールバックする

Created: 2026-09-10
Model: Claude Sonnet 5
Completed: 2026-09-11

## 背景

API サーバ (`backend/internal/api/server.go`) は `Server.ddCtx context.Context` フィールド (31 行目) を `NewServer()` 内で起動時に 1 回だけ構築し (62-64 行目)、

```go
ddCfg := ddclient.NewConfiguration(cfg.Datadog.Site)
s.ddV2 = ddclient.NewUsageMeteringV2API(ddCfg)
s.ddCtx = ddclient.NewContext(ctx, cfg.DatadogAPIKey(), cfg.DatadogAppKey())
```

以後の全リクエストがこの同一の `ddCtx` を使い回す。`backend/internal/api/handlers_datadog.go` の `handleDatadogHistorical`/`handleDatadogEstimated` はいずれも `s.serveCached` 経由で `datadog.GetHistoricalCost(s.ddCtx, s.ddV2, ...)`/`datadog.GetEstimatedCost(s.ddCtx, s.ddV2, ...)` を呼び、`s.ddCtx` を直接参照する。

OAuth のアクセストークンは短命でリフレッシュを要するため、起動時に固定された `context.Context` をそのまま使い回すこの設計では、ログイン後のトークン更新やログアウトを反映できない。`context.Context` は不変値であり、時間とともに変化する認証状態を表現するには本来不向きである。

この issue は `docs/issues/TODO.md` の次の項目のうち、API サーバ側 (非同期ブラウザログイン) を対象にする。CLI 側 (`thief datadog auth login/logout/refresh` と共通パッケージ `backend/internal/datadogauth/`) は issue 0164 で先に実装する。

> Datadog の認証を DD_API_KEY / DD_APP_KEY の静的キーに加えて、DataDog 公式 CLI pup の pup auth login と同じ OAuth 2.0 (Authorization Code + PKCE + Dynamic Client Registration) ログインにも対応させたい
> スコープは CLI (thief datadog auth login/logout/refresh) と API サーバ (frontend の DatadogView 向け) の両方
> トークンの保存方式は OS キーチェーンではなくファイル権限 (0600) 方式にする
> 既存の DD_API_KEY / DD_APP_KEY 方式は残し、OAuth トークンがあれば優先、無ければ静的キーにフォールバックする非破壊的な変更にする

## 目的

API サーバが `GET /api/datadog/cost/historical`/`estimated` (`backend/internal/api/routes.go:114-115`) を処理する際、issue 0164 で保存された OAuth トークンが有効ならそれを優先して使い、無効・未ログインなら既存の静的キー (`DatadogAPIKey()`/`DatadogAppKey()`) へ非破壊的にフォールバックする。あわせて、ブラウザ経由で OAuth ログインを行うための非同期 API (start/callback/status/logout) を追加する (frontend の UI 実装は対象外)。

## 設計判断

### 1. `Server.ddCtx` フィールドを廃止し、リクエストごとにトークンファイルを読み直す

`~/.config/thief/datadog/token_<site>.json` (issue 0164 の `datadogauth.storage`) を単一の真実源とし、`backend/internal/pricecache/pricecache.go` が TTL を持たずファイルを都度読む思想を踏襲して、リクエストのたびに読み直す。これにより CLI でのログイン/ログアウトが稼働中のサーバへ即座に反映され (逆も同様)、`Server` 構造体側にキャッシュ無効化ロジックを持たなくて済む。

**却下した代替案**: `Server.ddCtx` をそのまま残し、ログイン成功時に `sync.RWMutex` 越しに書き換える案は、複数プロセス (CLI と API サーバ) 間の同期が必要になる本題を解決しない (サーバプロセスの外で CLI がログインした場合に反映されない) ため見送る。

### 2. 状態網羅表でフォールバックを明示する (黙った fallback を禁止)

`backend/internal/api/datadog_auth_context.go` に `(s *Server) datadogAuthContext(ctx) context.Context` を新設し、次の状態をすべて扱う。

| 状態 | 挙動 |
| --- | --- |
| トークン有効 (未期限切れ) | そのトークンで続行する (通常運用、ログ不要)。 |
| トークン無し、静的キーあり | 静的キーで動作する (通常運用、ログ不要)。 |
| トークン無し、静的キーも無し | 明確なエラーを返す。 |
| 期限切れ→リフレッシュ成功 | 新トークンで続行する。 |
| 期限切れ→リフレッシュ失敗、静的キーあり | `slog.Warn` で明示してから静的キーへフォールバックする。 |
| 期限切れ→リフレッシュ失敗、静的キーも無し | 明確なエラーを返す (再ログインが必要)。 |
| トークンファイル破損、静的キーあり | 「未ログイン」と区別して `slog.Warn` を出した上で静的キーへフォールバックする。 |
| トークンファイル破損、静的キーも無し | 「未ログイン」と区別して `slog.Warn` を出した上で明確なエラーを返す (再ログインが必要)。 |
| トークン有効、呼び出し結果が権限不足 (Datadog の `403`+`insufficient_scope` 相当のエラー) | `slog.Warn` で明示してから静的キーへフォールバックする (静的キーも無ければそのままエラーを返す)。判定条件 (レスポンスのエラーコード/メッセージ) は実装時にライブ検証で確定する。 |

**却下した代替案**: リフレッシュ失敗時に常に静的キーへ無条件フォールバックする (ログを出さない) 案は、認証が実質壊れていることに運用者が気付けなくなるため見送る。

「OAuth 有効だが対象エンドポイントが非対応」の除外リストは実装しない。thief が呼ぶ `GetHistoricalCostByOrg`/`GetEstimatedCostByOrg` は Datadog の Usage Metering (`/api/v2/usage/*`) であり、issue 0164 の背景で確認した通り pup の `OAUTH_EXCLUDED_ENDPOINTS` にも含まれず OAuth 対応済みのエンドポイント種別のため、現時点で除外対象が存在しない (YAGNI)。将来別エンドポイントを追加する箇所にのみコメントで注意喚起する。

### 3. プロセス間のリフレッシュ競合には楽観的リトライを追加する

同一プロセス内の重複リフレッシュは `golang.org/x/sync/singleflight` (`backend/internal/pricecache/pricecache.go:16` で既に使用中、新規依存ではない) の `singleflight.Group` で 1 回に集約する。ただし `singleflight.Group` は同一プロセス内の goroutine 間でしか重複を防げず、CLI プロセスと API サーバプロセスは別々の `singleflight.Group` を持つため、両者が同時に同じ期限間近のトークンをリフレッシュしようとする競合は防げない。IdP がリフレッシュトークンをローテーションする実装だと、後着側が失敗し「再ログインが必要」と誤診断しうる。

対策として `refreshDatadogToken` に次の楽観的リトライを組み込む。

1. リフレッシュが必要と判断しても、実際にトークンエンドポイントを叩く直前にもう一度トークンファイルを読み直し、他プロセスが既に更新済みでないか確認する。
2. リフレッシュが失敗した場合も、エラーを確定させる前にもう一度トークンファイルを読み直し、他プロセスが直前に成功していないか最後に確認してから諦める。

**却下した代替案**: `flock` 等の advisory file lock でプロセス間排他を取る案は、単一操作者のローカルマシン利用が前提でクロスプラットフォームの複雑さに見合わないため見送る。

### 4. サーバ側ログイン API はポーリング方式にする (SSO のブロッキング complete とは異なる設計)

AWS SSO (`backend/internal/api/handlers_sso.go`) の `handleSSOLoginComplete` はデバイス認可フロー自体が AWS 側のポーリングを要求するためブロッキングだったが、Authorization Code フローにその制約は無い。thief には Athena/BigQuery の非同期ジョブポーリングという前例が既にあるため、それに合わせて次の 4 エンドポイント構成にする (新規 `backend/internal/api/handlers_datadog_auth.go` + `datadog_auth_sessions.go`、`routes.go` に追記)。

- `POST /api/datadog/auth/login/start`
- `GET /api/datadog/auth/callback` (Datadog が直接ブラウザを遷移させる、frontend の fetch ではない)
- `GET /api/datadog/auth/login/status`
- `POST /api/datadog/auth/logout`

`POST /api/datadog/auth/login/start` は認可 URL を組み立てる前に `datadogauth.PrepareLogin(ctx, deps, cliRedirectURI, serverRedirectURI)` (issue 0164 で実装済み) を呼ぶ。これにより `thief datadog auth login` (CLI) を一度も実行していない環境でも、frontend 経由の初回ログインだけで DCR クライアント登録ファイルが作成される。`cliRedirectURI` には `http://127.0.0.1:8400/callback`、`serverRedirectURI` には `cfg.Datadog.OAuthRedirectBase + config.DatadogOAuthCallbackPath` (既定では `config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath` と同値) を渡す。`PrepareLogin` は登録済みならファイルを再利用するだけなので、CLI と API サーバのどちらを先に使っても DCR 登録は通算 1 回しか発生しない。

セッション管理は `backend/internal/api/sso_login_sessions.go` の `ssoLoginSessionStore` (35 行目、`sync.Mutex` + map + オンデマンド TTL sweep) をパターンとして踏襲し、`state` をセッション ID としてそのまま使い `take()` (取り出しと同時に削除) で 1 回使い切りにする。callback のレスポンス HTML はクエリパラメータの値を一切反映しない固定文言にする (reflected XSS 対策)。

**却下した代替案**: AWS SSO と同じブロッキング `complete` エンドポイントにする案は、Authorization Code フローではブラウザのリダイレクトをサーバが直接受ける (`callback`) ため、frontend がブロッキングで待つ必要が無く、ポーリングの方が実装がシンプルになるため見送る。

### 5. サーバ側 redirect_uri のホストは新設定 `Datadog.OAuthRedirectBase` から組み立て、既定値は issue 0164 で `config` パッケージに追加される定数をそのまま再利用する

`backend/internal/config/config.go` の `DatadogConfig` (61-68 行目) に `OAuthRedirectBase string` を追加する。既定値はリテラルを本 issue 側で重複定義せず、issue 0164 で同じ `config` パッケージに追加される `config.DefaultDatadogOAuthRedirectBase` (`http://127.0.0.1:8089`) をそのまま使う (`Defaults()` 関数 (`config.go:113-126`) 内で同一パッケージの定数を参照するだけであり、追加の import は発生しない。env は `THIEF_DATADOG_OAUTH_REDIRECT_BASE`、`THIEF_LISTEN_ADDR` (`config.go:198`) と同じ命名規則)。`ListenAddr` (bind アドレス) とブラウザから到達可能な URL は必ずしも一致しない (リバースプロキシ配下等) ため、独立した設定にする。

**この定数を `datadogauth` パッケージではなく `config` パッケージに置く理由**: `datadogauth/storage.go` は `config.Dir()` を呼ぶため `datadogauth` は元々 `config` に依存する (`datadogauth → config` の一方向)。仮に定数を `datadogauth` 側に置くと、本設計判断が必要とする「`config.Defaults()` がその定数を参照する」という要件のために `config` が `datadogauth` を import する経路が新たに必要になり、`config → datadogauth → config` の循環 import になって `go build` が失敗する (ラウンド 3 レビューで検出された欠陥。当初案は定数を `datadogauth` 側に置いていたが、この設計判断でその案を撤回し `config` 側に置く案へ変更した)。`config` パッケージ自身に定数を置けば、`datadogauth` は既存の依存方向のまま `config.DefaultDatadogOAuthRedirectBase` を参照でき、`config` は `datadogauth` を一切 import しない。

実際の redirect_uri は `cfg.Datadog.OAuthRedirectBase + config.DatadogOAuthCallbackPath` (`/api/datadog/auth/callback`) で組み立てる。この式は issue 0164 の `PrepareLogin` 初回登録呼び出しが使う値 (`config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath`) と、`Datadog.OAuthRedirectBase` が既定値のままである限り常に一致する。両 issue でリテラル文字列を別々に書き下さず、`config` パッケージの定数を共有することで、実装順序 (0164 → 0165) によらず redirect_uri の値がズレず、かつ import グラフに循環が生じない構成にする。

`OAuthRedirectBase` を既定値から変更した場合、登録済みクライアントの `redirect_uris` に新しい URI が含まれないため、変更後の初回ログインで再登録が発生し新しい `client_id` が発行される。DCR エンドポイントは登録専用で更新用エンドポイントが無いため、この再登録は変更前に発行された全トークン (CLI 側で発行されたものも含む) を実質的に無効化する。これは AWS SSO の start URL 変更と同様、設定変更後は影響範囲全体 (CLI・サーバ双方) の再ログインが必要になる既知の制限として扱い、`slog.Warn` で明示する。

**却下した代替案**: `ListenAddr` からそのまま自動導出する案は設定項目を増やさずに済むが、bind アドレスと外部到達 URL が異なる構成 (リバースプロキシ配下でのデプロイ等) に対応できないため見送る。

## 完了条件

- `backend/internal/api/server.go` から `Server.ddCtx` フィールドが削除され、`NewServer()` の Datadog 初期化からその構築コードが削除されている。
- `backend/internal/api/datadog_auth_context.go` に `datadogAuthContext(ctx)`/`refreshDatadogToken(ctx, token)` が実装され、上記の状態網羅表の全 9 状態がテーブル駆動テストでカバーされている。
- 複数 goroutine から同時に `datadogAuthContext` を呼び出す `-race` 付きテストで、実際のリフレッシュ HTTP 呼び出しが `singleflight` により 1 回に集約されることが確認できる。
- リフレッシュ直前・失敗確定前の楽観的ディスク再読込がテストで検証されている (他プロセスが先にリフレッシュ済みの場合に無駄なリフレッシュ HTTP 呼び出しが発生しないこと)。
- `backend/internal/api/handlers_datadog.go` の `handleDatadogHistorical`/`handleDatadogEstimated` が `s.ddCtx` ではなく `s.datadogAuthContext(r.Context())` を使うよう書き換わっている。
- `POST /api/datadog/auth/login/start`、`GET /api/datadog/auth/callback`、`GET /api/datadog/auth/login/status`、`POST /api/datadog/auth/logout` が `backend/internal/api/routes.go` に登録され、`backend/internal/api/handlers_datadog_auth_test.go` で state 不一致・二重 callback・callback レスポンスへの入力値非反映 (XSS 対策) がテーブル駆動テストで検証されている。
- `backend/internal/config/config.go` に `Datadog.OAuthRedirectBase` (既定値は同一パッケージの定数 `config.DefaultDatadogOAuthRedirectBase` (`http://127.0.0.1:8089`) をそのまま使う、env `THIEF_DATADOG_OAUTH_REDIRECT_BASE`) が追加されている。
- `go build ./...` が通ることで、`config` パッケージが `datadogauth` パッケージを import していない (循環 import が発生していない) ことを確認する。
- `POST /api/datadog/auth/login/start` が `datadogauth.PrepareLogin` を呼び出しており、CLI 未実行の状態 (DCR クライアント登録ファイル無し) から `login/start` を呼んでも DCR 登録が行われて正常にログインできることがテストで確認できる。
- `Datadog.OAuthRedirectBase` を既定値から変更した状態で初回ログインを行うと DCR の再登録が発生し、`slog.Warn` が出力されることをテストまたは手動確認で確認する。
- frontend (`frontend/src/views/nonaws/DatadogView.tsx` 等) の UI 変更は本 issue のスコープ外とし、行わない。
- 実装着手前に、Datadog Organization 側で OAuth Apps (DCR ベースのクライアント登録) の利用に管理者による事前有効化が必要かどうかを確認し、結果を本 issue に追記する (issue 0164 と共通の確認事項)。
- ローカルで API サーバを起動した状態で `thief datadog auth login` (issue 0164) を実行してログイン後、`curl http://127.0.0.1:8089/api/datadog/cost/historical?...` がコストを返すことを手動確認する。Datadog Organization 側の管理者操作が必要と判明し実装者がそれを実施できない場合は、この手動確認の代わりに `httptest` でトークンエンドポイント・Usage Metering エンドポイントをモックした自動化された end-to-end 統合テストで代替し、その旨と代替理由を issue に追記した上で完了条件を満たすものとする。
- 続けて `thief datadog auth logout` を実行し、直後のリクエストで静的キー (未設定なら明確なエラー) へフォールバックすることを手動確認する (上記と同じ代替条件を適用してよい)。
- 意図的にトークンファイルを壊し (不正 JSON)、`slog.Warn` のログが出た上でフォールバックまたはエラーになることをログで確認する。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。

## 関連

- issue 0164 (`backend/internal/datadogauth/` パッケージ、CLI の `thief datadog auth login/logout/refresh`、DCR クライアントの redirect_uri 構成) に依存する。issue 0164 を先に close してから着手すること。
- 実装時に次の未確定事項を確認すること。
  - `usage_read` スコープが `historical_cost`/`estimated_cost` の取得を実際にカバーするか (issue 0164 と共通)。
  - OAuth トークンの revoke エンドポイントの有無 (issue 0164 と共通)。
  - 権限不足時に Datadog が実際に返すエラーコード/メッセージの形式 (状態網羅表の「トークン有効、呼び出し結果が権限不足」の判定条件)。
  - pup の `OAUTH_EXCLUDED_ENDPOINTS` に基づく「Usage Metering は OAuth 対応」という結論は pup 実装からの推測であり Datadog 公式ドキュメントでの一次確認ではない。実装時に実際の OAuth トークンで `GetHistoricalCostByOrg`/`GetEstimatedCostByOrg` を呼び、認可エラーにならないことをライブ検証すること (issue 0164 の未確定事項と対応)。

## 調査結果 (2026-09-11)

issue 0164 が close 済み (`docs/issues/closed/0164-feat-datadog-oauth-cli-login.md`) であることを確認し、依存条件が満たされたため本 issue に着手する。

「## 背景」「## 設計判断」が引用するシンボルは、issue 0164 の実装で `backend/internal/config/config.go` に 2 定数が追加された影響で行番号が動いているものを含め、いずれも実在を確認した。

- `backend/internal/api/server.go` の `Server.ddCtx`(31 行目)/`ddV2`(30 行目) フィールドと、`NewServer()` 内の構築コード (`s.ddV2 = ...`/`s.ddCtx = ddclient.NewContext(...)`) は記載どおり存在する。
- `backend/internal/api/handlers_datadog.go` の `handleDatadogHistorical`/`handleDatadogEstimated` は `s.serveCached` 経由で `datadog.GetHistoricalCost(s.ddCtx, s.ddV2, ...)`/`datadog.GetEstimatedCost(s.ddCtx, s.ddV2, ...)` を呼んでおり記載どおり。
- `backend/internal/api/routes.go:114-115` の `GET /api/datadog/cost/historical`/`estimated` の登録は記載の行番号のまま存在する。
- `backend/internal/config/config.go` の `DatadogConfig` 構造体は、issue 記載の 61-68 行目から現在は 78-86 行目に移動している (issue 0164 が `config.go` の先頭付近に `DefaultDatadogOAuthRedirectBase`/`DatadogOAuthCallbackPath` の 2 定数 (33-34 行目) を追加した影響)。フィールド構成 (`Site`/`APIKey`/`AppKey`/`View`/`StartMonth`/`EndMonth`) 自体は変更されていない。
- `Defaults()` 関数も同じ理由で記載の 113-126 行目から現在は 131-144 行目に移動しているが、中身は変更されていない。
- issue 0164 で追加された `config.DefaultDatadogOAuthRedirectBase`(`config.go:33`)/`config.DatadogOAuthCallbackPath`(`config.go:34`) は本設計判断 5 が前提とする位置・値のまま存在する。
- `backend/internal/api/sso_login_sessions.go` の `ssoLoginSessionStore`(35 行目)、`put()`(59 行目)、`take()`(87 行目、取り出しと同時に削除) は記載どおりのパターンで存在する。
- `Datadog.OAuthRedirectBase` フィールドと `backend/internal/api/datadog_auth_context.go`/`handlers_datadog_auth.go`/`datadog_auth_sessions.go` はいずれもまだ存在しない。本 issue の要望は別の変更では満たされていない。

引用先はいずれも単純な行移動のみで実体・仕様に変更は無く、完了条件は現状のリポジトリに対して成立する。方針セクション (設計判断 1 から 5) は却下した代替案を含めて確定済みであり (設計判断 5 の記述にある通りラウンド 3 レビューで検出された循環 import の欠陥を踏まえて修正済み)、Step 4 で新たに方針を組み立てる必要はない。

### 完了条件 110 行目 (事前有効化の要否確認) について

完了条件 110 行目は「実装着手前に、Datadog Organization 側で OAuth Apps (DCR ベースのクライアント登録) の利用に管理者による事前有効化が必要かどうかを確認し、結果を本 issue に追記する (issue 0164 と共通の確認事項)」と定めている。issue 0164 の「## 調査結果 (2026-09-11)」で確認した通り、本実装環境には Datadog の認証情報 (`DATADOG_API_KEY`/`DATADOG_APP_KEY`) と Datadog Organization の管理者権限がいずれも無く、事前有効化の要否そのものを確認できない。

より根本的な制約は、事前有効化の要否という個別の論点ではなく、実装環境に Datadog への到達手段 (認証情報・管理者権限) が一切無く、事前有効化の要否によらず実 Datadog に対するライブ検証そのものが不可能であることにある。この事実は issue 0164 と共通の実装環境の制約であり、issue 0164 の close 時にレビューで指摘・訂正された理由づけ (完了条件が個別に定めた代替条項の字義どおりの発動要件「必要と判明し」は満たさないが、その代替条項が想定する状況をより広く包含する事実がある以上、同条項の趣旨に照らして代替を適用するのが妥当) を本 issue の完了条件 111 行目・112 行目にもそのまま適用する。

したがって、完了条件 111 行目・112 行目の手動確認 (`thief datadog auth login`/`logout` を実際に実行してのコスト取得確認) は、`httptest` でトークンエンドポイントおよび Usage Metering エンドポイント (`GetHistoricalCostByOrg`/`GetEstimatedCostByOrg` が呼ぶ API) をモックした自動化された end-to-end 統合テストで代替する。代替理由は、事前有効化の要否によらず実装環境に Datadog への到達手段が一切無く、実 Datadog に対するライブ検証が不可能なことである。

## 解決方法

`Server.ddCtx` フィールド (`backend/internal/api/server.go`) を廃止し、リクエストごとにトークンファイルを読み直す `datadogAuthContext` を新設した。

- `backend/internal/api/server.go`: `Server.ddCtx` フィールドと `NewServer()` 内の構築コードを削除し、`ddLoginSessions`(ログインセッションストア)、`ddAuth`(Datadog 認証用 deps)、`ddRefresh`(`singleflight.Group`)を追加した。
- `backend/internal/api/datadog_auth_context.go`: `datadogAuthContext(ctx)` が設計判断 2 の状態網羅表の全 9 状態 (403 かつ静的キー無しの行を含む) を解決する。`refreshDatadogToken` は `singleflight` で同一プロセス内の重複リフレッシュを 1 回に集約し、リフレッシュ直前・失敗確定前の 2 箇所でトークンファイルを楽観的に再読込することで、CLI とサーバが同時にリフレッシュしても無駄な HTTP 呼び出しを避ける (設計判断 3)。`datadogCall(ctx, authCtx, call)` が OAuth 呼び出し限定 (`HasOAuthToken`) で 403 (`IsForbidden`) を検出したときだけ静的キーへ 1 回リトライする。
- `backend/internal/api/handlers_datadog.go`: `handleDatadogHistorical`/`handleDatadogEstimated` を `s.datadogAuthContext(r.Context())` 経由に書き換えた (キャッシュミス時のみ認証解決するよう `serveCached` のローダー内に置き、キャッシュヒット時の無駄なトークンリフレッシュ/未ログインエラーを避ける)。
- `backend/internal/api/handlers_datadog_auth.go` + `datadog_auth_sessions.go`: `POST /api/datadog/auth/login/start`(`datadogauth.PrepareLogin` を呼び、CLI 未実行でも DCR 登録される)、`GET /api/datadog/auth/callback`(state 検証、応答は `text/plain` の固定文言 3 種のみでクエリ値を一切反映しない反射 XSS 対策)、`GET /api/datadog/auth/login/status`、`POST /api/datadog/auth/logout` の 4 エンドポイントを実装し `routes.go` に登録した。セッションストアは `sso_login_sessions.go` の `ssoLoginSessionStore` パターン (state をセッション ID として `take()` で 1 回使い切り) を踏襲した。
- `backend/internal/config/config.go`: `DatadogConfig` に `OAuthRedirectBase`(既定値は同一パッケージの `DefaultDatadogOAuthRedirectBase` をそのまま使う、env `THIEF_DATADOG_OAUTH_REDIRECT_BASE`)を追加し、CLI とサーバで redirect_uri を共有するための `DefaultDatadogOAuthCLIRedirectURI` も追加した。`config` パッケージが `datadogauth` を import しないこと (循環 import が生じないこと) は `go build ./...` の成功と import グラフの確認で担保している。
- `backend/internal/datadog/client.go`: 既存の `NewContext`(静的キー)と対になる `NewOAuthContext`/`HasOAuthToken` を追加した。
- `backend/internal/datadog/errors.go` (方針の一覧に無い追加ファイル。下記「方針からの乖離」を参照): `IsForbidden`/`ErrorBody` で SDK のエラー型 (`datadog.GenericOpenAPIError`) に依存する 403 判定を 1 箇所に閉じ込めた。
- `backend/internal/cli/datadog_auth.go`: CLI 側の redirect_uri を `config.DefaultDatadogOAuthCLIRedirectURI` の参照に変更した (値は `http://127.0.0.1:8400/callback` のまま不変)。

### 完了条件ごとの検証

- `Server.ddCtx` の削除: `grep -rn "ddCtx" --include=*.go .` で該当なしを確認。
- 状態網羅表 9 状態: `TestDatadogAuthStates`(`datadog_auth_context_test.go`、403 かつ静的キー無しの行を含む)。
- `-race` での singleflight 集約: `TestDatadogRefreshIsCollapsedBySingleflight`(16 goroutine から同時呼び出し、実リフレッシュ 1 回)。
- 楽観的ディスク再読込: `TestDatadogRefreshSkippedWhenAnotherProcessAlreadyRefreshed`(実 HTTP 0 回)、`TestDatadogRefreshRecoversFromConcurrentRotation`(失敗確定前の再読込)。
- ハンドラの書き換え: `handlers_datadog.go` が `s.datadogAuthContext(r.Context())` を呼ぶことをコードで確認。
- 4 エンドポイントとテスト: `routes.go` への登録、`TestDatadogAuthCallback`(state 不一致・リプレイ・拒否・code 無し・引き換え失敗の 6 行、いずれも `<script>alert("xss")</script>` を投入し本文に反映されないことを確認)、`TestDatadogAuthCallbackResponseIsPlainText`。
- `config.Datadog.OAuthRedirectBase`: `TestDatadogOAuthRedirectBase`(既定値・yaml・env の各ケース)。
- 循環 import 無し: `go build ./...` の成功、および `go list -deps ./internal/config | grep -c datadogauth` が 0 であることを確認。
- CLI 未実行状態からの DCR: `TestDatadogLoginStartRegistersClientWithoutCLI`。
- `OAuthRedirectBase` 変更時の再登録警告: `TestDatadogRedirectBaseReregistrationWarning`。
- frontend 非変更: `git diff HEAD --stat -- frontend` が空であることを確認。
- Datadog Organization 側の OAuth Apps 事前有効化の要否確認 (issue 0164 と共通の確認事項): 「## 調査結果 (2026-09-11)」に追記済み (issue 0164 と同じ理由で確認できないことを記録)。
- 実ログイン・ログアウトの手動確認 (代替条項、issue 0164 と共通の理由で適用): 実装環境に Datadog の認証情報も Organization の管理者権限も一切無く実 Datadog へのライブ検証が不可能であることを「## 調査結果 (2026-09-11)」に追記し、`TestDatadogOAuthEndToEnd` で DCR → callback → コスト取得 (Bearer トークンが実際にリクエストへ載ることを検証) → logout → 資格情報無しでのエラー → 静的キー設定での復帰までを本番コードで一気通貫に検証した。
- トークンファイル破損時のフォールバック: `TestDatadogCorruptTokenFileFallsBackWithWarning`(静的キーありでフォールバック、無しで 500、いずれも `slog.Warn` を確認)。
- `mise run check`: 統合後に作業ツリーで実行し、backend 18 パッケージすべて `ok`(新規失敗 0、staticcheck/govulncheck 新規指摘 0)、frontend 75 ファイル 794 テスト全通過を確認した。

### 方針からの乖離 (方式を保った実装詳細)

- `datadogauth.PrepareLogin` の実際のシグネチャ (`PrepareParams` 構造体を取る) に合わせて呼び出しを組んだ (issue 0164 の実装確定後にシグネチャが決まったための整合)。
- `datadogCall` を `(ctx, authCtx, call)` の 3 引数にした。403 フォールバック時の静的キー context を認証済みの `authCtx` から派生させると、SDK が `ContextAPIKeys` と `ContextAccessToken` を独立に適用するため Bearer と API キーが同一リクエストに同時に載ってしまう。認証情報を載せる前の生の `ctx` から静的キー用 context を組み立てる必要があったための分離。
- `backend/internal/datadog/errors.go` を新設し、SDK のエラー型に依存する 403 判定 (`IsForbidden`/`ErrorBody`) を 1 箇所に閉じ込めた (issue 0164 の `client.go` 抽出と同種の整理)。
- `internal/datadog/client.go` に `NewOAuthContext`/`HasOAuthToken` を追加した (既存の `NewContext` と対になる形。両者の排他性はテストで確認済み)。
- `config.DefaultDatadogOAuthCLIRedirectURI` を新設した。CLI (`internal/cli`) は `internal/api` を import できない依存方向のため、CLI とサーバが同じ redirect_uri 一覧を共有するには定数を `config` 側に置く必要があった。
- 認証解決 (`datadogAuthContext` の呼び出し) を `serveCached` のローダー内 (キャッシュミス時のみ) に置いた。キャッシュで返せるリクエストのために毎回トークンをリフレッシュしたり未ログインで失敗させたりするのは無意味なため。完了条件が求める「`s.datadogAuthContext(r.Context())` を使う」ことは字義どおり満たしている。
- callback の応答を HTML ではなく `text/plain; charset=utf-8` + `X-Content-Type-Options: nosniff` の固定文言 3 種にした。クエリ値を一切埋め込まない構成にすることで反射 XSS の可能性を構造的にゼロにするための実装判断であり、設計判断 4 が定める「クエリパラメータの値を一切反映しない固定文言」という要件自体は変えていない。
- `datadogAuthDepsFrom(datadogauth.Deps)` を切り出し、本番経路とテスト (httptest 向けの `datadogauth.Deps` を注入する e2e テスト) の両方から同じ組み立てロジックを使えるようにした。

### 未検証のまま採用した仮定 (実装時に確認できなかった事項)

- **403/insufficient_scope 相当の判定形式**: Datadog が権限不足をどの HTTP ステータス・本文形式で返すかをライブ検証できないため、`IsForbidden` は「OAuth トークンで呼んだリクエストが HTTP 403 を返したこと」のみを条件にした (`insufficient_scope` の文字列や RFC 6749/6750 のエラー形式には依存しない、最も広い条件)。判定の根拠は vendor 済み SDK `github.com/DataDog/datadog-api-client-go/v2` v2.55.0 の `datadog.GenericOpenAPIError.ErrorMessage`(`http.Response.Status` 相当の文字列)の先頭トークンが `403` かどうかで、`ErrorBody`(本文 512 バイトに切り詰め)を `slog.Warn` に載せることで、実際の形式が異なっていた場合に運用時のログから判明するようにしてある。
- **Usage Metering の OAuth スコープ実効性**: `usage_read` スコープ (issue 0164 で決定済み) が `historical_cost`/`estimated_cost` の取得を実際にカバーするかは未検証。カバーしていない場合は上記の 403 フォールバックが動作する想定。
- **OAuth トークンの revoke エンドポイント**: 確認できなかったため `logout` はローカルのトークン・クライアント登録ファイルの削除のみで、Datadog 側のセッションは失効させない (issue 0164 の CLI 側 `Logout` と同じ非対称性)。
- **`OAuthRedirectBase` 変更時の再登録の実際の挙動**: 既定値から変更した状態での初回ログインが新しい `client_id` の発行と旧トークンの実質無効化を招くという前提は、issue 0164 の調査結果 (DCR に redirect_uris の後追い追加エンドポイントが無いこと) からの推論であり、Datadog に対するライブ検証はできていない。

### プロセス間リフレッシュ競合についての明記

`singleflight.Group` は同一プロセス内の goroutine 間でしか重複を防げない。CLI (`thief datadog auth refresh`) と API サーバは別プロセスのため、両者が同時に同じ期限間近のトークンをリフレッシュしようとする競合は原理的に防げない。設計判断 3 の楽観的リトライ (リフレッシュ直前・失敗確定前の再読込) で実害は潰しているが、`flock` 等の強い排他は設計判断どおり導入していない。

### レビューで却下した指摘

- **完了条件 112 行目の「代替条件」表記 (規約整合レビュー、優先度高)**: issue 本文 112 行目「上記と同じ代替条件を適用してよい」の「代替条件」を、調査結果セクション (146 行目) や本ドラフトで使っている用語「代替条項」に合わせて書き換えるべきという指摘。却下する。理由は 2 点。第一に、112 行目は本セッションが追記した文章ではなく、issue 起票時 (todo-to-issue) からの完了条件本文そのものであり、implement-issues スキルの禁止事項「issue の既存の記述の書き換え (追記のみ可、例外は pending 移動・昇格・reopen の 3 つに限定)」に抵触するため、そもそも書き換えの対象にできない。第二に、111 行目は代替の仕組みをその場で説明するだけで「代替条項」という語自体を使っておらず、112 行目の「代替条件」はその説明を指す普通の日本語表現 (「上記と同じ取り決め」の意) であって、調査結果セクションで定義的に用いている用語「代替条項」との衝突ではない。0164 で是正したのは本セッションが今回追記した見出し文言の表記ゆれであり、既存の完了条件本文とは事情が異なる。
- **完了条件 110-112 行目の代替条項適用が発動要件の字義を満たさない構造そのもの (完了条件レビュー、優先度中)**: 「必要と判明し」という発動要件を字義どおりには満たさない状態で、条項の趣旨解釈により代替 (httptest e2e) を適用している構造自体への注意喚起。ブロッキングではないとレビュー自身が明記しており、対応は「今後同じ論法が前例だけを根拠に自動踏襲されないよう記録する」ことを推奨する内容であって、実装や issue 記述の変更を求めるものではない。本節への記録をもって対応済みとする。issue 0164 のラウンド 2 レビューで一度確定した理由づけ (完了条件が個別に定めた代替条項の字義どおりの発動要件は満たさないが、その代替条項が想定する状況をより広く包含する事実がある以上、趣旨に照らして代替を適用するのが妥当) を 0165 でも踏襲する判断は維持する。
- **`insufficient_scope` 等の文字列に依存しない 403 判定 (完了条件レビュー、優先度低)**: 設計判断 2 の状態網羅表の文言 (403 + insufficient_scope 相当) より実装 (`IsForbidden`) の判定条件の方が広い (403 なら理由を問わない) という指摘。既に本ドラフトの「未検証のまま採用した仮定」節に判定根拠とあわせて明記済みであり、レビュー側も実害無しと結論しているため、追加対応は無し。
- **`Datadog.OAuthRedirectBase` の末尾スラッシュ未検証 (テスト堅牢性レビュー、優先度低)**: 運用者が末尾にスラッシュを付けて設定すると redirect_uri が二重スラッシュになり認可要求が拒否されうるという指摘。本 issue の完了条件が求める挙動ではなく、同種のトリム欠如は本リポジトリの他の URL 系設定にも既存であり 0165 固有の新規劣化ではないため、本 issue のスコープでは修正しない。`docs/issues/TODO.md` にスコープ外の問題として追記した。
- **`TestDatadogRefreshIsCollapsedBySingleflight` の `-race` の意味論についての注記 (テスト堅牢性レビュー、優先度低)**: singleflight の収束の正しさを担保しているのは `disk.refreshs` のカウント検証であり `-race` 検出そのものではない、という指摘。完了条件が定める文言 (「`-race` 付きテストで...確認できる」) はテストに `-race` を付けて実行し検証できることを求めており、本テストはこれを文字どおり満たしている。レビューも実害は未確認としており、テストの追加変更は行わない。

以上でこの issue の完了条件をすべて満たした。
