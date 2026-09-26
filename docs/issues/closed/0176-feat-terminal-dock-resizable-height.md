# ターミナルドックの高さをドラッグで変更できるようにする

Created: 2026-09-16
Model: Claude Fable 5.1
Completed: 2026-09-16

## 背景

`docs/issues/TODO.md` の次の項目に対応する。

> - [ ] Terminal の画面 (常駐ターミナルドック) もサイズ変更できるようにしたい
>     - Drawer はドラッグで高さ / 幅を変えられるが、docs/issues/closed/0174 で作ったターミナルドックは高さが 352px 固定 (0174 の設計判断 8 で高さのドラッグ変更を扱わない範囲にし、必要になれば別 issue で扱うとした)

### ドックの高さは 2 つの定数だけで決まる

`frontend/src/components/Terminal/TerminalDock.tsx` の `TerminalDock` は、タブバーの高さ `TAB_BAR_HEIGHT = 32` と本文 (`div.terminal-dock-body`) の高さ `BODY_HEIGHT = 320` の 2 定数から `rootHeight` (折りたたみ時 32、展開時 352) を導き、ルート要素 `div.terminal-dock` の inline `style.height` に反映する。
同じ値を `useEffect` で CSS 変数 `--terminal-dock-h` (`DOCK_HEIGHT_VAR`) として `document.documentElement` に設定し、セッションが 0 のときは `0px` にする。
高さを保持する state も高さを変える操作も無く、折りたたみ以外に高さが変わる経路は無い。

`frontend/src/hooks/useTerminalSessions.ts` のストア (`useSyncExternalStore` で購読するモジュール変数) の状態は `tabs`、`sessions`、`collapsed` の 3 つで、高さのフィールドは無い。
このストアは `localStorage` に書かない (docs/issues/closed/0174 の設計判断 3)。

`frontend/src/components/Terminal/TerminalDock.test.tsx` の最後のテストは、ルート要素の `style.height` と `--terminal-dock-h` が展開時 `352px`、折りたたみ時 `32px` になることを検証している。

### CSS は本文の高さ 320px を下限として前提にしている

`frontend/src/app.css` では `.terminal-dock-body` が `flex: 1; min-height: 0`、`.terminal-dock-pane` が `height: 100%`、`.terminal-panel` が `height: 100%; min-height: 320px` を持つ。
ルート要素の高さを変えれば本文とペインは追従するが、`.terminal-panel` の `min-height: 320px` により、本文を 320px より低くするとパネルが本文からあふれる (0174 の設計判断 2 がこの値を `BODY_HEIGHT` に揃えた理由)。
`.terminal-panel` は `frontend/src/components/Terminal/Terminal.tsx` の `Terminal` のルート要素で、`Terminal` を描画するのは 0174 以降 `TerminalDock` だけである (`frontend/src/components/Drawer/DrawerTerminal.tsx` は Connect ボタンとヒント文を描く起動口で、`Terminal` を描画しない)。
したがって `.terminal-panel` の `min-height` はドックの本文の下限としてだけ働いている。

### 端末の再レイアウトは既存の ResizeObserver が担う

`frontend/src/components/Terminal/Terminal.tsx` は `.terminal-container` を `ResizeObserver` で監視し、寸法が 0 でなければ `fitAddon.fit()` を呼ぶ。
`fit()` の結果は `term.onResize` から `{"type":"resize","cols":N,"rows":N}` として backend に送られる。
ドックの高さが変われば、この経路で端末の行数が変わる。

### Drawer は自前の実装でドラッグリサイズと永続化を行う

`frontend/src/components/Drawer/Drawer.tsx` の `startResize` は、`pointerdown` で `document` に `pointermove` / `pointerup` を登録し、下配置では `window.innerHeight - ev.clientY - 8` を `[220, window.innerHeight * 0.85]` にクランプして高さにする。
高さは `useState` の `DrawerSize` と `localStorage` のキー `cloudlens:drawerSize` (`DRAWER_SIZE_KEY`) に保存し、描画時に `sizeStyle` で `Math.min(size.height, window.innerHeight * 0.85)` と再クランプする (docs/issues/closed/0003)。
ハンドルは `div.resize-handle` (`app.css` の `.drawer .resize-handle`、`.rh-top` / `.rh-left`) で、`title="Drag to resize"` を持つ。
この実装は Drawer 専用で、`lib/storage.ts` の `PersistedState` を使わない。

`.drawer` と `.drawer.pos-bottom` の `bottom` は `calc(var(--terminal-dock-h, 0px) + 8px)`、`.drawer.pos-bottom` の閉じ位置は `translateY(calc(100% + var(--terminal-dock-h, 0px) + 16px))` で、`--terminal-dock-h` の値に追従する。

