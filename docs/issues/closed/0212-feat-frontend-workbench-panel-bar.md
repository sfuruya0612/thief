# workbench レイアウトで一覧の上段 (toolbar / stats / facets) を 1 行にする

Created: 2026-09-29
Completed: 2026-09-29
Model: Claude Fable 5.1

## 背景

docs/issues/0210 の第 2 段 (シェルの構造の後半)。AWS の一覧 (`views/AccountView.tsx` の `ServicePanel`) と Google Cloud の一覧 (`views/GcpView.tsx`) は、`.toolbar` (サービス名 + 補足)、`StatsRow` (統計カード、AWS のみ)、`ResourceCountChart` (ECS のみ)、`FacetBar` (絞り込みチップ) を縦に積む。案 B はこれを 36px の 1 行 (サービス名 · リージョン · `8 total · 6 running` の要約 · 絞り込みチップ) にまとめる。0210 の寸法の切替だけでは、この 3 段は詰まるだけで残る。

`StatsRow` は `resources` と `cost` から `StatItem[]` (label / value / tone) を組み立て、`Stat` (カード) で描画する。`FacetBar` は `.facets` の行にチップと Clear all を出す。

## 目的

`layout === 'workbench'` のとき、`ServicePanel` (AWS) と Google Cloud の一覧の上段が 1 行の `.panel-bar` になる。`standard` の DOM と見た目は変えない。分岐は `ServicePanel` / `GcpView` の上段の組み立てだけに限り、`StatsRow` と `FacetBar` は表示の variant を持つ。

## 設計判断

- `StatsRow` に `variant?: 'cards' | 'inline'` (既定 `cards`) を足す。`inline` は `<span className="stats-inline">` に `label` と `value` を `·` で並べる (`<b>8</b> total · <b>6</b> running · …`)。`StatItem[]` の組み立て (`costStats` を含む) は共通。
- `FacetBar` は DOM を変えず、`.panel-bar .facets` の CSS で高さと余白を詰める (チップは `--facet-h` で既に 24px)。
- `ServicePanel` / `GcpView`: `useTweaks().tweaks.layout` を読み、`workbench` なら `<div className="panel-bar">` に `title` (サービス名 + 補足)、`StatsRow variant="inline"` (AWS のみ)、`FacetBar` を並べる。`standard` なら従来の 3 段。`ResourceCountChart` (ECS のタスク数) は `workbench` では `panel-bar` の下に折りたたみ可能なパネル (`<details>`、既定は閉) として置く。`SSOExpiredBanner` / `ErrorBanner` の位置は変えない。
  - 却下案: `panel-bar` を別コンポーネントにして `standard` でも使う。`standard` の DOM を変えないという前提に反する。
- `.panel-bar` は `shell.css` (`.toolbar` の隣) に置く: 高さ 36px、`gap: 8px`、`border-bottom: 1px solid var(--line-1)`、子は `nowrap`、`.stats-inline` は `min-width: 0; overflow: hidden; text-overflow: ellipsis`、`.facets` は `margin-left: auto; border: 0; min-height: 0; padding: 0`。
- 他のビュー (Athena / CloudWatch Logs / BigQuery / Cloud Logging / Cost Explorer / Pricing / Datadog / TiDB) の `.toolbar` は本 issue では変えない (0210 の寸法の切替で詰まるだけ)。

## 完了条件

- `workbench` で AWS の一覧の上段が 1 行 (`.panel-bar`) になり、統計が要約の文字、絞り込みチップが同じ行の右側に出る。`standard` では従来の 3 段で、`AccountView.test.tsx` / `StatsRow.test.tsx` の既存テストが通る。
- Google Cloud の一覧も同じ 1 行になる (統計は無い)。
- ECS の `workbench` でタスク数のグラフが折りたたみで開ける。
- `StatsRow` の `inline` の描画 (文字列と順序) を `StatsRow.test.tsx` で固定する。
- 縦のクローム (最初の行まで) が `workbench` で約 105px (TopBar 40 + panel-bar 36 + th 28) になる。
- 開発サーバで EC2 / ECS / S3 と Cloud Run を目視する。
- `CHANGES.md` の `## develop` に `[ADD]` として記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0210、0211 (先に実装する)、0213。
- docs/issues/closed/0186 (ECS のタスク数のグラフ)、0203 (`Stat` / `StatsRow`)。

