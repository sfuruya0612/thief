# frontend の app.css を層 (tokens / base / shell / primitives / features) に分割する

Created: 2026-09-28
Model: Claude Fable 5.1

## 背景

frontend の刷新案として「案 A 整流」(見た目は現行のまま、CSS とシェルの構造だけを組み直す) を採ることにした。本 issue はその第 1 段で、`frontend/src/app.css` の分割だけを扱う。見た目は変えない。

2026-09-28 時点 (コミット 3562f56) の `app.css` は 4,341 行、クラス 331 個、CSS 変数 90 個が 1 ファイルにある。`git log` では 294 コミットのうち 43 回がこのファイルに触れている。中身は次の状態にある。

- feature 固有の接頭辞を持つクラスが全体の 8 割を占める (`qe-` 133、`lv-` 103、`pr-` 77、`session-` 64、`drawer-` 34、`terminal-` 26)。汎用の部品 (`.btn` 9、`.facet` 9、`.stat` 7、`.seg` 4) は薄い。
- セクションの見出しコメントが「Linear-style reskin」「クエリエディタ デザイン 3」「モック 5a 実測」「モック 4a 実測」「デザイン Turn 8/9」と、feature ごとに起こしたモックの世代を残している。同じ性質の規則 (テーマ変数、部品、feature) がファイルの中で散らばり、新しい issue で触る場所が決まらない。
- `AGENTS.md` の frontend 「コーディングスタイル」は「CSS は `app.css` に集約されたクラスを再利用する」と定めている。集約の方針が、置き場所の判断を先送りにする側に働いている。

`app.css` の規則の並び順は、同じ詳細度の規則の勝ち負け (カスケード) を決めている。分割で並び順が変わると、見た目が変わりうる。

## 目的

`app.css` を、役割で分けた複数のファイルにする。規則の中身と、カスケードに影響する並び順は変えない。変更後、新しい規則の置き場所が「トークンなら `tokens.css`、部品なら `primitives.css`、feature 固有なら `features/<feature>.css`」と決まる。

## 設計判断

- ファイルは `frontend/src/styles/` に置き、`styles/index.css` が `@import` で読み込み順を決める。`main.tsx` の `import './app.css'` は `import './styles/index.css'` に変える。Vite は `@import` を組み込みで解決するため、依存の追加は無い。
  - 却下案: `main.tsx` から各ファイルを個別に import する。読み込み順が TSX 側に散り、CSS だけを見て順序を判断できなくなるため採らない。
- 分割は、現行 `app.css` のセクション見出し (`/* ===== */`) の境界で切る **連続した範囲** を基本とし、結合したときの並び順を現行と同じに保つ。読み込み順は次のとおり (括弧内は現行 `app.css` の行範囲)。
  1. `tokens.css` — `:root`、`[data-theme='light']`、`[data-density]`、`[data-accent]` (1〜213)
  2. `base.css` — reset、`body`、`button`、`input`、スクロールバー (214〜267)
  3. `shell.css` — `.app`、`.topbar`、セッションタブとピッカーとカード (`.session-*`)、`.body`、`.pane`、`.sidebar`、`.main`、`.toolbar` (268〜957)。加えて「View switcher」(2153〜2181) をここへ移す
  4. `primitives.css` — `.btn` (958〜1033)、`.stats` / `.stat` (1034〜1093)、`.facets` / `.facet` / `.chip-search` (1094〜1183)、`table.dt` (1184〜1325)、`.cb` / `.status` / `.tag` / `.svc-pill` / `.cell-bar` など (1386〜1527)。加えて「Tweaks panel」にある `.seg` / `.swatches` / `.toggle` をここへ移す
  5. `features/cost.css` — `table.dt.cost-cross-table` (1326〜1385)
  6. `features/drawer.css` — `.drawer-backdrop`、`.drawer`、`.kv`、`.metric`、`.logbox`、`.object-edit-textarea` (1528〜 `.terminal-panel` の直前)
  7. `features/terminal.css` — `.terminal-*` (〜2030)
  8. `features/tweaks.css` — `.tweaks-panel` (2031〜2152 のうち `.seg` / `.swatches` / `.toggle` を除く)
  9. `features/nonaws.css` — 「非 AWS ビュー共通」(2182〜2327)
  10. `features/storage.css` — 「S3 アップロード」(2328〜2396)
  11. `features/query.css` — 「クエリエディタ」(2397〜3202)
  12. `utilities.css` — 「Utilities」(3203〜3277)
  13. `features/logviewer.css` — 「ログビューア」(3278〜3878)
  14. `features/pricing.css` — 「Pricing」(3879〜4329)
  15. `features/charts.css` — 「台数の推移グラフ」(4330〜4341)