### 横方向のドラッグには共通ヘルパーがある

`frontend/src/lib/panelResize.ts` の `startPanelResize` は、`min` / `max` / `cssVar` / `getLeftEdge` / `onWidthChange` を受け取って `onPointerDown` ハンドラを返す。
`getLeftEdge()` は `pointerdown` 時に 1 回だけ呼んで左端を固定し、`pointermove` ごとに `ev.clientX` からその左端を引いた値を `[min, max]` にクランプし、CSS 変数を `document.documentElement` に直接書き、`onWidthChange` で呼び出し側に通知する。
呼び出し側は `frontend/src/lib/sidebarResize.ts` (`--sidebar-w`)、`frontend/src/components/logviewer/LogViewerShell.tsx` と `frontend/src/components/query/SchemaTree.tsx` (`--resource-panel-w`) の 3 つで、`frontend/src/lib/panelResize.test.ts` にユニットテストがある。
幅の算出が `clientX` に固定されているため、縦方向にはそのまま使えない。
`startPanelResize` は docs/issues/closed/0042 が `sidebarResize.ts` の `startSidebarResize` から切り出した汎用版で、docs/issues/closed/0015 が列幅とサイドバー幅のドラッグ処理を共通化した流れを継いでいる。
Drawer の `startResize` はどちらの issue の対象にもならず、独立実装として残っている。

### 幅の永続化には `PersistedState` のパターンがある

`frontend/src/App.tsx` の `usePersistedSidebarWidth` と `frontend/src/hooks/useResourcePanelWidth.ts` の `useResourcePanelWidth` は、`useState` の初期値を `loadPersisted().sidebarWidth` (未設定なら既定値) の形で読み、`setWidth` で `savePersisted({ ...loadPersisted(), sidebarWidth: w })` の形で書く。
`frontend/src/lib/storage.ts` の `PersistedState` に `sidebarWidth?: number` と `resourcePanelWidth?: number` がある。
`frontend/AGENTS.md` の「UI 状態と永続化」は、新しい永続化フィールドをこのパターンで `PersistedState` に追加するよう定めている。

### 0174 で先送りした範囲

docs/issues/closed/0174 の設計判断 8 は「ドックの高さをドラッグで変える機能と、高さ / 折りたたみ状態の永続化」を扱わない範囲とし、必要になれば別 issue で扱うとした。
本 issue はそのうちドラッグによる高さの変更と高さの永続化を扱う。
折りたたみ状態の永続化は扱わない (2026-09-16 にユーザーが「高さだけ永続化する」を選択した)。

## 目的

ターミナルドックの上端をドラッグして本文の高さを変えられるようにし、変えた高さがページのリロード後も保たれるようにする。

## 設計判断

### 1. ハンドルはドックの上端に重ねる

`div.terminal-dock` の先頭の子として `div.terminal-dock-resizer` を置く。
`app.css` に `.terminal-dock-resizer` を追加し、`position: absolute; top: 0; left: 0; right: 0; height: 6px; cursor: ns-resize; z-index: 1` とし、`:hover` で `.panel-resizer:hover` と同じ `background: var(--accent); opacity: 0.5` にする。
`.terminal-dock` は `position: relative` を持つ (0174 の reopen で追加) ため、absolute の基準になる。
ハンドルには `title` として `drawerStorage` ネームスペースの新しいキー `terminal.resizeDock` (値「ドラッグして高さを変更する」) を付ける。
ドックの他の操作 (`terminal.collapseDock` など) と同じネームスペースに置き、Drawer の英語ハードコード `title="Drag to resize"` には揃えない。
`frontend/AGENTS.md` の「多言語対応」が英語ハードコードとするのは Drawer のタブ名と AWS 由来のメッセージであり、ドックの文字列は 0174 で `terminal.*` のキーに載せている。

折りたたみ中は本文 (`div.terminal-dock-body`) が `hidden` で高さを変える対象が無いため、ハンドルを描画しない。
展開はトグルボタンで行う。

