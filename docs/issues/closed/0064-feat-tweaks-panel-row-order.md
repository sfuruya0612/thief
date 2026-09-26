# Tweaks パネルの項目順を Theme, Language, Detail panel, Accent にする

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

`docs/issues/TODO.md` に「Tweaks の順は Theme, Language, Detail panel, Accent にする」という要望がある。

現在 `components/TweaksPanel.tsx` の行の順は Theme, Detail panel, Accent, Language であり、後から追加した Language 行が末尾にある。

## 目的

`TweaksPanel` の行順を Theme, Language, Detail panel, Accent に並べ替える。

## 完了条件

- `TweaksPanel` の行を Theme, Language, Detail panel, Accent の順に並べ替える。
- 各行の機能 (テーマ / 言語 / Detail panel 位置 / アクセントカラーの切り替え) は変更しない。
- `mise run check` が全て通過する。

## 解決方法

- `components/TweaksPanel.tsx` で末尾にあった Language 行 (`t('lang.label')` のセグメントコントロール) を Theme 行の直後へ移動し、行順を Theme, Language, Detail panel, Accent に並べ替えた。各行の中身 (ハンドラ・表示) は変更していない。

### 検証

- `mise run frontend:lint`: 0 errors、警告 9 件 (既存のみ)。
- `mise run frontend:test`: 57 ファイル / 506 テスト全て pass。
- `mise run frontend:fmt`: 変更ファイルは整形済み (unchanged)。
