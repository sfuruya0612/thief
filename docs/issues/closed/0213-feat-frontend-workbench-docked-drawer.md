# workbench レイアウトで Drawer を docked なペインにする

Created: 2026-09-29
Completed: 2026-09-29
Model: Claude Fable 5.1

## 背景

docs/issues/0210 の第 3 段。`standard` の Drawer はオーバーレイ (`position: fixed`、右 `min(600px, 52vw)` / 下 `min(46vh, 520px)`、分割中は `contained` でペインの中に `absolute`) で、表の上に被さる。案 B は Drawer をターミナルドックと同じ「docked なペイン」にし、表の右 (または下) に列として並べる。表は狭くなるが、backdrop が無く、表と詳細を同時に操作できる。

docs/issues/closed/0204 で配置は `DrawerFrame` に隔離されている。呼び出し側 (`ServicePanel` / `GcpView`) は `.main` の中に `DataTable` と `Drawer` を並べている。

## 目的

`layout === 'workbench'` のとき `DrawerFrame` が `docked` になり、`.main` の中で表の右 (`drawerPos: 'right'`) か下 (`'bottom'`) に列として並ぶ。分割中は各ペインの中に自然に収まる (`contained` の位置計算は要らない)。`standard` の振る舞いは変えない。分岐は `DrawerFrame` の 1 か所に限る。

## 設計判断

- `DrawerFrame` に `mode?: 'overlay' | 'docked'` (既定 `overlay`) を足す。`docked` のときは backdrop を描画せず、`.drawer.docked` (右: `position: static; width: <size.width ?? 380px>; height: auto; border-left: 1px solid var(--line-2); border-radius: 0; box-shadow: none; transform: none`、下: `height: <size.height ?? 40%>; border-top`) を描画する。閉じているとき (`open=false`) は描画しない (スライドのアニメーションは無い)。ESC と `closeOnEscape` は同じ。リサイズは既存の `startResize` で、基準の矩形は `offsetParent` (ペインの `.main`) を使う。寸法の永続化キー `cloudlens:drawerSize` は共有する (右幅 / 下高さの意味が同じため)。
- `Drawer` は `useTweaks().tweaks.layout` を読んで `mode` を `DrawerFrame` に渡す (呼び出し側の `ServicePanel` / `GcpView` は変えない)。
- `.main` の中で表と Drawer を並べるため、`ServicePanel` / `GcpView` の `DataTable` と `Drawer` を `<div className="main-row">` で包む (両レイアウト共通。`standard` では `.main-row { display: flex; flex-direction: column; flex: 1; min-height: 0 }` で従来と同じ縦積みになり、`fixed` / `absolute` の Drawer は流れの外なので影響しない。`contained` の `offsetParent` は `.main` (`position: relative`) のまま)。`[data-layout='workbench'] .main-row { flex-direction: row }` (下配置は `column`) で docked の列になる。
  - 却下案: `DrawerFrame` が `docked` のとき自分で `.main` の `flex-direction` を変える。親のレイアウトを子が触るのは追いにくい。包む要素を 1 つ足す方が読める。
- `styles/styles.test.ts` に `.drawer.docked` が `position: static` で `transform` を持たないことを固定するテストを足す。`DrawerFrame.test.tsx` に `docked` の描画 (backdrop 無し、閉じているときは無し、右 / 下のクラス) のテストを足す。

## 完了条件

- `workbench` で行を選ぶと Drawer が表の右 (Detail panel = Right) か下 (Bottom) に列として開き、表と同時にスクロール・操作できる。backdrop が無い。ESC で閉じる。ドラッグで幅 / 高さを変えられ、再読み込み後も残る。
- 分割表示中は各ペインの中に docked で開き、もう一方のペインへはみ出さない。
- `standard` の Drawer (オーバーレイ、`contained`) は変わらない (`DrawerFrame.test.tsx` の既存 16 テストが通る)。
- `styles/features/drawer.css` の `--terminal-dock-h` の参照は `.drawer` の 1 か所のまま。
- 開発サーバで、右 / 下 × 分割の有無 × ターミナルドックの開閉で Drawer を目視する。
- `CHANGES.md` の `## develop` に `[ADD]` として記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0210〜0212 (先に実装する)。
- docs/issues/closed/0174 / 0175 / 0176 / 0204 (Drawer の配置の経緯)。
- docs/adr/0012 (常駐ドック)、0025 (分割表示)。docked は方針の追加なので、実装後に 0025 へ追記する。

## 解決方法