却下した案: ハンドルをタブバーの上に独立した行 (高さ 4px から 6px) として置く。
折りたたみ時のルート要素の高さが 32px から変わり、`--terminal-dock-h` と Drawer の `bottom` に影響する。
タブバーの上端 6px に重ねる方式では、タブの上端 6px のクリックがリサイズになる以外に既存の寸法を変えない。
`.panel-resizer` がパネル内側の右端 6px を占めるのと同じ扱いになる。
この重なりは CSS の幾何 (`position: absolute`、`top: 0`、`height: 6px`、`z-index: 1`) で決まり、jsdom はヒットテストを行わないため、テストでは `app.css.test.ts` でこれらの宣言を固定する。
実際のクリック範囲は完了条件の手動確認の項目で確認する。
手動確認は 0174 の完了条件と同じ形にし、実 AWS 環境が使えず未実施のときは理由を issue に記録する。
0174 は 1 回目の close で手動確認が未実施のまま閉じられ、その後にユーザーが実 AWS 環境で手動確認を行って 2 件の不具合が見つかり reopen された。
reopen 後の 2 回目の close でも手動確認は未実施のままである。

却下した案: 折りたたみ中もハンドルを描画し、ドラッグで展開する。
展開の操作が 2 つになり、ドラッグの開始点 (32px のタブバーの上端) からわずかに動かしただけで展開と高さ変更が同時に起きる。

### 2. ドラッグ処理は `panelResize.ts` に縦方向の関数を追加して共用する

`frontend/src/lib/panelResize.ts` に `startPanelHeightResize` を追加する。
オプションは `{ min, max, cssVar, getBottomEdge, onHeightChange }` で、`pointermove` ごとに `getBottomEdge() - ev.clientY` を `[min, max]` にクランプし、`cssVar` を `document.documentElement` に書き、`onHeightChange` で通知する。
`document` へのリスナー登録と解除、`body` の `cursor` / `userSelect` の設定と復元は、`startPanelResize` と共通の内部関数に寄せる (カーソルは `ns-resize` と `ew-resize` で分ける)。
既存の `startPanelResize` のシグネチャと挙動は変えない。

`getBottomEdge` は `startPanelResize` の `getLeftEdge` と同じく `pointerdown` 時に 1 回だけ呼び、ドラッグ中はその値を使う。

`TerminalDock` は `getBottomEdge` にルート要素の `getBoundingClientRect().bottom` を渡す。
`.app` は `height: 100vh` の flex 列でドックはその末尾の子なので、この値はビューポートの下端に等しいが、`window.innerHeight` を直接使うより要素の実位置に基づく方が前提が少ない。
`cssVar` は `--terminal-dock-h` とする。
`min` と `max` には、設計判断 4 の `terminalDockBodyHeightRange(window.innerHeight)` が返す本文の高さの下限と上限に `TAB_BAR_HEIGHT` を足したルート要素の高さを渡す。
ドラッグ時のクランプと描画時の再クランプ (設計判断 4 の `clampTerminalDockBodyHeight`) は同じ関数から範囲を得るため、下限、係数、`TAB_BAR_HEIGHT` のどれを変えても両者は食い違わない。
`onHeightChange` はルート要素の高さから `TAB_BAR_HEIGHT` を引いて本文の高さにし、設計判断 3 のフックへ渡す。
ドラッグ中は CSS 変数がヘルパーから、ルート要素の `style.height` が state の再描画から更新され、`useEffect` が同じ値を CSS 変数に再設定する。

却下した案: `Drawer.tsx` の `startResize` を `TerminalDock.tsx` に写す。
docs/issues/closed/0015 と docs/issues/closed/0042 が共通化してきたドラッグ処理に新しい重複を増やす。

却下した案: `startPanelResize` に `axis: 'x' | 'y'` オプションを足す。
`getLeftEdge` と `onWidthChange` の名前が縦方向では意味を持たず、改名すると既存の 3 呼び出し元と `sidebarResize.ts` に波及する。

却下した案: `Drawer.tsx` の `startResize` も同時に共通ヘルパーへ寄せる。
Drawer は CSS 変数を使わず `useState` の px 値と独自の `localStorage` キーで動いており、寄せるには Drawer の永続化方式の変更が要る。
本 issue の範囲を超えるため扱わない範囲に置く。

### 3. 高さは本文の高さとして `PersistedState` に永続化する

`frontend/src/hooks/useTerminalDockHeight.ts` を新設し、`useResourcePanelWidth` と同じ形の `useTerminalDockHeight()` が `{ bodyHeight, setBodyHeight }` を返す。
保持する値は本文 (`div.terminal-dock-body`) の高さ (px) で、既定値は現在の `BODY_HEIGHT` と同じ 320 とする。
タブバーの高さはレイアウトの定数であり、折りたたみ時のルート要素の高さ (32px) はこれまでどおり `TAB_BAR_HEIGHT` だけで決まる。
`frontend/src/lib/storage.ts` の `PersistedState` に `terminalDockHeight?: number` を追加し、値が本文の高さであることをコメントに書く。

