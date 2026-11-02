# アクセスログ集計のドメイン型と SQL ビルダーを追加する

Created: 2026-11-02
Model: DeepSeek V4.1 Flash 1017

## 背景

ALB、CloudFront、WAF のアクセスログをアクセス元の IP とリクエスト URI で集計するには、ログの形式ごとに列名と意味が異なる差を吸収し、同じ形の集計結果に正規化する必要がある。
ALB の legacy ログは引用符を含むスペース区切り、CloudFront の legacy ログはタブ区切り、CloudFront の標準ログ v2 は JSON、WAF のログは JSON Lines である。
CloudFront の標準ログ v2 は列を選べるため、同じ v2 でもテーブルの列名が変わる。

複数の ALB、CloudFront、WAF が同じ S3 のバケットとプレフィックス、または同じ CloudWatch Logs のロググループにログを出すことがある。
また、ALB、CloudFront、WAF のいずれかでログの出力が無効になっていることもある。
集計は、共有された出力先の中でリソースごとの内訳を保ち、ログが無効なリソースを「データが無い」と混同しない形で扱う必要がある。

集計の SQL を API ハンドラやフロントエンドに直接書くと、ソースごとの差、パーティションの述語、利用者の入力のエスケープ、WAF のアクションの重ね合わせが散らばる。
ADR 0037 は、S3 のログを Athena、CloudWatch Logs のログを Logs Insights で集計し、バックエンドで共通の形に正規化することを決めた。

## 目的

AWS SDK に依存しない純粋なパッケージ `backend/internal/accesslog` を追加し、次の処理を単体テストできる形で提供する。

- ログソース、出力先、スキーマプロファイル、列の対応付け、期間、フィルタの型
- スキーマプロファイルの判定と必須列の検証
- Athena の集計 SQL と Logs Insights のクエリの生成
- パーティション述語の生成
- パーティションを切る DDL の生成
- ソースごとの集計結果のマージと、IP の attribution の規則
- センシティブパスとクエリ文字列の有無の判定

## 設計判断

- パッケージは `backend/internal/accesslog` とし、`internal/aws` と `internal/api` から import する。
  このパッケージは AWS SDK を import しない。
- ログソースの種類は `SourceType` (`alb`、`cloudfront`、`waf`) で表す。
  クエリのエンジンは `Engine` (`athena`、`logs_insights`) で表す。
- 出力先は `Destination` で表し、`s3`、`cloudwatch_logs`、`firehose`、`none` を持つ。
  S3 はバケット、プレフィックス、フォーマット、レコードのフィールドを持つ。
  CloudWatch Logs はロググループ名を持つ。
  `none` はログの設定が無いことを表す。
- ソースはリソースの識別子 `SourceKey` と、リソースの識別子をログから取り出せるかを表す `Attribution` を持つ。
  - ALB は `elb`
  - CloudFront の標準ログ v2 は `distributionid`
  - CloudFront の legacy は `cs_host` または `x_host_header`
  - WAF は `webaclid`
  `Attribution` が false のソースは、同じ出力先を共有するほかのリソースと同時に集計できない。
- 同じ出力先を共有するソースは、`SharedBy` に共有するソースの ID の一覧を持つ。
  1 つの Athena テーブルまたは 1 つの CloudWatch Logs のロググループに対して、複数のソースをまとめて 1 回のクエリで集計できるようにする。
- スキーマプロファイルは `SchemaProfile` で表し、次の 4 つを用意する。
  - `alb_legacy`: `client_ip`、`request_url`、`elb_status_code`、`actions_executed`、`time`、`elb`、`user_agent`
  - `cloudfront_legacy`: `c_ip`、`cs_uri_stem`、`sc_status`、`date`、`time`、`cs_user_agent`、`cs_host`
  - `cloudfront_v2_json`: `c-ip`、`cs-uri-stem`、`sc-status`、`date`、`time`、`cs(User-Agent)`、`x-host-header`
  - `waf_json`: `httprequest.clientip`、`httprequest.uri`、`action`、`timestamp`、`terminatingruleid`、`httprequest.httpmethod`、`httprequest.headers`
  プロファイルはテーブルの列名から判定する。
  判定できない場合は利用者にプロファイルを選ばせる。
- 列の対応付けは `ColumnMapping` で表し、スキーマプロファイルの既定値と、利用者が選んだ列名を分ける。
  列名は `^[A-Za-z0-9_]+$` または `^[A-Za-z0-9_.]+$` で検証する。
  CloudFront v2 の `c-ip` や `cs(User-Agent)` のように、そのままでは識別子にならない列は、プロファイル側で引用した式を持つ。
- 期間は `TimeRange` で表し、内部は UTC とする。
- フィルタは `Filter` で表し、IP の完全一致、URI の前方一致、WAF のアクションの種類、4xx と 5xx の絞り込みを持つ。
  v1 では自由文のフィルタを扱わない。
- Athena の集計は 2 種類の SQL を生成する。
  - `BuildPairQuery`: `(source_key, client_ip, uri_path)` の集計
  - `BuildIPQuery`: `client_ip` の集計
  SQL は `SELECT` だけとし、利用者が入力した値は `?` の実行パラメータに置く。
  テーブル名、データベース名、カラム名は呼び出し側で検証済みのものを受け取る。
  生成した SQL は `sqlguard.ValidateReadOnly` を通す。
- 同じ Athena テーブルを共有する複数のソースは、`WHERE source_key IN (?, ?)` で選択したソースに絞り、`GROUP BY source_key` でリソースごとの行を返す。
  選択していないリソースのレコードを集計に混ぜない。
  テーブルが単一のリソース専用で、`source_key` の列が無い場合は、ソースの絞り込みを省略し、`Attribution` を false にする。
