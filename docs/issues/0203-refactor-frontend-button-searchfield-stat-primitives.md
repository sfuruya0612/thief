# frontend の Button / SearchField / Stat を TSX の部品にし、className の直書きを置き換える

Created: 2026-09-28
Model: Claude Fable 5.1

## 背景

刷新案 A (docs/issues/0201 の背景を参照) の第 3 段。汎用の見た目 (`.btn`、`.chip-search`、`.stat`) が className の直書きで使われており、変種の組み合わせや DOM の形が呼び出し側の裁量になっている。

2026-09-28 時点 (コミット 3562f56) の実測は次のとおり。

- `.btn` は 36 ファイル 88 か所で使われる。組み合わせは `btn sm` 58、`btn sm ghost` 17、`btn` 10、`btn sm primary` 6、feature 固有のクラスを足したもの 3 (`btn sm lv-range-select`、`btn sm lv-copy-btn`、`btn sm ghost clear-btn`)。要素は `<button>` が 84 か所で、`<select className="btn sm">` が 3 か所 (`Sidebar.tsx` のリージョン、`pricing/RateGroupSection.tsx` の期間、`nonaws/DatadogDashboardView.tsx` のダッシュボード)、`<a className="btn sm">` が 1 か所 (`DatadogDashboardView.tsx` の外部リンク) ある。
- `.chip-search` は 7 ファイル (`Drawer/DrawerObjectBrowser.tsx`、`MonthlyCostPanel.tsx`、`pricing/ServiceCard.tsx`、`session/AddSessionPicker.tsx`、`views/CostExplorerPanel.tsx`、`nonaws/DatadogMetricsView.tsx`、`nonaws/TiDBView.tsx`) で、すべて `<span className="chip-search"><Icons.search size={12} /><input … /></span>` の同じ形を繰り返している。`<input>` の属性 (`value` / `onChange` / `placeholder` / `aria-label` / `onKeyDown` / `title`) だけが違う。
- `.stat` は 7 ファイル (`StatsRow.tsx`、`charts/StatTile.tsx`、`charts/ResourceCountChart.tsx`、`MonthlyCostPanel.tsx`、`views/CostExplorerPanel.tsx`、`nonaws/DatadogMetricsView.tsx`、`nonaws/DatadogDashboardView.tsx`) で、`<div className="stat"><div className="label">…</div><div className="value">…<span className="unit">…</span></div><div className="delta">…</div></div>` の形を繰り返している。`charts/StatTile.tsx` は既にこの形を部品にしているが、Datadog の query_value 専用 (値の丸めと hover の文言を持つ) で、他の 6 ファイルは使っていない。
- `.facet` (絞り込みチップ) は `FacetBar.tsx` の 1 か所にしかない。刷新案 A は `Chip` の部品化を挙げていたが、呼び出しが 1 つの部品を切り出しても重複は減らないため、本 issue では扱わない (下の設計判断を参照)。

`components/primitives/` にあるのは `StatusBadge` / `Money` / `TagList` / `CellBar` / `FetchFailedWarning` の 5 つで、`index.ts` から再輸出している。

## 目的

`Button` / `SearchField` / `Stat` を `components/primitives/` の部品にし、className の直書きをそれに置き換える。DOM とクラス名は変えず、見た目も変えない。変種 (`size` / `variant`) と DOM の形が部品の型で決まる。

## 設計判断

- `Button` (`components/primitives/Button.tsx`): `<button>` を描画する。Props は `React.ButtonHTMLAttributes<HTMLButtonElement>` に `size?: 'md' | 'sm'` (既定 `md`)、`variant?: 'default' | 'primary' | 'ghost'` (既定 `default`) を足したもの。`className` は追加のクラスとして `btn` の後ろに連結する (feature 固有の 3 か所のため)。`type` は既定で `"button"` にする (現行の `<button>` は `type` を書いておらず、`<form>` の中に置かれた場合に submit になる。現行コードに `<form>` の中の `.btn` は無いので挙動は変わらない)。`ref` は `forwardRef` で通す。
  - `<select className="btn sm">` 3 か所と `<a className="btn sm">` 1 か所は、`Button` に `as` を持たせず、クラスの直書きを残す。理由: 4 か所のために多相の型を持ち込むより、「ボタンの見た目を持つ select / a」として例外に列挙する方が読める。`primitives.css` の `.btn` の直前のコメントに、この 4 か所を例外として書く。
  - 却下案: `IconButton` (`.iconbtn`) も同時に部品化する。`.iconbtn` は `TopBar.tsx` の 3 か所と Drawer の閉じるボタンにしかなく、本 issue の目的 (呼び出し側に散った組み合わせを型で決める) に対する効果が薄いため扱わない。