`setBodyHeight` は呼ばれるたびに `savePersisted` で書く (`useResourcePanelWidth` の `setWidth`、`Drawer.tsx` の `saveDrawerSize` と同じ)。
`pointermove` ごとの `localStorage` 書き込みになるが、既存の 2 実装と同じ頻度であり、`pointerup` でだけ書く仕組みを共通ヘルパーに足すより単純である。

初期値の読み取りでは、`loadPersisted().terminalDockHeight` が有限の数値でない場合 (未設定、手で書き換えた文字列、`NaN`) に既定値を使う。
値の範囲は設計判断 4 の関数で描画時にクランプする。

高さの state は `TerminalDock` の中で持つ。
`App.tsx` は `TerminalDock` を常にマウントし、セッションが 0 のときは `TerminalDock` が `null` を返すだけなので、セッションを全部閉じて開き直しても state は保たれる。

却下した案: `useTerminalSessions.ts` のストアに高さを足す。
このストアはセッションの集合と折りたたみを扱い、永続化しない方針を持つ。
高さは永続化するため、`sidebarWidth` と同じ `PersistedState` の経路に置く方が既存パターンと一致する。

却下した案: Drawer と同じく独自の `localStorage` キー (例: `cloudlens:terminalDockHeight`) に書く。
`frontend/AGENTS.md` の「UI 状態と永続化」が定める `PersistedState` のパターンから外れる。

却下した案: 折りたたみ状態も永続化する。
ユーザーの選択で高さだけを永続化する。
TODO の要望はサイズ変更であり、折りたたみ状態の永続化は求めていない。

### 4. 下限と上限

本文の高さの下限は 160px とする。
`Terminal.tsx` の xterm は `fontSize: 13` で、`.terminal-container` の `padding: 8px` と `border: 1px` を除いた 142px に 8 行から 9 行程度が入る (セルの高さはフォントに依存するため概算)。
コマンド 1 つとその出力数行を見る最小の面積として決める。
これを成立させるため、`app.css` の `.terminal-panel` から `min-height: 320px` を削除する。
`Terminal` を描画するのはドックだけで、本文の下限はドラッグ時のクランプと描画時の再クランプが保証するため、CSS 側に下限を二重に持たない。
`.terminal-panel` の他の宣言 (`position`、`display`、`flex-direction`、`height: 100%`) は変えない。
0174 の設計判断 2 が `.terminal-panel` の規則を変えないとしたのは本文の高さを 320px に固定する前提でのことで、本 issue はその前提を変える。

ルート要素の高さの上限は `Math.round(window.innerHeight * 0.85)` とする。
係数 0.85 は Drawer のドラッグ時と描画時のクランプ (docs/issues/closed/0003) と同じで、ドックと Drawer で画面に残すメインコンテンツ (`.app > .body`) の割合を揃える。
上限はドラッグ開始時 (`pointerdown`) の `window.innerHeight` で計算して `startPanelHeightResize` の `max` に渡す。

本文の高さの範囲は純関数 `terminalDockBodyHeightRange(innerHeight: number): { min: number; max: number }` として `useTerminalDockHeight.ts` に置く。
`min` は 160、`max` は `Math.max(min, Math.round(innerHeight * 0.85) - TAB_BAR_HEIGHT)` で、上限が下限を下回るほど小さいウィンドウでは `max` も 160 になる。
クランプは同じファイルの純関数 `clampTerminalDockBodyHeight(bodyHeight: number, innerHeight: number): number` が行い、`terminalDockBodyHeightRange(innerHeight)` の `min` と `max` に丸める。
ドラッグ時の `min` / `max` (設計判断 2) もこの範囲関数から導くため、クランプの規則を持つ場所は `terminalDockBodyHeightRange` の 1 か所になる。
これに合わせて `TAB_BAR_HEIGHT` (32) を `TerminalDock.tsx` から `useTerminalDockHeight.ts` へ移して export し、`TerminalDock.tsx` は import して使う。
`BODY_HEIGHT` (320) は既定値として同じファイルの `TERMINAL_DOCK_DEFAULT_BODY_HEIGHT` に置き換える。
`TerminalDock` は描画時に永続化された値をこの関数で現在の `window.innerHeight` に対して再クランプし (Drawer の `sizeStyle` と同じ)、クランプ後の値でルート要素の高さと CSS 変数を決める。
再クランプの結果は `localStorage` に書き戻さない (Drawer と同じ。ウィンドウを元の大きさに戻せば元の高さに戻る)。

却下した案: 下限を現在の 320px のままにし、拡大だけを許す。
TODO は「サイズ変更」を求めており、縮小できない変更は要望の半分しか満たさない。