- 同じ CloudWatch Logs のロググループを共有する WAF のソースは、`webaclId` で絞り込み、`GROUP BY webaclId` でリソースごとの行を返す。
  Logs Insights のフィールド名は JSON の名前 (`webaclId`、`httpRequest.clientIp`) を使い、Athena の lowercase の列名と混同しない。
- パーティションの述語は、テーブルのパーティションキーから生成する。
  - ALB: `day` (`yyyy/MM/dd`)
  - CloudFront legacy: `year`、`month`、`day`
  - CloudFront v2: `year`、`month`、`day`、`hour`
  - WAF S3: `log_time` (`yyyy/MM/dd/HH/mm`) または `year`、`month`、`day`、`hour`
  配信の遅延を考慮し、開始の 1 日前から終了の 1 日後までを述語に含める。
  イベントの時刻の列で厳密に絞る。
- Logs Insights のクエリは `BuildWAFInsightsQuery` で生成する。
  `httpRequest.clientIp`、`httpRequest.uri`、`action` でグループ化し、件数と最初と最後の時刻を返す。
  利用者の入力は、IP の完全一致と URI の前方一致だけを許可し、引用符とバックスラッシュをエスケープする。
- DDL は `BuildDDL` で生成する。
  - ALB の legacy は公式の RegexSerDe の DDL を基にし、`day` のパーティション投影を付ける。
  - CloudFront の legacy はタブ区切りの DDL に `year`、`month`、`day`、`hour` のパーティション投影を付ける。
  - CloudFront の v2 JSON は `recordFields` から列を組み立て、JSON SerDe とパーティション投影を付ける。
  - WAF の S3 JSON は JSON SerDe と `log_time` のパーティション投影を付ける。
  DDL は返すだけで、実行しない。
- 集計結果のマージは `Merge` で行う。
  - `PairRow` は `client_ip`、`uri_path`、`request_count`、`blocked_count`、`challenged_count`、`count_matched`、`client_error_count`、`server_error_count`、`sources`、`first_seen`、`last_seen`、`sample_user_agent`、`sample_rule_id`、`has_query`、`sample_query` を持つ。
  - 同じ `(client_ip, uri_path)` の行をソース別の内訳を保ったままマージする。
  - `source_key` が異なる行は、同じ IP と URI であってもリソース別の内訳を保持する。
  - `Attribution` が false のソースは、ほかのソースと同時にマージできない。
    単独で選ばれた場合だけ、`source_key` を空のまま 1 つの行にまとめる。
  - WAF はリクエスト数を加算せず、`blocked_count`、`challenged_count`、`count_matched` だけを加算する。
  - CloudFront の origin の ALB は、CloudFront が選ばれている場合に `request_count`、`client_error_count`、`server_error_count` を IP の集計に加算しない。
  - 並び順は `blocked_count`、`challenged_count`、`client_error_count`、`request_count`、`last_seen` の降順、`client_ip` と `uri_path` の昇順とする。
- センシティブパスの判定は `IsSensitivePath` で行う。
  判定は絞り込みの材料であり、悪意の判定ではない。
- `has_query` と `sample_query` は、URI のクエリ文字列から作る。
  `sample_query` は最大 512 バイトに切り詰める。
- この issue では AWS API の呼び出し、API のハンドラ、フロントエンドを含めない。

## 完了条件

- `backend/internal/accesslog` に、ソース、出力先、スキーマプロファイル、列の対応付け、期間、フィルタ、集計行、マージ、DDL、SQL ビルダーの型と関数がある。
- スキーマプロファイルの判定が、ALB、CloudFront legacy、CloudFront v2 JSON、WAF の列名の組に対してテストされている。
- 必須列の欠落が、どの列が足りないかを示すエラーになる。
- Athena の集計 SQL が、ALB、CloudFront legacy、CloudFront v2 JSON、WAF の各プロファイルについてゴールデン文字列でテストされている。
- SQL の実行パラメータの順序、期間の上限と下限、パーティションの述語、ソースの絞り込み、LIMIT がテストされている。
- 複数の ALB、CloudFront、WAF が同じ Athena テーブルまたは同じ CloudWatch Logs のロググループを共有する場合に、`source_key` で絞り込み、`GROUP BY source_key` でリソースごとの行を返す SQL がテストされている。
- `Attribution` が false のソースを、ほかのソースと同時に集計しようとするとエラーになることがテストされている。
- ログの設定が無いソースが `destination_type=none`、`logging_enabled=false` として扱われ、集計の対象にならないことがテストされている。
- 利用者が入力した IP と URI が SQL に連結されず、実行パラメータに入ることがテストされている。
- Logs Insights のクエリが WAF のプロファイルについてテストされ、引用符とバックスラッシュのエスケープがテストされている。
- DDL が ALB、CloudFront legacy、CloudFront v2 JSON、WAF の各プロファイルについて生成され、パーティション投影が含まれることがテストされている。
- `Merge` が WAF のリクエスト数を加算せず、CloudFront の origin の ALB を IP の集計から外し、ソース別の内訳を保つことがテストされている。
- センシティブパス、IPv6 の正規化、クエリ文字列の切り詰め、空の結果のテストがある。
- `mise run backend:test` が通る。
- `mise run check` が通る。

## 関連

- ADR 0037
- issue 0236、0237、0238、0239、0240
- `backend/internal/sqlguard/sqlguard.go`
- `backend/internal/aws/athena.go`、`backend/internal/aws/cloudwatchlogs.go`
