# Datadog Organizations API と Sub Organization タブ UI を追加する

Created: 2026-09-11
Model: Claude Sonnet 5
Completed: 2026-09-12

## 背景

issue 0167 で Sub Organization (以下 Sub Org) ごとに独立した OAuth named session を持てるようになったが、それだけでは「どの Sub Org が存在するか」「どのタブに切り替えるか」という UI が無く、利用者は Sub Org の ID を手動で把握する必要がある。

ユーザーからは「Sub Org は AWS, Google Cloud のようにタブで分けるようにしたい」という要望があり、thief には既に AWS profile タブ (`AwsSessionTabs.tsx`) / GCP project タブ (`GcpSessionTabs.tsx`) という同種の UI パターンが存在する。この既存パターンをそのまま Datadog Sub Org にも適用する。

Sub Org の一覧取得自体は Datadog Organizations API (`GET /api/v1/org`、公式 SDK `datadogV1.OrganizationsApi` に含まれる) で行える。このエンドポイントは親組織の認証で呼べる (Sub Org 個々のデータ取得とは異なり、一覧 API は親組織スコープで動作する)。公式 SDK `github.com/DataDog/datadog-api-client-go/v2` は thief に既にベンダリング済みで Organizations API も同 SDK に含まれるため、**新規の Go 依存は不要**。

## 目的

Datadog ビューの `TopBar` 直下に Sub Org タブを追加し、タブ切り替えで対象 Sub Org を選択できるようにする。未ログインの Sub Org を選択した場合は、その Sub Org 向けの OAuth ログイン (issue 0167 の `org` 対応) を開始できる導線を用意する。既存の Cost 機能 (`DatadogView.tsx`) は選択中の Sub Org (`orgId`) に応じたデータを表示するように変更する。

## 設計判断

### 1. Backend: Organizations API のラップとハンドラ

- `backend/internal/datadog/organizations.go` (新規): `datadogV1.OrganizationsApi` をラップし `ListOrgs(ctx) ([]OrgInfo, error)` (`GET /api/v1/org`) を実装する。既存の `backend/internal/datadog/usage.go` が SDK をラップするパターンをそのまま踏襲する。
- `backend/internal/api/server.go`: `s.ddOrgV1` フィールドを追加する (既存の `s.ddV2` と同じ初期化パターン、`NewServer` 内で `ddclient.NewOrganizationsV1API(ddCfg)` 相当を呼ぶ)。
- `backend/internal/api/handlers_datadog_orgs.go` (新規): `handleDatadogOrgs` を実装する。既存の `serveCached`/`cacheKey`/`s.datadogCall` パターンを踏襲する。一覧取得は親組織の認証 (`org == ""`) で行う。
- ルート登録 (`registerRoutes`): `GET /api/datadog/orgs`。

### 2. Frontend: SessionTabs パターンの Datadog 版

既存の `useSessionTabs.ts`/`GcpSessionTabs.tsx` の組み立てをそのまま写す。

