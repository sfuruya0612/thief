# Datadog Metrics をタブ内で時系列グラフ表示できるようにする

Created: 2026-09-11
Model: Claude Sonnet 5
Completed: 2026-09-14

## 背景

issue 0168 で Sub Organization (以下 Sub Org) をタブで切り替えられるようになり、issue 0169 でダッシュボードの参照が可能になったが、ユーザー要望の「メトリクスを参照できるようにしたい」に対応する、任意のメトリクスクエリを指定して時系列グラフを見る手段がまだ無い。ユーザーとは「特定メトリクスを選んで時系列グラフ表示」する方針で合意済み。

Metrics のクエリ実行 API (`GET /api/v1/query`) は公式 SDK `datadogV1.MetricsApi.QueryMetrics` に含まれており、thief には SDK が既にベンダリング済みのため**新規の Go 依存は不要**。Dashboard の timeseries ウィジェット (issue 0169) が持つ `Q` 文字列は、このクエリ API へそのまま渡せる形式であり、両機能の間に綺麗な対応関係がある。

## 目的

選択中の Sub Org に対して Datadog クエリ言語の文字列を入力し、指定期間の時系列グラフを表示する画面を追加する。

## 設計判断

### 1. Backend: Metrics クエリ API のラップとハンドラ

- `backend/internal/datadog/metrics.go` (新規): `QueryTimeseries(ctx, from, to, query) (TimeseriesResult, error)` を実装する。`datadogV1.MetricsApi.QueryMetrics` (`GET /api/v1/query`) をラップする。
- `backend/internal/api/handlers_datadog_metrics.go` (新規): `handleDatadogMetricsQuery` (query 文字列 + from/to → timeseries)。issue 0167 の `datadogAuthContext(ctx, org)` を `org` 付きで呼び、選択中の Sub Org の認証で取得する。
- ルート登録: `GET /api/datadog/metrics/query`。`org` クエリパラメータ必須とする (issue 0169 と同じ理由で Sub Org を跨いだクエリは行わない)。
- クエリ文字列は利用者の自由入力であり Datadog へそのまま転送するため、リクエストサイズ制限 (`AGENTS.md` の「HTTP / Web API サーバ」節が定めるミドルウェアチェーンの標準項目) が適用されることを確認する。

### 2. Frontend: クエリ入力とグラフ表示

