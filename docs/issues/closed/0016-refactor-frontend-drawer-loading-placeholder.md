# Drawer リストタブの Loading プレースホルダを共通コンポーネント化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`<div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>` というインラインの Loading プレースホルダが Drawer 配下の 9 箇所にコピペされている。

- `DrawerECRImages.tsx` (1 箇所)
- `DrawerECSServices.tsx` (1 箇所)
- `DrawerECSTasks.tsx` (1 箇所)
- `DrawerELBListeners.tsx` (2 箇所)
- `DrawerELBTargets.tsx` (2 箇所)
- `DrawerDynamoItems.tsx` (2 箇所)

スタイル (padding / 文字色) が 9 箇所に分散しており、変更時に漏れが生じる。

## 対応方針

同一マークアップを返す `DrawerLoading` コンポーネントを `components/Drawer/` に追加し、9 箇所を置き換える。

注意: `DataTable` の `isLoading` prop (スピナー表示) に寄せる統一は、ローディング表示の見た目がテキストからスピナーへ変わるため行わない。現状のテキスト表示のマークアップをそのまま維持する。

## 画面表示への影響

なし。マークアップ・スタイル・文言は同一。

## 解決方法

- `frontend/src/components/Drawer/DrawerLoading.tsx` を新規作成し、従来と同一のマークアップ (`<div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>`) を返す `DrawerLoading` コンポーネントを定義した。
- DrawerECRImages (1) / DrawerECSServices (1) / DrawerECSTasks (1) / DrawerELBListeners (2) / DrawerELBTargets (2) / DrawerDynamoItems (2) の計 9 箇所を `<DrawerLoading />` に置換した。
- DataTable の isLoading (スピナー) への統一は表示が変わるため、方針どおり行っていない。
- `mise run check` 通過 (frontend: 0 errors / 105 tests passed) を確認した。
