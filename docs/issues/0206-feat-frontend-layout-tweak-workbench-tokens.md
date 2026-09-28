# frontend に Tweaks の Layout (standard / workbench) を足し、寸法をトークン化して workbench の密度を CSS で切り替える

Created: 2026-09-29
Model: Claude Fable 5.1

## 背景

刷新案 A (docs/issues/closed/0201〜0205) を土台に、案 B「ワークベンチ」(ダーク・高密度・IDE 型のシェル) を **Tweaks で切り替えられるレイアウト** として乗せることにした。B を「見た目の方向」ではなく部品ごとの差分に分解すると、寸法 (CSS 変数だけで済む)、シェルの構造 (ヘッダの統合、サイドバーの rail、ツールバーの 1 行化)、Drawer の配置 (docked) の 3 段に分かれる。本 issue は第 1 段で、切り替えの仕組みと寸法だけを扱う。テーマ (ダーク / ライト) と accent は Layout と独立のまま (B の「ダーク第 1」は採らない)。

2026-09-29 時点 (コミット b89724f) で次の状態にある。

- `Tweaks` に Layout の軸は無い (`layout: 'tabs-top'` は参照が無く docs/issues/closed/0205 で削除した。`useTweaks.ts` の `getSnapshot` は保存済みの `layout` を読み捨てる)。
- `Tweaks.density` (`compact` / `cozy` / `comfortable`、`--row-h` 36 / 42 / 50px) は型と CSS があるが、`TweaksPanel` に切り替えの UI が無く、既定の `compact` 以外になることが無い。
- シェルと部品の寸法は `shell.css` / `primitives.css` に px で直書きされている: TopBar 46px、セッションタブバー 38px、`.toolbar` の padding `var(--sp-3) var(--sp-4)` と `h1` 14.5px、`.stats` の padding 12px 16px と `.stat` の padding 10px 14px と値 19px、`.facets` の min-height 42px、`.facet` / `.chip-search` 26px、`table.dt thead th` 32px、`.btn` 28px と `.btn.sm` 24px、`.nav-item` 30px、`body` の font-size 13px、角 `--radius-sm` 5 / `--radius` 7 / `--radius-lg` 10、影 `--shadow-sm` / `--shadow-pop`。
- 最初の行が見えるまでの縦のクロームは約 290px (TopBar 46 + セッションタブ 38 + toolbar 52 + stats 79 + facets 42 + th 32)。

## 目的

- `Tweaks.layout: 'standard' | 'workbench'` を足し、`html` の `data-layout` に反映し、`TweaksPanel` で切り替えられるようにする。既定は `standard` で、既存の見た目は変わらない。
- 上のシェルと部品の寸法をトークン (CSS 変数) にし、`[data-layout='workbench']` のブロックで値を上書きするだけで密度を切り替えられるようにする。第 1 段の workbench は縦のクロームが約 290px → 約 230px、行高 36 → 28px になる。

## 設計判断

- `types/common.ts` に `Layout = 'standard' | 'workbench'` と `Tweaks.layout` を足し、`DEFAULT_TWEAKS.layout = 'standard'`。`useTweaks.ts` の `getSnapshot` は、保存済みの `layout` が `'standard'` / `'workbench'` 以外 (0205 以前の `'tabs-top'` を含む) なら `'standard'` に読み替える (0205 の読み捨てをこの正規化に置き換える)。`useEffect` で `document.documentElement` の `data-layout` に反映する (`data-theme` などと同じ)。
  - 却下案: 別のフィールド名 (`shell` / `mode`) にする。`layout` が意味として正しく、旧値の正規化で衝突は起きないため `layout` に戻す。