却下した案: `.terminal-panel` の `min-height` を残し、`.terminal-dock-pane .terminal-panel { min-height: 0; }` でドック内だけ打ち消す。
`Terminal` の描画箇所はドックだけなので、打ち消される側の規則に他の利用者が無く、規則が 1 つ余る。

却下した案: `.terminal-panel` の `min-height` を下限の 160px に下げる。
同じ値を TypeScript の定数と CSS の 2 か所に持つことになり、片方だけ変えるとパネルが本文からあふれる結合 (0174 が 320px で抱えていたものと同じ) が残る。

却下した案: 上限を「`window.innerHeight` からトップバーの高さ 46px とメインコンテンツの最小高さを引いた値」にする。
メインコンテンツの最小高さという新しい定数を増やす。
Drawer と同じ係数を使えば、既存の設定値と整合し、定数は増えない。

却下した案: `window` の `resize` イベントを購読して再クランプする。
Drawer も購読していない。
ウィンドウを縮めたときの追従は次の再描画 (タブ切替、折りたたみ、セッションの開閉) で起きる。

### 5. CSS 変数と Drawer は変えない

`--terminal-dock-h` の意味 (ルート要素の高さ、セッションが 0 なら `0px`) は変えない。
`.drawer` / `.drawer.pos-bottom` の `bottom` と閉じ位置の `transform` はこの変数を参照しているため、変更なしで追従する。
`frontend/src/app.css.test.ts` の既存のテストがこの参照を固定している。

### 6. `Terminal.tsx` は変えない

ドックの高さが変わると `.terminal-container` の寸法が変わり、既存の `ResizeObserver` のコールバックが `fitAddon.fit()` を呼ぶ。
`fit()` の後の `term.onResize` が backend へ行数を送る。
`active` の切り替え時の `fit()` と `focus()` もそのまま使う。

### 7. テスト

- `frontend/src/lib/panelResize.test.ts`: `startPanelHeightResize` について、`getBottomEdge() - clientY` が CSS 変数と `onHeightChange` に反映されること、下限と上限でクランプすること、`pointerup` 後の `pointermove` を無視することを、`startPanelResize` の 3 テストと対称に書く。
- `frontend/src/hooks/useTerminalDockHeight.test.ts`: `terminalDockBodyHeightRange` の `min` が 160、`max` が `Math.round(innerHeight * 0.85) - 32`、上限が下限を下回る `innerHeight` では `max` も 160 になること。`clampTerminalDockBodyHeight` の下限、上限、範囲内の値。`useTerminalDockHeight` の初期値 (未設定なら 320、有限の数値でなければ 320、数値ならその値) と `setBodyHeight` による `localStorage` (`cloudlens:v1` の `terminalDockHeight`) への保存。
- `frontend/src/components/Terminal/TerminalDock.test.tsx`: 既存の 352px / 32px のテストは既定値の検証としてそのまま残す。追加するのは次の 4 つ。展開時に `.terminal-dock-resizer` があり折りたたみ時に無いこと。ハンドルの `pointerdown` と `document` の `pointermove` (ルート要素の `getBoundingClientRect` を `bottom: 800` に差し替え、`clientY: 300` を送る) でルート要素の `style.height` と `--terminal-dock-h` が `500px` になり、`terminalDockHeight` に 468 が保存されること。`terminalDockHeight` に 500 を入れてマウントするとルート要素が `532px` になること。上限を超える値 (例: 5000) と下限を下回る値 (例: 100) を入れてマウントするとクランプ後の値になること。ドラッグのテストでは `window.innerHeight` を 800 に設定し、`clientY: 0` (下端との差 800) で `style.height` が `32 + terminalDockBodyHeightRange(800).max` px、`clientY: 790` (差 10) で `32 + terminalDockBodyHeightRange(800).min` px になることも確認し、ドラッグ時のクランプが描画時の再クランプと同じ範囲関数に従うことを固定する。上限の検証では `window.innerHeight` をテスト内で明示的に設定する (`Drawer.test.tsx` の再クランプのテストと同じ)。
- `frontend/src/app.css.test.ts`: `.terminal-dock-resizer` の `position` が `absolute`、`top` が `0`、`height` が `6px`、`z-index` が `1`、`cursor` が `ns-resize` であることを `declarationOf` で固定し、`.terminal-panel` の宣言部に `min-height` が無いことを `declarationsOf` で固定する。

### 8. 追加の API 呼び出しと権限

不要。
変更は frontend の DOM、CSS、`localStorage` に閉じる。
backend と AWS / Google Cloud の API は呼ばない。

### 9. 扱わない範囲

