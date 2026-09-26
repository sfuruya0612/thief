# EC2 のインスタンス一覧に渡す絞り込みフィルタを検証するモックテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/ec2.go` の `ListEC2Instances` (118-130 行) は、`EC2ListOptions.Running` が真のとき `instance-state-name=running` フィルタを、`InstanceIDs` が非空のとき `instance-id` フィルタを `DescribeInstancesInput.Filters` に追加する。
この構築を削除してもコンパイルと API 呼び出しは成功するが、running / instance-id の絞り込みが静かに効かなくなり全インスタンスが返る。
`ec2_test.go` は変換関数 `ec2InstanceInfoFromSDK` のテストのみで、`Filters` の構築は検証していない。

## 対応方針

- docs/issues/closed/0102 と同じ方式を採る。`DescribeInstances` を持つ狭いインターフェースを定義し、`ListEC2Instances` の呼び出しロジックをインターフェースを受け取る内部関数に抽出する。
- 受け取った `DescribeInstancesInput` を記録する手書きモックで、オプションの組み合わせ (両方なし / Running のみ / InstanceIDs のみ / 両方) ごとに `Filters` の Name と Values を検証する。
- ページネータ経由の呼び出しであるため、0102 と同様に複数ページのレスポンスでも全呼び出しでフィルタが維持されることを検証する。
- 組み合わせの検証はテーブル駆動テスト 1 本にまとめる。組み合わせごとに別のテスト関数を作る案は、モックの準備と検証の構造が同じで重複になるため採らない。
- `ListEC2Resources` も同じインターフェースに乗せて一緒に抽出する案は採らない。同関数は Input にフィールドを設定せず (terminated 除外はレスポンス側の判定)、検証対象が無い箇所にモック配線を増やすだけになるためである。

## 完了条件

- `ListEC2Instances` の `DescribeInstances` 呼び出しが、狭いインターフェースを受け取る関数に抽出されている。
- オプションの組み合わせごとに `Filters` の構築を検証するテストが存在し、複数ページの全呼び出しでフィルタが維持されることも検証する。
- `Filters` の構築を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法

`ListEC2Instances` の `DescribeInstances` 呼び出しを狭いインターフェースを受け取る内部関数へ抽出し、`EC2ListOptions` の組み合わせごとに `Filters` の構築を検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/ec2.go)

`ec2DescribeInstancesClient` インターフェース (`DescribeInstances` 1 メソッド) を新設した。docs/issues/closed/0102 の `elastiCacheDescribeClustersClient` と同じ形で、`ec2.NewDescribeInstancesPaginator` が要求する `ec2.DescribeInstancesAPIClient` と同じメソッドセットである。

`ListEC2Instances` はクライアントを生成して `listEC2Instances` へ委譲する形にし、`Filters` の構築とページングはすべて `listEC2Instances` に置いた。

対応方針のとおり `ListEC2Resources` は同じインターフェースに乗せていない。同関数は Input にフィールドを設定せず (terminated の除外はレスポンス側の判定)、検証対象が無い箇所にモック配線を増やすだけになるためである。

### テスト (backend/internal/aws/ec2_test.go)

既存の ec2_test.go に追記した。

- `mockEC2DescribeInstancesClient` — 受け取った `DescribeInstancesInput` を呼び出し順に記録する手書きモック。`pages` に用意したレスポンスを 1 呼び出しにつき 1 ページ返し、用意した数を超える呼び出しはエラーにする。SDK のページネータは呼び出しごとに Input を値でコピーして `NextToken` だけ差し替えるため、記録した後に内容が書き換わることはなく、ポインタのまま記録している。
- `ec2DescribeInstancesPages` — `NextToken` で連結した 2 ページ構成のレスポンス。
- `TestListEC2InstancesSendsFilters` — 対応方針のとおり、オプションの組み合わせ (両方なし / Running のみ / InstanceIDs のみ / 両方) をテーブル駆動の 1 本にまとめ、各ケースで 2 回の呼び出しすべての `Filters` を検証する。オプション未指定時に `Filters` が nil のままであることも固定する。

`Filters` の期待値は実装の呼び出しを通さず AWS API の文字列 (`instance-state-name` / `instance-id`) で直接書き下し、オプションとフィルタ名の対応そのものを固定している。フィルタの値もケースごとに変え、オプションを無視して固定値を送る実装に変えると失敗するようにした。`InstanceIDs` のケースは ID を 2 件指定する。1 件だけにすると、先頭の 1 件しか `Values` に載せない実装に変えても検出できない。

1 回目の呼び出しに `NextToken` が無く、2 回目に 1 ページ目の `NextToken` が引き継がれていることも確かめる。2 回目が同一ページの再取得ではなく実際のページ送りであることを保証し、「複数ページの全呼び出しでフィルタが維持される」という完了条件の検証が空回りしないようにするためである。

返り値についても、両ページのインスタンスが集約されること (`i-page1` と `i-page2` がこの順で並ぶこと) を確かめる。先例の docs/issues/closed/0102 の `TestListElastiCacheSetsShowCacheNodeInfo` が同じ 2 ページ構成で集約を検証しており、それに揃えた。実装時に一度この検証を「レスポンス側の検証でスコープ外」として外したが、レビューの指摘で戻した。ページ送りが起きたことを `NextToken` で固定しても、集約側が壊れていること (最後のページだけを残す、`Reservations` の入れ子ループが崩れる) は検出できないためである。

### 検出力の確認

完了条件が指定する `Filters` の構築の削除に加え、フィルタ名、フィルタ値、条件判定、フィルタの並び順、ページネータへ渡す Input、ページの集約を 1 つずつ壊し、そのたびにテストが失敗することを実測した (全 4 サブテスト中の失敗数)。

| 壊した箇所 | 失敗したサブテスト数 |
| --- | --- |
| running フィルタの `append` を削除 | 2 |
| instance-id フィルタの `append` を削除 | 2 |
| `instance-state-name` を `instance-id` に変更 | 2 |
| `instance-id` を `instance-state-name` に変更 | 2 |
| `running` を `stopped` に変更 | 2 |
| `Values: opts.InstanceIDs` を `opts.InstanceIDs[:1]` に変更 | 1 |
| ページネータに `input` ではなく空の Input を渡す | 3 |
| instance-id フィルタを先頭に差し込む (並び順の入れ替え) | 1 |
| `if opts.Running` の条件を反転 | 4 |
| `if len(opts.InstanceIDs) > 0` の条件を反転 | 4 |
| ページごとに `instances` を初期化する (最後のページだけを残す) | 4 |

確認後、実装は元に戻し、`diff` で復元を確認した。

### 今回のテストで検証していない範囲

モックのレスポンスは `InstanceId` だけを持つため、`ec2InstanceInfoFromSDK` による他フィールドの変換は通らない (既存の `TestEc2InstanceInfoFromSDK` が担当する)。`DescribeInstances` が失敗した場合のエラー経路も検証していない。

いずれも本 issue の完了条件が対象とする `Filters` の構築の検証ではないためスコープ外とした。エラー経路の未検証はレビューで指摘されたが、cfn_test.go / elasticache_test.go を含む同種のテスト群すべてに共通する範囲であり、この issue だけで方針を変えると一連の issue 群の中で浮くため今回は対象に含めない。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。
