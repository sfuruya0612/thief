# ECR のイメージ一覧に渡すタグ絞り込みとページサイズを検証するモックテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/ecr.go` の `ListECRImageInfos` (206-209 行) は、all 引数に応じて `DescribeImagesInput` に次を設定する。

- all=true: `MaxResults = ecrImagesPageSizeAll` (1000)
- all=false: `MaxResults = ecrImagesPageSizeTagged` (30) と `Filter: &ecrtypes.DescribeImagesFilter{TagStatus: ecrtypes.TagStatusTagged}`

`Filter` を削除しても all=false の呼び出しは成功するが、「タグ付きイメージのみ表示」という絞り込みが静かに壊れ、タグなしイメージが混在するようになる。
`MaxResults` を削除すると AWS 既定のページサイズになり、意図した件数制御が変わる。
`ecr_test.go` は変換関数 `ecrImageFromDetail` のテストのみで、Input の構築は検証していない。

## 対応方針

- docs/issues/closed/0102 と同じ方式を採る。`DescribeImages` を持つ狭いインターフェースを定義し、`ListECRImageInfos` の呼び出しロジックをインターフェースを受け取る内部関数に抽出する。
- 受け取った `DescribeImagesInput` を記録する手書きモックで、all=true / all=false の両分岐について `MaxResults` と `Filter` (all=true のとき nil、all=false のとき TagStatusTagged) を検証する。
- all=true の経路は `NextToken` による自前ループで継続するため、複数ページのレスポンスで `MaxResults` が全呼び出しで維持されることを検証する。all=false の経路は 1 ページで打ち切る仕様のため、単一呼び出しで検証する。
- 抽出と同時に自前ループを SDK のページネータ (`NewDescribeImagesPaginator`) へ置き換える案は、挙動を変えないテスト整備の issue に動作変更の可能性を持ち込むため採らない。自前ループのまま抽出する。
- all=true / all=false で別のモック型を作る案は、記録する内容が同じで重複になるため採らない (docs/issues/closed/0102 と同じ判断)。

## 完了条件

- `ListECRImageInfos` の `DescribeImages` 呼び出しが、狭いインターフェースを受け取る関数に抽出されている。
- all=true / all=false の両分岐で `MaxResults` と `Filter` の設定を検証するテストが存在する (all=true は複数ページの全呼び出しで `MaxResults` が維持されることを含む)。
- `Filter` または `MaxResults` の設定行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法」

## 解決方法

`ListECRImageInfos` の `DescribeImages` 呼び出しを狭いインターフェースを受け取る内部関数へ抽出し、all の分岐ごとに `MaxResults` と `Filter` の構築を検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/ecr.go)

`ecrDescribeImagesClient` インターフェース (`DescribeImages` 1 メソッド) を新設した。docs/issues/closed/0102 の `elastiCacheDescribeClustersClient` と同じ形である。

`ListECRImageInfos` はクライアントを生成して `listECRImageInfos` へ委譲する形にし、Input の構築、`NextToken` の自前ループ、`PushedAt` 降順のソートはすべて `listECRImageInfos` に置いた。

対応方針のとおり、自前ループを `ecr.NewDescribeImagesPaginator` へ置き換えることはしていない。挙動を変えないテスト整備の issue に動作変更の可能性を持ち込まないためである。

インターフェースの godoc は 2 行構成とし、`ec2DescribeInstancesClient` や `cfnListStacksClient` が持つ「ページネータが要求する〜と同じメソッドセットである」の 1 行は書いていない。レビューでこの 1 行の欠落を指摘されたが、この 1 行はページネータを使う抽出にのみ付いており、自前ループで抽出した `cwLogsFilterEventsClient` (0113) は同じく 2 行構成である。ページネータの有無で書き分ける既存の形に従っている。

### テスト (backend/internal/aws/ecr_test.go)

既存の ecr_test.go に追記した。

- `mockECRDescribeImagesClient` — 受け取った `DescribeImagesInput` を呼び出し順に記録する手書きモック。`pages` に用意したレスポンスを 1 呼び出しにつき 1 ページ返し、用意した数を超える呼び出しはエラーにする。`listECRImageInfos` の自前ループはページごとに Input を新しく確保するため、記録した後に内容が書き換わることはなく、ポインタのまま記録している。
- `ecrDescribeImagesPages` — `NextToken` で連結した 2 ページ構成のレスポンス。1 ページ目の `ImagePushedAt` を 2 ページ目より古くしてあるため、両ページを集約して `PushedAt` 降順に並べると 2 ページ目のイメージが先に来る。
- `TestListECRImageInfosSendsFilterAndMaxResults` — 対応方針のとおり、all=true / all=false をテーブル駆動の 1 本にまとめる。

