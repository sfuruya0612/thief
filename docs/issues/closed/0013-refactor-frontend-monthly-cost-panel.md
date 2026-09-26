# Datadog / TiDB の Cost タブ UI を共通コンポーネント化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`frontend/src/views/nonaws/DatadogView.tsx` と `frontend/src/views/nonaws/TiDBView.tsx` の cost 表示は、以下がほぼ逐語的に重複している。

- ローカルヘルパー・定数 (完全一致): `toMonthInputValue` / `defaultMonthRange` / `applyPreset` / `RANGE_PRESETS` (直近 3/6/12 ヶ月) / `MAX_SERIES = 8`
- グループ名フィルタの useMemo (同型): `allRows.filter((r) => r[groupBy].toLowerCase().includes(needle))`
- JSX ブロック (構造・クラス名・文言まで一致): Total を出す `.stats`、`.facets` (chip-search + `type="month"` の開始/終了 + presets + groupBy select)、`isLoading ? <Loading/> : <CostChart/> + <CostCrossTable/>`

月単位のコスト表示を持つビューを追加するたびに同じ 80 行前後が複製される。

なお `CostExplorerPanel.tsx` (AWS) も同型の骨格を持つが、`type="date"` の日次範囲・Granularity / Metric セレクト・Total ラベルなど差分が本質的なため、本 issue の対象外とする。

## 対応方針

- 月範囲ヘルパー (`toMonthInputValue` / `defaultMonthRange` / プリセット定義) を `lib/monthRange.ts` に集約する。
- 月次コスト表示の共通コンポーネント `MonthlyCostPanel` を追加する。月範囲・groupBy・グループ名フィルタの state を内部に持ち、データ取得はカスタムフックを props (`useCostRows(start, end)`) で受け取る (Datadog の historical / estimated 両クエリの呼び出し順を保つため、フックは常に両方呼んだ上で選択する)。
- DatadogView / TiDBView の cost 表示をこのコンポーネントで置き換える。

## 画面表示への影響

なし。DOM 構造・クラス名・placeholder / title 文言・エラーバナーの表示位置を現状と同一に保つ。

## 解決方法

- `frontend/src/lib/monthRange.ts` を新規追加し、`toMonthInputValue` / `defaultMonthRange` / `lastMonthsRange` / `MONTH_RANGE_PRESETS` を集約した。
- `frontend/src/components/MonthlyCostPanel.tsx` を新規追加し、Total の stats・facets (グループ名フィルタ + 月範囲 + プリセット + groupBy select)・Loading / CostChart / CostCrossTable の共通 JSX を移した。グループ名フィルタと集計 (MAX_SERIES = 8) の useMemo もパネル内に持つ。
- 起票時案の「データ取得フックを props で受け取る」形は採らず、パネルは表示専用とし state (期間 / groupBy / フィルタ) と TanStack Query の呼び出しは親ビューに残した。理由: TiDB はタブ切替でパネルが unmount されるため、state をパネル内に持つとタブを戻したときに選択期間・フィルタがリセットされ、cost クエリの事前フェッチも失われて現状の表示挙動が変わるため。
- `DatadogView.tsx` / `TiDBView.tsx` の重複ブロック (各約 80 行) を `<MonthlyCostPanel>` 呼び出しに置き換えた。エラーバナーの位置・表示条件は従来のまま親ビュー側に維持。
- `mise run check` 全通過を確認した。
