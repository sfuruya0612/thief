# ログ・クエリ系ビューの左パネルの幅をリサイズできるようにする

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8

## 背景

TODO の「リソースバー」を、各ビューで対象を選択するための左パネルと解釈する。
コード上に「リソースバー」という要素は存在せず、長い Log Group 名やテーブル名が収まらないのはこの左パネルであるため、これを対象とする。

Athena、CloudWatch Logs、BigQuery、Cloud Logging のビューが持つ左パネルは 2 系統に分かれている。

- CloudWatch Logs と Cloud Logging は幅 248px の `.lv-tree` を使う。`.lv-tree` は `components/logviewer/LogViewerShell.tsx` の要素であり、中身の `components/logviewer/LogTree.tsx` のルートは `.lv-tree-inner` である。幅を変える対象は `LogViewerShell.tsx` 側になる。
- Athena と BigQuery は幅 248px の `.qe-schema` を使う。これは `components/query/SchemaTree.tsx` のルート要素である。

いずれも幅が 248px の固定値 (`app.css`) であり、Log Group 名やテーブル名などの長い文字列が表示領域に収まらず、末尾が切れて判別できない場合がある。

リサイズ機構は既にアプリ内に存在する。
サイドバーの幅は `lib/sidebarResize.ts` の `startSidebarResize` がポインタドラッグで CSS 変数 `--sidebar-w` を更新し、`storage.ts` の `sidebarWidth` として永続化する。
列幅のリサイズには `hooks/useColumnResize.ts` がある。

## 目的

上記 4 ビューの左パネルの幅をユーザが調整できるようにし、長い名前を判別できるようにする。

## 対応内容

### frontend

- `.lv-tree` (`LogViewerShell.tsx`) と `.qe-schema` (`SchemaTree.tsx`) の固定幅を、専用の CSS 変数で駆動する可変幅へ変更する
- 境界のドラッグで横方向にリサイズできるようにする
- リサイズ処理は既存の `sidebarResize.ts` の方式を一般化して共通化し、両パネルから利用する

## 設計上の論点

- `sidebarResize.ts` の `startSidebarResize` は幅を `ev.clientX` で算出しており、左端が viewport の x=0 にあるメインサイドバー専用である。ログ / スキーマの左パネルはコンテンツ領域内で左端に非ゼロのオフセットを持つため、パネル自身の左端 (`getBoundingClientRect().left`) を基準に幅を算出する形へ一般化する必要がある。
- CSS 変数 `--sidebar-w` はメインサイドバーのグリッド幅に既に割り当てられている (`app.css`)。左パネル用には別の変数 (例: `--lv-tree-w` / `--qe-schema-w`) を用意し、衝突を避ける。
- 調整した幅を永続化するか、セッション内のみ保持するかを決める。永続化する場合は `storage.ts` の `PersistedState` にフィールドを追加し、既存の `sidebarWidth` と同じパターンに従う。
- 幅を `.lv-tree` 系と `.qe-schema` 系で共有するか、別々に保持するかを決める。
- ドラッグによる幅可変ではなく、パネル内の横スクロールで対処する代替案も検討する。

## 完了条件

- 4 ビューの左パネルの幅をドラッグで変更できる
- 幅を広げると切れていた長い名前が判別できる
- 幅の下限 (例: 200px) と上限が設定され、レイアウトが破綻しない

## 解決方法

設計上の論点は以下の方針とした (ユーザ判断)。

- 4 ビュー (Athena / CloudWatch Logs / BigQuery / Cloud Logging) は幅を共有する 1 つの CSS 変数
  `--resource-panel-w` を使う。
- 調整した幅は `localStorage` に永続化する (`storage.ts` の `resourcePanelWidth`、既存の
  `sidebarWidth` と同じパターン)。
- ドラッグによる幅可変を採用し、横スクロールでの代替は行わない。

### frontend

- `lib/sidebarResize.ts` の `startSidebarResize` (left edge が常に 0 の viewport 専用実装) から、
  左端オフセットを `getLeftEdge()` で注入できる汎用版 `lib/panelResize.ts` の
  `startPanelResize` を切り出した。`startSidebarResize` はこれを `getLeftEdge: () => 0` で
  呼び出す薄いラッパーに変更した (既存の見た目・挙動は変えていない)。
- `hooks/useResourcePanelWidth.ts` を新設し、`--resource-panel-w` の初期反映と
  `localStorage` への永続化を担う (`usePersistedSidebarWidth` と同じパターン)。
- `components/logviewer/LogViewerShell.tsx` の `.lv-tree` と
  `components/query/SchemaTree.tsx` の `SchemaTreePanel` (`.qe-schema`) それぞれに、
  自身の `ref` から `getBoundingClientRect().left` を求めて `startPanelResize` を呼ぶ
  リサイズハンドル (`.panel-resizer`) を追加した。CloudWatch Logs / Cloud Logging は
  `LogViewerShell` を、Athena / BigQuery は `SchemaTreePanel` を共通利用しているため、
  4 ビューとも変更不要でリサイズに対応した。
- 単一の `AppView` (aws / gcp) しか同時にマウントされないため、各ビューが個別に
  `useResourcePanelWidth()` を呼んでも `localStorage` と CSS 変数を介して自然に同期する
  (プロップドリリングは不要)。
- `.lv-tree` / `.qe-schema` は親 (`overflow: hidden`) の内側に収まる必要があるため、
  `.sidebar-resizer` (`right: -3px`、境界線をまたぐ) とは異なり `.panel-resizer` は
  `right: 0` (パネル内側) に置いた。
- 幅の下限は 200px、上限は 480px とした (既存のサイドバーの下限 160px よりわずかに広くし、
  左パネルの最小可読幅を確保する)。

### 完了条件の確認

- `lib/panelResize.test.ts` で、左端オフセットを差し引いた幅算出・下限上限クランプ・
  pointerup 後に pointermove を無視することを確認した。
- 実ブラウザでのドラッグ操作の目視確認は本セッションでは行っていない (ヘッドレスブラウザ
  操作ツールが利用できないため)。ロジックは単体テストで検証済みで、CSS 変数駆動の可変幅
  (`--resource-panel-w`) と `.panel-resizer` の配置は既存の `.sidebar-resizer` と同じ仕組みを
  流用している。
