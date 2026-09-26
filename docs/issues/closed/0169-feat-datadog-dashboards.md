# Datadog Dashboards をタブ内で参照できるようにする

Created: 2026-09-11
Model: Claude Sonnet 5
Completed: 2026-09-12

## 背景

issue 0168 で Sub Organization (以下 Sub Org) をタブで切り替えられるようになったが、切り替えた先で見られるのは依然としてコスト情報のみである。ユーザー要望は「Sub Organization ごとのダッシュボード... を参照できるようにしたい」であり、単なるダッシュボード一覧+外部リンクではなく「ウィジェットのクエリ結果を取得して thief 内でグラフ再描画する」ことをユーザーと合意済み。

Dashboards API (`GET /api/v1/dashboard`、`GET /api/v1/dashboard/{id}`) は公式 SDK `datadogV1.DashboardsApi` に含まれており、thief には SDK が既にベンダリング済みのため**新規の Go 依存は不要**。フロントの可視化は `echarts-for-react` が `CostChart.tsx` で既に導入済みのため**新規の npm 依存も不要**。グラフ描画には社内規範である `dataviz` スキル (色の固定順割当、系列過多は Other に集約、二軸グラフ禁止、ホバー必須等) に従う。

## 目的

選択中の Sub Org のダッシュボード一覧を取得し、ダッシュボードを選択するとそのウィジェット (timeseries・query_value) を thief 内で再描画する画面を追加する。未対応のウィジェット種別は「Datadog で開く」リンクへのフォールバックとする。

## 設計判断

### 1. Backend: Dashboards API のラップとハンドラ

- `backend/internal/datadog/dashboards.go` (新規): `ListDashboards(ctx) ([]DashboardInfo, error)` (`GET /api/v1/dashboard`)、`GetDashboard(ctx, id) (DashboardDetail, error)` (`GET /api/v1/dashboard/{id}`) を実装する。

  ウィジェットは **timeseries (`Q` 文字列を持つもの) と query_value のみ抽出**し、他の種別 (table/toplist/heatmap/group 等) は「未対応ウィジェット」として種別名だけを返す。理由は、Datadog のウィジェット種別は多岐にわたり、全種別を thief 内で再現するコストが要望の範囲 (時系列グラフとサマリ値の参照) に対して過大なため。group ウィジェット (他ウィジェットを入れ子にする) の内側も timeseries/query_value のみ再帰的に抽出する。

- `backend/internal/api/handlers_datadog_dashboards.go` (新規): `handleDatadogDashboards` (list)、`handleDatadogDashboard` (get)。いずれも issue 0167 の `datadogAuthContext(ctx, org)` を `org` 付きで呼び、選択中の Sub Org の認証で取得する (静的キーへのフォールバックは issue 0167 の方針どおり行わない)。
- ルート登録: `GET /api/datadog/dashboards`、`GET /api/datadog/dashboards/{id}`。いずれも `org` クエリパラメータ必須とする (Sub Org を跨いだ一覧取得は行わない)。

### 2. Frontend: 共通チャート部品と Dashboards ビュー