- `frontend/src/views/nonaws/DatadogMetricsView.tsx` (新規): Datadog クエリ言語の文字列をそのまま入力させる形を第一弾とする。メトリクス名・タグ・集計関数を GUI で組み立てる専用クエリビルダーは本 issue のスコープ外とする (自由入力の文字列フィールドと期間指定のみ)。issue 0169 で新設した `TimeseriesChart` を再利用してグラフを描画する。選択中の Sub Org (`orgId`) を props で受け取る。
- Dashboard 側 (issue 0169 の `DatadogDashboardView`) の timeseries ウィジェットに「この Q をここで開く」動線を追加し、`DatadogMetricsView` へクエリ文字列を引き継げるようにする。
- `types/nonaws.ts`/`normalizeNonAws.ts`/`api/{endpoints,queries}.ts` にメトリクス関連の型・正規化・クエリフックを追加する。
- `DatadogView.tsx` 配下の Cost/Dashboards/Metrics を切り替えるタブまたはセクションに Metrics を追加する (「## 全体構成」で示した 3 セクション構成の最後のピース)。

### 3. 却下した代替案

**専用クエリビルダー UI**: メトリクス名・タグ・集計関数を GUI で選択させる方式は、実装コストが大きく、Datadog クエリ言語の全機能を UI で再現するのは過大なため、本 issue では見送り第一弾は自由入力の文字列フィールドとする。将来的にクエリビルダーが必要になった場合は別 issue として追加できる。

## 完了条件

- `backend/internal/datadog/metrics.go` の `QueryTimeseries` がテーブル駆動テストでカバーされている (SDK 呼び出しはモック、from/to の境界値・空クエリ結果を含む)。
- `GET /api/datadog/metrics/query` が `httptest` ベースのテストで `org` パラメータ必須・エラー変換 (不正なクエリ文字列に対する Datadog 側のエラーを含む) を確認できる。
- `DatadogMetricsView` がクエリ入力・期間指定・グラフ表示・エラー表示を行えることが `@testing-library/react` のテストで確認できる。
- Dashboard の timeseries ウィジェットから「この Q をここで開く」動線でクエリ文字列が引き継がれることがテストで確認できる。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。
- UI 動作確認: `mise run backend:run` + `mise run frontend:run` で、実際の Datadog Sub Org 環境に対してメトリクスクエリを入力し、時系列グラフが表示されることをブラウザで確認する。

## 関連

- issue 0167 (Sub Org 単位の OAuth 認証)、issue 0168 (Sub Org タブ UI) に依存する。両 issue を先に close すること。
- issue 0169 (Dashboards) が新設する `TimeseriesChart` に依存する。実装順序としては issue 0169 を先に close することを推奨するが、`TimeseriesChart` のインターフェースさえ揃えば並行実装も可能。
- 未確定事項: OAuth スコープ (metrics_read 相当) の付与可否。issue 0169 と同じ理由で机上確認できなかった。実装時に OAuth App 設定画面または実トークンで確認する。付与できない場合は、issue 0167 の方針により明示的なエラーとして利用者に伝える。

## 着手時の読み替え (2026-09-13)

issue 0169 の実装中に発覚した設計判断の欠落 (ウィジェットの定義取得だけでは実データを描画できない) への対処として、本 issue が設計判断 1 で予定していた内容の大半が issue 0169 で前倒し実装済みになった。着手前に現状を確認し、次のとおり読み替える。

- **`backend/internal/datadog/metrics.go`・`GET /api/v1/query` のラップ**: 実装済み。関数名は設計判断 1 が挙げた `QueryTimeseries(ctx, from, to, query)` ではなく `QueryMetrics(ctx, api *MetricsV1API, query string, from, to time.Time)` になっているが、`GET /api/v1/query` をラップし timeseries の値を返す点は同一。新規実装しない。
- **`backend/internal/api/handlers_datadog_metrics.go`・`GET /api/datadog/metrics/query`**: 実装済み (`handleDatadogMetricsQuery`)。`org` クエリパラメータ必須、issue 0167 の `datadogAuthContext(ctx, org)` 経由、エラー変換 (Datadog 側のクエリエラーを含む) も実装済み。新規実装しない。
- **設計判断 1 の 3 つ目 (リクエストサイズ制限の確認)**: クエリ文字列は GET のクエリパラメータとして渡されるため、リクエストボディではなく URL (リクエストライン) の一部であり、`net/http` の `Server.MaxHeaderBytes` (未設定時は既定の 1 MiB) の対象になる。専用のミドルウェアは無いが、既存の `http.Server` 設定 (`server.go`) がアプリ全体にこの既定値を適用しており、本 issue で新たに追加するものは無い。完了条件の当該行は、この既定動作を確認するテストとして扱う。
- **`frontend/src/lib/timeseries.ts` (`collapseSeries`/`seriesColors`/`MetricsWindow`/`metricsWindow`) と `TimeseriesChart`/`StatTile`**: 実装済み。再利用する。新規実装しない。
- **`frontend/src/api/{endpoints,queries}.ts` の `getDatadogMetricsQuery`/`useDatadogMetricsQueries`**: 実装済み。`useDatadogMetricsQueries(org, queries: string[], range: MetricsWindow)` は複数クエリを `useQueries` で並行実行する形 (`DatadogDashboardView` がウィジェット群に使う)。本 issue の `DatadogMetricsView` は単一クエリの入力なので、既存フックへ長さ 1 の配列を渡して再利用するか、単数形の薄いラッパーを追加するかは Step 4 の実装詳細として扱う (どちらも方式を変える乖離ではない)。

本 issue に残るスコープは次のとおりで、設計判断 2 のうち以下を実施する。

- `frontend/src/views/nonaws/DatadogMetricsView.tsx` (新規): クエリ文字列の自由入力 + 期間指定 + `TimeseriesChart` によるグラフ表示 + エラー表示。
- `DatadogDashboardView` (issue 0169) の timeseries ウィジェットに「この Q をここで開く」動線を追加し、`DatadogMetricsView` へクエリ文字列を引き継ぐ。
- `DatadogView.tsx` の Cost/Dashboards の 2 択セグメントコントロールに Metrics を追加し 3 択にする。
- `types/nonaws.ts`/`normalizeNonAws.ts` のうち、単一クエリ入力画面向けに不足する型・正規化があれば追加する (`DatadogMetricQueryResultRaw`/`Row` 等の基礎部分は issue 0169 で追加済み)。

完了条件のうち、`QueryTimeseries`/`GET /api/datadog/metrics/query` のテーブル駆動テスト・httptest は issue 0169 で既に書かれているため、本 issue では新規に重複したテストを書かず、既存テスト (`metrics_test.go`・`handlers_datadog_metrics_test.go`) の存在と通過をもって満たされているとみなす。本 issue で新たにテストを書くのは `DatadogMetricsView` と「この Q をここで開く」動線の 2 点になる。

## 解決方法

### 1. backend: issue 0169 の実装を再利用

「## 着手時の読み替え (2026-09-13)」で確認したとおり、`backend/internal/datadog/metrics.go` の `QueryMetrics` と `backend/internal/api/handlers_datadog_metrics.go` の `handleDatadogMetricsQuery` (`GET /api/datadog/metrics/query`) は issue 0169 で実装済みであり、本 issue では変更していない。完了条件のうち該当する 2 行は、既存の `metrics_test.go`/`handlers_datadog_metrics_test.go` の存在と通過をもって満たされているとみなす。

### 2. `frontend/src/views/nonaws/DatadogMetricsView.tsx`: クエリ入力・期間指定・グラフ表示

Datadog クエリ言語の自由入力フィールド + 期間選択 (Last 1 hour / 4 hours / 1 day / 1 week、`metricsWindow(spanSeconds)` に分丸めを委ねる) + `TimeseriesChart` によるグラフ表示を実装した。入力値 (`queryInput`) と実行中のクエリ (`runningQuery`) を分離し、Run ボタンまたは Enter キーでのみ実行する (入力の 1 文字ごとに Datadog へ問い合わせると、未完成のクエリが毎回エラーになるため)。前後の空白は実行時に除去する。

単一クエリの取得には issue 0169 の `useDatadogMetricsQueries` (複数クエリ用) をそのまま再利用し、長さ 1 の配列を渡す形にした (専用の単数形ラッパーは追加していない。未入力の間は空配列を渡すことで取得自体が起きない)。単位付き値の表示形式 (`unitFormatter`) は `DatadogDashboardView` のローカル関数だったものを `lib/timeseries.ts` へ移し、両画面で共有した (挙動は変えていない)。

`DatadogMetricsView.test.tsx` (10 ケース: 未入力時は取得しない・Run で実行・Enter で実行・空白除去・系列と単位付き書式をグラフへ渡す・期間変更で窓の長さが変わる・取得中・不正クエリのエラー・401 での再ログイン導線・引き継いだクエリは入力済み実行済みの状態で開く) で検証した。

### 3. Dashboard からの「この Q をここで開く」引き継ぎ導線

`DatadogDashboardView.tsx` の timeseries ウィジェットに、クエリごとの引き継ぎボタンを追加した (`onOpenQuery(query)` プロパティ)。ウィジェットが複数クエリを重ねている場合に区別できるよう、アクセシブル名を `Open in Metrics: <query>` にした。`DatadogView.tsx` が `metricsQuery` state で引き継ぎ値を保持し、`DatadogMetricsView` へ `initialQuery` として渡して実行済みの状態で開く。

`DatadogDashboardView.test.tsx` の該当ケース (2 件: 単一クエリの引き継ぎ、複数クエリの区別) と `DatadogView.test.tsx` の該当ケース (引き継ぎ導線で Metrics へクエリを渡して切り替わる) で検証した。

### 4. `DatadogView.tsx`: 3 択セグメントコントロール

Cost/Dashboards の 2 択セグメントコントロールに Metrics を追加し 3 択にした。`useDatadogHistorical`/`useDatadogEstimated` の `enabled: section === 'cost'` (issue 0169 で追加済み) はそのまま維持し、Metrics 表示中も Cost の API 呼び出しは発火しない。

`DatadogView.test.tsx` の該当ケース (Metrics を押すと切り替わる、Metrics に切り替えると Cost の historical/estimated 取得を止める) で検証した。

### 5. 方針を保った実装上の逸脱

- **`unitFormatter` を `lib/timeseries.ts` へ移動**: `DatadogDashboardView` のローカル関数だったものを、`DatadogMetricsView` からも使うために共有関数化した。同じ「単位付き値の表示形式」を 2 画面で別実装にすると表示規範が分岐するため。挙動は変えていない。
- **単数形ラッパーフックを追加しなかった**: 既存の `useDatadogMetricsQueries` (複数形) へ長さ 1 の配列を渡す形で再利用した。ラッパーを足しても `queries.ts` に 1 段の間接が増えるだけで得るものが無いと判断した。

### 完了条件の充足

- `QueryMetrics`/`GET /api/datadog/metrics/query` のテーブル駆動テスト・httptest: issue 0169 で実装済みの既存テストで満たす (上記 1)。
- `DatadogMetricsView` のクエリ入力・期間指定・グラフ表示・エラー表示のコンポーネントテスト: 満たす (`DatadogMetricsView.test.tsx` 10 ケース、上記 2)。
- Dashboard の timeseries ウィジェットからクエリ文字列が引き継がれるテスト: 満たす (上記 3)。
- `mise run check` の通過: 満たす (下記テスト結果)。
- UI 動作確認 (実 Datadog Sub Org 環境でのブラウザ確認): **未実施**。実装環境に実 Datadog の認証情報・複数 Sub Org を持つ環境が無く、OAuth の認可画面を開けないため。自動テストで代替した (issue 0167〜0169 と同じ制約)。

### テスト結果

`mise run check` を worktree (実装エージェント) と統合後の作業ツリー (親) の両方で実行し、いずれも exit 0 (今回はコンテナの OOM は発生しなかった)。backend は全 18 パッケージ `ok` (internal/datadog のカバレッジ 92.9%、変更無し)。frontend は 87 files / 915 tests 全通過 (issue 0169 close 時点のベースライン 86 files / 898 tests から +1 files / +17 tests)。lint は 0 errors (既存の警告 10 件のみでベースラインから増えていない、本 issue の変更ファイルに新規警告は無い)。govulncheck も自コードに対する脆弱性 0 件。ベースラインからの新規失敗は無い。

### 多観点レビューでの指摘と対応

- 完了条件充足観点・規約整合観点 (中優先度・修正済み): 「解決方法」内のテストケース列挙が 9 ケースと誤記され、`引き継いだクエリは入力済み・実行済みの状態で開く` の 1 件が抜けていた (実際は 10 ケース、テスト結果の集計値 87 files/915 tests とは元々整合していた)。列挙を 10 ケースに修正した。
- 実装品質観点 (低優先度・却下): Enter キーでの実行は `disabled` 制約を経由しないため、空白のみの入力で Enter を押すと `runningQuery` が空文字にリセットされ「未入力」表示に戻り、Run ボタン (disabled) との挙動が非対称。取得自体は発火せず実害が無く、UX の微修正であり本 issue の完了条件にも関わらないため見送る。UX 改善が必要になった場合に別途検討する。

### スコープ外で見つけた問題

- `DatadogMetricsView` のアンマウントで入力中のクエリ文字列が失われる (Metrics → Dashboards → Metrics と往復すると、直前の入力ではなく最後に引き継いだ `metricsQuery` に戻る)。永続化するには入力値を `DatadogView` 側か `lib/storage.ts` に持ち上げる必要がある。本 issue は引き継ぎ導線の要件を満たす最小の実装にとどめた。バグではなく設計上の未対応点のため `docs/issues/TODO.md` へ追記する。
- ウィジェットの取得が失敗している場合でも「この Q をここで開く」ボタンを表示する (失敗したクエリを Metrics 側で修正して再実行できる方が有用と判断した設計上の選択であり、バグではない)。UX 方針の統一が必要なら別途検討する。
- Metrics のグラフ表示に Dashboards と共通の `.stats`/`.stat` レイアウト (1 カラム) を流用しており、Metrics 専用のレイアウト CSS は追加していない (見た目の改善余地であり、バグではない)。