- 折りたたみ状態の永続化 (ユーザーの選択で本 issue から外す)。
- `Drawer.tsx` の `startResize` を共通ヘルパーへ寄せること (設計判断 2 の却下案)。
- ウィンドウの `resize` イベントに追従した再クランプ (設計判断 4 の却下案)。
- キーボードによる高さの変更。Drawer のハンドルにも無い。
- ドックの幅の変更。ドックは `.app` の幅いっぱいに置かれており、幅を変える要望は TODO に無い。

## 完了条件

- `div.terminal-dock` の先頭の子に `div.terminal-dock-resizer` があり、`title` が `drawerStorage` の `terminal.resizeDock` の値である。折りたたみ中 (`collapsed` が真) は描画されない。
- `frontend/src/lib/panelResize.ts` に `startPanelHeightResize({ min, max, cssVar, getBottomEdge, onHeightChange })` があり、`getBottomEdge` を `pointerdown` 時に 1 回だけ呼び、`pointermove` ごとに「その下端 - `ev.clientY`」を `[min, max]` にクランプした値を `cssVar` と `onHeightChange` に反映する。`startPanelResize` の既存テスト 3 件は変更せずに通る。
- ハンドルをドラッグすると、`div.terminal-dock` の `style.height` と `--terminal-dock-h` が「ルート要素の下端 - ポインタの `clientY`」を `[32 + terminalDockBodyHeightRange(window.innerHeight).min, 32 + terminalDockBodyHeightRange(window.innerHeight).max]` にクランプした値 (px) になり、本文の高さ (その値 - 32) が `cloudlens:v1` の `terminalDockHeight` に保存される。
- `frontend/src/lib/storage.ts` の `PersistedState` に `terminalDockHeight?: number` がある。
- `frontend/src/hooks/useTerminalDockHeight.ts` に `TAB_BAR_HEIGHT` (32)、`terminalDockBodyHeightRange(innerHeight)`、`clampTerminalDockBodyHeight(bodyHeight, innerHeight)`、`useTerminalDockHeight()` がある。`terminalDockBodyHeightRange` の `min` は 160、`max` は `Math.max(160, Math.round(innerHeight * 0.85) - 32)` で、`clampTerminalDockBodyHeight` はこの `min` と `max` に丸める。`terminalDockHeight` が未設定または有限の数値でないときの本文の高さは 320 である。
- `terminalDockHeight` に保存された値でマウントすると、展開時のルート要素の高さと `--terminal-dock-h` が `32 + clampTerminalDockBodyHeight(保存値, window.innerHeight)` px になる。折りたたみ時は `32px` のまま、セッションが 0 のときは `--terminal-dock-h` が `0px` のままである。
- `app.css` に `.terminal-dock-resizer` (`position: absolute`、`top: 0`、`height: 6px`、`z-index: 1`、`cursor: ns-resize`) があり、`.terminal-panel` に `min-height` の宣言が無い。`.terminal-panel` の他の宣言、`.drawer` / `.drawer.pos-bottom` の規則、`--terminal-dock-h` の意味は変わらない。
- `frontend/src/components/Terminal/Terminal.tsx`、`frontend/src/hooks/useTerminalSessions.ts`、`frontend/src/components/Drawer/Drawer.tsx` は変更しない。
- 設計判断 7 のテストが追加され、`TerminalDock.test.tsx` の既存の 352px / 32px のテストは変更せずに通る。
- 手動確認: `mise run backend:run` と `mise run frontend:run` で起動し、実 AWS 環境で EC2 インスタンスへのセッションを開いて次を確認する。タブバーの上端 6px にポインタを置くとカーソルが `ns-resize` になり、そこから上下にドラッグするとドックの高さがポインタに追従し、タブの切り替えは起きない。タブバーの上端 6px より下をクリックするとタブの切り替えが起きる。本文を下限 (160px) まで縮めた状態でも、ターミナルの最終行がドックの本文の中に表示され、本文からはみ出さない。ブラウザを再読み込みしてセッションを開き直すと、ドラッグ後の高さでドックが表示される。実 AWS 環境が使えない場合は、未実施であることと理由を issue に記録する。
- `CHANGES.md` の `## develop` に `[ADD]` のエントリを追加する。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0174: ターミナルドックの導入。設計判断 8 で本 issue の内容を先送りした。`--terminal-dock-h` と `.terminal-dock` の CSS はそこで決めたものを使う。
- docs/issues/closed/0015: 列幅とサイドバー幅のドラッグ処理の重複を共通化した issue。設計判断 2 はこの方針に従う。
- docs/issues/closed/0042: `startPanelResize` の切り出しと、`useResourcePanelWidth` による `resourcePanelWidth` の永続化。設計判断 2 と 3 の型になる。
- docs/issues/closed/0003: Drawer の高さのクランプ係数 0.85 と描画時の再クランプ。設計判断 4 の型になる。