検証は対象フィールドを個別に取り出さず、期待する `DescribeImagesInput` 全体を呼び出しごとに `cmp.Diff` で比較する形にした。0102 および 0112 から 0116 までの同種のテストは Input から対象フィールドだけを取り出して比較しており、この issue で意図的に方式を変えている。理由は次の 3 点である。

- `MaxResults` と `Filter` に加えて `RepositoryName` と `NextToken` の引き継ぎを 1 つの比較で固定できる。
- 「期待していないフィールドが新たに設定される」変更 (`RegistryId` や `ImageIds` の混入) を検出できる。個別フィールドの比較では検出できない。
- `ecr.DescribeImagesInput` は unexported フィールドを持つため、どのみち `cmpopts.IgnoreUnexported` が必要になる (無いと `cmp.Diff` が panic する)。全体比較にしても記述量は増えない。

既に close した 0102 と 0112 から 0116 を全体比較へ書き換えることはしていない。動いているテストを検出力の向上を伴わずに触ることになるためである。以降の同種の issue でどちらに揃えるかは、残りの 0118 から 0121 を実装する中で判断する。

期待値の要素数が期待する呼び出し回数を兼ねており、all=true は 2 回、all=false は 1 回で固定される。

期待値は実装の定数 (`ecrImagesPageSizeAll` / `ecrImagesPageSizeTagged`) を参照せず、件数 (1000 / 30) と `TagStatus` を直接書き下している。定数を参照すると、定数の値を変えたときにテストも一緒に動いてしまい検出できない。

取得したページのイメージが集約されることも `ImageDigest` の並びで確認する。all=false で 2 ページ目のイメージが混ざっていないこと (先頭ページで打ち切る仕様) と、all=true で 1 ページ目が捨てられていないことをここで検出する。

### 検出力の確認

完了条件が指定する `Filter` / `MaxResults` の設定行の削除に加え、`TagStatus`、ページサイズの定数、条件判定、`RepositoryName`、`NextToken` の引き継ぎ、打ち切り条件、ページの集約を 1 つずつ壊し、そのたびにテストが失敗することを実測した (全 2 サブテスト中の失敗数)。

| 壊した箇所 | 失敗したサブテスト数 |
| --- | --- |
| `input.Filter` の設定行を削除 | 1 |
| all=true の `input.MaxResults` の設定行を削除 | 1 |
| all=false の `input.MaxResults` の設定行を削除 | 1 |
| `TagStatusTagged` を `TagStatusUntagged` に変更 | 1 |
| `ecrImagesPageSizeAll` を 1000 から 100 に変更 | 1 |
| `ecrImagesPageSizeTagged` を 30 から 50 に変更 | 1 |
| `if all` の条件を反転 | 2 |
| `RepositoryName` を引数ではなく固定値に変更 | 2 |
| `nextToken = o.NextToken` を `nextToken = nil` に変更 (引き継がない) | 1 |
| `if !all \|\| o.NextToken == nil` から `!all` を削除 (all=false で打ち切らない) | 1 |
| ページごとに `images` を初期化する (最後のページだけを残す) | 1 |

`NextToken` については、Input の `nextToken` を `nil` に固定するミューテーションも試したが、`nextToken` 変数が読まれなくなりコンパイルが通らないため、代わりに引き継ぎ元の代入を潰す形で測定した。ビルド失敗を「失敗 0 件」と誤って数えないよう、測定はビルド結果を判定してから失敗数を数えている。確認後、実装は元に戻し、`git diff` で復元を確認した。

### 今回のテストで検証していない範囲

モックのレスポンスは `ImageDigest` と `ImagePushedAt` だけを持つため、`ImageTags` の連結や `ImageSizeInBytes` の文字列化といった変換は通らない (既存の `TestEcrImageFromDetail` が変換関数側を担当する)。`DescribeImages` が失敗した場合のエラー経路も検証していない。

いずれも本 issue の完了条件が対象とする `MaxResults` と `Filter` の構築の検証ではないためスコープ外とした。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。
