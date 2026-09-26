# 0008. frontend の型を Raw 型と Row 型の 2 層に分ける

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

backend の JSON はフィールド名が snake_case で、値の形も API の都合で決まる。
UI の表と Drawer は camelCase の、表示に都合のよい形を使いたい。
backend の形を UI の部品が直接扱うと、backend の変更が UI の全体に波及する。

## 決定

- `frontend/src/types/*.ts` に、backend の JSON の形 (Raw 型) と UI の形 (Row 型) を分けて定義する。
- Raw 型から Row 型への変換は、`frontend/src/lib/normalize*.ts` の純関数 (`xxxFromRaw`) に集める。
  ファイルは `normalize.ts` (AWS)、`normalizeNonAws.ts`、`normalizeGcp.ts`、`normalizePricing.ts`、`normalizeQuery.ts` に分かれる。
  `AGENTS.md` の frontend 「ディレクトリ構造」は前の 2 つだけを書いている。
- UI の部品に snake_case のフィールドを渡さない。

## 検討した代替案

記録なし。

## 結果

- backend の形の変更は、Raw 型と変換関数の中で止まる。
- 型の一致の検査は Raw 型だけを対象にできる (ADR 0007)。
- 変換関数は純関数なので、表駆動の単体テストで検査する。

## 根拠資料

- `AGENTS.md` の frontend 「ディレクトリ構造」
- `frontend/src/lib/normalize.ts`、`normalizeNonAws.ts`、`normalizeGcp.ts`、`normalizePricing.ts`、`normalizeQuery.ts`
