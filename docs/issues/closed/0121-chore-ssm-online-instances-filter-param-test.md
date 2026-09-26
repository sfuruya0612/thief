# SSM のオンラインインスタンス一覧に渡すフィルタを検証するモックテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-08

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/ssm.go` の `ListSSMOnlineInstanceIDs` (194-199 行) は `DescribeInstanceInformationInput.Filters` に `PingStatus=Online` と `ResourceType=EC2Instance` を設定して呼び出す。
この設定を削除してもコンパイルと API 呼び出しは成功するが、関数名とコメントが示す「Session Manager で接続可能な EC2 インスタンスのみ返す」という意味が壊れ、オフラインのインスタンスや EC2 以外のリソースタイプまで静かに混入する。
ssm.go に対応するテストファイル (ssm_test.go) は存在せず、この構築は何のテストにも守られていない。

## 対応方針

- docs/issues/closed/0102 と同じ方式を採る。`DescribeInstanceInformation` を持つ狭いインターフェースを定義し、`ListSSMOnlineInstanceIDs` の呼び出しロジックをインターフェースを受け取る内部関数に抽出する。
- 受け取った `DescribeInstanceInformationInput` を記録する手書きモックで、`Filters` に PingStatus=Online と ResourceType=EC2Instance の 2 つが設定されることを検証する。
- 呼び出しは `NewDescribeInstanceInformationPaginator` 経由のため、0102 と同様に複数ページのレスポンスでも全呼び出しでフィルタが維持されることを検証する。
- ssm.go の他の関数 (パラメータ取得系) まで同じインターフェースに乗せて一括で抽出する案は採らない。それらの Input は識別子のみで (0102 の棚卸しで対象なしと確認済み)、検証対象が無い箇所にモック配線を増やすだけになるためである。

## 完了条件

- `ListSSMOnlineInstanceIDs` の `DescribeInstanceInformation` 呼び出しが、狭いインターフェースを受け取る関数に抽出されている。
- `Filters` の 2 条件の設定を全呼び出しで検証するテストが存在する。
- `Filters` の設定を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法

`ListSSMOnlineInstanceIDs` の呼び出しロジックを狭いインターフェースを受け取る内部関数へ抽出し、`DescribeInstanceInformationInput` の `Filters` を検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/ssm.go)

`ListSSMOnlineInstanceIDs` はクライアントを生成して `listSSMOnlineInstanceIDs` へ委譲する形にした。Input の構築、ページネータのループ、インスタンス ID の集約はすべて内部関数側に置いてある。

内部関数が受け取る型は自前で定義せず、SDK が公開する `ssm.DescribeInstanceInformationAPIClient` をそのまま使った。

```go
func listSSMOnlineInstanceIDs(ctx context.Context, client ssm.DescribeInstanceInformationAPIClient) ([]string, error)
```

このインターフェースのメソッドは `DescribeInstanceInformation` 1 つのみで、`ssm.NewDescribeInstanceInformationPaginator` が要求する型でもある。自前で同じメソッドセットのインターフェースを定義すると重複になるため避けた。

対応方針は「狭いインターフェースを定義し」と書いているが、この点は 0118 の判断に従って読み替えた。0118 も対応方針では同じ表現を使っていたが、実装時に `ListClusters` 単体のインターフェースを自前定義することをレビューで指摘され、SDK が公開する `ecs.ListClustersAPIClient` を直接使う形に改めている。0120 (SQS) もこれを踏襲し、`sqs.ListQueuesAPIClient` を複合インターフェースに埋め込んでいる。0102 / 0112 / 0116 は独自型を定義しているが、いずれも 0118 より前の実装である。単一メソッドで足りるケースで SDK の型を使うのが現行の判断であり、0121 もそれに揃えた。

SQS (0120) は一覧経路が「URL の列挙 → キューごとの属性取得 → タグ取得」の 3 段構えで 3 メソッドの複合インターフェースを要したが、SSM は `DescribeInstanceInformation` だけで完結するため単一メソッドで足りる。

対応方針のとおり、`ssm.go` の他の関数 (パラメータ取得系) は変更していない。それらの Input は識別子のみで検証対象が無い。

### テスト (backend/internal/aws/ssm_test.go)

`ssm_test.go` を新規に追加した。

`mockSSMDescribeInstanceInformationClient` は受け取った Input を呼び出し順に記録し、用意したページを順に返す。用意したページ数を超えて呼ばれた場合は `errSSMNoMorePages` をラップしたエラーを返し、想定外の呼び出しが黙って成功しないようにしてある。呼び出しはページネータ経由で逐次行われるため、SQS のような並列呼び出しは無くロックは要らない。

`TestListSSMOnlineInstanceIDsSendsPingStatusAndResourceTypeFilters` を追加した。レスポンスを 2 ページに分け、ページ送り後の呼び出しでも `Filters` が維持されることを確かめる。1 ページ目には `InstanceId` が nil の要素を混ぜ、集約から落とされることも確認する。

期待する Input 全体を `cmp.Diff` で比較する。0117 から 0120 で採った方式に揃えたもので、`Filters` の内容と順序に加えて、期待していないフィールドが新たに設定される変更も検出できる。スライス全体を比較しているため呼び出し回数も同時に固定され、2 要素目の `NextToken` でページ送りが行われたことも固定される。ページネータは `Limit` 未指定のため `MaxResults` を設定しない。`ssm.DescribeInstanceInformationInput` と `ssmtypes.InstanceInformationStringFilter` は unexported フィールドを持つため `cmpopts.IgnoreUnexported` を渡している。

期待値のキーと値 (`"PingStatus"` / `"Online"` / `"ResourceType"` / `"EC2Instance"`) は実装の式を経由せず AWS API の値で直接書き下している。

`TestListSSMOnlineInstanceIDsPropagatesPageError` を追加した。途中のページ取得が失敗したときにエラーを握り潰さず、取得済みの部分的な結果も返さないことを検証する。同種の issue 群 (0102 および 0112 から 0120) はエラー経路を対象外にしてきたが、`ssm_test.go` はファイルごと新規で、`listSSMOnlineInstanceIDs` も今回新設した関数のため、そのエラー分岐が一度も実行されない状態を残さないことにした。モックには既に用意したページを使い切ったときのガードがあり、追加のモック配線は要らない。同パッケージには `pricing_test.go` の `TestFetchOnDemandAndReservedErrorAbortsWithoutPartialData` という先例もある。

このテストはエラーの文言そのものを固定せず、`errors.Is` で `%w` によるラップが維持されることだけを検証する。文言を固定すると、下記の issue 0123 で直す予定のラップ文言をテスト側で固着させてしまうためである。

### 検出力の確認

`Filters` の削除に加え、フィルタ 1 件ずつの削除、値とキー名の差し替え、ID の集約、エラー経路を 1 つずつ壊し、そのたびにテストが失敗することを実測した。

| 壊した箇所 | 結果 |
| --- | --- |
| `Filters` を丸ごと削除 | 失敗する |
| `PingStatus` のフィルタだけ削除 | 失敗する |
| `ResourceType` のフィルタだけ削除 | 失敗する |
| `PingStatus` の値を `Offline` に変更 | 失敗する |
| `ResourceType` の値を `ManagedInstance` に変更 | 失敗する |
| `PingStatus` のキー名を `PingStatuses` に変更 | 失敗する |
| `ids` の集約を無効化 | 失敗する |
| `ids` のページ集約を最後のページのみに変更 | 失敗する |
| エラーのラップを `%w` から `%v` に変更 | 失敗する |
| エラー時に取得済みの `ids` を返すよう変更 | 失敗する |
| `NextPage` のエラーを握り潰してループを抜けるよう変更 | 失敗する |

`Filters` の削除では `aws` と `ssmtypes` の import が `ssm.go` の他の関数でも使われているため未使用にならず、ビルド失敗をテスト失敗と取り違える余地は無い (測定はビルド結果を判定してから失敗数を数えている)。行番号を指定しない場合はパターンがファイル内に 1 箇所しかないことを検査してから置換している。確認後、実装は元に戻し、`git diff` で復元を確認した。

### 今回のテストで検証していない範囲

- context のキャンセル伝播。受け取った `ctx` をそのまま `NextPage` へ渡すだけである。
- `InstanceInformationList` が空または nil の 0 件ページ。
- 公開関数 `ListSSMOnlineInstanceIDs` 自体 (資格情報の解決失敗など)。コアのみをモックで分離して検証する設計は意図的なものである。
- ページネータが同一の `NextToken` を返し続けた場合の無限ループ。`ssm.NewDescribeInstanceInformationPaginator` は `StopOnDuplicateToken` を既定の false のまま使っている。これはリポジトリ内のページネータ呼び出し 30 箇所超すべてに共通する既存パターンであり、今回の抽出で新たに持ち込まれたものではないため、この issue の範囲外とした。

### レビューで見つかり別 issue にしたもの

`listSSMOnlineInstanceIDs` のエラーラップ文言 `failed to get next page` が、対象リソース名を含まない `internal/` 唯一の例である点をレビューで指摘された。この行は抽出前から無変更であり、書き換えると「挙動は変えていない」という本 issue の前提が崩れるため、ここでは直さず issue 0123 として起票した。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。
