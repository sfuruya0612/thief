# workbench レイアウトで TopBar とセッションタブを 1 段にし、サイドバーを rail に畳めるようにする

Created: 2026-09-29
Model: Claude Fable 5.1

## 背景

docs/issues/0210 の第 2 段 (シェルの構造の前半)。`standard` では `App.tsx` が `TopBar` (46px) の下に `AwsSessionTabs` / `GcpSessionTabs` / `DatadogOrgSessionTabs` (38px) を別の行として描画する。案 B はこの 2 段を 40px の 1 段にまとめ (左にマーク、プロバイダの切替、続けてセッションタブ、右に操作)、サイドバーを 44px の icon rail に畳めるようにする。

`SessionTabs.tsx` はタブ列の幅を `ResizeObserver` で測り、収まらないタブを「他 N ▾」に畳む (`lib/sessionTabsLayout.ts`)。タブ本体、ドラッグ並べ替え、ピッカーはこのまま使える。`Sidebar.tsx` (AWS) と `views/GcpSidebar.tsx` (Google Cloud) は同じ形 (セッションカード + カテゴリ + `nav-item`) で、幅は `--sidebar-w` (`App.tsx` が永続化) をドラッグで変える (`lib/sidebarResize.ts`)。

## 目的

`layout === 'workbench'` のとき、TopBar がセッションタブを内包した 1 段 (`--topbar-h` 40px) になり、サイドバーを rail (44px、アイコンと件数だけ) に畳めるようにする。`standard` の DOM と見た目は変えない。分岐は `App.tsx` (ヘッダの組み立て) と `Sidebar` / `GcpSidebar` (rail) の 2 か所に限る。

## 設計判断

- `TopBar` に `sessionTabs?: ReactNode` を足す。`App.tsx` は `layout === 'workbench'` のときセッションタブの要素を `TopBar` に渡し、`standard` のときは従来どおり `TopBar` の下に描画する。`TopBar` は `sessionTabs` があれば、プロバイダ切替 (`.view-switch`) の右に `.topbar-sessions` として置く。`.session-tabs` は `.topbar` の中では高さ `--topbar-h`、背景なし、アクティブタブは上角の丸ではなく 2px の下線 (`accent`) にする (`shell.css` の `.topbar .session-tabs*` で上書き)。`SessionTabs` の幅の計測は `rootRef` の `clientWidth` なので、置き場所が変わっても動く。`session-tabs-hint` (ドラッグの案内) は 1 段のときは出さない。
  - 却下案: `TopBar` が `useTweaks` を読んで自分でセッションタブを描画する。セッションタブは view (AWS / GCP / Datadog) ごとに違う部品で、その選択は `App.tsx` が持っているため、`App.tsx` が組み立てて渡す方が責務が分かれる。
- rail: `Sidebar` / `GcpSidebar` に `collapsed` と `onToggleCollapsed` を渡す。畳んだときは `aside.sidebar.rail` になり、幅 44px (`--rail-w`)、セッションカードはマーク (環境ドット) だけ、カテゴリ見出しは非表示、`nav-item` はアイコン + 件数を縦に、`title` にサービス名を持たせる。`pane-mark` (分割中のペイン番号) は残す。`sidebar-resizer` は畳んだときは出さない。折りたたみ状態は `PersistedState.sidebarCollapsed` に永続化する (`usePersistedSidebarWidth` と同じ薄いフック)。`standard` では `collapsed` を渡さない (rail にならない)。
  - 畳む操作は rail の下端のボタン (`⌘B` の表示付き) と、`⌘B` / `Ctrl+B` のショートカット。ショートカットは `App.tsx` の 1 つの `keydown` リスナーで、入力中 (`input` / `textarea` / `contenteditable`) は無視する。
  - 却下案: 幅を 44px までドラッグで縮める。ラベルの表示 / 非表示が幅から決まって中途半端な状態ができるため、状態として持つ。
- `.body` のグリッドは `rail` のとき `--rail-w 1fr` にする (`.body.rail`)。`App.tsx` が `data-sidebar` を付けるのではなく、`AccountView` / `GcpView` が `collapsed` を受けて `.body` にクラスを付ける。
- `AGENTS.md` の frontend 節に「Layout の分岐は `App.tsx` (ヘッダ)、`Sidebar` / `GcpSidebar` (rail)、`ServicePanel` の上段 (0212)、`DrawerFrame` (0213) に限る。他の部品は `data-layout` の CSS 変数で差を吸収する」を書く。

## 完了条件

- `workbench` で TopBar がセッションタブを内包し、高さが 40px の 1 段になる。`standard` では従来どおり 2 段で、DOM も変わらない (`App.test.tsx` / `TopBar` のテストで固定する)。
- タブの畳み (「他 N ▾」)、ドラッグ並べ替え、ピッカーが 1 段でも動く (`SessionTabs.test.tsx` の既存テストが通る)。
- `workbench` でサイドバーを rail に畳め、`⌘B` / `Ctrl+B` でも切り替わり、再読み込み後も残る。`standard` では畳めない。
- `AGENTS.md` に分岐の規約が書かれている。
- 開発サーバで AWS / Google Cloud / Datadog の各 view で 1 段のヘッダを目視し、分割表示中の rail のペイン番号を確認する。
- `CHANGES.md` の `## develop` に `[ADD]` として記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0210 (先に実装する)、0212、0213。
- docs/issues/closed/0020 (セッションタブ)、0027 (サイドバーのカテゴリ)、0175 (分割表示のペイン番号)。