## 解決方法

`frontend/src/hooks/useTerminalDockHeight.ts` を新設し、常駐ターミナルドックの本文 (`div.terminal-dock-body`) の高さを状態として持つ `useTerminalDockHeight()` を追加した。
定数 `TAB_BAR_HEIGHT` (32)、`TERMINAL_DOCK_DEFAULT_BODY_HEIGHT` (320)、`TERMINAL_DOCK_MIN_BODY_HEIGHT` (160)、`TERMINAL_DOCK_MAX_HEIGHT_RATIO` (0.85) と、範囲を返す `terminalDockBodyHeightRange(innerHeight)`、値を範囲へ丸める `clampTerminalDockBodyHeight(bodyHeight, innerHeight)` を同ファイルに置き、ドラッグ時のクランプと描画時の再クランプの両方がこの関数を通す (設計判断 4)。
`frontend/src/lib/storage.ts` の `PersistedState` に `terminalDockHeight?: number` を追加し、`useTerminalDockHeight` の `setBodyHeight` が呼ばれるたびに `savePersisted` で保存する。値が未設定または有限の数値でないときは既定値 320 にフォールバックする。

`frontend/src/lib/panelResize.ts` に縦方向のドラッグ処理 `startPanelHeightResize({ min, max, cssVar, getBottomEdge, onHeightChange })` を追加した (設計判断 2)。`pointerdown` 時に `getBottomEdge()` を 1 回だけ呼んで下端を固定し、`pointermove` ごとに「下端 - `ev.clientY`」を `[min, max]` にクランプして `cssVar` と `onHeightChange` に反映する。`document` へのリスナー登録 / 解除と `body` のカーソル・選択抑止は、横方向の `startPanelResize` と共通の内部関数 `beginDrag` に寄せた (カーソルは `ew-resize` と `ns-resize` を引数で分ける)。`startPanelResize` のシグネチャと挙動は変えていない。

`frontend/src/components/Terminal/TerminalDock.tsx` にハンドル `div.terminal-dock-resizer` を `div.terminal-dock` の先頭の子として追加した (設計判断 1)。折りたたみ中 (`collapsed` が真) は描画しない。`title` は `drawerStorage` の新規キー `terminal.resizeDock` (`frontend/src/i18n/locales/ja/drawerStorage.json` に追加) を使う。ハンドルの `onPointerDown` はドラッグ開始時の `window.innerHeight` から `terminalDockBodyHeightRange` で範囲を求め、`startPanelHeightResize` に `min`/`max` (`TAB_BAR_HEIGHT` を加えた値)、`cssVar` (`--terminal-dock-h`)、`getBottomEdge` (ルート要素の `getBoundingClientRect().bottom`)、`onHeightChange` (本文の高さを `setBodyHeight` へ) を渡す。ルート要素の高さ `rootHeight` は、展開時は `TAB_BAR_HEIGHT + clampTerminalDockBodyHeight(bodyHeight, window.innerHeight)`、折りたたみ時は `TAB_BAR_HEIGHT` になる (Drawer の `sizeStyle` と同じく描画のたびに再クランプし、クランプ後の値を書き戻さない)。

`frontend/src/app.css` に `.terminal-dock-resizer` (`position: absolute`、`top: 0`、`left: 0`、`right: 0`、`height: 6px`、`cursor: ns-resize`、`z-index: 1`) と `:hover` の規則 (`.panel-resizer:hover` と同じ `background: var(--accent); opacity: 0.5`) を追加し、`.terminal-panel` の `min-height: 320px` を削除した (設計判断 1、4)。`.terminal-panel` の他の宣言、`.drawer` / `.drawer.pos-bottom` の規則、`--terminal-dock-h` の意味は変えていない。

`frontend/src/components/Terminal/Terminal.tsx`、`frontend/src/hooks/useTerminalSessions.ts`、`frontend/src/components/Drawer/Drawer.tsx` は変更していない。

### 完了条件の検証