- `TweaksPanel` に「Layout」の行 (`.seg` で `Standard` / `Workbench`) を Theme の直後に足す。既存 4 行 (Theme / Language / Detail panel / Accent、docs/issues/closed/0064) の相対順は変えない。Density の UI は本 issue では足さない (workbench の行高は Layout 側で決める。下記)。
- 寸法のトークン化: `tokens.css` の `:root` に「シェルと部品の寸法」の群を足し、直書きを `var(--…)` に置き換える。値は現行と同じ。
  - `--font-size-base: 13px` (`body`)、`--font-size-table: 12.5px` (`table.dt`)、`--font-size-title: 14.5px` (`.toolbar h1`)、`--font-size-stat: 19px` (`.stat .value`)
  - `--topbar-h: 46px`、`--session-bar-h: 38px`、`--th-h: 32px`、`--btn-h: 28px`、`--btn-sm-h: 24px`、`--facet-h: 26px` (`.facet` と `.chip-search`)、`--nav-item-h: 30px`、`--facets-min-h: 42px`
  - `--toolbar-pad: var(--sp-3) var(--sp-4)`、`--stats-pad: 12px 16px`、`--stat-pad: 10px 14px`
  - `--row-h` は既存。列フィルタ行の `top: 32px` (th の高さに追従する sticky) は `var(--th-h)` にする。
- `[data-layout='workbench']` のブロック (`tokens.css`、`[data-density]` のブロックより **前** に置く。`--row-h` は workbench で 28px にするが、明示的に `cozy` / `comfortable` を選んだときはそちらを優先させるため): `--font-size-base: 12.5px`、`--font-size-table: 12px`、`--font-size-title: 13px`、`--font-size-stat: 16px`、`--topbar-h: 40px`、`--session-bar-h: 34px`、`--th-h: 28px`、`--row-h: 28px`、`--btn-h: 26px`、`--btn-sm-h: 22px`、`--facet-h: 24px`、`--nav-item-h: 26px`、`--facets-min-h: 34px`、`--toolbar-pad: 6px 10px`、`--stats-pad: 8px 10px`、`--stat-pad: 6px 10px`、`--radius-sm: 3px`、`--radius: 4px`、`--radius-lg: 6px`、`--shadow-sm: none`、`--shadow-pop: 0 0 0 1px var(--line-2)` (浮き影を消し、縁だけ残す)。
  - サイドバーの幅 (`--sidebar-w`) は利用者がドラッグした値を `App.tsx` が `html` の inline style に書くため、CSS のブロックでは上書きしない (rail への折りたたみは docs/issues/0207)。
  - 却下案: workbench 用の CSS を feature ごとのファイルに散らす。値の上書きは 1 か所 (`tokens.css`) に置く方が、0201 の層の規約に合う。
- `styles/styles.test.ts` に、`[data-layout='workbench']` ブロックが `[data-density='cozy']` より前にあること (行高の優先の前提) を固定するテストを足す。`useTweaks.test.tsx` に、`layout` の既定・切替・`data-layout` の反映・旧値 (`tabs-top`) の正規化のテストを足す。`TweaksPanel.test.tsx` に Layout の行のテストを足す。

## 完了条件

- `Tweaks.layout` (`standard` / `workbench`) があり、`TweaksPanel` で切り替えると `html` の `data-layout` が変わり、再読み込み後も残る。保存済みの `layout: 'tabs-top'` は `standard` として読み込まれる。
- `shell.css` / `primitives.css` / `base.css` に、背景に挙げた寸法の px 直書きが無く、`tokens.css` の変数を参照している (grep で判定する)。`standard` での見た目は変わらない (寸法の値が同じ)。
- `[data-layout='workbench']` で、TopBar 40px、セッションタブバー 34px、th 28px、行 28px、`.btn` 26px、`.facet` 24px、`.nav-item` 26px、`body` 12.5px、角 3 / 4 / 6px、`--shadow-sm` が none になる。
- `docs/issues/closed/0201` の規約どおり、新しい変数は `tokens.css` にだけある。
- 開発サーバで Layout を切り替え、EC2 一覧・Athena・Pricing をライト / ダークで目視する (standard は変化なし、workbench は詰まる)。
- `CHANGES.md` の `## develop` に `[ADD]` として記載されている。
- `mise run check` が通過する。

## 関連

- 刷新案 B (Design System アーティファクト「thief」の「案 B ワークベンチ」)。本 issue はその第 1 段。
- docs/issues/0207 (ヘッダの統合とサイドバーの rail)、0208 (ツールバーの 1 行化)、0209 (Drawer の docked) が続く。番号順に実装する。
- docs/issues/closed/0064 (Tweaks の行順)、0205 (`layout` の削除と読み捨て)。
- docs/adr/0006: 状態は React の state と `localStorage`。ルーターは入れない。本 issue は方針を変えない。