- `frontend/src/lib/storage.ts`: `PersistedState` に `datadogOrgSessions?: SessionTabsState` を追加する。Datadog に対応する旧形式の単一選択フィールドは存在しないため、GCP 導入時のような `migrateSessions` (レガシーフィールドからの移行) は不要。
- `frontend/src/hooks/useSessionTabs.ts`: `SessionScope` に `'datadogOrgSessions'` を追加する。`MIRROR_FIELD: Record<SessionScope, 'activeProfile' | 'gcpProject'>` は、mirror 先を持たない scope (`'datadogOrgSessions'`) を許容する形 (`Partial` 化 + ガード) に変更する。Datadog Sub Org には AWS の `activeProfile`/GCP の `gcpProject` に相当する旧形式のミラー先フィールドが存在しないため。
- `frontend/src/hooks/useDatadogOrgs.ts` (新規): `useActiveDatadogOrg()`。`hooks/useGcpProjects.ts` の `useActiveGcpProject()` と対称の構造 (`useDatadogOrgs()` クエリ + `useSessionTabs('datadogOrgSessions')` + 初回自動オープン)。
- `frontend/src/api/endpoints.ts`: `getDatadogOrgs(opts?: { refresh?: boolean })` (`GET /api/datadog/orgs`)。
- `frontend/src/api/queries.ts`: `useDatadogOrgs()` (`staleTime: 5 * 60 * 1000`、GCP project 一覧と同じ緩さ)、`useRefreshDatadogOrgs()`。
- `frontend/src/types/nonaws.ts`: `DatadogOrgRaw`/`DatadogOrgRow` (id・name・ログイン済みかどうかのフラグを含む)。ログイン済みかどうかは、issue 0167 で `org` 単位に保存されたトークンの有無を backend 側で判定し、`handleDatadogOrgs` のレスポンスに含める。
- `frontend/src/lib/normalizeNonAws.ts`: `datadogOrgFromRaw()`。
- `frontend/src/components/session/DatadogOrgSessionTabs.tsx` (新規): `GcpSessionTabs.tsx` を写した組み立てレイヤ。
- `frontend/src/lib/sessionMeta.ts`: `datadogOrgPickerItems()` を追加する。GCP には無い「未ログイン Sub Org」の badge 状態を追加し、クリックすると issue 0167 の org 対応 OAuth ログインを開始する動線にする (既存の `DatadogAuthBanner` パターンを流用)。
- `frontend/src/App.tsx`: `useActiveDatadogOrg()` を呼び出し、`{view === 'datadog' && <DatadogOrgSessionTabs .../>}` を `TopBar` 直下に追加する (GCP と対称の兄弟要素として配置し、ネストしない)。`<DatadogView>` の呼び出しを `activeOrgId ? <DatadogView key={activeOrgId} orgId={activeOrgId} /> : <SessionEmptyState .../>` に変更する。
- `frontend/src/views/nonaws/DatadogView.tsx`: `orgId: string` を props で受け取る形に変更し、既存の Cost 機能の API 呼び出し (`useDatadogHistorical`/`useDatadogEstimated`) に `orgId` を反映させる。

### 2. 未ログイン Sub Org タブの扱い

未ログインの Sub Org タブを開いたときは、既存の `DatadogAuthBanner`/`useDatadogLogin` パターンを流用し、その Sub Org 向け (`org` パラメータ付き) の OAuth ログインを開始する UI にする。クリックで自動的にログインフローを開くか、明示のログインボタンを置くかの UX 詳細は実装時に既存 `DatadogAuthBanner` の挙動を踏まえて決定する (本 issue のスコープ内で確定させる)。

### 3. 却下した代替案

**Sub Org 選択をドロップダウンにする案**: AWS/GCP と一貫した UI にするため見送る。ユーザー要望も「AWS, Google Cloud のようにタブで分けたい」であり、既存パターンとの一貫性を優先する。

## 完了条件

- `backend/internal/datadog/organizations.go` の `ListOrgs` がテーブル駆動テストでカバーされている (SDK 呼び出しはモック)。
- `GET /api/datadog/orgs` が `httptest` ベースのテストでキャッシュ動作・エラー変換を含めて確認できる。
- `PersistedState.datadogOrgSessions` の読み書きが既存の `storage.test.ts` と同じ形式のテストでカバーされている。
- `useSessionTabs('datadogOrgSessions')` が `MIRROR_FIELD` に mirror 先を持たない状態で正しく動作することがテストで確認できる。
- `DatadogOrgSessionTabs` がタブの追加・切り替え・削除を GCP と同等に行えることが `@testing-library/react` のテストで確認できる。
- 未ログイン Sub Org タブを開いたときに OAuth ログイン導線が表示されることがコンポーネントテストで確認できる。
- `DatadogView` が `orgId` prop の変化に応じて別 Sub Org のコストデータを取得し直すことがテストで確認できる (`key={activeOrgId}` によるコンポーネント再マウント込み)。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。
- UI 動作確認: `mise run backend:run` + `mise run frontend:run` で、複数 Sub Org を持つ Datadog 環境において実際にタブ切り替え・未ログイン Sub Org へのログイン誘導ができることをブラウザで確認する。

## 関連

- issue 0167 (Datadog OAuth 認証の Sub Org 対応) に依存する。issue 0167 を先に close すること。
- issue 0169 (Dashboards)、issue 0170 (Metrics) は本 issue が追加する Sub Org タブ (`orgId`) を前提に、選択中の Sub Org のダッシュボード/メトリクスを表示する。本 issue を先に close すること。

## 申し送り (issue 0167 の実装レビューより、2026-09-12)

issue 0167 の `datadogauth.ValidateOrg` は、org 文字列 (本 issue の `ListOrgs` が返す `public_id` を想定) を小文字英数字・ハイフン・アンダースコアのみに限定した (大文字を拒否する)。理由はケース非依存ファイルシステム (macOS の既定である APFS/HFS+、Windows) 上で大文字小文字だけが異なる 2 つの org 識別子が同一のトークン・クライアント登録ファイルに衝突し、別の Sub Organization の認証情報を誤って使い回すことを防ぐためである。

