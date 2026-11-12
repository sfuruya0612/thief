# 0037. ALB / CloudFront / WAF のアクセスログを Athena と CloudWatch Logs Insights で集計する

Created: 2026-11-02
Model: DeepSeek V4.1 Flash 1017
Status: Accepted
Decided: 2026-11-02

## 状況

thief は Athena のクエリエディタと CloudWatch Logs のビューを持つが、ALB、CloudFront、WAF のアクセスログを横断し、アクセス元の IP とリクエスト URI で集計する専用のビューを持たない。
不正なリクエストが届いていないかを確認するには、AWS のコンソールで複数のログを開き、Athena のクエリを組み立てる必要がある。
この作業を thief の中で完結させたい。

ログの出力先は 1 つではない。
ALB の legacy アクセスログと CloudFront の legacy 標準ログは S3 に置かれる。
WAF のログは CloudWatch Logs、S3、Firehose のいずれかを選べる。
CloudFront の標準ログ v2 と ALB の vended logs は CloudWatch Logs、S3、Firehose に配信でき、CloudFront の標準ログ v2 は JSON、Plain、W3C、Raw、Parquet の形式を選べる。
ログの形式と列は配信の設定によって変わる。

アクセスログをローカルにダウンロードして Go で解析する方法は取れない。
ALB のログは引用符を含むスペース区切りで、CloudFront の legacy ログはタブ区切り、WAF のログは JSON Lines である。
大量のログを端末に持ってくると、転送量と解析時間が実用の範囲を超える。

セキュリティの判断に使う集計では、IP の取り違えを避けなければならない。
CloudFront の背後に ALB がある構成では、ALB の `client_ip` は CloudFront のエッジ IP になる。
これをそのまま攻撃元 IP として表示すると、誤った相手を攻撃者として示す。

thief はデータベースを持たず、取得結果をメモリにキャッシュする (ADR 0005)。
Athena と BigQuery のクエリは読み取り専用とし、`sqlguard` が DDL と DML を拒否する (ADR 0017)。
API サーバは利用者の認証を持たない (ADR 0032)。

## 決定

- AWS ビューの Security グループに **Access Logs** を追加する。
  サービスキーは `accesslogs`、副題は `Threat check` とする。
  一覧型のサービスではなく、`AccountView` から専用のビューを描画する。
  新しいトップレベルビュー (`AppView`) は追加しない。
  TUI と CLI は対象に含めない (ADR 0036)。

- ログの出力先に応じてクエリエンジンを切り替える。
  - S3 のログは Athena で集計する。
  - CloudWatch Logs のログは CloudWatch Logs Insights で集計する。
  - v1 の必須は、ALB の legacy S3 アクセスログ、CloudFront の legacy または v2 の S3 標準ログ、WAF の CloudWatch Logs とする。
  - WAF の S3 ログは、互換する Athena テーブルがある場合だけ対象とする。
  - ALB と CloudFront の vended logs、Firehose 宛先、CloudFront 標準ログ v2 の Plain、W3C、Raw は v1 の対象外とし、`unsupported` と表示する。

- ログソースを検出して画面に表示する。
  - ALB は `DescribeLoadBalancers` と `DescribeLoadBalancerAttributes` で legacy の S3 設定を取得する。
  - CloudFront は `ListDistributions` と `GetDistributionConfig` で legacy の S3 設定を取得する。
    v2 は CloudWatch Logs の `DescribeDeliverySources`、`DescribeDeliveries`、`DescribeDeliveryDestinations` を us-east-1 で呼んで取得する。
  - WAF は `ListWebACLs` で REGIONAL と CLOUDFRONT の全 Web ACL を列挙し、`ListLoggingConfigurations` と照合して CloudWatch Logs、S3、Firehose の宛先を判定する。
    ログの設定が無い Web ACL もソースとして返し、`logging_enabled` を false にする。
  - ALB と CloudFront も、ログが無効なリソースをソースとして返し、`logging_enabled` を false にする。
  - 同じ S3 のバケットとプレフィックス、または同じ CloudWatch Logs のロググループを複数のリソースが共有する場合は、出力先を単位としてソースをまとめ、`shared_by` に共有するリソースの一覧を持つ。
  - グローバル資源は us-east-1 で扱い、画面で Global と明示する。
  - 検出結果は 1 時間キャッシュし、TopBar の Refresh で破棄する。
  - ソースの状態は 1 つの enum で表さない。
    `logging_enabled`、`destination_type`、`destination_readable`、`table_configured`、`table_valid`、`data_present`、`last_delivery_at`、`source_key`、`attribution`、`shared_by`、`issues` を独立して持つ。
  - 1 つのソースの取得に失敗しても、ほかのソースの結果は返す。

