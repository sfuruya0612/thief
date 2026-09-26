# CloudFormation の ListStacks に渡すステータスフィルタを検証するモックテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/cfn.go` は 2 箇所で `ListStacksInput.StackStatusFilter` を設定して呼び出す。

- `ListCFNStacks` (60 行): DELETE_FAILED を含まない 18 種のステータスを列挙する。
- `ListCfnStackSummaries` (417 行): DELETE_FAILED や CleanupInProgress 系を含む別のステータス集合を列挙する (レガシー CLI 互換)。

`StackStatusFilter` を削除してもコンパイルと API 呼び出しは成功するが、一覧に含まれるスタックの集合が静かに変わる。
また 2 つの関数が意図的に異なるステータス集合を持つ事実もテストで固定されておらず、どちらかの列挙を誤って書き換えても検知できない。
`cfn_test.go` は変換関数 (`cfnEventFromSDK` 等) のテストのみで、リクエスト構築は検証していない。
cfn.go には ElastiCache の `elastiCacheDescribeClustersClient` に相当するテスト用インターフェースが無く、`*cloudformation.Client` を直接使っているため、現状の構造ではリクエスト構築を単体テストで検証できない。

## 対応方針

- docs/issues/closed/0102 と同じ方式を採る。`ListStacks` を持つ狭いインターフェースを cfn.go に定義し、2 つの呼び出しロジックをインターフェースを受け取る内部関数に抽出する。
- 受け取った `ListStacksInput` を記録する手書きモックを cfn_test.go に追加し、2 つの経路それぞれで `StackStatusFilter` の中身 (ステータス集合そのもの) を検証する。
- ページネータ経由の呼び出しであるため、0102 と同様に複数ページのレスポンスでも全呼び出しでフィルタが維持されることを検証する。
- 2 つの関数のステータス集合を共通の定数に統合してから 1 つの期待値で検証する案は採らない。2 つの集合は意図的に異なる (レガシー CLI 互換) ためであり、統合はその差分を消してしまう。テストは各関数の集合をそれぞれ別の期待値として固定する。
- 経路ごとに別のモック型を作る案は、記録する内容が同じで重複になるため採らない (docs/issues/closed/0102 と同じ判断)。

## 完了条件

- `ListCFNStacks` と `ListCfnStackSummaries` の `ListStacks` 呼び出しが、狭いインターフェースを受け取る関数に抽出されている。
- 2 つの経路それぞれで、複数ページの全呼び出しにおいて `StackStatusFilter` が期待するステータス集合で送られることを検証するテストが存在する。
- `StackStatusFilter` の設定を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。
- docs/issues/0122: 本 issue の実装中に発見した別バグ。`listCFNStacks` が削除されていない 4 状態を取りこぼす。

## 解決方法

`ListCFNStacks` と `ListCfnStackSummaries` の `ListStacks` 呼び出しを、docs/issues/closed/0102 と同じ方式で狭いインターフェースを受け取る内部関数に抽出し、`StackStatusFilter` の構築を検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/cfn.go)

`ListStacks` 1 メソッドだけを持つ `cfnListStacksClient` を定義した。SDK のページネータ `cloudformation.NewListStacksPaginator` が要求する `cloudformation.ListStacksAPIClient` と同じメソッドセットであるため、このインターフェース値をそのままページネータへ渡せる。

```go
type cfnListStacksClient interface {
	ListStacks(ctx context.Context, params *cloudformation.ListStacksInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error)
}
```

公開関数 `ListCFNStacks` / `ListCfnStackSummaries` はクライアント生成だけを担い、呼び出しロジックを `listCFNStacks(ctx, client)` / `listCfnStackSummaries(ctx, client)` に移した。関数の中身は移動のみで、`statusFilter` の列挙、ページ送り、変換処理には手を入れていない。公開関数のシグネチャは変えていないため、呼び出し元 (`internal/api/handlers_aws.go:233` と `internal/cli/cfn.go:87`) は無変更である。

### テスト (backend/internal/aws/cfn_test.go)

- `mockCfnListStacksClient` — 受け取った `*ListStacksInput` を呼び出し順に記録し、用意したページを 1 呼び出しにつき 1 ページ返す手書きモック。ページ数を超える呼び出しはエラーを返して検知する。
- `cfnListStacksPages()` — `NextToken` で連結した 2 ページ構成のレスポンス。ページ送りが起きる状況を作る。
- `assertCfnStackStatusFilterOnAllCalls()` — 2 回の全呼び出しで `StackStatusFilter` が期待集合と一致すること、および 2 回目の呼び出しに 1 ページ目の `NextToken` が引き継がれている (実際にページ送りが起きた) ことを検証する。フィルタは集合として意味を持つため `cmpopts.SortSlices` で順序非依存に比較する。
- `TestListStacksSendsStackStatusFilter` — 2 経路をテーブル駆動で検証する。期待値は SDK の定数ではなく AWS API のステータス文字列 (`"CREATE_COMPLETE"` 等) で書き、実装の列挙をそのまま写した循環的なテストにならないようにした。`listCFNStacks` は 18 種、`listCfnStackSummaries` は `DELETE_COMPLETE` のみを除く 22 種。

### 検出力の確認

両経路の `StackStatusFilter: statusFilter` / `StackStatusFilter: statusFilters` の行を一時的に削除した状態で `go test -run TestListStacks ./internal/aws/` を実行し、次の 2 件のサブテストがいずれも失敗することを実測した。

- `TestListStacksSendsStackStatusFilter/listCFNStacks_sends_the_18_status_web_api_set`
- `TestListStacksSendsStackStatusFilter/listCfnStackSummaries_sends_the_22_status_legacy_cli_set`

あわせて、ステータス集合から 1 種だけを落とすミューテーションと、ページ送りのループを 1 ページで打ち切るミューテーションでも失敗することを確認した。確認後に実装を元に戻し、`go test -count=30 -race -run TestListStacks ./internal/aws/` で 30 回連続の通過 (テストの決定性) も実測した。

### レビューで削除したテスト

当初は 2 経路のステータス集合の差集合を固定する `TestListStacksStatusFilterSetsDiffer` も追加していたが、issue の完了条件にも対応方針にも根拠が無く、かつ `TestListStacksSendsStackStatusFilter` が 2 経路の集合をそれぞれ独立に完全一致で固定しているため検出力の上乗せが無い。完了条件を超える変更にあたるため削除した。

### 実装中に発見した別バグ

`listCFNStacks` の 18 種は、コメントの宣言 (`// Exclude deleted stacks.`) に反して、削除されていない 4 状態 (`DELETE_FAILED` / `UPDATE_ROLLBACK_IN_PROGRESS` / `UPDATE_COMPLETE_CLEANUP_IN_PROGRESS` / `UPDATE_ROLLBACK_COMPLETE_CLEANUP_IN_PROGRESS`) を取りこぼしている。CloudFormation の `StackStatus` 23 種のうちスタックが存在しなくなるのは `DELETE_COMPLETE` だけであり、`listCfnStackSummaries` の 22 種はこれと一致する。本 issue は挙動を変えないテスト整備であるため修正せず、docs/issues/0122 として登録した。テストは現状の集合をそのまま固定し、`TestListStacksSendsStackStatusFilter` のコメントに 0122 への参照を残した。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。