ただし、ベンダリング済み SDK (`github.com/DataDog/datadog-api-client-go/v2` の `Organization.PublicId`) にも Datadog 公式ドキュメントにも `public_id` の大文字小文字に関する仕様記載が無く、実際に Datadog が返す `public_id` が常に小文字であるかは実機検証ができておらず未確認である。本 issue で `ListOrgs` を実装し実際の `public_id` を取得できるようになった時点で、次のいずれかを確認して対応すること。

- 実機で取得した `public_id` が常に小文字であれば、そのまま `org` として使ってよい。
- 大文字を含む `public_id` が存在する場合、`credPath`/`ValidateOrg` に渡す前に小文字正規化するか、`ValidateOrg` 側の制約を緩めるかを検討する (後者を選ぶ場合、issue 0167 が想定したケース非効性衝突対策を別の方法 (例: 保存前に小文字化してから比較する) で維持する必要がある)。

## 調査結果・設計決定 (2026-09-12)

### 申し送りの決着: `public_id` は backend で小文字正規化する

上記の申し送りについて、ユーザーに確認し次のとおり決着した。`ListOrgs` が `datadogV1.Organization.PublicId` から `OrgInfo` を組み立てる際、`org` として使う識別子は `strings.ToLower` で小文字正規化してから返す。issue 0167 の `datadogauth.ValidateOrg`/`orgRe` (`^[a-z0-9_-]+$`) は変更せず、ケース非依存ファイルシステムでの衝突対策をそのまま維持する。

理由: `ValidateOrg` の制約を緩める代替案は、issue 0167 の実装をやり直す手戻りが大きいうえ、緩めた場合でも別の衝突対策 (保存前の正規化) を結局実装する必要があり、二度手間になる。`ListOrgs` の返り値の時点で正規化してしまえば、`org` は API 呼び出しから frontend の表示、ファイル名の組み立てまで一貫して小文字の 1 つの文字列として扱えるため、以後のレイヤに大文字小文字の分岐が一切生じない。Datadog の Organizations API のレスポンス自体は `name` フィールドに元の表示名を別途持つため、正規化した `org`(小文字の public_id)を識別子として使っても、UI 上の表示名 (Sub Org タブのラベル)は `name` を使えば損なわれない。

### 未ログイン Sub Org タブの UX 決着: クリックで自動的にログインフローを開始する

「### 2. 未ログイン Sub Org タブの扱い」が実装時に決定するとしていた UX は、タブをクリックした瞬間に `DatadogAuthBanner`/`useDatadogLogin` 相当の OAuth ログインフローを自動的に開始する形に決定した。AWS/GCP の未接続セッションタブ (`SessionEmptyState` 等) と同じ「開けば繋がる」体験に揃えるためで、明示的なログインボタンを別途配置する案は、タブを開くたびに追加のクリックを要求するだけで防げる誤操作が無いため見送る。

## 解決方法

### 1. `backend/internal/datadog/organizations.go`: Organizations API のラップ

