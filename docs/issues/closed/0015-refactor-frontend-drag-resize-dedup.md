# 列幅リサイズとサイドバー幅リサイズのドラッグ処理の重複を共通化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

1. 列幅リサイズ `startColResize` (約 27 行: thead から実描画幅のスナップショット取得 → pointermove で幅更新 → pointerup で listener 解除 + cursor / userSelect 復元) が 3 箇所でほぼ逐語一致している。

   - `frontend/src/components/DataTable.tsx` (103-130 行)
   - `frontend/src/components/Drawer/DrawerDynamoItems.tsx` (80-106 行)
   - `frontend/src/components/tables/CostCrossTable.tsx` (41-67 行。ファイル内コメント自身が「DataTable.tsx の startColResize と同型」と重複を明言)

   付随して `colWidths` state・`theadRowRef`・`MIN_COL_WIDTH = 60` も 3 箇所に重複。

2. サイドバー幅リサイズ `startResize` が 2 箇所で完全一致している。

   - `frontend/src/components/Sidebar.tsx` (57-74 行)
   - `frontend/src/views/GcpSidebar.tsx` (47-64 行)

   `SIDEBAR_MIN_WIDTH` / `SIDEBAR_MAX_WIDTH` 定数も両方に重複。

## 対応方針

- `hooks/useColumnResize.ts` を追加し、`{ colWidths, theadRowRef, startColResize }` を返すフックに集約する。DataTable 固有の `setResized(true)` は `onResizeStart` コールバックオプションで注入する。
- サイドバー側は `startSidebarResize(onWidthChange)` ヘルパ (純関数) を切り出し、両サイドバーで共用する。

なお Sidebar / GcpSidebar の骨格 (SECTIONS マップ + SvcItem) にも構造的重複があるが、profile-card の中身・queryKey・観測可否判定など差分が本質的であり、slot 化は間接化に対して得るものが小さいため対象外とする。

## 画面表示への影響

なし。ドラッグ挙動・クランプ値・DOM は同一。

## 解決方法

- `frontend/src/hooks/useColumnResize.ts` を新規作成し、`colWidths` state・`theadRowRef`・`startColResize` (スナップショット取得 → pointermove 更新 → pointerup 解除)・`MIN_COL_WIDTH = 60` を集約した。DataTable 固有の `setResized(true)` は `onResizeStart` オプションとして注入する形にした。
- `frontend/src/lib/sidebarResize.ts` を新規作成し、`startSidebarResize(onWidthChange?)` が onPointerDown ハンドラを返す純関数として `SIDEBAR_MIN_WIDTH = 160` / `SIDEBAR_MAX_WIDTH = 480` のクランプと `--sidebar-w` CSS 変数更新を集約した。
- 消費側 5 ファイルの重複実装を削除し共通実装への参照に置換した。
  - `DataTable.tsx`: `useColumnResize({ onResizeStart: () => setResized(true) })`
  - `Drawer/DrawerDynamoItems.tsx`: `useColumnResize()`
  - `tables/CostCrossTable.tsx`: `useColumnResize()` (DEFAULT_GROUP/TOTAL/CATEGORY_WIDTH と sticky left 計算は維持)
  - `Sidebar.tsx` / `views/GcpSidebar.tsx`: `onPointerDown={startSidebarResize(onWidthChange)}`
- `mise run check` (fmt / lint / test 両側) 通過を確認した。lint warning は全て既存ファイル由来で今回の変更ファイルには存在しない。