- ハンドルの位置、title、折りたたみ時の非描画: `TerminalDock.test.tsx`「リサイズハンドルは展開中だけ、ドックの先頭の子として描画される」で検証。満たす。
- `startPanelHeightResize` の仕様と既存 `startPanelResize` テスト 3 件の無改変: `panelResize.test.ts` に 4 件追加、既存 3 件は import 行以外無変更で通過。満たす。
- ドラッグによる `style.height` / `--terminal-dock-h` の追従と保存: `TerminalDock.test.tsx`「ハンドルのドラッグでルート要素の高さが追従し、本文の高さが永続化される」(`bottom: 800`、`clientY: 300` → 500px、保存値 468)、上限 / 下限のクランプ (`clientY: 0` → `32 + range.max`、`clientY: 790` → `32 + range.min`) で検証。満たす。
- `PersistedState.terminalDockHeight?: number`: `frontend/src/lib/storage.ts` に追加。満たす。
- フックの 4 シンボルと下限 160 / 上限式 / 既定 320: `useTerminalDockHeight.ts` に実装、`useTerminalDockHeight.test.ts` の 9 件で検証。満たす。
- 保存値でのマウント、折りたたみ 32px、セッション 0 で 0px: `TerminalDock.test.tsx`「永続化された本文の高さでマウントすると…」(532px → 折りたたみ 32px → 閉じて 0px) と上限 / 下限クランプの 2 件で検証。満たす。
- `.terminal-dock-resizer` の CSS 宣言と `.terminal-panel` の `min-height` 削除: `app.css.test.ts` の「app.css のターミナルドックの高さ変更」2 件で検証。`.drawer` 系と `--terminal-dock-h` の既存テストも無改変で通過。満たす。
- `Terminal.tsx` / `useTerminalSessions.ts` / `Drawer.tsx` の無変更: `git status` に変更が出ないことで確認。満たす。
- 設計判断 7 のテスト追加と既存 352px / 32px テストの無改変: `TerminalDock.test.tsx` の既存テストの差分は `beforeEach`/`afterEach` の追加 (`localStorage.clear()`、`window.innerHeight` の復元) のみでアサーションは無変更。満たす。
- 手動確認: 実 AWS 環境とブラウザがこの実行環境から使えないため未実施。
- `CHANGES.md` の `[ADD]` エントリ: 追加済み (下記コミットに含む)。
- `mise run check` の通過: 統合後の作業ツリーで実行し exit 0 (frontend 987 件全通過、eslint 0 エラー / 10 警告でベースラインと同数)。満たす。

### 方針からの乖離 (方式を保った実装詳細)

- ハンドルの `onPointerDown` を毎回のイベントで組み立て、`terminalDockBodyHeightRange(window.innerHeight)` をその場で読んでから `startPanelHeightResize` を呼ぶ形にした。設計判断 2 が求める「上限はドラッグ開始時の `window.innerHeight` で計算する」を、レンダー時に固定したクロージャではなく呼び出し時点の値で満たすため。
- `getBottomEdge` に `rootRef.current?.getBoundingClientRect().bottom ?? window.innerHeight` というフォールバックを付けた。`rootRef.current` の型が `null` を許すためで、ハンドルはルート要素の子として描画されるため実行時にはフォールバック側へは到達しない。
- `panelResize.ts` に `beginDrag` と `clamp` の 2 つの内部関数を切り出した。設計判断 2 が求める「リスナー登録 / 解除と body のスタイルの共通化」の範囲内で、横方向と縦方向のクランプ式が同一のため `clamp` も合わせて共通化した。
- `TERMINAL_DOCK_MIN_BODY_HEIGHT` (160) と `TERMINAL_DOCK_MAX_HEIGHT_RATIO` (0.85) を `terminalDockBodyHeightRange` の内部に埋め込まず、コメント付きの定数として export した。完了条件が要求する 4 シンボルはそのまま存在し、値の由来 (下限 160 の根拠、Drawer と同じ係数であること) を 1 か所にまとめるための整理にとどまる。
- `app.css` の `.terminal-dock-body` の上のコメントを、`.terminal-panel` の `min-height: 320px` に言及していた記述から、その宣言が無くなった事実に合わせて更新した。宣言そのものは変えていない。

### 多観点レビューでの指摘と対応

close 前の多観点レビュー (完了条件の充足 / テストと堅牢性 / 規約と整合) を 1 ラウンド実施した。

- 完了条件の充足: 指摘 0 件。
- テストと堅牢性: 高 / 中 0 件。低 3 件 (`getBottomEdge` のフォールバック分岐が未到達、上限クランプ側テストで保存値の確認を省いている、範囲テストの一部が実装式のコピーになっている) はいずれも完了条件が要求する範囲外のため、追加のテストは行わず却下した。
- 規約と整合: 高 1 件。close 成果物ドラフトの `docs/issues/TODO.md` 書き換え案がインデントされた子行 (Drawer との比較の補足) を削除していた。「TODO.md の書き換え規則」に反するため、子行を残したまま親行だけを書き換える形に修正した (実装コードの変更を伴わないため追加レビューと再テストは行っていない)。