- `DrawerFrame` に `mode?: 'overlay' | 'docked'` (既定 `overlay`、型 `DrawerMode`) を足した。`docked` では backdrop を描画せず、閉じているとき (`open=false`) は `null` を返す (スライドのアニメーションは無い)。`.drawer` のクラスは `docked` になり、`contained` は付けない (分割中もペインの `.main-row` に収まるため、`contained` の位置計算は不要)。ESC と `closeOnEscape` は overlay と同じ。リサイズは既存の `startResize` で、基準の矩形は `contained` と同じく `offsetParent` (ペインの `.main`、`position: relative`) を使い、端の 8px (overlay が浮いている分) は引かない。右幅の下限は docked では 280px (overlay は 380px)、上限は 85% で共通。寸法の永続化キー `cloudlens:drawerSize` は overlay と共有する。
- `Drawer` は `useTweaks().tweaks.layout === 'workbench'` なら `mode='docked'` を `DrawerFrame` に渡す。呼び出し側 (`ServicePanel` / `GcpView`) の props は変えていない。
- `ServicePanel` / `GcpView` は `DataTable` と `Drawer` を `<div className="main-row">` (下配置なら `main-row drawer-bottom`) で包む (両レイアウト共通)。`shell.css` の `.main-row` は `display: flex; flex-direction: column; flex: 1; min-height: 0; min-width: 0` で、`standard` では従来と同じ縦積み (overlay の Drawer は `fixed` / `absolute` で流れの外。`contained` の `offsetParent` は `.main` のまま)。`[data-layout='workbench'] .main-row` は `row`、`.drawer-bottom` なら `column`。`.main-row > .table-wrap { min-width: 0 }` で表が行方向に縮む。
- CSS (`features/drawer.css`、`.drawer.contained.pos-bottom` の後): `.drawer.docked` は `position: relative; inset: auto; flex: 0 0 auto; width: min(380px, 60%); max-width: 85%; height: auto; border: 0; border-left: 1px solid var(--line-2); border-radius: 0; box-shadow: none; transform: none; transition: none; z-index: auto`、`.drawer.docked.pos-bottom` は `width: auto; height: 40%; max-height: 85%; border-top`。見出しと本文の余白は少し詰めた (`.drawer.docked .dh` / `.dbody`)。`--terminal-dock-h` の参照は `.drawer` の `--drawer-lift` の 1 か所のまま (`styles.test.ts` の既存テストで固定)。
- 設計判断からの差分: `position: static` ではなく `relative`。リサイズハンドル (`.resize-handle`、`position: absolute`) の基準を Drawer 自身にするため。`relative` では `.drawer` の `top` / `right` / `bottom` (`bottom` は `--drawer-lift`) が位置をずらすので `inset: auto` で解く。幅の既定は 380px 固定ではなく `min(380px, 60%)` (分割中の 532px のペインで 319px)。
- 検証: ビルド後の CSS で、実装と同じ DOM (`.app > .topbar + .body > .sidebar + .pane > .main > .panel-bar + .main-row > .table-wrap + .drawer`) の静的なモックを Chromium (1280×720) で描画し、workbench の右 / 下 × 分割の有無の 4 通りと standard の 4 通りの矩形を確認した: workbench 1 ペインで TopBar 40 + panel-bar 36 + th 28 = 104px で最初の行、右配置は表 684px + Drawer 380px、下配置は Drawer 258px (40%)、分割中は Drawer 319px がペイン (532px) の中に収まりもう一方へはみ出さない。standard は 4 通りとも 0212 以前と同じ矩形 (overlay は `fixed`、分割中は `absolute` で 85%)。ターミナルドックの開閉との組み合わせと実データでの目視は開発サーバで行う (この環境では未実施、docs/issues/closed/0201 と同じ)。
- テスト: `DrawerFrame.test.tsx` に docked の 7 件 (backdrop 無し・contained 無し・右のハンドル、下配置、閉じているときは描画しない、既定の overlay は従来どおり、右のリサイズが包含ブロック基準で 8px を引かない、下のリサイズの上限 85%、ESC)、`Drawer.test.tsx` に Layout から `mode` を渡す 2 件、`styles.test.ts` に `.drawer.docked` (relative / inset auto / transform none / `bottom` と `--drawer-lift` を持たない) と `.main-row` の 2 件、`AccountView.test.tsx` に両レイアウトの `.main-row` と workbench で行を選ぶと docked で開く 1 件を足した。既存の `DrawerFrame.test.tsx` 16 件と分割表示のテストはそのまま通る。
- `docs/adr/0025` の決定に docked を追記した。frontend の `npm run lint` (エラー 0、警告 9 は既存分)、`npm run test` (117 ファイル 1,338 テスト成功)、`npm run build` の通過を確認した。backend には変更が無い。