- ログを共通の形に正規化する。
  - 集計の単位は `(client_ip, uri_path)` とする。
  - URI はパスだけをグルーピングに使い、クエリ文字列は `sample_query` と `has_query` で見せる。
    パスを URL デコードしてグルーピングすると、攻撃の variant が 1 つにまとまるため、生の値を使う。
  - ALB は `client_ip`、`request_url`、`elb_status_code`、`actions_executed`、`time` を使う。
  - CloudFront は `c_ip`、`cs_uri_stem`、`sc_status`、`date` と `time` を使う。
    `x_forwarded_for` は既定では使わない。
  - WAF は `httpRequest.clientIp`、`httpRequest.uri`、`action`、`terminatingRuleId`、`timestamp` を使う。
  - 時刻は UTC で保持し、画面では端末のタイムゾーンで表示する。

- IP の attribution をソースの構成に合わせる。
  - CloudFront の origin のドメイン名と ALB の DNS 名を照合し、CloudFront の origin になっている ALB を検出する。
  - CloudFront とその origin の ALB を同時に選んだ場合、閲覧者の IP の正は CloudFront とする。
    ALB は origin 側の状態 (件数、4xx、5xx) だけを表示し、IP の集計には加算しない。
  - WAF はリクエスト件数の加算対象にしない。
    WAF の `BLOCK`、`CAPTCHA`、`CHALLENGE` を同じ `(client_ip, uri_path)` のオーバーレイとして表示する。
  - 構成を判定できない CloudFront と ALB を同時に選んだ場合は、IP の統合表示を行わず、どちらを正とするかを利用者に選ばせる。

- ログの出力がないリソースを、集計の失敗として扱わない。
  - ALB、CloudFront、WAF のいずれも、ログの設定が無いリソースをソースとして一覧に返し、`logging_enabled` を false、`destination_type` を `none` とする。
  - 画面は「ログが無効」と「ログは有効だがデータがまだ無い」を区別して表示する。
  - ログが無効なソースは集計の対象に選べない。
    すべてのソースが無効な場合は、集計の実行ボタンを無効にし、ログの有効化の手順を表示する。
  - WAF は `ListLoggingConfigurations` が返さない Web ACL を取りこぼさないよう、`ListWebACLs` の一覧を基準にする。
  - CloudFront の legacy と標準ログ v2 は別の出力先として扱う。
    片方だけが有効な場合は、有効な方をソースとして表示し、もう片方は「未設定」と表示する。

- 複数のリソースが同じ出力先を共有する場合、リソースごとの内訳を保つ。
  - ソースは出力先だけでなく、リソースの識別子 (`source_key`) を持つ。
    - ALB は `elb` (例: `app/prod-alb/50dc6c495c0c9188`)
    - CloudFront の標準ログ v2 は `distributionid`
    - CloudFront の legacy は `cs_host` または `x_host_header`
    - WAF は `webaclid` (v2 の ARN)
  - 同じ Athena テーブルまたは同じ CloudWatch Logs のロググループを共有する複数のソースは、1 回のクエリで `source_key` を `WHERE` で絞り、`GROUP BY` に含める。
    結果は `source_key` ごとに分け、選択していないリソースのレコードを混ぜない。
  - テーブルまたはロググループに `source_key` に使える列が無い場合は、そのソースの `attribution` を false とする。
    `attribution` が false のソースは、ほかのリソースと同時に選べない。
    単独で選んだ場合も、結果をリソース別に分けられないことを表示する。
  - 画面は、同じ出力先を共有するソースに「共有」の印と、共有するリソースの一覧を表示する。
  - テーブルの対応付けは出力先を単位として保存し、同じ出力先を使う複数のリソースで 1 つのテーブルを共有できるようにする。

