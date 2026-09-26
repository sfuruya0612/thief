# Theme の切り替えを Tweaks パネル内のみに集約する

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

`docs/issues/TODO.md` に「Theme の選択は Tweaks の中だけにする」という要望がある。

現在 Theme (Dark/Light) の切り替え手段は 2 箇所に存在する。

- `components/TopBar.tsx`: sun/moon アイコンのトグルボタン (`onToggleTheme`)。
- `components/TweaksPanel.tsx`: Theme 行のセグメントコントロール。

同じ設定に対する入口が 2 つあると、UI 設定の集約先が `TweaksPanel` であるという方針 (AGENTS.md frontend 節「テーマ・密度・アクセントカラー等は hooks/useTweaks.ts の Tweaks 型に集約」) と整合しない。

## 目的

Theme の切り替え UI を `TweaksPanel` の Theme 行のみに一本化し、`TopBar` からテーマトグルボタンを削除する。

## 完了条件

- `TopBar` から sun/moon テーマトグルボタンと関連する props (`theme` / `onToggleTheme`) を削除する。
- `App.tsx` の `handleToggleTheme` と TopBar への受け渡しを削除する。
- Theme の切り替えは `TweaksPanel` の Theme 行で引き続き行える。
- `mise run check` が全て通過する。

## 解決方法

- `components/TopBar.tsx` から sun/moon のテーマトグルボタンを削除し、`TopBarProps` の `theme` / `onToggleTheme` と、未使用になった `Theme` 型の import を削除した。
- `App.tsx` から `handleToggleTheme` (`useCallback`) と `TopBar` への `theme` / `onToggleTheme` の受け渡しを削除した。テーマ更新にしか使っていなかった `useTweaks()` の `update` の分割代入も外し、`tweaks` のみを取得するようにした。
- Theme の切り替えは `TweaksPanel` の Theme 行 (セグメントコントロール) に一本化された。

### 検証

- `mise run frontend:lint`: 0 errors、警告 9 件 (すべて本変更前から存在する既存の警告で、変更由来の新規警告なし)。
- `mise run frontend:test`: 57 ファイル / 506 テスト全て pass。
- `mise run frontend:fmt`: 変更ファイルはいずれも整形済み (unchanged)。
