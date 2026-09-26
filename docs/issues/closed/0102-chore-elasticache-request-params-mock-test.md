# ElastiCache の DescribeCacheClusters に渡すリクエストパラメータを検証するモックテストを整備する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「DescribeXxxInput に渡すパラメータを検証するモックテストを整備したい (issue 0093 のスコープ外として送り)」に対応する。

`backend/internal/aws/elasticache.go` は 2 箇所 (58 行と 184 行) で `DescribeCacheClustersInput` に `ShowCacheNodeInfo: aws.Bool(true)` を設定して呼び出す。
このパラメータが無いとレスポンスに `CacheNodes` が含まれず、docs/issues/closed/0093 で追加したノードごとの AZ 表示が全て空になる。
しかし現状のテストは `xxxFromYyy` 系の変換関数だけを対象にしており、リクエスト構築は検証していない。
`ShowCacheNodeInfo` の行を削除してもテストは 1 つも落ちない。

リクエストパラメータの検証手法の先例はリポジトリ内にある。
`backend/internal/aws/rds.go` の `rdsClusterParameterClient` (145-148 行) は必要な API メソッドだけを持つ狭いインターフェースで、`rds_test.go` (16-29 行) の手書きモックが受け取った Input を記録し、テストがその中身を検証する。

TODO の子行は「internal/aws 全体に共通する」と指摘するが、本 issue は症状の起票元である ElastiCache に限定する (設計判断を参照)。

## 対応方針

- `elasticache.go` に `DescribeCacheClusters` を持つ狭いインターフェースを定義し、現在具象型 `*elasticache.Client` を直接使っている 2 箇所の呼び出しロジックを、インターフェースを受け取る内部関数に抽出する。AWS SDK for Go v2 のページネータは API クライアントインターフェース (`elasticache.DescribeCacheClustersAPIClient`) を受け取れるため、この抽出は SDK の利用方法として標準的である。
- `elasticache_test.go` に、受け取った `DescribeCacheClustersInput` を記録する手書きモックを追加し、2 つの呼び出し経路の両方で `ShowCacheNodeInfo` が `true` に設定されることを検証する。モックの型は 1 つだけ定義し、2 つの経路のテストで共有する。経路ごとに別のモック型を作る案は、記録する内容が同じで重複になるため採らない。
- どちらの呼び出し箇所もページネータ (`NewDescribeCacheClustersPaginator`) 経由のため、モックは `Marker` 付きの複数ページレスポンスを返し、2 ページ目以降の呼び出しでも `ShowCacheNodeInfo` が `true` のまま送られることを検証する。ページネータがページ送りで元のパラメータを維持することは SDK の実装に依存しており、この検証でその依存を明示的に固定する。
- モック生成に mockery は使わない。`mise run backend:mocks` は定義されているが、リポジトリに `.mockery.yaml` が無く mocks/ ディレクトリも無い実質未使用のタスクであり、rds_test.go の手書きモックが既に先例としてある。パラメータ記録だけの単純なモックに生成ツールを持ち込む理由が無い。
- internal/aws の他サービスへの修正の横展開はスコープ外とする。サービスごとに「消えると壊れる必須パラメータ」の有無が異なり、一律に展開するとパラメータ検証の必要が無い箇所にまでモック配線を増やす。ただし TODO の子行が「internal/aws 全体に共通する」と指摘しているため、他サービスに同種の必須リクエストパラメータ (削除しても既存テストが検知しないもの) が無いかの棚卸しは本 issue で行い、見つかったものの修正は個別に起票する。

## 完了条件

- `elasticache.go` の `DescribeCacheClusters` の 2 つの呼び出し経路が、狭いインターフェースを受け取る関数に抽出されている。
- 2 つの経路の両方について、複数ページのレスポンスでも全ての呼び出しで `ShowCacheNodeInfo` が `true` で送られることを検証するテストが存在する。
- `ShowCacheNodeInfo: aws.Bool(true)` の行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- internal/aws の他サービスには変更を加えない。
- `backend/internal/aws/` 直下の全 Go ファイル (テストファイルを除く) を対象に、削除しても既存テストが検知しない必須リクエストパラメータの棚卸しを行い、確認したファイルの一覧と結果 (対象なしの場合を含む)、見つかった場合に起票した issue の番号を本 issue に記録する。棚卸しの確認は、各ファイルの API 呼び出しの Input 構築を読み、そこに設定されるフィールドを既存テストが検証しているかを突き合わせる目視の確認とする。パラメータを実際に削除してテストを実行する実験までは求めない。
- `mise run check` が通る。

## 棚卸しの結果

`backend/internal/aws/` 直下の全 Go ファイル 40 件 (テストファイルを除く) を対象に、削除しても既存テストが検知しない必須リクエストパラメータの目視確認を行った。
調査は読み取り専用エージェント 4 体で分担し、発見の該当行は本体で grep と本文の読解により実在を確認した。
調査エージェントの報告のうち、`ListECSTasks` の所在は ecs.go とされていたが、実在確認で ecs_exec.go (126-129 行) と判明したため訂正して記録した。