- 「悪意がある」と断定しない。
  - 主シグナルは WAF の `BLOCK`、`CAPTCHA`、`CHALLENGE` とする。
  - 4xx と 5xx は補助情報として別の列に表示する。
  - センシティブパスへのアクセスと、同一 IP からの URI の多さは絞り込みの条件として提供し、判定ラベルにはしない。
  - WAF のログフィルタが有効な場合は、ログに残ったレコードだけであることを表示し、`blocked / total` のような率を出さない。
  - `UriPath` が redaction されている WAF のソースは `unsupported` とする。

- Athena のテーブルは利用者が用意する。
  - thief は `CREATE TABLE` を実行しない (ADR 0017)。
  - 検出した S3 のバケットとプレフィックスから、パーティションを切る DDL のテンプレートを生成し、コピーできるようにする。
  - 既存のテーブルを選べるようにし、`GetTableMetadata` と `DESCRIBE FORMATTED` または smoke query で必須列、パーティション、Location、SerDe を検証する。
    複数のリソースが同じテーブルを共有する場合は、`source_key` に使う列 (`elb`、`distributionid`、`cs_host`、`webaclid`) があることも検証する。
  - DDL のテンプレートは、ALB の legacy、CloudFront の legacy の TSV、CloudFront の標準ログ v2 の JSON、WAF の S3 JSON を用意する。
    DDL は同じ出力先を共有する複数のリソースを 1 つのテーブルで扱えるようにし、`source_key` の列を残す。
  - CloudFront の標準ログ v2 の Parquet は、Glue Crawler で作ったテーブルを選ぶ手順を画面に示す。
  - ワンクリックでテーブルを作る機能は v1 に含めない。

- クエリのコストと安全性を制限する。
  - Athena のクエリは `SELECT` だけを生成する。
  - 利用者が入力した値は `ExecutionParameters` の WHERE 句に置き、SQL に連結しない。
  - テーブル名とカラム名は `^[A-Za-z0-9_]+$` で検証して引用する。
  - 生成した SQL に `sqlguard.ValidateReadOnly` を掛ける。
  - パーティションの述語は、配信の遅延を考慮して開始の 1 日前から終了の 1 日後までを対象にし、イベントの時刻で厳密に絞る。
  - 同じ Athena テーブルを共有するソースは 1 回のクエリにまとめ、`source_key` でグループ化して Go 側で分割する。
    同じ CloudWatch Logs のロググループを共有する WAF のソースも、`webaclId` でグループ化して 1 回のクエリにまとめる。
  - 1 回に選べるソースは最大 5 つ、結果は最大 200 行とする。
  - ログが無効なソースと `attribution` が false のソースの組み合わせは、API 層で拒否する。
  - 期間は既定を 24 時間とし、7 日はパーティションが有効で利用者が確認した場合だけ許可する。
  - CloudWatch Logs Insights は 24 時間を上限とする。
  - Athena のワークグループに `BytesScannedCutoffPerQuery` があることを確認し、無い場合は実行前に警告する。
  - クエリの結果はキャッシュしない。
  - スキャンしたバイト数とレコード数をソースごとに表示する。

- 集計のジョブは API サーバのメモリで管理する。
  - ジョブは ID、プロファイル、リージョン、ソース、実行 ID、状態、TTL を持つ。
  - TTL の期限切れと利用者の取り消しで、実行中の Athena と CloudWatch Logs Insights のクエリを停止する。
  - 1 つのソースが失敗しても、ほかのソースの結果とソース別のエラーを返す。

- ソース間の重複を v1 では排除しない。
  - 同じリクエストが CloudFront、WAF、ALB の複数のログに現れることがある。
  - 合計値を 1 つだけ表示せず、ソース別の内訳を必ず表示する。
  - リクエスト ID による行レベルの重複排除は v1 に含めない。

- v1 に含めないものは、リアルタイムの検知、Live Tail、WAF のルール変更や IP のブロック、アラート、保存ビュー、CSV のエクスポート、ベースラインとの比較、時系列グラフ、生ログの大量取得、CLI と TUI である。

## 検討した代替案

- S3 のログを Go で直接読んで集計する。
  Athena のテーブルが不要になるが、ALB の引用符付きのスペース区切り、CloudFront の TSV、WAF の JSON Lines を自前で解析することになる。
  大量のログを端末に転送し、パーティションの絞り込みもできないため、スキャンの費用と時間が読めない。
- Athena だけを使う。
  S3 のログには適するが、WAF の CloudWatch Logs のログを読めない。
