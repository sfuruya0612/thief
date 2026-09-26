# Athena のクエリ履歴取得でワークグループ絞り込みパラメータを検証するテストを追加する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/athena.go` の `listAthenaQueryHistory` (369-371 行) は、workgroup 引数が空でないとき `ListQueryExecutionsInput.WorkGroup` を設定して呼び出す。
この設定が無いと Athena はワークグループを跨いだ全実行履歴を返すため、「指定ワークグループの履歴」という絞り込みが静かに壊れる。
しかし `athena_test.go` の `TestListAthenaQueryHistoryOrdersAndBatches` は workgroup を渡しているのに、fake の `listQueryExecutions` コールバックが受け取った Input を参照しておらず、`input.WorkGroup = aws.String(workgroup)` の行を削除してもテストは 1 つも落ちない。

同ファイルの `StartQueryExecutionInput` や `GetQueryResultsInput.MaxResults` は既にリクエスト構築の検証テストがあり、この経路だけが未検証である。

## 対応方針

- `athenaAPI` インターフェースと fake によるテスト基盤は既にあるため、抽出は不要。既存の fake の `listQueryExecutions` コールバックで受け取った `ListQueryExecutionsInput` を記録し、検証を追加するだけでよい。
- workgroup 指定時に `WorkGroup` が設定されること、空のとき未設定 (nil) であることの両方を検証する。

## 完了条件

- `listAthenaQueryHistory` について、workgroup 指定時に `ListQueryExecutionsInput.WorkGroup` が設定され、空のとき nil であることを検証するテストが存在する。
- `input.WorkGroup = aws.String(workgroup)` の行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法 (Input を記録するモック) の先例。

## 解決方法

`backend/internal/aws/athena_test.go` に `TestListAthenaQueryHistorySendsWorkGroup` を追加した。実装 (`athena.go`) は変更していない。

- 既存の `fakeAthena` の `listQueryExecutions` コールバックで、受け取った `ListQueryExecutionsInput` を値コピーして記録する。`listAthenaQueryHistory` は同一の Input ポインタを使い回して `NextToken` だけを書き換えるため、ポインタのまま記録すると呼び出し時点の値が失われる。
- テーブル駆動で workgroup 指定時 (`"analytics"`) と空文字の 2 ケースを回し、`WorkGroup` が前者で `aws.String("analytics")`、後者で nil であることを `cmp.Diff` で検証する。
- 1 ページ目に `NextToken` を返して 2 回呼び出させ、ページ送り後の呼び出しでも `WorkGroup` が維持されることを両呼び出しで検証する。併せて 1 回目の `NextToken` が nil、2 回目が 1 ページ目の値であることを確認し、2 回目が実際のページ送りであることを固定する。

完了条件の検証。

- 「workgroup 指定時に `ListQueryExecutionsInput.WorkGroup` が設定され、空のとき nil であることを検証するテストが存在する」: `TestListAthenaQueryHistorySendsWorkGroup` の 2 サブテスト (`workgroup given` / `workgroup empty`) で検証した。`go test -run TestListAthenaQueryHistory ./internal/aws/` が ok。
- 「`input.WorkGroup = aws.String(workgroup)` の行を削除するとテストが失敗する」: 該当行を一時削除して実行し、`workgroup given` が call 1 / call 2 の両方で WorkGroup mismatch により FAIL することを確認した。併せて逆方向として条件分岐を外し常に `WorkGroup` を設定する改変も試し、`workgroup empty` が nil と `&""` の差分により FAIL することを確認した。いずれも確認後に `athena.go` を復元し、`git diff` で差分が無いことを確認した。
- 「`mise run check` が通る」: 全通過 (frontend 732 tests passed、backend 全パッケージ ok、lint・govulncheck クリーン)。ベースライン (実装前) も失敗ゼロで、新たな失敗は無い。