## 解決方法

- `StatsRow` に `variant?: 'cards' | 'inline'` (既定 `cards`) を足した。`inline` は `<span class="stats-inline" title="Resources: 8 · Running: 6 · …">` に `<span class="stats-inline-item"><b>8</b> Resources</span>` を並べ、区切りの `·` は CSS (`.stats-inline-item + .stats-inline-item::before`) で描く。`StatItem[]` の組み立て (`costStats` を含む) は `cards` と共通で、`ResourceCountChart` が使う `Stat` / `.stats` には触れていない。
- `ServicePanel` (`views/AccountView.tsx`) と `GcpView` は `useTweaks().tweaks.layout === 'workbench'` で上段だけを分岐する。`workbench` では `<div class="panel-bar">` に `title` (サービス名 + 補足)、`StatsRow variant="inline"` (AWS のみ)、`FacetBar` を置き、`standard` では従来の `.toolbar` / `.stats` / `.facets` の 3 段 (DOM は変えていない)。`SSOExpiredBanner` / `ErrorBanner` は両方で `panel-bar` (または `toolbar`) の直後。ECS の `ResourceCountChart` は `workbench` では `<details class="panel-collapsible">` (既定は閉、`summary` は `countChartTitle`) に入れ、`standard` では従来どおり常時表示。
- CSS: `.panel-bar` は `shell.css` の `.toolbar` の隣 (高さ 36px、`gap: 8px`、`padding: 0 10px`、`border-bottom: 1px solid var(--line-1)`、`overflow: hidden`)。`.panel-bar .title` は `nowrap` で縮まず、`.panel-bar .stats-inline` は `flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis` (全文は `title` 属性)、`.panel-bar .facets` は `margin-left: auto; border: 0; min-height: 0; padding: 0; flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none` (収まらないチップは横スクロールで届く)、`.panel-bar .facet` は `min-width: 0; flex-shrink: 0` (`.facet` の `min-width: 150px` を解く)。`.stats-inline` 自体の文字 (12px、`text-3`、値は `<b>` で `text-1`) は `primitives.css` の `.stat` の隣。`.panel-collapsible > summary` は 28px、`text-2`、既定のマーカーを消して `▸` / `▾` を `::before` で出す。
- 縦のクローム (最初の行まで) は `workbench` で TopBar 40 + panel-bar 36 + th 28 = 104px (ECS は `summary` の 28px が乗る)。`standard` は 0210 以前と同じ。
- テスト: `StatsRow.test.tsx` に `inline` の 2 件 (文字列と順序、`title`、`.stat` を出さないこと、`ecr` の集計が `cards` と同じ)、`AccountView.test.tsx` に上段の describe 3 件 (standard の 3 段、workbench の `.panel-bar` と要約とチップ、ECS の `details.panel-collapsible` が既定で閉じ standard では出ない)、`GcpView.test.tsx` に 2 件 (standard の 2 段、workbench の `.panel-bar`) を足した。Layout の切替は `App.test.tsx` と同じく `localStorage` の `cloudlens:v1` に `tweaks.layout` を書いて `resetTweaksForTest()` で読み直す。
- 完了条件の「開発サーバでの目視 (EC2 / ECS / S3 / Cloud Run)」はこの環境では未実施 (docs/issues/closed/0201 と同じ)。frontend の `npm run lint` (エラー 0、警告 9 は既存分)、`npm run test` (117 ファイル 1,326 テスト成功)、`npm run build` の通過を確認した。backend には変更が無い。
