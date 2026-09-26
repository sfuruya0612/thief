# 0006. frontend の状態は TanStack Query と React の state で持ち、ルーターを入れない

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

frontend には、サーバから取得するデータ (リソース一覧、コスト) と、UI の状態 (開いているタブ、選択中のビュー、表示設定) がある。
画面はトップレベルのビュー 4 つ (`AppView`: `aws`、`gcp`、`datadog`、`tidb`) と、その中のサービスの切り替えでできている。

## 決定

- サーバの状態は TanStack Query (`@tanstack/react-query`) で持つ。
  `queryKey` はドメインを先頭に置く配列にする (例: `['aws', service, profile, region]`)。
- UI の状態は `useState`、`useReducer`、カスタムフックで持つ。
  Redux、Zustand、Jotai などは導入しない。
- react-router などのルーターを導入しない。
  ビューとサービスの選択は React の state で持つ。
- 永続が要る UI の状態は、原則として `localStorage` のキー `cloudlens:v1` に保存する (`frontend/src/lib/storage.ts` `STORAGE_KEY`、`PersistedState`)。
  新しい永続フィールドは `PersistedState` に足し、`usePersistedXxx()` の形のフックで読み書きする。
  例外は Drawer の大きさ (`cloudlens:drawerSize`、`frontend/src/components/Drawer/Drawer.tsx` `DRAWER_SIZE_KEY`) と、クエリエディタのタブと文脈 (`cloudlens:qe:v1:<service>:<scope>:<kind>`、`frontend/src/lib/queryEditorStorage.ts`) である。
- 複数のコンポーネントで共有する状態は、`useSyncExternalStore` のストアで持つ (例: `frontend/src/hooks/useTerminalSessions.ts`、`frontend/src/hooks/useTweaks.ts`)。

## 検討した代替案

記録なし。
`AGENTS.md` は YAGNI を理由に挙げている。

## 結果

- URL で画面を共有できない。
  再読み込みの後の復元は `localStorage` に頼る。
- キー名 `cloudlens` の由来は記録に無い。
  履歴に残る改名は snatch から thief への改名だけである (コミット f67dba3、2025-01-26 に旧名の残りを直した)。
  キーを変えると、利用者の保存済みの状態が失われる。
- 永続の形式を変えるときは、読み込み時に移行する。
  issue 0020 はセッションタブの導入時に、旧フィールド (`activeProfile`、`gcpProject`) を冪等な純関数 (`migrateSessions`) で移行した。
- タブ、分割表示などの UI の状態遷移は、純関数 (`frontend/src/lib/sessionTabsState.ts`、`frontend/src/lib/splitPanes.ts`) に切り出してテストする。

## 根拠資料

- `AGENTS.md` の frontend 「基本方針」「サーバ状態」「UI 状態と永続化」
- `docs/issues/closed/0020`
- `frontend/src/lib/storage.ts`、`frontend/src/main.tsx`