- CloudWatch Logs Insights だけを使う。
  WAF の CloudWatch Logs には適するが、ALB と CloudFront の legacy の S3 ログを読めない。
- Amazon Security Lake を使う。
  OCSF に正規化されたログを横断できるが、Security Lake の導入が前提になり、thief の利用者の環境でいつでも使えるとは限らない。
- WAF の `GetSampledRequests` を使う。
  設定なしで直近のサンプルを取得できるが、サンプルに限られ、ALB と CloudFront のログは対象外である。
- thief から Athena の `CREATE TABLE` を実行する。
  利用者の操作が減るが、ADR 0017 の読み取り専用の原則を崩し、認証を持たないローカルの API から外部の資源を変更できる経路を増やす。
  v1 では採用しない。

## 結果

- 利用者は AWS のプロファイルとリージョンを選び、検出された ALB、CloudFront、WAF のソースから対象を選んで、期間を指定して集計を実行できる。
- 画面は IP と URI の組、IP 単独の集計、ソースのセットアップのタブを持つ。
- IP と URI の組は、リクエスト数、WAF のブロック数、チャレンジ数、4xx、5xx、ソース、最初と最後の時刻、User-Agent の例、ルール ID の例を表示する。
- CloudFront の origin の ALB は IP の集計から外れるため、エッジ IP を攻撃元として表示しない。
- WAF のブロックは、同じ IP と URI の行にオーバーレイされ、リクエスト数には加算されない。
- 利用者は既存の Athena テーブルを選ぶか、パーティションを切る DDL をコピーして Athena で実行し、そのテーブルを選ぶ。
- ログが無効な ALB、CloudFront、WAF も一覧に残り、「ログが無効」であることと有効化の手順が表示される。
  すべてのソースが無効な場合でも、画面は空にならず、何を有効にすればよいかが分かる。
- 複数のリソースが同じ S3 のバケットとプレフィックス、または同じ CloudWatch Logs のロググループを共有していても、`source_key` でリソースごとの内訳を保つ。
  共有する出力先は 1 つのテーブルまたはロググループで扱える。
- ソースのログが無効、未対応の形式、権限不足、配信の遅延のいずれであっても、その状態がソースごとに表示される。
- 集計の実行中はジョブの状態が表示され、利用者は取り消せる。
- スキャンしたバイト数とレコード数がソースごとに表示され、結果はキャッシュされない。
- 新しい IAM の権限として、ELB、CloudFront、WAFv2、CloudWatch Logs の配信、Athena、Glue、S3、CloudWatch Logs の参照が必要になる。
  Athena は結果を S3 に書き出すため、結果のバケットへの書き込み権限も必要になる。
- ログの配信には遅延がある。
  ALB は約 5 分、CloudFront は 1 時間程度、遅い場合は 24 時間である。
  直近の期間を選んだときに結果が 0 件でも、攻撃が無いとは限らない。
- ソース間の重複は排除しないため、複数のソースを選んだときの件数はログのレコード数であり、リクエスト数とは限らない。

## 根拠資料

- PRD `docs/prd/thief.md` の FR-2、FR-3、FR-9、FR-13、FR-14、FR-15、FR-16、FR-29
- ADR 0005、ADR 0009、ADR 0015、ADR 0016、ADR 0017、ADR 0020、ADR 0032、ADR 0036
- `backend/internal/aws/athena.go`、`backend/internal/aws/cloudwatchlogs.go`、`backend/internal/sqlguard/sqlguard.go`
- `frontend/src/views/AthenaView.tsx`、`frontend/src/views/CloudWatchLogsView.tsx`、`frontend/src/lib/serviceMeta.ts`
- Amazon Athena の Application Load Balancer ログのテーブル: https://docs.aws.amazon.com/athena/latest/ug/create-alb-access-logs-table-partition-projection.html
- Amazon Athena の CloudFront ログのテーブル: https://docs.aws.amazon.com/athena/latest/ug/create-cloudfront-table-standard-logs.html
- Amazon Athena の AWS WAF ログのテーブル: https://docs.aws.amazon.com/athena/latest/ug/create-waf-table-partition-projection.html
- CloudFront の標準ログ (v2): https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/standard-logging.html
- AWS WAF のログの出力先: https://docs.aws.amazon.com/waf/latest/developerguide/logging.html
- CloudWatch Logs Insights のクエリ構文: https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/CWL_QuerySyntax.html