- `frontend/src/components/charts/TimeseriesChart.tsx` (新規): `echarts-for-react` のラップ。`CostChart.tsx` の実装パターン (テーマ対応、Raw→Row 変換は呼び出し側の責務) を踏襲し、`dataviz` スキルの指針に従う。Dashboard の timeseries ウィジェットと issue 0170 (Metrics) の両方から使う共通部品として設計する。
- `frontend/src/components/charts/StatTile.tsx` (新規): query_value ウィジェット用の単一値カード。
- `frontend/src/views/nonaws/DatadogDashboardView.tsx` (新規): ダッシュボード一覧 → 選択 → ウィジェット群を `TimeseriesChart`/`StatTile`/未対応フォールバック (「Datadog で開く」リンク) で描画する。選択中の Sub Org (`orgId`) を props で受け取る。
- `types/nonaws.ts`/`normalizeNonAws.ts`/`api/{endpoints,queries}.ts` にダッシュボード関連の型・正規化・クエリフックを追加する (既存の Cost 機能と同じ Raw/Row 分離パターンに従う)。
- `DatadogView.tsx` (issue 0168 で `orgId` 対応済み) の中に Cost/Dashboards を切り替えるタブまたはセクションを追加する (「## 全体構成」で示した `DatadogView (Sub Org タブの中身)` 配下の構造)。

### 3. 却下した代替案

**全ウィジェット種別への対応**: table/toplist/heatmap 等を含む全種別を thief 内で再現する案は、実装コストが要望に対して過大であり、Datadog 自体が高機能な可視化ツールであるため「Datadog で開く」フォールバックで十分と判断し見送る。timeseries/query_value に限定するのは、コスト画面が既に持つ時系列グラフ・サマリ表示のパターンと一貫させるためでもある。

## 完了条件

- `backend/internal/datadog/dashboards.go` の `ListDashboards`/`GetDashboard` がテーブル駆動テストでカバーされている (SDK 呼び出しはモック)。group ウィジェットの再帰的抽出、未対応ウィジェットの種別名フォールバックの両方をテストする。
- `GET /api/datadog/dashboards`/`GET /api/datadog/dashboards/{id}` が `httptest` ベースのテストで `org` パラメータ必須・キャッシュ動作・エラー変換を含めて確認できる。
- `TimeseriesChart`/`StatTile` が `dataviz` スキルの指針 (色の固定順割当、系列過多時の Other 集約、二軸グラフ禁止、ホバー必須) に沿っていることをコードレビューで確認する。
- `DatadogDashboardView` がダッシュボード一覧表示・選択・ウィジェット描画・未対応ウィジェットのフォールバック表示を行えることが `@testing-library/react` のテストで確認できる。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。
- UI 動作確認: `mise run backend:run` + `mise run frontend:run` で、実際の Datadog Sub Org 環境のダッシュボードを開き、timeseries/query_value ウィジェットが正しく再描画されることをブラウザで確認する。

## 関連

- issue 0167 (Sub Org 単位の OAuth 認証)、issue 0168 (Sub Org タブ UI) に依存する。両 issue を先に close すること。
- issue 0170 (Metrics) は本 issue が新設する `TimeseriesChart` を共有する。実装順序としては本 issue を先に close することを推奨するが、`TimeseriesChart` のインターフェースさえ揃えば並行実装も可能。
- 未確定事項: OAuth スコープ (dashboards_read 相当) の付与可否。`docs.datadoghq.com/api/latest/scopes/` が JS 描画のため机上確認できなかった。実装時に OAuth App 設定画面または実トークンで確認する。付与できない場合は、issue 0167 の方針 (Sub Org 文脈では静的キーへフォールバックしない) により明示的なエラーとして利用者に伝える。

## 実装中の方針変更 (2026-09-12)

実装エージェントが、目的・完了条件と設計判断の間の穴を発見した。目的は「ウィジェットのクエリ結果を取得して thief 内でグラフ再描画する」ことを求め、完了条件も「timeseries/query_value ウィジェットが正しく再描画される」ことを UI 確認の対象とするが、設計判断 1 が定める backend (`ListDashboards`/`GetDashboard`、Dashboards API) はウィジェットの**定義 (クエリ文字列)** しか返さず、**時系列の値そのもの**は返さない。値を得るには別 API (`GET /api/v1/query` 相当) の呼び出しが要るが、これは設計判断に無く、issue 0170 (Metrics) の領域だった。

ユーザーに確認し、次のとおり決着した。`backend/internal/datadog/metrics.go` (`GET /api/v1/query` のラップ) と `GET /api/datadog/metrics/query` ハンドラ (`org` 必須、既存の `serveCached`/`datadogAuthContext`/`writeDatadogError` パターンを踏襲) を本 issue に前倒しで追加し、ウィジェットのクエリ文字列を実行して実データを描画するところまでを本 issue のスコープとする。理由は、追加せずに設計判断どおり定義取得までに留めると、`TimeseriesChart`/`StatTile` に流し込む実データが無く、成果物が「動かない画面」になり、ユーザーと合意済みの要望 (単なる一覧 + 外部リンクではない) を満たせないため。

この前倒しにより、issue 0170 (Metrics) のバックエンド (`metrics.go`、`GET /api/datadog/metrics/query`) は本 issue で先に実装済みになる。issue 0170 の着手時に、この重複を踏まえて完了条件・設計判断を現状に合わせて読み替えること (別 API を新設せず、本 issue が追加したものをそのまま使う)。

## 解決方法

### 1. `backend/internal/datadog/dashboards.go`: Dashboards API のラップと再帰的ウィジェット抽出

`datadogV1.DashboardsApi` をラップした `ListDashboards(ctx) ([]DashboardInfo, error)` (`GET /api/v1/dashboard`) と `GetDashboard(ctx, id) (DashboardDetail, error)` (`GET /api/v1/dashboard/{id}`) を実装した。`extractWidgets` が `group` ウィジェットを再帰的に平坦化し、timeseries (`Q` 文字列を持つもの) と query_value だけをクエリ付きで抽出する。それ以外の種別は種別名だけを持つ「未対応ウィジェット」として返す。種別名の取得は、SDK の `WidgetDefinition` が判別共用体で未対応種別を型安全に取り出す公開 API を持たないため、定義を JSON へ marshal してから `type` フィールドを読む方式にした (未対応種別は種別名を表示するだけなので、この範囲に限定している)。`client.go` に `DashboardsV1API` を既存の `OrganizationsV1API` 等と同じ形で追加した (Go の新規依存は無い)。

`TestListDashboards`/`TestGetDashboard` (group 内の timeseries と toplist を再帰的に平坦化するケース、未知の `future_widget` でも種別名が保たれることを含む) で検証した。

### 2. `backend/internal/api/handlers_datadog_dashboards.go`: `GET /api/datadog/dashboards`・`GET /api/datadog/dashboards/{id}`

`handleDatadogDashboards`(list)・`handleDatadogDashboard`(get) を新設し、`org` を必須パラメータとして `datadogauth.ValidateOrg` で検証する (`datadogRequiredOrgFromQuery` ヘルパーに集約)。`s.datadogAuthContext(ctx, org)` → `s.datadogCall` → `serveCached`/`writeDatadogError` という既存チェーン (issue 0168 の orgs ハンドラと同じ) にそのまま乗せた。ダッシュボードの URL は SDK が相対パスで返すため、site を知っている `internal/api` 層で絶対 URL に組み立てている。

`TestDatadogDashboardsRequireOrg`(6 ケース)、`TestDatadogDashboardsCaching`(MISS→HIT、org ごとの分離、`?refresh=true`、ダッシュボードごとの詳細キャッシュ)、`TestDatadogDashboardsErrors`(Sub Org は静的キーへフォールバックせず 401、403 は 500)で検証した。

### 3. `frontend`: `TimeseriesChart`/`StatTile` と `DatadogDashboardView`

`TimeseriesChart.tsx`/`StatTile.tsx` を `CostChart.tsx` と同じ `echarts-for-react` ラップパターンで新設した。色の固定順割当と系列過多時の Other 集約は純関数 (`collapseSeries`/`seriesColors`) として `lib/timeseries.ts` に切り出し、描画から独立してテストできるようにした (issue 0170 からも再利用する)。dataviz の 4 原則 (色の固定順割当・Other 集約・二軸グラフ禁止・ホバー必須) の充足は「完了条件の充足」節に列挙する。

`DatadogDashboardView.tsx` はダッシュボード一覧 → 選択 → ウィジェット描画を行う。`types/nonaws.ts`/`normalizeNonAws.ts`/`api/{endpoints,queries}.ts` に Raw/Row 分離パターンでダッシュボード関連の型・正規化・クエリフックを追加した。`DatadogView.tsx` には Cost/Dashboards を切り替える既存の `.seg` (セグメントコントロール) を追加した。`useDatadogHistorical`/`useDatadogEstimated` には `enabled: section === 'cost'` を渡し、Dashboards 表示中は Cost の historical/estimated API を呼ばないようにした (レビューで判明した問題への対処。「多観点レビューでの指摘と対応」参照)。

`TimeseriesChart.test.tsx`・`StatTile.test.tsx`・`DatadogDashboardView.test.tsx`(12 ケース)・`timeseries.test.ts`・`normalizeNonAws.test.ts`・`DatadogView.test.tsx` で検証した。

### 4. 実装中に発覚した設計判断の欠落と、その決着 (metrics.go の前倒し追加)

目的・完了条件は「ウィジェットのクエリ結果を取得して thief 内でグラフ再描画する」ことを求めていたが、設計判断 1 が定めた Dashboards API はウィジェットの定義 (クエリ文字列) しか返さず、時系列の値そのものは返さない。ユーザーに確認し、`backend/internal/datadog/metrics.go` (`GET /api/v1/query` のラップ) と `GET /api/datadog/metrics/query` ハンドラを本 issue に前倒しで追加し、実データの取得・描画までをスコープに含めることで決着した (上記「## 実装中の方針変更 (2026-09-12)」参照)。

`metrics.go` の `QueryMetrics` は Datadog が「クエリの誤りを HTTP 200 + `status:"error"` で返す」落とし穴 (SDK の `err` は nil のまま) を `resp.GetError()` で検知してエラーに変換し、「単位が 2 要素 (主単位+分母) で返ることがある」落とし穴を `B/s` のように結合して吸収している。ハンドラは `org`/`query`/`from`/`to` を全て必須にした (時間窓を省略可能にすると、結果は変わるのにキャッシュキーが変わらない状態を生むため)。

フロントは `useDatadogMetricsQueries(org, queries, range)` がウィジェットの全クエリを `useQueries`+`combine` で並行実行し、`lib/timeseries.ts` の `metricsWindow()` が現在時刻を分単位に切り下げて安定したキャッシュキーを作る。`query_value` は末尾から null を飛ばして遡った直近の確定値を表示し、取得中は値を出さず待つ (欠測点は 0 に潰さずグラフを途切れさせる)。

`metrics_test.go`(`TestQueryMetrics` 12 ケース、`TestQueryMetricsSendsWindow`)、`handlers_datadog_metrics_test.go`(`TestDatadogMetricsQueryRequiresParams` 11 ケース、`TestDatadogMetricsQuery`、`TestDatadogMetricsQueryCaching`、`TestDatadogMetricsQueryErrors`)、`DatadogDashboardView.test.tsx` の該当ケースで検証した。

### 5. 方針を保った実装上の逸脱

- **`frontend/src/lib/timeseries.ts` の新設**: issue が挙げた `TimeseriesChart.tsx`/`StatTile.tsx` から、色の固定順割当と Other 集約のロジックを純関数として切り出した。描画から独立してテストするため、また issue 0170 からも再利用するため。
- **`metricsWindow()` による時間窓の分単位丸め**: 秒までの現在時刻をそのまま使うと再描画のたびにクエリキーが変わり、TanStack Query と backend 双方のキャッシュが効かなくなるため、分単位に切り下げて安定させた。
- **`from`/`to` を必須にした**: 既定値を補うと、呼び出し側が意図しない時間窓の結果をキャッシュヒットで受け取り得るため、明示的に必須とした。
- **Cost/Dashboards の切り替えを既存の `.seg` にした**: issue は「タブまたはセクション」としていたが、Sub Org タブ (上位) と二重のタブ UI を避けるため、`DatadogView` が既に持つセグメントコントロールを使った。
- **`datadogRequiredOrgFromQuery` ヘルパーの抽出**: `org` 必須 + `ValidateOrg` + 400 応答の 3 点セットが dashboards 系と metrics で重複したため共通化した。既存ハンドラの挙動は変えていない。
- **ダッシュボード URL の絶対 URL 化を `internal/api` 層で行った**: SDK が返す相対パスを、site を知っている層で絶対 URL に組み立てた。
- **ウィジェット種別名を JSON marshal 経由で取得した**: SDK の判別共用体に型安全な取り出し API が無いため。

### 多観点レビューでの指摘と対応

- **(中優先度・修正済み)** `DatadogView.tsx` の `useDatadogHistorical`/`useDatadogEstimated` に `enabled` 制御が無く、Dashboards セクション表示中も Cost の historical/estimated API 呼び出しが発火し続けていた。両フックに `options?: { enabled?: boolean }` を追加し、呼び出し側で `enabled: section === 'cost'` を渡すよう修正した。`DatadogView.test.tsx` に、Dashboards へ切り替えると両フックが `enabled: false` で呼ばれることを確認する回帰テストを追加し、`mise run backend:test`/`mise run frontend:test`/`mise run backend:lint`/`mise run frontend:lint` の再実行で解消を確認した。
- **(中優先度・却下)** `internal/datadog` パッケージの新規 exported 識別子 (`ListDashboards`/`GetDashboard`/`QueryMetrics` 等) の godoc コメントが英語のままで、日本語コメント規約 (グローバル規約「原則」) に反するという指摘があった。調査の結果、これは issue 0164〜0168 から続く同パッケージの既存慣習であり、本 issue 固有の新規逸脱ではないため、本 issue の範囲では修正しない (2 ファイルだけ日本語化するとパッケージ内で不統一になるため)。パッケージ全体の godoc 日本語化は別途 `docs/issues/TODO.md` に追記する (Step 10 のコミット後)。

### 完了条件の充足

- `ListDashboards`/`GetDashboard` のテーブル駆動テスト (group の再帰的抽出、未対応ウィジェットの種別名フォールバック込み): 満たす (上記 1)。
- `GET /api/datadog/dashboards`/`GET /api/datadog/dashboards/{id}` の httptest テスト (`org` 必須・キャッシュ・エラー変換): 満たす (上記 2)。
- `TimeseriesChart`/`StatTile` の dataviz 4 原則充足:
  - 色の固定順割当: `seriesColors()` が系列のインデックスで固定順に色を割り当てる (`timeseries.test.ts`、`TimeseriesChart.test.tsx`)。
  - 系列過多時の Other 集約: `collapseSeries()` が上位 8 系列を残し残りを Other へ合算する (`timeseries.test.ts`、`TimeseriesChart.test.tsx`)。
  - 二軸グラフ禁止: `yAxis` は単一オブジェクトのみで配列を渡す経路が無い (`TimeseriesChart.test.tsx`)。
  - ホバー必須: `tooltip: { trigger: 'axis' }` を無条件で設定 (`TimeseriesChart.test.tsx`)。
  - 満たす。
- `DatadogDashboardView` の一覧・選択・描画・未対応フォールバックのコンポーネントテスト: 満たす (`DatadogDashboardView.test.tsx` 12 ケース)。
- `mise run check` の通過: 満たす (下記テスト結果)。
- UI 動作確認 (実 Datadog Sub Org 環境でのブラウザ確認): **未実施**。実装環境に実 Datadog の認証情報・複数 Sub Org を持つ環境が無く、OAuth の認可画面を開けないため。自動テストで代替した。付随して、今回追加したメトリクス取得により OAuth スコープ (`timeseries_query` 相当) が新たに必要になる可能性が高く、dashboards_read 分と合わせて実環境で未確認のままである点を申し送る。

### テスト結果

`mise run check` を worktree (実装エージェント)・統合後の作業ツリー・レビュー指摘反映後の作業ツリーの計 3 回実行し、いずれも exit 0 (最後の 1 回はコンテナのメモリ制約により一括実行時に `backend:lint` が `Killed` になったため、`mise run backend:test`/`mise run backend:lint`/`mise run frontend:fmt`/`mise run frontend:test` を個別実行して確認した。個別実行はいずれも exit 0)。backend は全 18 パッケージ `ok` (internal/datadog のカバレッジ 92.9%)。frontend は 86 files / 898 tests 全通過 (レビュー対応の回帰テスト追加により issue 0168 close 時点のベースライン 82 files / 849 tests から +4 files / +49 tests)。lint は 0 errors (既存の警告 10 件のみでベースラインから増えていない)。govulncheck も自コードに対する脆弱性 0 件。ベースラインからの新規失敗は無い。

worktree 内での実行中、`mise run check` の並列タスクがコンテナのメモリを使い切り `signal: killed`/`Killed` になる事象が複数回発生したが、各タスクを個別実行するといずれも通過した。コードの欠陥ではなく実行環境のメモリ制約によるものと判断した。

### スコープ外で見つけた問題

- issue 0170 (Metrics) との重複: 上記「4. 実装中に発覚した設計判断の欠落」のとおり、`metrics.go`・`GET /api/datadog/metrics/query`・`lib/timeseries.ts`・`TimeseriesChart`・`useDatadogMetricsQueries` は本 issue で先に実装済みになった。issue 0170 の着手時に、これらを再実装せず流用する形へ完了条件・設計判断を読み替える (上記「## 実装中の方針変更」に記録済み、issue 0170 側は次の着手時に読み替える)。
- `internal/datadog` パッケージの godoc コメント英語化 (パッケージ全体の既存慣習): 上記「多観点レビューでの指摘と対応」のとおり、本 issue の範囲では対応せず `docs/issues/TODO.md` へ追記する。
- mise のタスク並列実行によるコンテナ OOM: 実行環境のメモリ制約によるもので、コードの欠陥ではない。再発するようならタスク並列度の見直しを検討する余地があるが、本 issue のスコープ外のため対応していない。
