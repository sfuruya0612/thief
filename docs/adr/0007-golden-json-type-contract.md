# 0007. backend と frontend の型の一致をゴールデン JSON で検査する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-08-05

## 状況

backend の Go の型と JSON タグを変えても、frontend の TypeScript の型 (Raw 型) は自動では変わらない。
OpenAPI の定義もコード生成も無い。
片方だけを変えると、食い違いは実行時まで見つからない。

## 決定

- backend が、検査の対象の型から決定的な値を埋めた JSON (ゴールデン) を生成する。
  対象の型は `backend/internal/contract/contract.go` の `Registry` に列挙する。
- ゴールデンは `frontend/src/types/__contract__/<Type>.json` に置く。
  `backend/` で `UPDATE_GOLDEN=1 go test ./internal/contract/` を実行して再生成する。
- frontend は `frontend/src/types/contract.check.ts` で、ゴールデンと Raw 型のキーの集合が双方向に一致し、型が適合することを `tsc --noEmit` で検査する (`npm run lint` に含まれる)。
- `omitempty` の有無は、別のゴールデン (`backend/internal/contract/testdata/tags.golden`) で検査する (issue 0110)。

## 検討した代替案

issue 0099 は 4 案を比べた。

- 案 1: 自作の変換ツール。
  保守の負担が大きいため採らなかった。
- 案 2: ゴールデン JSON。
  依存を足さずに `go test` と `tsc` だけで、双方向の欠落、改名、型の不一致を見つけられるため採った。
- 案 3: OpenAPI。
  定義の記述と生成の仕組みの導入が要るため採らなかった。
- 案 4: JSON タグの一覧だけを比べる。
  型の不一致を見つけられないため採らなかった。

## 結果

- backend の型か JSON タグを変えたら、ゴールデンの再生成が必須になる。
  再生成を忘れると `go test` が失敗する。
- `null` を許すかどうかの過不足は検出できない。
  issue 0099 はこれを検出の限界として受け入れた。
- 検査の対象は Raw 型だけである (ADR 0008)。

## 根拠資料

- `docs/issues/closed/0099`、`docs/issues/closed/0110`
- `AGENTS.md` の「backend / frontend の型契約 (golden JSON)」
