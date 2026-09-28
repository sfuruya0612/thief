# workbench レイアウトで一覧の上段 (toolbar / stats / facets) を 1 行にする

Created: 2026-09-29
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
