# ALB / CloudFront / WAF のアクセスログの出力先を検出する

Created: 2026-11-02
Model: DeepSeek V4.1 Flash 1017

## 背景

Access Logs ビューは、利用者が ALB、CloudFront、WAF のどのログを集計するかを選べる必要がある。
ログの出力先はリソースの設定によって異なり、ALB と CloudFront の legacy ログは S3、WAF は CloudWatch Logs、S3、Firehose のいずれかである。
CloudFront の標準ログ v2 は CloudWatch Logs の配信の仕組みで設定され、`GetDistributionConfig` の legacy のログ設定には現れない。
ALB も vended logs を CloudWatch Logs、S3、Firehose に配信できる。

ALB、CloudFront、WAF のすべてがログを出しているとは限らない。
ログが無効なリソースを一覧から落とすと、利用者は「リソースが無い」のか「ログが無効」なのかを区別できない。
また、複数の ALB、CloudFront、WAF が同じ S3 のバケットとプレフィックス、または同じ CloudWatch Logs のロググループにログを出すことがある。
出力先を単位としてソースをまとめ、どのリソースが共有しているかを示す必要がある。

出力先を利用者に手で入力させると、バケット名やロググループ名の間違いに気付きにくい。
リソースの一覧から出力先を検出し、使えるかどうかをソースごとに示す必要がある。

CloudFront の背後にある ALB では、ALB の `client_ip` が CloudFront のエッジ IP になる。
CloudFront の origin のドメイン名と ALB の DNS 名を照合し、どちらを閲覧者の IP の正とするかを判定する材料にする。

## 目的

`backend/internal/aws/accesslogs.go` を追加し、次の情報を返す関数を実装する。

- ALB の legacy の S3 アクセスログの設定
- CloudFront の legacy と v2 の標準ログの出力先
- WAF の CloudWatch Logs、S3、Firehose の出力先、ログフィルタ、redaction、managed の状態
- ログが無効な ALB、CloudFront、WAF のソース
- 同じ出力先を共有するリソースの一覧
- CloudFront の origin になっている ALB の対応
- ソースごとの状態と、検出に失敗したサービスの警告

## 設計判断

- 公開する型は `internal/accesslog` の `Source`、`Destination`、`SourceStatus` を使う。
  `internal/aws` は AWS SDK の型を `internal/accesslog` の型へ変換する。
- ALB は `DescribeLoadBalancers` で ALB の一覧を取り、`DescribeLoadBalancerAttributes` を上限付きの並列 (`errgroup.SetLimit(10)`) で呼ぶ。
  - `access_logs.s3.enabled` が `true` の場合は `alb_legacy` の S3 ソースを作る。
  - `access_logs.s3.bucket` と `prefix` を出力先にする。
  - `access_logs.s3.enabled` が `false` の場合は、`destination_type=none`、`logging_enabled=false` のソースを作る。
  - vended logs は CloudWatch Logs の配信の一覧から `ALB_ACCESS_LOGS` に相当するソースを探し、`unsupported` として表示する。
    ログが無効な `disabled` と混同しない。
  - 同じバケットとプレフィックスを使う ALB が複数ある場合は、`SharedBy` に ALB の ARN の一覧を入れる。
- CloudFront は `ListDistributions` で一覧を取り、`GetDistributionConfig` を上限付きの並列で呼ぶ。
  - `DistributionConfig.Logging.Enabled` が `true` の場合は `cloudfront_legacy` の S3 ソースを作る。
  - `DistributionConfig.Logging.Enabled` が `false` の場合は、`destination_type=none`、`logging_enabled=false` のソースを作る。
  - v2 は CloudWatch Logs の `DescribeDeliverySources` を us-east-1 で呼び、`service` が `cloudfront` で `logType` が `ACCESS_LOGS` のソースを探す。
    `DescribeDeliveries` と `DescribeDeliveryDestinations` で S3、CloudWatch Logs、Firehose の宛先、`recordFields`、`fieldDelimiter`、`outputFormat` を取得する。
  - v2 の S3 は JSON の出力形式だけを `ready` の候補にし、Plain、W3C、Raw、Parquet は `unsupported` とする。
    Parquet は既存の互換テーブルを選べる場合だけ `needs_table` とする。
  - legacy と v2 の両方が有効な場合は、同じディストリビューションに対して 2 つのソースを返し、両方を選ぶと重複することを `issues` に書く。
  - 同じバケットとプレフィックスを使う CloudFront のディストリビューションが複数ある場合は、`SharedBy` にディストリビューション ID の一覧を入れる。
  - legacy の `cs_host` と v2 の `distributionid` のどちらを `SourceKey` に使うかを、ソースに持たせる。
    テーブルに `distributionid` が無い legacy のソースは、`Attribution` を `cs_host` にし、`cs_host` が無い場合は false にする。