`datadogV1.OrganizationsApi.ListOrgs` をラップした `ListOrgs(ctx) ([]OrgInfo, error)` を新設した。`OrgInfo.ID` は `Organization.PublicId` を `strings.ToLower` で小文字正規化した値、`Name` は `Organization.Name` をそのまま使う (「## 調査結果・設計決定 (2026-09-12)」で確定した方針どおり)。`PublicId` を持たない要素は結果から除外し、SDK のエラーはラップして返す。`backend/internal/datadog/usage.go` と同じラップパターンを踏襲した。`backend/internal/datadog/client.go` に `OrganizationsV1API` の構築を追加し、`server.go` の `s.ddOrgV1` から使う (既存の `s.ddV2` と同じ DI パターン)。

`TestListOrgs` (6 ケース、`GET /api/v1/org` を叩くこと・`public_id` の小文字化・`public_id` 無しの除外・エラーのラップを検証) で確認した。

### 2. `backend/internal/api/handlers_datadog_orgs.go`: `GET /api/datadog/orgs`

`handleDatadogOrgs` を新設し、`registerRoutes` (`routes.go`) に `GET /api/datadog/orgs` を追加した。一覧取得自体は親組織の認証 (`datadogParentOrg`、`org == ""`) で行う (issue の想定どおり、Sub Org の一覧 API は親組織スコープで動作する)。

レスポンスの各要素にログイン済みかどうか (`logged_in`) を含める。判定は `datadogauth.LoadToken(dir, site, org)` でその org のトークンファイルの有無を見るだけで、追加の Datadog へのネットワーク呼び出しは行わない (トークンが期限切れかどうかまでは見ない。存在すれば `logged_in: true` とする)。

方針を保った実装上の逸脱として、`[]OrgInfo` 自体は既存の `serveCached` パターンでキャッシュするが、`logged_in` は毎リクエスト再計算する (キャッシュ TTL の間、ログイン直後も未ログインのまま返り続けてタブがログイン導線を表示し続ける事態を避けるため)。`TestDatadogOrgsCaching` でキャッシュ HIT でもログイン状態が更新されることを確認した。

`TestDatadogOrgsReturnsLowerCasedIdsAndLoginState` (`PARENT1` → `parent1` の小文字化を含む)、`TestDatadogOrgsCorruptTokenIsNotLoggedIn`、`TestDatadogOrgsCaching` で検証した。

### 3. `frontend`: SessionTabs パターンの Datadog 版

`useSessionTabs.ts` の `SessionScope` に `'datadogOrgSessions'` を追加し、`MIRROR_FIELD` を `Partial<Record<SessionScope, 'activeProfile' | 'gcpProject'>>` 化してミラー先を持たない scope を許容するようにした (既存の AWS/GCP の scope の動作は変えない)。`lib/storage.ts` の `PersistedState` に `datadogOrgSessions?: SessionTabsState` を追加した (GCP のような legacy フィールドからの `migrateSessions` は不要、設計判断どおり)。

`hooks/useDatadogOrgs.ts` に `useActiveDatadogOrg()` を新設し、`useGcpProjects.ts` の `useActiveGcpProject()` と対称の構造 (`useDatadogOrgs()` クエリ + `useSessionTabs('datadogOrgSessions')` + 初回自動オープン) にした。`api/endpoints.ts`/`api/queries.ts` に `getDatadogOrgs`/`useDatadogOrgs`/`useRefreshDatadogOrgs` を追加した。

`types/nonaws.ts` に `DatadogOrgRaw`/`DatadogOrgRow` を追加し、`lib/normalizeNonAws.ts` に `datadogOrgFromRaw()` を追加した。`components/session/DatadogOrgSessionTabs.tsx` を `GcpSessionTabs.tsx` を写す形で新設した。`lib/sessionMeta.ts` に `datadogOrgPickerItems()` を追加し、GCP には無い「未ログイン」バッジを表現した。

`App.tsx` に `useActiveDatadogOrg()` を呼び出し、`TopBar` 直下に `{view === 'datadog' && <DatadogOrgSessionTabs .../>}` を GCP と対称の兄弟要素として追加した。`<DatadogView>` は `activeOrgId ? <DatadogView key={activeOrgId} orgId={activeOrgId} /> : <SessionEmptyState .../>` に変更した。`DatadogView.tsx` は `orgId: string` を props で受け取り、`useDatadogHistorical`/`useDatadogEstimated` に反映させた。

`storage.test.ts` (3 ケース)、`useSessionTabs.test.tsx` (2 ケース、旧フィールドを持たないスコープが他スコープのミラーを壊さないこと)、`useDatadogOrgs.test.tsx` (3 ケース)、`DatadogOrgSessionTabs.test.tsx` (7 ケース)、`normalizeNonAws.test.ts`、`sessionMeta.test.ts` (4 ケース)、`DatadogView.test.tsx` で検証した。

### 4. 未ログイン Sub Org タブの UX: クリックで自動的にログインフローを開始

「## 調査結果・設計決定 (2026-09-12)」で確定した方針どおり、未ログインの Sub Org タブをクリックした瞬間に OAuth ログインフローを自動的に開始するようにした。バナー (`DatadogAuthBanner`) のボタン押下とタブの自動開始が同じログインフローを共有するため、`DatadogAuthBanner` が持っていた state (`session`/`started`/`tabUnavailable`) とハンドラを `hooks/useDatadogLogin.ts` の `useDatadogLoginFlow` へ抽出した (`DatadogAuthBanner.tsx` は薄い表示層になった)。

自動開始には次の安全策を入れた。

- 組織一覧に存在しない id では自動ログインを開始しない (ログイン状態が不明な組織で勝手に認可画面を開かないため)。
- 既にログインフローが進行中のときは再開始しない (連打で認可タブが増えるのを防ぐ)。
- ログイン済みの組織では開始しない。

`postDatadogLoginStart` の `org` 引数は省略可能にせず必須にした (省略可能にすると呼び忘れが型で検出できず、Sub Org のタブから始めたログインが黙って親組織のトークンを更新してしまうため)。

`DatadogOrgSessionTabs.test.tsx` の該当ケース (未ログインタブのクリックで自動的にログイン開始、進行中は重ねて開始しない、ログイン済みでは開始しない、一覧に無い組織では開始しない)、`useDatadogLogin.test.tsx` で検証した。

### 5. 方針を保った実装上の逸脱

- **コスト取得 2 エンドポイントへの `org` クエリ追加**: issue の設計判断は Organizations API の追加のみを挙げていたが、完了条件の「`DatadogView` が `orgId` prop の変化に応じて別 Sub Org のコストデータを取得し直す」を満たすには backend 側の受け口が必要なため、`handlers_datadog.go` の 2 ハンドラに `org` クエリパラメータを追加した。値は `datadogauth.ValidateOrg` で検証し、不正なら 400 を返す (未検証の値をキャッシュキーとファイルパスに埋め込まないため)。キャッシュキーは org 単位に分離した。
- **`writeDatadogCostError` → `writeDatadogError` への改名**: Organizations のエラーハンドラが同じ分類ロジックを再利用するための改名で、呼び出し 3 箇所とテスト 1 箇所を追随させた。挙動は変えていない (401 `DATADOG_NO_CREDENTIALS` は `ErrDatadogNoCredentials` のときだけ、他は 500 `INTERNAL_ERROR`)。

### 完了条件の充足

- `ListOrgs` のテーブル駆動テスト: 満たす (上記 1)。
- `GET /api/datadog/orgs` の `httptest` ベースのテスト (キャッシュ動作・エラー変換含む): 満たす (上記 2)。
- `PersistedState.datadogOrgSessions` の読み書きのテスト: 満たす (上記 3)。
- `useSessionTabs('datadogOrgSessions')` が mirror 先の無い状態で動作するテスト: 満たす (上記 3)。
- `DatadogOrgSessionTabs` のタブ追加・切替・削除のテスト: 満たす (上記 3)。
- 未ログイン Sub Org タブでの OAuth ログイン導線のコンポーネントテスト: 満たす (上記 4)。
- `DatadogView` が `orgId` prop の変化に応じてコストデータを取得し直すテスト: 満たす (上記 3、`key={activeOrgId}` の再マウント込み)。
- `mise run check` の通過: 満たす (下記テスト結果)。
- UI 動作確認 (複数 Sub Org を持つ実 Datadog 環境でのブラウザ確認): **未実施**。実装環境に実 Datadog の認証情報と複数 Sub Org を持つ環境が無く、OAuth の認可画面を開けないため。自動テスト (上記各節) で代替した。

### テスト結果

`mise run check` を worktree (実装エージェント) と統合後の作業ツリー (親) の両方で実行し、いずれも exit 0。backend は全 18 パッケージ `ok` (internal/datadog のカバレッジ 91.0%)。frontend は 82 files / 849 tests 全通過 (ベースライン 80 files / 824 tests から +2 files / +25 tests)。lint は 0 errors (既存の警告のみでベースラインから増えていない)。govulncheck も脆弱性無し。ベースラインからの新規失敗は無い。

多観点レビュー (完了条件の充足 / 実装品質・回帰 / 規約と整合の 3 観点) をそれぞれ 1 ラウンド実施し、いずれも指摘無し (重大な指摘ゼロ、マージ可判定)。実装品質・回帰観点で 1 件の軽微な所見 (初回自動オープンで選ばれた先頭組織が未ログインの場合、タブクリック経由の自動ログイン開始は発火しないが、`DatadogAuthBanner` のログインボタンで代替可能であり設計判断の文言 (「タブをクリックしたとき」) の範囲内) があったが、修正を要求しない観察のみだったため対応不要と判断した。

### スコープ外で見つけた問題

実装中に、親組織自身のタブが常にログイン済みとして扱われず未ログイン表示になる問題が見つかった (Organizations API が返す親組織自身のエントリの `public_id` と、issue 0167 が定めた親組織の予約値 `org == ""` が対応付けられていないため)。本 issue の指示 (その org のトークンファイルの有無で判定する) どおりに実装した結果であり、本 issue の実装の逸脱ではない。別 issue (docs/issues/0171-bug-datadog-parent-org-tab-shows-logged-out.md) として登録した。
