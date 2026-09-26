# Datadog OAuth 認証を Sub Organization 単位の named session に拡張する

Created: 2026-09-11
Model: Claude Sonnet 5
Completed: 2026-09-12

## 背景

thief の Datadog 統合は現在コスト (Usage Metering API) のみで、Sub Organization (以下 Sub Org) という概念を一切扱っていない。ユーザーから「Datadog を Sub Organization ごとのダッシュボードやメトリクスを参照できるようにしたい。Sub Org は AWS/Google Cloud のようにタブで分けたい」という要望があり、事前調査で設計の前提となる次の事実が判明した。

- Datadog の Sub Org (multi-org) は **データが完全に分離**されている (Datadog 公式ドキュメント: "Organizations within a parent organization do not have access to each other's data")。親組織の OAuth トークンや API Key では子組織のデータに一切アクセスできない。
- Datadog 公式 CLI「pup」(https://github.com/DataDog/pup) も、Sub Org ごとに `--org` フラグで named session (別々の OAuth ログイン) を持つ設計であり、1 トークンで全 Sub Org を横断する仕組みは存在しない。

したがって、Sub Org のデータへアクセスするには Sub Org ごとに個別の OAuth ログインが必要になる。thief には issue 0164/0165 で `backend/internal/datadogauth/` パッケージによる OAuth 2.0 (Authorization Code + PKCE + DCR) 実装が既に存在するが、現状は site (`https://api.{site}`) 単位でトークン・クライアント登録を 1 組しか保持しない設計 (`storage.go` の `TokenPath(dir, site)`/`ClientPath(dir, site)`) であり、Sub Org を区別できない。

pup 自体を外部依存として取り込む案は、サードパーティ統合が非公式であること、新規バイナリ依存が増えること、thief の依存最小化方針 (`AGENTS.md` の「依存関係の方針」) に反することから見送り、**pup と同じ「Sub Org ごとに named session」というアーキテクチャパターンを、thief 自前の OAuth 実装 (`internal/datadogauth`) に拡張する**方針とする (ユーザー合意済み)。

本 issue は、後続の issue 0168 (Organizations API + Sub Org タブ UI)、issue 0169 (Dashboards)、issue 0170 (Metrics) すべての前提となる。Sub Org 単位の認証が無ければ、Sub Org のデータには一切アクセスできない。

## 目的

`backend/internal/datadogauth` のトークン・クライアント登録の保存/読込、ログイン/リフレッシュ/ログアウトの一連の関数が、site に加えて Sub Org (`org`) 単位でも独立して動作するようにする。`org == ""` は現行の親組織 (後方互換) として扱い、既存の CLI/API の挙動を変えない。

## 設計判断

### 1. `storage.go` のパス生成を `(dir, site)` から `(dir, site, org)` に拡張する

`TokenPath(dir, site)`/`ClientPath(dir, site)` の第三引数として `org string` を追加する。`org == ""` の場合は現行どおり `token_<site>.json`/`client_<site>.json` を使う (後方互換)。`org != ""` の場合は `token_<site>_<org>.json`/`client_<site>_<org>.json` のように org をファイル名へ組み込む。

`ValidateSite` と同じ考え方で `org` 文字列にもパストラバーサル対策のバリデーションを追加する (英数字・ハイフン・アンダースコア以外を拒否する等)。org は Sub Org のパブリック ID (Organizations API が返す値、issue 0168 で取得) を想定するが、本 issue の時点では issue 0168 が未実装のため、org 文字列の由来をハンドラ層で検証する責務は issue 0168 側に持たせ、本 issue の `datadogauth` パッケージ内では「安全なファイル名として使える文字列か」のみを検証する。

### 2. `login.go` の `Deps`/`PrepareParams`/`Login`/`EnsureFreshToken`/`Logout` に `org` を通す

各関数シグネチャに `org string` を追加し、`storage.go` の変更後の関数へそのまま渡す。DCR で登録するクライアントの扱いは次の未確定事項に従う。

### 3. 未確定事項: DCR client_id の Sub Org スコープ

Datadog の DCR (`POST /api/v2/oauth2/register`) で登録した OAuth クライアントの `client_id` が、親組織と Sub Org の間で共有できるか、Sub Org ごとに別登録が必要かはドキュメント上に明記が無く、机上調査では確定できなかった。実装着手時に、実際に Sub Org を持つ Datadog 環境で次を検証する。

- 親組織で DCR 登録したクライアントの `client_id` を使い、Sub Org 向けの認可 URL (`https://app.{site}/oauth2/v1/authorize`) で Sub Org のログインが成立するか。
- 成立しない場合、`ClientPath` も `org` 単位で分離し、Sub Org ごとに個別の DCR 登録を行う。

検証結果は本 issue の「## 調査結果」セクションに実装時点で追記し、設計をその結果に合わせて確定させる。

### 4. `backend/internal/api/datadog_auth_context.go` を `org` 対応にし、Sub Org 文脈では静的キーへフォールバックしない

`s.datadogAuthContext(ctx)` を `s.datadogAuthContext(ctx, org string)` に拡張する。`org == ""` (親組織) の場合は現行どおり、OAuth トークンが無い/期限切れでリフレッシュ失敗のときに `DATADOG_API_KEY`/`DATADOG_APP_KEY` の静的キーへフォールバックする。

`org != ""` (Sub Org) の場合は**静的キーへのフォールバックを行わない**。静的キー (`DATADOG_API_KEY`/`DATADOG_APP_KEY`) は org 非依存のグローバル環境変数であり、これを Sub Org 文脈でも使ってしまうと、実際には親組織 (または別の Sub Org) のデータを誤って返すバグの温床になる。Sub Org の OAuth トークンが無効な場合は、静的キーへ倒さず明示的なエラー (`ErrDatadogNoCredentials` 相当、org 情報を含める) を返す。

### 5. `handlers_datadog_auth.go`/`cli/datadog_auth.go` に `org` を通す

`login/start`・`callback`・`status` の各エンドポイントに `org` クエリパラメータを追加する (省略時は `org == ""` として親組織を扱う、後方互換)。`thief datadog auth login/logout/status` CLI サブコマンドに `--org` フラグを追加する。

### 6. 却下した代替案

**pup をサブプロセスとして呼び出す案**: pup が持つ named session の仕組みをそのまま使えば実装コストは下がるが、非公式サードパーティバイナリへの依存が増え、thief のインストール手順・CI・依存関係管理が複雑化する。thief 自前の `datadogauth` パッケージは既に OAuth フロー一式を実装済みであり、org 次元を 1 つ追加するだけで同等のパターンを実現できるため、pup 依存は見送る。

## 完了条件

- `datadogauth/storage.go` の `TokenPath`/`ClientPath` が `org` を受け取り、`org == ""` で既存のファイル名 (後方互換) を維持し、`org != ""` で org を含んだファイル名を生成することがテーブル駆動テストで確認できる。
- `org` 文字列のバリデーション (パストラバーサル文字列の拒否を含む) がテーブル駆動テストでカバーされている。
- `login.go` の `PrepareLogin`/`CompleteLogin`/`EnsureFreshToken`/`Logout` が `org` ごとに独立したトークン・クライアント登録を読み書きすることがテストで確認できる (同一 site で異なる 2 つの org を渡した場合に、互いのファイルへ影響しないことを確認する)。
- 実装着手時に DCR client_id の Sub Org スコープを実機検証し、結果を本 issue の「## 調査結果」セクションに追記した上で、検証結果に応じた設計 (クライアント共有 or org 単位分離) を実装する。
- `datadog_auth_context.go` の `datadogAuthContext(ctx, org)` が、`org == ""` では現行どおり静的キーへフォールバックし、`org != ""` では静的キーへフォールバックせず明示的なエラーを返すことがテーブル駆動テストで確認できる。
- `handlers_datadog_auth.go` の `login/start`・`callback`・`status` が `org` クエリパラメータを受け取り、省略時に親組織として動作することが `httptest` ベースのテストで確認できる。
- `thief datadog auth login/logout/status` に `--org` フラグが追加され、既存の (フラグ省略時の) 挙動が変わらないことが既存テストの継続通過で確認できる。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。

## 関連

- issue 0164 (`backend/internal/datadogauth/` の新設)、issue 0165 (API サーバ側の OAuth 対応) の後続。両 issue が実装した仕組みを org 次元で拡張する。
- issue 0168 (Organizations API + Sub Org タブ UI)、issue 0169 (Dashboards)、issue 0170 (Metrics) はいずれも本 issue に依存する。本 issue を先に close すること。

## 調査結果 (2026-09-11)

### 引用シンボルの実在確認

「## 設計判断」が引用する次のシンボルは、いずれも記載された位置に実在する。完了条件は現在のリポジトリに対して成立する。

- `backend/internal/datadogauth/storage.go`: `ValidateSite` (31 行目)、`Dir` (39 行目)、`TokenPath(dir, site)` (48 行目)、`ClientPath(dir, site)` (53 行目)、`LoadToken` / `SaveToken` / `DeleteToken` (70 行目・84 行目・97 行目)、`LoadClient` / `SaveClient` / `DeleteClient` (108 行目・123 行目・136 行目)
- `backend/internal/datadogauth/login.go`: `Deps` (32 行目)、`DefaultDeps` (48 行目)、`PrepareParams` (100 行目)、`Login` (115 行目)、`PrepareLogin` (134 行目)、`CompleteLogin` (194 行目)、`EnsureFreshToken` (226 行目)、`Logout` (281 行目)
- `backend/internal/api/datadog_auth_context.go`: `datadogAuthContext(ctx)`、`datadogStaticKeyContext`、`datadogAuthDeps`
- `backend/internal/api/handlers_datadog_auth.go`: `handleDatadogAuthLoginStart`、`handleDatadogAuthCallback`、`handleDatadogAuthLoginStatus`、`handleDatadogAuthLogout`

### DCR client_id の Sub Org スコープ (設計判断 3 の決着)

「## 設計判断」3 が実機検証を求めていた「DCR で登録した client_id を親組織と Sub Org で共有できるか」は、この実装環境では検証できない。実装環境に Datadog の認証情報 (`DATADOG_API_KEY` / `DATADOG_APP_KEY` の環境変数、`~/.config/thief/datadog/` 配下の OAuth トークンファイル) が一切存在せず、Datadog Organization の管理者権限も無いため、DCR 登録・認可・トークン交換のいずれも実 Datadog に到達できない (issue 0164 の「## 調査結果 (2026-09-11)」に記録した制約と同一である)。

この決着をユーザーに確認し、**クライアント登録も org 単位で分離する設計に確定した**。理由は次のとおり。

- 検証結果がどちらであっても成立する安全側の設計だからである。Sub Org ごとに別クライアントの登録が必須である場合は、この設計でなければ動作しない。共有できる場合も、org ごとに別の `client_id` を持つこと自体は OAuth の仕様上も Datadog の DCR の仕様上も不正ではなく、動作する。
- 共有前提の設計を採ると、Datadog が org ごとに別クライアントを要求する場合に Sub Org のログインが成立せず、実環境で初めて失敗が判明する。

したがって `ClientPath` も `org` 単位化し、`TokenPath` と同じ規則 (`org == ""` で既存のファイル名、`org != ""` で org を含んだファイル名) でパスを生成する。

### 実機検証を求める完了条件の代替

「## 完了条件」の「実装着手時に DCR client_id の Sub Org スコープを実機検証し、結果を本 issue の「## 調査結果」セクションに追記した上で、検証結果に応じた設計 (クライアント共有 or org 単位分離) を実装する」の行は、次のとおり扱う。

- 実機検証の部分は、上記のとおり実装環境に Datadog への到達手段が無いため実行できない。この事実の記録をもって、検証の代わりとする。
- 設計の確定は、上記のユーザー判断 (org 単位分離) をもって満たす。
- 動作の検証は、`httptest` で DCR・トークンエンドポイントをモックした自動テストで代替する。issue 0164 が同じ制約に対して `TestDatadogAuthLoginEndToEnd` で採った代替と同じ方式である。代替理由は、実装環境に Datadog の認証情報と Organization の管理者権限が一切無く、実 Datadog に対するライブ検証が不可能なことである。
- 実 Datadog に対する検証は、認証情報を持つ環境で本機能を最初に使うときに行う。そのとき org 単位のクライアント登録が不要と判明した場合、`ClientPath` の org 単位化は動作を妨げないため、設計の是正は必須ではない (DCR 呼び出しが org ごとに 1 回増えるだけである)。

## 解決方法

### 1. `datadogauth/storage.go`: トークン・クライアント登録のパスを org 単位に拡張

`TokenPath`/`ClientPath`/`LoadToken`/`SaveToken`/`DeleteToken`/`LoadClient`/`SaveClient`/`DeleteClient` に `org string` を追加した。`org == ""` は既存のファイル名 (`token_<site>.json`/`client_<site>.json`) を維持し、`org != ""` は `token_<site>_<org>.json`/`client_<site>_<org>.json` を使う。`org` のバリデーションとして `ErrInvalidOrg` と `ValidateOrg` (`^[a-z0-9_-]+$`、64 文字上限) を新設した。

多観点レビューの指摘 (中優先度) を受け、`orgRe` は当初 `^[A-Za-z0-9_-]+$` (大文字許可) で実装したが、大文字小文字だけが異なる 2 つの org 識別子 (`SubOrg`/`suborg`) が、macOS の既定ファイルシステム (APFS/HFS+) や Windows のようなケース非依存のファイルシステム上では同一のトークン・クライアント登録ファイルを指してしまい、Sub Organization 間でデータが混線する経路になることが判明したため、小文字限定 (`^[a-z0-9_-]+$`) に修正して反映した。

`TestTokenAndClientPath` (テーブル駆動、後方互換行と org 埋め込み行)、`TestValidateOrg` (パストラバーサル文字列・バックスラッシュ・ヌルバイト・長さ上限に加え、大文字・混在ケースの拒否を含む)、`TestOrgCredentialsAreIndependent` で検証した。

この小文字限定化について、追加レビューで次の懸念が指摘された。Datadog の Sub Organization の `public_id` が実際に常に小文字であるかは、ベンダリング済み SDK にも公式ドキュメントにも仕様記載が無く実機検証もできておらず未確認である。もし大文字を含む `public_id` を持つ Sub Organization が実在すれば、この変更はその Sub Organization を恒久的にログイン不可にしてしまう。この懸念は本 issue の完了条件そのものを損なうものではない (本 issue の責務はファイル名として安全であることの検証であり、org の実在性・実際の文字種を確認する手段が本 issue の実装環境には無いため) が、`public_id` を実際に取得する issue 0168 で必ず検証が必要な前提であるため、issue 0168 のファイルに申し送りとして追記した。

### 2. `datadogauth/login.go`: `Deps`/`PrepareParams`/`Login`/`EnsureFreshToken`/`Logout` に org を通す

保存系 5 関数 (`LoadToken`/`SaveToken`/`DeleteToken`/`LoadClient`/`SaveClient` に対応する `Deps` のフィールド) を `func(site, org string) ...` の形に変更した。`PrepareParams.Org`/`Login.Org` を追加し、`EnsureFreshToken(ctx, site, org, deps)`/`Logout(site, org, deps)` とした。

Datadog へ実際に接続するネットワーク系 3 関数 (`RegisterClient`/`ExchangeCode`/`RefreshToken`) には org を追加しなかった (方式を保ったままの実装詳細の乖離)。DCR (`POST /api/v2/oauth2/register`) とトークン/リフレッシュ (`POST /oauth2/v1/token`) のいずれも URL が site 次元しか持たず、リクエストにも org を渡すフィールドが無いためである (どの Sub Organization を認可するかは利用者が認可画面で選ぶ)。org による分離は保存層 (`LoadClient`/`SaveClient`/`LoadToken`/`SaveToken`) がファイル名を org ごとに分けることで実現し、`PrepareLogin` が `LoadClient(site, org)` で org ごとの `client_id` を解決してからネットワーク層へ渡す (`CompleteLogin` の `ExchangeCode` 呼び出し、`EnsureFreshToken` の `RefreshToken` 呼び出しも同様)。この判断は `Deps` の godoc に記録した。

`TestEnsureFreshToken`/`TestLogout` (org 列と不正 org 行、親のトークンをサブ組織に流用しない行)、`TestLoginIsIndependentPerOrg` で検証した。

### 3. DCR client_id の Sub Org スコープ (「## 調査結果 (2026-09-11)」で確定した方針の実装)

クライアント登録を org 単位で分離した (`ClientPath` の org 対応、上記 1 に含む)。実機検証は実装環境の制約 (Datadog 認証情報・Organization 管理者権限が無い) により行えないため、`httptest` で DCR・トークンエンドポイントをモックした自動テストで代替した (issue 0164 の `TestDatadogAuthLoginEndToEnd` と同じ方式)。`TestDatadogOAuthPerOrgSessions` (`backend/internal/api/datadog_auth_e2e_test.go`) で、org `""`/`suborg1`/`suborg2` の 3 セッションが独立した DCR 登録回数・`client_id`・トークンを持つことを確認し、`TestLoginIsIndependentPerOrg` で保存層の本番実装を通した独立性も確認した。

### 4. `api/datadog_auth_context.go`: org 対応と Sub Org 文脈での静的キーフォールバック禁止

`datadogAuthContext(ctx, org)` に拡張し、`datadogParentOrg = ""` 定数を追加した。`org == ""` では現行どおり静的キーへフォールバックし、`org != ""` ではフォールバックせず `ErrDatadogNoCredentials` を返す (`datadogFallbackContext`)。singleflight のキーを `site + "_" + org` にし、`datadogCall` の 403 時の静的キー再試行も親組織 (`org == ""`) に限定した。既存のコスト系 2 ハンドラ (`handlers_datadog.go`) は `datadogParentOrg` を明示的に渡し、現行の挙動を変えていない。

`TestDatadogAuthContextSubOrgDoesNotFallBackToStaticKeys` (トークン無し/壊れたトークン/リフレッシュ失敗 × 親・サブの組み合わせ)、`TestDatadogCallSubOrgForbiddenDoesNotRetryWithStaticKeys` で検証した。既存の `TestDatadogAuthStates` (9 状態) は無改変で通過している。

### 5. `handlers_datadog_auth.go`/`cli/datadog_auth.go`: org クエリパラメータと `--org` フラグ

`login/start` と `logout` に `org` クエリパラメータを追加した (省略時は `org == ""`)。`ErrInvalidOrg` は 400 `BAD_REQUEST` にマップする。CLI の `login`/`logout`/`refresh` サブコマンドに `--org` フラグを追加した。

方針からの乖離として、`callback` と `login/status` には org クエリパラメータを追加しなかった。

- `callback`: OAuth の `redirect_uri` は RFC 6749 の 3.1.2.3 節・4.1.3 節により認可要求とトークン要求で完全一致していなければならず、Datadog の DCR に登録できる `redirect_uri` は固定文字列 1 個である。クエリを足すと別の URI として認可サーバに拒否され、org ごとに URI を登録し直す手段も無い (DCR に更新 API が無く、新規登録は既存トークンを無効化する)。org は `PrepareLogin` が `Login.Org` に記録し、`s.ddLoginSessions` を介して `state` から `callback` へ引き継ぐ形にした。この形は、`state` は正規のまま `org` だけ差し替えたコールバックで別 org のトークンファイルを上書きされる、という攻撃も防ぐ。
- `login/status`: `state` が暗号論的乱数で org をまたいで一意なため、org を追加しても絞り込みに寄与せず、受け取っても使わないパラメータになるため付けなかった。

なお、issue の完了条件には無い `logout` への `org` クエリパラメータ追加は、org 単位のトークン削除に必須なため行った。

`TestDatadogAuthOrgQueryParam` (login/start と logout の org 通し、不正 org → 400、callback が org クエリ無しで完了することを確認)、`TestDatadogAuthOrgFlag` (3 サブコマンドの `--org` とその伝播) で検証した。

### 完了条件の充足

- `storage.go` の `TokenPath`/`ClientPath` の org 対応: 満たす (上記 1)。
- org 文字列のバリデーション: 満たす (上記 1)。
- `login.go` の org ごとの独立性: 満たす (上記 2)。
- DCR client_id の Sub Org スコープの実機検証と設計確定: 「## 調査結果 (2026-09-11)」に記載のとおり実機検証は実施不能であり、ユーザー判断による設計確定と `httptest` モックでの動作確認をもって代替した (上記 3)。
- `datadogAuthContext(ctx, org)` のフォールバック分岐: 満たす (上記 4)。
- `login/start`・`callback`・`status` の org クエリパラメータ: `login/start` は満たす。`callback`/`login/status` は上記の理由により org クエリを持たない設計とした (方式を保ったままの実装詳細の乖離)。
- `--org` CLI フラグと既存挙動の維持: 満たす (上記 5、既存テストは無改変で通過)。
- `mise run check` の通過: 満たす (下記テスト結果)。

### テスト結果

`mise run check` を worktree (実装エージェント) と統合後の作業ツリー (親) の両方で実行し、いずれも exit 0。backend は全パッケージ `ok` (datadogauth のカバレッジ 92.0%)、frontend は 80 files / 824 tests 全通過。ベースライン (exit 0、失敗テスト無し) からの新規失敗は無い。
