# workbench レイアウトで Drawer を docked なペインにする

Created: 2026-09-29
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
