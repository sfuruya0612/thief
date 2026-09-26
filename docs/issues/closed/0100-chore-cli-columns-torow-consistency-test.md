# internal/cli の Columns と ToRow() の順序対応を検証するテストを整備する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「internal/cli/*.go の Columns と torow.go の ToRow() の順序対応を検証するテストを整備したい (issue 0092 のスコープ外として送り)」に対応する。

CLI のテーブル表示は、各コマンドが定義する列 (`[]util.Column`) と、リソース型の `ToRow()` が返す値のスライスを位置で対応させている。
例えば `backend/internal/cli/kinesis.go` の `newKinesisCmd` (17-23 行) は関数内のインラインリテラルで 6 列を定義し、`backend/internal/aws/torow.go` の `KinesisResource.ToRow()` が返す 6 要素と暗黙に対応する。
列定義は 15 ファイル (bq / cfn / cloudwatchlogs / datadog / ec2 / ecr / ecs / elasticache / iam / rds / s3 / secretsmanager / ssm / sso / tidb) では既にパッケージレベル変数に抽出済みで、関数内のインラインリテラルのまま残るのは cloudfront / cost / elb / kinesis / lambda の 5 ファイルである。
非 AWS の `gcp.go` にも関数内の列定義が 7 箇所あるが、`ToRow()` のパターンを使わないため、この集計と本 issue の抽出対象には含めない (対応方針を参照)。
両者を突き合わせる場所は `backend/internal/cli/helper.go` の `toRows` (76-83 行) だが、ここは `ToRow()` を呼ぶだけで、列数や順序の対応は検証しない。

このため、片側だけに列を追加したり順序を入れ替えたりしても、コンパイルもテストも通ったまま、CLI の表示で値がずれる。
docs/issues/closed/0092 は Kinesis の列定義と `ToRow()` の双方に Mode の要素を追加しており、この種の変更で順序対応を検証する自動テストは無い。
TODO.md はこの項目を 0092 のスコープ外として記録している。

## 対応方針

- インラインリテラルのまま残る 5 ファイル (cloudfront / cost / elb / kinesis / lambda) の `[]util.Column` を、他の 15 ファイルと同じパッケージレベルの変数 (例: `kinesisColumns`) に抽出する。関数内リテラルのままではテストから参照できない。抽出は定義位置の移動のみで、列の内容と順序は変えない。
- `internal/cli` にテーブル駆動テストを追加し、リソース型ごとに「全フィールドに相異なる値を入れたフィクスチャの `ToRow()` の結果」と「列変数のヘッダ」を並べ、要素数の一致と、各値が意図した列位置にあることを検証する。
- テスト対象の型は、`ToRow()` が CLI から実際に呼ばれる次の 3 経路で機械的に洗い出す。
  1. `ListConfig[T]` と `runList` を経由する呼び出し。型引数は `grep -rn 'ListConfig\[' backend/internal/cli` で列挙する。`backend/internal/aws/torow.go` の 18 メソッドのうちこの経路で到達するのは 7 つ (`LambdaResource` / `SSOAccountResource` / `KinesisResource` / `CloudFrontResource` / `ELBResource` / `CostResource` / `ForecastResource`) で、各サービスの Info / Summary 系の型もこの経路で到達する。
  2. `helper.go` 以外での `toRows` の直接呼び出し。`grep -rn 'toRows(' backend/internal/cli` で列挙する。`ec2.go` (113 行) の `EC2InstanceInfo`、`cfn.go` (123 / 130 / 137 / 164 行) の `CfnParameter` / `CfnOutput` / `CfnTag` / `CfnChangeDetail`、`ecs.go` (141 / 191 行) の `ECSServiceInfo` / `ECSTaskInfo` が該当する。これらの列定義 (`ec2Columns` / `cfnParameterColumns` / `ecsServiceColumns` 等) は既にパッケージレベル変数である。
  3. `ToRow()` の単発呼び出し。`grep -rn '.ToRow()' backend/internal/cli` で列挙する。`ssm.go` の `SSMParameterValue` が該当する。
- 対象は `internal/aws` パッケージの型に限る。bq / datadog / gcp / tidb の非 AWS コマンドは `ToRow()` / `util.Row` のパターンを使わず `printRowsOrGroupBy` に直接値を渡す構造のため対象外とする。
- torow.go の呼び出し元の無い 11 メソッドは docs/issues/0101 が削除するため、本 issue のテスト対象に含めると 0101 の実施でテストが壊れる。テスト対象に含めない。

## 完了条件

- `internal/cli` の各コマンドの列定義がパッケージレベル変数になり、`newXxxCmd` はそれを参照する。列の内容と順序は変更しない。
- 対応方針の 3 経路 (`ListConfig[` のインスタンス化の型引数、`helper.go` 以外での `toRows` の直接呼び出し、`ToRow()` の単発呼び出し) を grep で列挙した `internal/aws` パッケージの全対象型について、`ToRow()` の要素数が列変数の要素数と一致し、フィクスチャの各値が対応する列位置に現れることを検証するテストが存在する。列挙に使ったコマンドと得られた型の一覧を、テストコードのコメントまたは本 issue に記録する。
- 非 AWS コマンド (bq / datadog / gcp / tidb) と、torow.go の到達不能な 11 メソッド (docs/issues/0101 の削除対象) はテスト対象に含めない。
- `mise run check` が通る。

## 実装詳細の乖離

対応方針の「全フィールドに相異なる値を入れたフィクスチャ」を、「`ToRow()` が参照する全フィールドに相異なる値を入れたフィクスチャ」として実装した。
`ToRow()` が参照しないフィールド (例: `LogGroupInfo.ARN`) は行に現れず、値を入れても列位置の検証に影響しないためである。
フィクスチャの各値を対応する列位置と突き合わせる方式そのものは変えていない。

## 関連

- docs/issues/closed/0092: 本 issue の起票元。TODO.md がこの項目を 0092 のスコープ外として記録している。
- docs/issues/0101: 到達不能な `ToRow()` の削除。本 issue はテスト対象を到達可能な型に限定することで、0101 と実施順序に依存せず独立に close できる。

## 解決方法

- cloudfront / cost / elb / kinesis / lambda の 5 ファイルで関数内のインラインリテラルだった列定義を、他のファイルと同じパッケージレベル変数 (`cloudfrontColumns` / `costColumns` / `forecastColumns` / `elbColumns` / `kinesisColumns` / `lambdaColumns`) に抽出した。列の内容と順序は変えていない。
- `backend/internal/cli/columns_torow_test.go` を追加し、対応方針の 3 経路の grep で列挙した 29 型 (runList 経由 21 型、`toRows` 直接呼び出し 7 型、`ToRow()` 単発呼び出し 1 型) について、`ToRow()` の要素数と列変数の要素数の一致、およびフィクスチャの各値が対応する列位置に現れることを検証するテーブル駆動テストを整備した。列挙コマンドと型一覧はテスト冒頭のコメントに記録した。
- フィクスチャは「実装詳細の乖離」のとおり、`ToRow()` が参照する全フィールドに相異なる値を入れる形とした。
- 非 AWS コマンド (bq / datadog / gcp / tidb) と torow.go の到達不能な 11 メソッドはテスト対象に含めていない。
- `mise run check` の通過を確認した。
