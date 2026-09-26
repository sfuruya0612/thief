# cost 集計 3 実装を汎用関数へ統合し全グループ×全行の二重走査を解消する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

### 重複

`frontend/src/lib/costAggregate.ts` (`aggregateCost`)、`costAggregateDatadog.ts` (`aggregateDatadogCost`)、`costAggregateTiDB.ts` (`aggregateTiDBCost`) の 3 関数は、アルゴリズム・制御構造・戻り値形状 (`{ categories, series, crossTableRows, total }`) が完全に同型で、差分は 3 つのアクセサのみ。

- カテゴリ軸: `r.timePeriod` / `r.month` / `r.billedDate`
- グループキー: `r.service` / `r[groupBy] || '(unknown)'` (Datadog と TiDB の `groupKeyOf` は完全同一の重複)
- 金額: `amountOf(r, metric)` / `r.cost` / `r.totalCost`

### 最適化

3 実装とも、series 構築で上位グループごとに `rows.filter(...)`、クロス表構築で全グループごとに `rows.filter(...)` を実行しており、グループ数 G に対して O(G × N) の走査になる。TiDB / Datadog のようにグループ数が行数に比例しうるデータでは実質 O(N^2)。加えて Other 集計と total でさらに 2 周する。

## 対応方針

- アクセサ (`categoryOf` / `groupKeyOf` / `amountOf`) を受け取る汎用関数 `aggregateCostRows<T>` を追加し、既存 3 関数はアクセサを渡す薄いラッパとして残す (呼び出し側・テストは無変更)。
- 最初の走査で `Map<group, Map<period, amount>>` を構築し、series / クロス表はルックアップで組み立てる。走査回数を O(G × N) から O(N) に落とす。

### 出力同一性の保証 (重要)

現実装の series / クロス表は `new Map(rows.filter(...).map(...))` による「同一 (group, period) 重複時は最後の行が勝つ」挙動。単一パス化では `Map.set` の上書き (同じく last-wins、同じ走査順) で構築し、Other の加算ループと total の `reduce` は現状の別走査のまま残すことで、浮動小数点の加算順序を含め全入力に対してビット同一の出力を保つ。

## 画面表示への影響

なし。上記の通り出力はビット同一。既存の 3 テストスイート (costAggregate.test.ts / costAggregateDatadog.test.ts / costAggregateTiDB.test.ts) を無変更で通すことを完了条件とする。

## 解決方法

- `frontend/src/lib/costAggregateCore.ts` を新規追加し、アクセサ (`categoryOf` / `groupKeyOf` / `amountOf`) を受け取る `aggregateCostRows<T>` に集計本体を統合した。
- 最初の走査で `Map<group, Map<category, amount>>` を構築し、series / クロス表はルックアップで組み立てるようにした (O(グループ数 x 行数) の filter 繰り返しを解消)。同一 (group, category) 重複時の last-wins 挙動は `Map.set` の上書き (同じ走査順) で維持し、Other の加算ループと total の reduce は従来の別走査のまま残して浮動小数点の加算順序も保存した。
- `costAggregate.ts` / `costAggregateDatadog.ts` / `costAggregateTiDB.ts` はアクセサを渡す薄いラッパにした (呼び出し側・既存テストは無変更)。ドメイン別の result 型は外部から未使用だったため共通の `CostAggregateResult` に一本化した。
- `costAggregateCore.test.ts` を新規追加し、空入力・重複行の last-wins・Other 集約・ソート順の回帰テストを追加した。既存 15 テストも無変更で通過。
- `mise run check` 全通過を確認した。