- `SearchField` (`components/primitives/SearchField.tsx`): `<span className="chip-search"><Icons.search size={12} /><input … /></span>` を描画する。Props は `React.InputHTMLAttributes<HTMLInputElement>` そのままで、すべて `<input>` に渡す。`ref` は `<input>` に `forwardRef` で通す (`AddSessionPicker.tsx` が focus のために ref を使う)。`className` は `<span>` ではなく `<input>` には渡さず、外側の `<span>` に追加クラスとして付ける (現行に追加クラスの例は無い)。
  - 却下案: `<span>` に `onClick` などを受ける props を分ける。現行の 7 か所で外側の要素に属性を付けている例は無いため、`<input>` の属性だけを受ける。
- `Stat` (`components/primitives/Stat.tsx`): `<div className="stat">` を描画する。Props は `label: ReactNode`、`value: ReactNode`、`unit?: ReactNode`、`delta?: ReactNode`、`tone?: 'pos' | 'neg'` (`delta` に `pos` / `neg` のクラスを付ける)、`title?: string`。`charts/StatTile.tsx` は値の丸めと hover の文言を計算して `Stat` を描画する薄い部品として残す (Datadog 専用の書式は `Stat` に持ち込まない)。
  - `StatsRow.tsx` は内部の `Stat` 型 (`{ label, value, tone }`) を持っている。部品の `Stat` と名前が衝突するため、内部の型は `StatItem` に改名する。
  - 却下案: `StatTile` を `Stat` に統合する。`StatTile` の丸めと hover は Datadog の query_value の要件 (docs/issues/closed/0169) で、汎用の統計カードには要らない。
- `components/primitives/index.ts` から 3 つを再輸出する。呼び出し側は `import { Button, SearchField, Stat } from '../primitives'` (既存の `StatusBadge` と同じ経路) にする。
- 置き換えは機械的に行い、置き換え後の DOM が同じであることを既存のコンポーネントテストで確認する。既存テストの多くは `getByRole('button', { name })` や `getByPlaceholderText` で要素を取るため、クラス名と要素が同じなら通る。加えて 3 部品それぞれに、変種ごとのクラスと DOM の形を固定する単体テストを `components/primitives/*.test.tsx` に置く。
- `AGENTS.md` の frontend 「コンポーネント設計」に、汎用の見た目は `components/primitives/` の部品を使い、新しい見た目の部品は同じ場所に足す旨を書く。

## 完了条件

- `components/primitives/Button.tsx` / `SearchField.tsx` / `Stat.tsx` とそのテストがあり、`index.ts` から再輸出されている。
- `frontend/src` の `.tsx` (テストを除く) で、`className` に `btn` を含む要素が `<select>` 3 か所と `<a>` 1 か所だけである (grep で判定する)。`chip-search` の直書きが `SearchField.tsx` 以外に無い。`className="stat"` の直書きが `Stat.tsx` 以外に無い。
- `StatTile` が `Stat` を使って描画している。`StatTile.test.tsx` が通る。
- 置き換えた要素の DOM (要素名、クラス名、属性、子の順) が置き換え前と同じである。`Button` / `SearchField` / `Stat` の単体テストで変種ごとに固定する。
- `.btn` の例外 4 か所が `primitives.css` の `.btn` のコメントに列挙されている。
- `AGENTS.md` の frontend 「コンポーネント設計」に部品の置き場所の規約が書かれている。
- 開発サーバで EC2 一覧 (toolbar と Drawer のボタン、stats)、Cost Explorer (検索)、セッション追加ピッカー (検索の focus) を目視する。
- `CHANGES.md` の `## develop` の `### misc` に変更が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0201、0202 (先に実装する)。
- 刷新案 A の「部品の TSX 化は Button と Chip と Stat の 3 つまで」。`Chip` を `SearchField` に差し替えた理由は背景のとおり。
- docs/issues/closed/0078: `components/tables/cells.tsx` への列セル部品の集約。本 issue はその方針を汎用の見た目に広げるもの。
- docs/issues/closed/0169: `StatTile` の出所。