- WAF は `ListWebACLs` で REGIONAL と CLOUDFRONT の全 Web ACL を列挙し、`ListLoggingConfigurations` の結果と ARN で照合する。
  - `LogDestinationConfigs` の ARN で CloudWatch Logs、S3、Firehose を判定する。
  - CloudWatch Logs はロググループ名を出力先にする。
  - S3 はバケットとプレフィックスを出力先にする。
  - Firehose は `unsupported` とする。
  - ログの設定が無い Web ACL は、`destination_type=none`、`logging_enabled=false` のソースにする。
  - `LoggingFilter` がある場合は `issues` に「ログに残るレコードだけである」と書く。
  - `RedactedFields` に `UriPath` が含まれる場合は `unsupported` とする。
  - `ManagedByFirewallManager` が `true` の場合は `issues` にその旨を書く。
  - 同じ CloudWatch Logs のロググループまたは同じ S3 のバケットとプレフィックスを共有する Web ACL が複数ある場合は、`SharedBy` に Web ACL の ARN の一覧を入れる。
- CloudFront の origin の ALB の判定は、`GetDistributionConfig` の `Origins.Items[].DomainName` と ALB の `DNSName` を大文字と小文字を区別せずに比較して行う。
  - 一致した ALB のソースに `origin_of` として CloudFront のディストリビューション ID を持たせる。
  - この対応はソースの表示と、IP の集計から ALB を外す判断に使う。
- ソースの状態は `accesslog.SourceStatus` の直交したフラグで返す。
  - `logging_enabled`
  - `destination_type`
  - `destination_readable`
  - `table_configured`
  - `table_valid`
  - `data_present`
  - `last_delivery_at`
  - `issues`
  `internal/aws` は `logging_enabled`、`destination_type`、`destination_readable`、`issues` までを埋める。
  テーブルの状態は API 層が埋める。
- グローバル資源 (CloudFront、CLOUDFRONT スコープの WAF) は us-east-1 のクライアントで検出し、ソースの `Region` に `us-east-1` と `Global` の印を持つ。
- サービスの検出は `errgroup` で並列に行い、1 つのサービスの失敗は `Warnings` にまとめて返す。
  すべてのサービスが失敗した場合だけエラーを返す。
- 検出結果は 1 時間キャッシュする。
  キャッシュのキーは `access-logs-sources:<profile>:<region>` とする。
  TopBar の Refresh は `aws` ビューのキャッシュを破棄するため、既存の `viewOwnsCacheKey` でこのキーも破棄される。
- 新しい AWS SDK の依存は追加しない。
  CloudWatch Logs の配信の API は既存の `cloudwatchlogs` モジュールに含まれる。
- この issue ではテーブルの検証、Athena と Logs Insights のクエリ、API のハンドラ、フロントエンドを含めない。

## 完了条件

- `backend/internal/aws/accesslogs.go` に、ALB、CloudFront、WAF のソースを検出する公開関数がある。
- ALB の legacy の S3 が有効な場合に、バケット、プレフィックス、`alb_legacy` のスキーマプロファイルを持つソースが返る。
- ALB の legacy の S3 が無効な場合に、`destination_type=none`、`logging_enabled=false` のソースが返る。
- ALB の vended logs が有効な場合に、`unsupported` のソースが返り、ログが無効な場合の `disabled` と区別される。
- CloudFront の legacy の S3 が有効な場合に、バケット、プレフィックス、`cloudfront_legacy` のソースが返る。
- CloudFront の legacy の S3 が無効な場合に、`destination_type=none`、`logging_enabled=false` のソースが返る。
- CloudFront の v2 が S3、CloudWatch Logs、Firehose のいずれかに配信されている場合に、宛先と `recordFields`、`outputFormat` を持つソースが返る。
- WAF の CloudWatch Logs のソースが、ロググループ名と `waf_json` のスキーマプロファイルを持つ。
- WAF の S3 のソースが、バケットとプレフィックスを持つ。
- ログの設定が無い WAF の Web ACL が、`destination_type=none`、`logging_enabled=false` のソースとして返る。
- WAF の Firehose のソースが `unsupported` になる。
- 同じ S3 のバケットとプレフィックス、または同じ CloudWatch Logs のロググループを共有する複数の ALB、CloudFront、WAF が、`SharedBy` に互いの識別子を持つ。
- 共有された出力先のソースが `SourceKey` と `Attribution` を持ち、`SourceKey` の列が無い場合は `Attribution` が false になる。
- WAF のログフィルタ、`UriPath` の redaction、Firewall Manager の管理が `issues` に反映される。
- CloudFront の origin の ALB の対応が、ディストリビューション ID と ALB の ARN の組で返る。
- REGIONAL と CLOUDFRONT の WAF の両方が検出される。
- 1 つのサービスの `AccessDenied` が、ほかのサービスのソースを失わせず、警告として返る。
- 検出のテストが、手書きのフェイクのクライアントで、ALB、CloudFront legacy、CloudFront v2、WAF の各分岐、ログが無効なリソース、共有された出力先、部分失敗、グローバル資源、origin の照合を確認する。
- WAF のテストが、ログの設定がある Web ACL と無い Web ACL の両方を返すことを確認する。
- キャッシュのキーが `access-logs-sources` で始まり、既存の `aws` ビューの破棄の対象になることをテストで確認する。
- `mise run backend:test` が通る。
- `mise run check` が通る。

## 関連

- ADR 0037
- issue 0235、0237、0238、0239、0240
- `backend/internal/aws/elb.go`、`cloudfront.go`、`waf.go`、`cloudwatchlogs.go`
- `backend/internal/api/cache_helpers.go`、`handlers_cache.go`
