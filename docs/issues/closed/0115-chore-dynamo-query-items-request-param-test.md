# DynamoDB のアイテム検索に渡す Limit とフィルタ式を検証するモックテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/dynamo.go` の `QueryDynamoItems` は 2 経路で Input を構築する。

- キー未指定の経路 (299 行付近): `ScanInput` に `Limit` と、属性フィルタ指定時の `FilterExpression` / `ExpressionAttributeNames` / `ExpressionAttributeValues` を設定する。
- キー指定の経路 (337 行付近): `QueryInput` に `KeyConditionExpression` / `Limit` と、属性フィルタ指定時の `FilterExpression` とフィルタ由来の名前と値のマージを設定する。

`Limit` を削除してもコンパイルと API 呼び出しは成功するが、AWS 側のデフォルトページサイズで返るようになり「Limit 件のみ返す」という UI の前提が静かに崩れる。
属性フィルタ由来の `FilterExpression` 系を削除すると絞り込みが効かず全件が返る。
`dynamo_test.go` にあるのは `dynamoAttrFilterExpression` / `resolveDynamoItemLimit` などの値組み立てロジックの単体テストのみで、組み立てた値が実際に `ScanInput` / `QueryInput` に代入されることは検証していない。

## 対応方針

- docs/issues/closed/0102 と同じ方式を採る。`Scan` / `Query` / `DescribeTable` を持つ狭いインターフェースを定義し、`QueryDynamoItems` の呼び出しロジックをインターフェースを受け取る内部関数に抽出する (`describeDynamoTableWith` は既に `*dynamodb.Client` を引数に取る形なので、同じインターフェースに揃える)。
- 受け取った `ScanInput` / `QueryInput` を記録する手書きモックで、2 経路それぞれについて `Limit` と、フィルタ指定時の `FilterExpression` / 名前 / 値の設定を検証する。フィルタ未指定時に `FilterExpression` が nil であることも検証する。
- Scan 用と Query 用でインターフェースを分ける案は、`QueryDynamoItems` の 1 関数が引数に応じて両方を呼び分けるため採らず、1 つのインターフェースにまとめる。
- `dynamoAttrFilterExpression` 等の値組み立てロジックの単体テストを拡充するだけで済ませる案は、組み立てた値が実際に `ScanInput` / `QueryInput` に代入される配線 (背景に書いた既存テストの不足そのもの) を検証できないため採らない。

## 完了条件

- `QueryDynamoItems` の Scan / Query 呼び出しが、狭いインターフェースを受け取る関数に抽出されている。
- Scan 経路と Query 経路の両方で、`Limit` とフィルタ式関連フィールドの設定 (指定時に設定され、未指定時に nil であること) を検証するテストが存在する。
- `Limit` またはフィルタ式の設定行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法

`QueryDynamoItems` の Scan / Query 呼び出しを狭いインターフェースを受け取る内部関数へ抽出し、2 経路それぞれが `ScanInput` / `QueryInput` に何を載せるかを検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/dynamo.go)

`dynamoItemQueryClient` インターフェース (`Scan` / `Query` / `DescribeTable`) を新設した。対応方針のとおり Scan 用と Query 用に分けず 1 つにまとめている。`QueryDynamoItems` の 1 関数が `req.PKValue` の有無で両者を呼び分けるため、分けると呼び出し側が 2 つのインターフェースを同時に要求することになる。

`QueryDynamoItems` はクライアントを生成して `queryDynamoItems` へ委譲する形にし、Input の組み立てと呼び分けはすべて `queryDynamoItems` に置いた。`describeDynamoTableWith` の第 2 引数も `*dynamodb.Client` から同じインターフェースへ変えた (対応方針の指示どおり)。`DescribeDynamoTable` からの呼び出しは `*dynamodb.Client` がインターフェースを満たすためそのまま通る。

### テスト (backend/internal/aws/dynamo_test.go)

既存の dynamo_test.go に追記した。

- `mockDynamoItemQueryClient` — 受け取った `ScanInput` / `QueryInput` を呼び出し順に記録する手書きモック。`queryDynamoItems` は Scan か Query のどちらかを 1 回だけ呼んで返り、ページングで Input を使い回さないため、ポインタのまま記録している。`DescribeTable` は問い合わせられたテーブル名を記録したうえで、`pkName` / `skName` フィールドから組み立てたキースキーマを返し、`skName` が空のときはソートキーを持たないテーブルを表す。
- `TestQueryDynamoItemsScanInput` — PK 未指定の経路について、`TableName`、`Limit`、フィルタ式関連の 3 フィールドを検証する。フィルタ未指定時に 3 フィールドが nil のままであることも固定する。
- `TestQueryDynamoItemsQueryInput` — PK 指定の経路について、上記に加えて `KeyConditionExpression` と、キー条件由来とフィルタ由来をマージした後の名前と値のマップを検証する。