発見あり (13 ファイル、11 issue を起票):

- athena.go: `listAthenaQueryHistory` の `ListQueryExecutionsInput.WorkGroup` → issue 0111
- cfn.go: `ListCFNStacks` / `ListCfnStackSummaries` の `ListStacksInput.StackStatusFilter` → issue 0112
- cloudwatchlogs.go: `FilterLogEvents` の `FilterPattern` / `StartTime` / `EndTime` / `StartFromHead` と `StartLiveTail` の `LogEventFilterPattern` → issue 0113
- costexplorer.go: `getCostDetails` の `GroupBy` (テストファイル自体が存在しない) → issue 0114
- dynamo.go: `QueryDynamoItems` の `ScanInput` / `QueryInput` の `Limit` とフィルタ式 → issue 0115
- ec2.go: `ListEC2Instances` の `Filters` (running / instance-id) → issue 0116
- ecr.go: `ListECRImageInfos` の `MaxResults` と `Filter` (TagStatusTagged) → issue 0117
- ecs.go / ecs_exec.go / ecs_cli.go: `DescribeClustersInput.Include` (ClusterFieldTags)、`ListTasksInput.ServiceName`、`ListTasksInput.DesiredStatus` → issue 0118
- pricing.go: `fetchSavingsPlans` の `SavingsPlanTypes` / `ServiceCodes` / リージョンの `Filters` → issue 0119
- sqs.go: `ListSQSResources` の `GetQueueAttributesInput.AttributeNames` (QueueAttributeNameAll) → issue 0120
- ssm.go: `ListSSMOnlineInstanceIDs` の `DescribeInstanceInformationInput.Filters` (PingStatus / ResourceType、テストファイル自体が存在しない) → issue 0121

対象なし (18 ファイル。Input を構築するが、該当フィールドが無いか、識別子等のクラス B のみか、既にテストされている):

- apigw.go (NextToken のみ)
- cloudfront.go (識別子と必須パラメータのみ)
- cost.go (Filter / GroupBy / Metrics は既存テストで検証済み)
- elb.go (全フィールドが ARN)
- iam.go (UserName / RoleName のみ)
- identity.go (フィールド無し)
- kinesis.go (StreamName のみ)
- lambda.go (フィールド無し)
- natgw.go (フィールド無し)
- rds.go (識別子のみ。識別子の受け渡しは既存の手書きモックテストで検証済み)
- regions.go (フィールド無し)
- s3.go (Bucket のみ)
- s3_object.go (呼び出し元引数の伝播のみ)
- secretsmanager.go (識別子と値のみ)
- ssm_session.go (識別子のみ)
- sso.go (識別子のみ)
- sso_oidc.go (識別子と引数の伝播のみ)
- waf.go (Scope は欠落時に API エラーになる必須値、ResourceType は SDK の全種別ループでテスト済み)

API 呼び出しの Input 構築なし (8 ファイル): client.go, errors.go, profiles.go, resource.go, session.go, sso_cache.go, sso_token.go, torow.go

elasticache.go (1 ファイル) は本 issue で対応した。

## 関連

- docs/issues/closed/0093: 本 issue の起票元。`ShowCacheNodeInfo` の必須性が AZ 表示の実装で確認された。
- docs/issues/0111 から docs/issues/0121: 棚卸しで見つかった同種の必須リクエストパラメータの個別起票。

## 解決方法

- `elasticache.go` に狭いインターフェース `elastiCacheDescribeClustersClient` を定義し、`DescribeCacheClusters` の 2 つの呼び出し経路をインターフェースを受け取る内部関数 (`listElastiCacheResources` / `listElastiCacheClusterInfos`) に抽出した。公開関数の挙動は変えていない。
- 受け取った `DescribeCacheClustersInput` を記録する手書きモックを 1 つ追加し、テーブル駆動テスト 1 本のサブテストとして 2 経路を検証した。モックは `Marker` で連結した 2 ページ構成のレスポンスを返し、全呼び出しで `ShowCacheNodeInfo` が `true` であること、1 回目の呼び出しに `Marker` が無く 2 回目に 1 ページ目の `Marker` が引き継がれること (実際にページ送りが起きたこと)、両ページのクラスタが集約されることを検証する。
- `ShowCacheNodeInfo: aws.Bool(true)` の 2 行を一時的に削除し、両サブテストが `ShowCacheNodeInfo = <nil>, want true` で失敗することを確認してから復元した。
- 完了条件の棚卸しを実施し、結果を「## 棚卸しの結果」に記録した。発見のあった 13 ファイルを 11 件の issue (docs/issues/0111 から docs/issues/0121) として起票し、`docs/issues/SEQUENCE` を 0122 に更新した。
- `mise run check` の通過を確認した。