- 連続した範囲から外れて **位置が変わる規則は 3 群** だけで、いずれも同じ要素に当たる規則が移動の前後に無いことを grep で確かめた上で移す。
  - 「View switcher」(`.view-switch*`) を 2153 行目から `shell.css` へ (前方へ移動)。`.view-switch` を含む規則は他に無い。
  - `.seg` / `.swatches` / `.toggle` を 2031 行目付近から `primitives.css` へ (前方へ移動)。これらを含む規則は 4339 行目の `.timeseries-head .seg` だけで、移動後も後ろにある。
  - `table.dt.cost-cross-table` を 1326 行目から `features/cost.css` へ (後方へ移動)。1528 行目より後ろに `table.dt` を含む規則は無い。
  - 却下案: すべてを意味で分類し直し、並び順の変化を目視で確かめる。4,341 行の目視は漏れる。並び順を保つ機械的な分割にし、例外を 3 群に限る。
- セクション見出しのコメント (`CloudLens — Linear-style reskin`、`デザイン 3`、`モック 5a` などのモック世代の記述) は、ファイル名が役割を表すようになるため削除する。規則の直前にある説明コメント (issue 番号の参照を含む) は残す。
- 分割の検証は、規則ブロック (行頭のセレクタから対応する `}` まで) を単位に、現行 `app.css` と新しいファイルを読み込み順に結合したものとを突き合わせる。ブロックの集合が一致し、上の 3 群を除いて順序が一致することを確認する。この検証は一時的なスクリプトで行い、リポジトリには残さない (`解決方法` に結果を書く)。
- `src/app.css.test.ts` (規則のテキストで固定している不変条件) は `src/styles/styles.test.ts` に移し、`?raw` で読むファイルを `features/drawer.css`、`features/terminal.css`、`shell.css` に変える。検証する不変条件は変えない。`vite.config.ts` の `test.css.include` (`.css?raw` を通す設定) はそのまま使える。
- TSX と TS のコメント 7 か所が `app.css` を参照している (`Drawer.tsx`、`Drawer.test.tsx`、`chartTheme.ts`、`TerminalDock.test.tsx`、`TerminalDock.tsx`、`SqlEditor.tsx`、`normalizeGcp.ts`、`serviceMeta.ts`)。参照先のファイル名に書き換える。
- `AGENTS.md` の frontend 「コーディングスタイル」の CSS の項を、分割後の規約に書き換える: 新しい規則は `styles/features/<feature>.css` に書き、2 つ以上の feature で同じ形の規則が要るときに `primitives.css` へ昇格させる。色・寸法の値は `tokens.css` の変数を使う。`app.css` に集約する記述は消す。

## 完了条件

- `frontend/src/app.css` が無く、`frontend/src/styles/index.css` と上の 15 ファイルがある。`main.tsx` は `./styles/index.css` を import する。
- 現行 `app.css` の規則ブロックと、新しいファイルを `index.css` の順に結合したものの規則ブロックが、集合として一致する (件数が同じで、各ブロックのセレクタと宣言が同じ)。順序は、上に挙げた 3 群を除いて一致する。
- `src/app.css.test.ts` が無く、`src/styles/styles.test.ts` が同じ不変条件 (下配置の Drawer の閉じ位置、`.terminal-dock` の z-index と flex、`.terminal-dock-resizer`、`.terminal-panel`、`.pane.single`) を検証している。
- `frontend/src` のコード・コメントに `app.css` という文字列が残っていない (grep で判定する)。
- `AGENTS.md` の frontend 「コーディングスタイル」に、分割後のファイルの置き場所の規約が書かれている。
- 開発サーバで EC2 一覧 (Drawer を開く)、Athena、Cloud Logging、Pricing の 4 画面をライト / ダークで目視し、分割前と同じ見た目であることを確認する。
- `CHANGES.md` の `## develop` の `### misc` に変更が記載されている。
- `mise run check` が通過する。

## 関連

- 刷新案 A (Design System アーティファクト「thief」の「案 A 整流」と「現状の棚卸しと課題」)。本 issue は共通土台の手順 1 に当たる。
- docs/issues/0202 (直書きの色とテーマ上書きのトークン化)、0203 (部品の TSX 化)、0204 (Drawer の分離)、0205 (死んだ設定の削除) が本 issue の後に続く。番号順に実装する。
- docs/issues/closed/0174 / 0175 / 0176: `app.css.test.ts` が固定している不変条件の出所。
- `AGENTS.md` frontend 「コーディングスタイル」
