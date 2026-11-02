# Athena と CloudWatch Logs Insights でアクセスログを集計する

Created: 2026-11-02
Model: DeepSeek V4.1 Flash 1017

## 背景

ADR 0037 は、S3 のアクセスログを Athena、CloudWatch Logs のアクセスログを Logs Insights で集計することを決めた。
既存の `backend/internal/aws/athena.go` はクエリの開始、状態、結果、停止を持つが、`ExecutionParameters` に対応していない。
`backend/internal/aws/cloudwatchlogs.go` は `FilterLogEvents` と `StartLiveTail` を持ち、Logs Insights の `StartQuery`、`GetQueryResults`、`StopQuery` を持たない。
どちらのエンジンも、期間の指定、ソースの絞り込み、結果の正規化、実行の取り消しを共通の形で扱う必要がある。

## 目的

次の取得層を追加し、Access Logs の API から共通の形で集計を実行できるようにする。

- Athena の `ExecutionParameters`、テーブルメタデータの取得、ワークグループの取得
- CloudWatch Logs Insights の開始、状態と結果の取得、停止
- アクセスログの SQL と Logs Insights のクエリを実行し、`accesslog` の集計行に変換するアダプタ
- Athena のテーブルの検証と、スキーマプロファイルの判定

## 設計判断

- `StartAthenaQueryInput` に `ExecutionParameters []string` を追加し、`StartQueryExecutionInput` に渡す。
  既存の呼び出し元は空のスライスを渡すため、互換性は保たれる。
- `athenaAPI` に `GetTableMetadata` と `GetWorkGroup` を追加する。
  `GetTableMetadata` は列とパーティションキーを返す。
  `GetWorkGroup` は `BytesScannedCutoffPerQuery` と結果の出力先を確認するために使う。
- テーブルの検証は `GetTableMetadata` と `DESCRIBE FORMATTED` で行う。
  - `GetTableMetadata` で列とパーティションキーを取得し、スキーマプロファイルを判定する。
  - `DESCRIBE FORMATTED` を Athena で実行し、Location、InputFormat、SerDe、パーティションの有無を確認する。
  - `DESCRIBE FORMATTED` は読み取り専用で、`sqlguard` の DDL と DML の語を含まない。
  - 検証の結果は `TableValidation` として返し、どの列が足りないか、パーティションが無いかを示す。
  - 検証のクエリも `ExecutionParameters` は使わず、検証済みのデータベース名とテーブル名だけを引用する。
- Athena の集計は `internal/accesslog` の `BuildPairQuery` と `BuildIPQuery` を使う。
  アダプタは、テーブルを共有するソースを 1 つのクエリにまとめ、`source_key` でグループ化する。
  テーブルに `source_key` の列が無いソースが単独で選ばれている場合は、`source_key` を定数にして 1 つのリソースとして扱う。
  共有されたテーブルで `source_key` が無い場合は、クエリを開始せずに `Attribution` の不足をエラーにする。
  結果の文字列は、件数を `int64`、時刻を epoch ミリ秒として解析する。
- CloudWatch Logs Insights は、WAF のソースをロググループ単位でまとめ、`BuildWAFInsightsQuery` を使う。
  同じロググループを共有する複数の Web ACL は、`webaclId` で絞り込み、`stats` の `by` に `webaclId` を加えて 1 回のクエリで集計する。
  `StartQuery` の `StartTime` と `EndTime` は epoch 秒で渡す。
  `GetQueryResults` の `Status` が `Complete`、`Failed`、`Cancelled`、`Timeout` になるまで 1 秒間隔でポーリングする。
  `Results` のフィールドを `accesslog.PairRow` に変換し、`action` ごとの行を Go 側で合算する。
  `StopQuery` で取り消す。
- 期間の上限は、Athena が 7 日、Logs Insights が 24 時間とする。
  7 日の Athena は、パーティションキーがあるテーブルだけで許可する。
- 1 回の実行で選べるソースは最大 5 つ、結果は最大 200 行とする。
  API 層でも同じ制限を掛けるが、取得層でも防御する。
- `destination_type=none` または `logging_enabled=false` のソースが渡された場合は、クエリを開始せずにエラーを返す。
- `Attribution` が false のソースと、同じ出力先を共有する別のソースが同時に渡された場合は、クエリを開始せずにエラーを返す。
- クエリの結果はキャッシュしない。
- 実行の状態は `QueryState` (`queued`、`running`、`succeeded`、`failed`、`cancelled`) で表す。
  Athena の `QUEUED` と `RUNNING` を `queued` と `running` に、`SUCCEEDED` を `succeeded` に、`FAILED` と `CANCELLED` を `failed` と `cancelled` に写す。
  Logs Insights の `Scheduled` と `Running` を `queued` と `running` に、`Complete` を `succeeded` に、`Failed` と `Timeout` を `failed` に、`Cancelled` を `cancelled` に写す。
- エラーは `%w` でラップし、`AccessDenied` と SSO の期限切れは既存の `writeAWSError` の方針に合わせる。
- この issue では API のハンドラ、ジョブの管理、フロントエンドを含めない。

## 完了条件

- `StartAthenaQueryInput.ExecutionParameters` が `StartQueryExecutionInput` に渡り、既存の Athena のテストが通る。
- `GetTableMetadata` と `GetWorkGroup` が `athenaAPI` に追加され、手書きのフェイクでテストされている。
- `DESCRIBE FORMATTED` を使ったテーブル検証が、必須列の欠落、Location の不一致、SerDe の不一致、パーティションの有無を検出する。
- Athena の集計アダプタが、ALB、CloudFront legacy、CloudFront v2 JSON、WAF の各プロファイルのクエリを開始し、状態を取得し、結果を `accesslog.PairRow` に変換する。
- 同じテーブルを共有する複数のソースが 1 回のクエリにまとまり、`source_key` で分割される。
- 共有されたテーブルに `source_key` の列が無い場合に、クエリを開始せずにエラーになる。
- 同じ CloudWatch Logs のロググループを共有する複数の WAF が 1 回のクエリにまとまり、`webaclId` で分割される。
- `destination_type=none` または `logging_enabled=false` のソースが拒否される。
- Logs Insights のアダプタが、WAF のクエリを開始し、`Complete` までポーリングし、`Results` を `accesslog.PairRow` に変換し、`StopQuery` で停止する。
- Logs Insights の `Failed`、`Cancelled`、`Timeout` が区別される。
- 期間の上限、ソース数の上限、結果行数の上限が取得層で強制される。
- context のキャンセルでポーリングが止まり、AWS のクエリの停止が呼ばれる。
- 実行の状態の変換が、Athena と Logs Insights の全状態についてテストされている。
- `mise run backend:test` が通る。
- `mise run check` が通る。

## 関連

- ADR 0037
- issue 0235、0236、0238、0239、0240
- `backend/internal/aws/athena.go`、`cloudwatchlogs.go`
- `backend/internal/accesslog` (issue 0235)
