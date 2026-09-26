# SQS のキュー属性取得に渡す AttributeNames を検証するモックテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-08

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/sqs.go` の `ListSQSResources` (78-81 行) は `GetQueueAttributesInput` に `AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll}` を設定して呼び出す。
この設定が無いと SQS は属性を一切返さず、`sqsFromAttributes` が参照する QueueArn / FifoQueue / MessageRetentionPeriod / メッセージ数が全て静かにゼロ値やデフォルト (ID は URL フォールバック、Type は常に Standard、カウント類は 0) に落ちる。
ShowCacheNodeInfo (issue 0102) と同型のパターンである。
`sqs_test.go` の `TestSQSFromAttributes` / `TestFetchSQSQueueTags` は変換関数のテストのみで、`GetQueueAttributesInput` の構築は検証していない。

## 対応方針

- docs/issues/closed/0102 と同じ方式を採る。一覧経路用の狭いインターフェースを新たに定義し、既存の `sqsQueueTagsClient` を埋め込んだうえで `ListQueues` と `GetQueueAttributes` を加え、`ListSQSResources` の呼び出しロジックをそのインターフェースを受け取る内部関数に抽出する (一覧経路は `fetchSQSQueueTags` を呼ぶため 3 メソッド全てを要する)。
- 既存の `sqsQueueTagsClient` 自体にメソッドを加える案は採らない。タグ取得だけを必要とする `fetchSQSQueueTags` の要求まで広げることになり、受け取り側インターフェースは使うメソッドだけを持つという方針 (consumer-defined interfaces) に反するためである。
- 受け取った `GetQueueAttributesInput` を記録する手書きモックで、全キューの呼び出しで `AttributeNames` に `QueueAttributeNameAll` が設定されることを検証する。
- キューごとの詳細取得は errgroup で並列実行されるため、モックの記録はロックで保護するか、キュー数 1 のケースで検証する。

## 完了条件

- `ListSQSResources` の `GetQueueAttributes` 呼び出しが、狭いインターフェースを受け取る関数に抽出されている。
- `AttributeNames` に `QueueAttributeNameAll` が設定されることを検証するテストが存在する。
- `AttributeNames` の設定行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法

`ListSQSResources` の一覧取得ロジックを狭いインターフェースを受け取る内部関数へ抽出し、`GetQueueAttributesInput` の `AttributeNames` を検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/sqs.go)

対応方針のとおり、既存の `sqsQueueTagsClient` は変更せず、新たに `sqsQueueListClient` を定義した。定義順は `sqsQueueTagsClient` を先に置き、後方参照が生じないようにインターフェースをファイル上部にまとめてある (ecs.go と同型)。

```go
type sqsQueueListClient interface {
	sqs.ListQueuesAPIClient
	sqsQueueTagsClient
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}
```

`ListQueues` は自前で書き直さず、SDK が公開する `sqs.ListQueuesAPIClient` を埋め込む。ページネータ (`sqs.NewListQueuesPaginator`) が要求する型がこれであり、同じメソッドセットを重複して定義せずに済む。0118 で ECS に採ったのと同じ方針である。タグ取得は既存の `sqsQueueTagsClient` を埋め込む。一覧経路は `fetchSQSQueueTags` を呼ぶため 3 メソッドすべてを要する。

`ListSQSResources` はクライアントを生成して `listSQSResources` へ委譲する形にした。キュー URL の列挙、属性取得、タグ取得、errgroup による並列化はすべて内部関数側に置いてある。

`listSQSResources` は `profile` と `region` を引数で受け取る。これらはフェーズごとの所要時間ログ (`sqs list queues done` / `sqs get queue attributes done`) の属性にのみ使う。ログを呼び出し元へ移すには各フェーズの所要時間を戻り値で返す必要があり、フェーズごとに関数を分けると `ListQueues` の結果が `GetQueueAttributes` へ渡ることの検証がテストから抜ける。ログ出力の内容と順序を抽出前と完全に一致させることを優先した。抽出前後で 4 本のログ (`sqs client created` / `sqs list queues done` / `sqs get queue attributes done` / `sqs list all done`) の属性・順序・計測区間はいずれも変わっていない。

この 2 引数を `*slog.Logger` 1 個に置き換える案も検討したが採らなかった。AGENTS.md のロギング節が `With` での属性付与と引数渡しを挙げているのは HTTP のリクエストスコープ (`request_id` / `user_id`) についてであり、`internal/` 配下に `*slog.Logger` を引数で受け取る関数は 1 つも存在しない。この 1 関数だけに導入すると 15 ファイル規模のパッケージ内で単独の異物になるうえ、issue の対応方針にない範囲外の設計変更になる。

### テスト (backend/internal/aws/sqs_test.go)

`mockSQSQueueListClient` を追加した。キューごとの `GetQueueAttributes` と `ListQueueTags` は errgroup で並列に呼ばれるため、記録は mutex で保護する。対応方針が挙げた 2 案 (ロックで保護する / キュー数 1 で検証する) のうちロックを採ったのは、キュー数 1 では「両ページの URL が集約されて全キューについて属性取得が行われる」ことを確かめられないためである。

`TestListSQSResourcesSendsAttributeNamesAll` を追加した。`ListQueues` の結果を 2 ページに分け、両ページのキューについて `GetQueueAttributes` が呼ばれることを確かめる。呼び出し順は errgroup により不定のため、`g.Wait()` の後に `QueueUrl` 順へ整列してから比較する。整列と読み取りは全 goroutine の終了後に行うためロックは要らない。

`GetQueueAttributes` に加えて `ListQueues` と `ListQueueTags` の Input も検証する。`ListQueues` は 2 回目の呼び出しに `NextToken` が引き継がれること (ページネータは `Limit` 未指定のため `MaxResults` を設定しない) を固定する。`ListQueueTags` はキューごとに呼ばれることを固定する。タグ取得を丸ごと省いてもキュー情報は返るため、この検証が無いと一覧経路からのタグ取得の削除を検出できない。

期待する Input 全体を `cmp.Diff` で比較する。0117 から 0119 で採った方式に揃えたもので、`AttributeNames` に加えて `QueueUrl` の対応付けも固定でき、期待していないフィールドが新たに設定される変更も検出できる。スライス全体を比較しているため呼び出し回数も同時に固定される。SQS の各 Input は unexported フィールドを持つため `cmpopts.IgnoreUnexported` を渡している。

期待値の `AttributeNames` は実装の定数式を経由せず AWS API の値 (`"All"`) で直接書き下している。

### 検出力の確認

`AttributeNames` の設定の削除に加え、値の差し替え、`QueueUrl` の対応付け、ページ集約、ページネータへ渡す Input、タグ取得を 1 つずつ壊し、そのたびにテストが失敗することを実測した。このテストはサブテストを持たないため失敗の有無で示す。

| 壊した箇所 | 結果 |
| --- | --- |
| `AttributeNames` の設定行を削除 (未使用になる sqstypes の import も併せて削除) | 失敗する |
| `QueueAttributeNameAll` を `QueueAttributeNameQueueArn` に変更 | 失敗する |
| `GetQueueAttributesInput` の `QueueUrl` を nil に変更 | 失敗する |
| `urls` のページ集約を最後のページのみに変更 | 失敗する |
| `ListQueuesInput` に `MaxResults` を追加 | 失敗する |
| `fetchSQSQueueTags` の呼び出しを削除して空のタグに置換 | 失敗する |
| `fetchSQSQueueTags` へ渡すキュー URL を先頭キューに固定 | 失敗する |

`AttributeNames` の設定行だけを削除すると `sqstypes` の import が未使用になりビルドが通らず、ビルド失敗を「テストが失敗した」と数えることになる。import 行も併せて削除して測り直している。測定はビルド結果を判定してから失敗数を数え、行番号を指定しない場合はパターンがファイル内に 1 箇所しかないことを検査してから置換している。確認後、実装は元に戻し、`git diff` で復元を確認した。

### 今回のテストで検証していない範囲

`sqsQueueConcurrency` による同時実行数の上限そのものは検証していない。タグ取得が失敗したときの縮退 (`tagsFetchFailed`) と、キャンセルの伝播は既存の `TestFetchSQSQueueTags` が担当する。`GetQueueAttributes` が失敗した場合のエラー経路は、同種の issue 群 (0102 および 0112 から 0119) と同じく対象に含めていない。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。