どちらのテストも、使われなかった側の API が呼ばれていないこと (Scan 経路で `Query` と `DescribeTable` が 0 回、Query 経路で `Scan` が 0 回) を確かめる。PK 指定時に Scan を使わないことは、実装のコメントが挙げるコスト/負荷の前提そのものである。Query 経路では `DescribeTable` に渡すテーブル名が検索対象と一致することも固定する。

`Limit` は条件ごとに別の値 (7 / 23 / 31 / 42 / 5 / 88) を渡す。全ケースで同じ値にすると、引数を無視してその値を固定で送る実装に変えてもテストが通ってしまうためである。両経路に `Limit` 未指定 (0) のケースを 1 つずつ置き、`resolveDynamoItemLimit` を通さず `req.Limit` をそのまま載せる実装に変えると失敗するようにした。

「PK と SK」のケースはパーティションキーを S、ソートキーを N とし、キー属性の型を分けている。両方を S にすると、ソートキーの値の変換に `SortKey.Type` ではなく `PartitionKey.Type` を渡す取り違えが検出できない。

### 検出力の確認

完了条件が指定する `Limit` とフィルタ式の設定行の削除に加え、経路の呼び分けと `Limit` の解決を 1 つずつ壊し、そのたびにテストが失敗することを実測した。

| 壊した箇所 | 失敗したサブテスト数 |
| --- | --- |
| Scan の `Limit: aws.Int32(limit)` を削除 | 3 |
| Scan の `input.FilterExpression = aws.String(filterExpr)` を削除 | 1 |
| Scan の `input.ExpressionAttributeNames = filterNames` を削除 | 1 |
| Scan の `input.ExpressionAttributeValues = filterValues` を削除 | 1 |
| Query の `Limit: aws.Int32(limit)` を削除 | 5 |
| Query の `input.FilterExpression = aws.String(filterExpr)` を削除 | 1 |
| Query のフィルタ由来の名前のマージを削除 | 1 |
| Query のフィルタ由来の値のマージを削除 | 1 |
| Scan の `limit` を `req.Limit` に変更 (既定値の解決を迂回) | 1 |
| Query の `limit` を `req.Limit` に変更 (同上) | 1 |
| Query の SK 条件の追加を削除 | 1 |
| Query の `schema.Table.SortKey != nil` 判定を削除 | 1 |
| Query の SK の値の変換に `PartitionKey.Type` を渡す (型の取り違え) | 1 |
| `describeDynamoTableWith` に検索対象と違うテーブル名を渡す | 5 |
| `if req.PKValue == ""` を `if false` に変更 (常に Query) | 3 |
| `if req.PKValue == ""` を `if true` に変更 (常に Scan) | 5 |

「Query の `limit` を `req.Limit` に変更」は、Query 経路に `Limit` 未指定のケースを足す前は 0 件だった。実測してから当該ケースを追加している。表の 13 行目 (SK の値の変換の型の取り違え) と 14 行目 (`describeDynamoTableWith` へ渡すテーブル名) の 2 つはレビューの指摘を受けて追加した検査で、指摘前はいずれも 0 件だった。確認後、実装は元に戻し、`diff` で復元を確認した。

### 今回のテストで検証していない範囲

モックは空のレスポンスを返すため、`dynamoUnmarshalItems` による Item の変換は通らない (既存の `TestDynamoUnmarshalItems` が担当する)。`DescribeTable` が失敗した場合とパーティションキー名が空だった場合のエラー経路も検証していない。

いずれも本 issue の完了条件が対象とする `Limit` とフィルタ式関連フィールドの設定の検証ではないためスコープ外とした。

### レビューで採らなかった指摘

CHANGES.md の文末を、0111 から 0113 の定型句「(テストの追加のみで挙動は変えない)」に揃える指摘があったが採らない。本 issue はインターフェースの新設と関数の抽出という実装の変更を伴うため、「テストの追加のみ」は事実に反する。同じ理由で 0114 のレビューでも短縮形へ直しており、そちらに揃えて「(挙動は変えない)」とする。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。
