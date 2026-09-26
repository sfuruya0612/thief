# backend の一覧取得に残る直列の AWS API 呼び出しを並列化する

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 背景

TODO.md の次の項目のうち、「backend は処理速度の向上を目指す」の部分に対応する。

> Frontend, Backend の処理の最適化をしたい。
> - 具体的には処理の共通化、frontend はキャッシュの利用頻度をあげ API コールによるユーザの体験向上、backend は処理速度の向上を目指す

処理の共通化は docs/issues/0078、frontend のキャッシュ利用は docs/issues/0080 で扱う。

「一覧 API で ID を集め、各アイテムに対して詳細 API を直列に呼ぶ」という構造は、Cloud Run (docs/issues/closed/0043、44913ms から 6317ms へ短縮) と S3 (docs/issues/closed/0067) で `errgroup` による並列化が済んでいる。
同型の直列構造が次の 5 箇所に残っている。
括弧内は詳細呼び出しの失敗時の現状の扱いで、並列化後もこの区別を変えない。

- `backend/internal/aws/kinesis.go` の `ListKinesisResources`: ストリームごとに `DescribeStreamSummary` (失敗は全体エラー) を直列 (N 本)。
- `backend/internal/aws/dynamo.go` の `ListDynamoResources`: テーブルごとに `DescribeTable` (全体エラー) と `ListTagsOfResource` (無視) を直列 (2N 本)。
- `backend/internal/aws/sqs.go` の `ListSQSResources`: キューごとに `GetQueueAttributes` (全体エラー) と `ListQueueTags` (無視) を直列 (2N 本)。
- `backend/internal/aws/waf.go` の `listWAFACLs`: Web ACL ごとに `GetWebACL` (全体エラー) と `ListTagsForResource` (無視)、REGIONAL は加えて `ListResourcesForWebACL` (無視) を直列 (REGIONAL は 3N 本、CLOUDFRONT は 2N 本)。さらに `ListWAFResources` が REGIONAL と CLOUDFRONT の 2 スコープを直列に取得している。
- `backend/internal/aws/iam.go` の `ListIAMResources`: ユーザごとに `ListMFADevices` / `ListGroupsForUser` / `ListAttachedUserPolicies` の 3 本、ロールごとに `ListAttachedRolePolicies` の 1 本を直列。この 4 本はすべて失敗が無視される (`err == nil` ガード)。他の 4 箇所と違い、ページネーションのループ内で直接詳細を呼んでいるため、並列化には「先に全件を収集してから詳細を取る」形への再構成が要る。

前例 (docs/issues/closed/0043, 0067) は 1 サービス 1 issue だが、本 issue は 5 箇所を束ねる。
束ねる理由は、ゴールが「全箇所を同一の並列イディオムへ収斂させる」という単一の設計判断であり、分割すると同じ設計判断を各 issue に複製するだけになるため。
進捗と失敗の切り分けは、完了条件と計測の記録をサービス単位にすることで担保する。
iam の再構成は一段大きい変更になるが、ゴールが同じイディオムへの収斂であることから含める。
実装中に iam の再構成が他の 4 箇所と独立した設計判断を要する規模になった場合は、iam だけを別 issue に切り出す。
切り出しの判断と理由は本 issue に記録する。
1 変更で複数の直列箇所を束ねた前例には、BigQuery のデータセットとテーブル一覧の並列化 (CHANGES.md の `## develop` に記載) がある。
AWS 以外の backend (gcp / datadog / tidb / bigquery) には同型の per-item 直列構造は残っていない (gcp の Cloud Run は docs/issues/closed/0043 で、BigQuery のデータセットとテーブル一覧は上記の変更で並列化済み) ため対象外。
`backend/internal/aws/apigw.go` の REST API と v2 の 2 系統の直列収集は、per-item の詳細呼び出しを持たず本 issue の同型構造に当たらないため対象外とする。
WAF の 2 スコープ直列も per-item の同型構造ではないが、同一ファイルの per-item 改修に付随するため例外的に含める。
apigw の 2 系統の並列化は、効果が見込まれる場合に別途起票する。
`backend/internal/aws/ecs.go` の `DescribeClusters` は、`ecsDescribeClustersBatchSize` 件ずつのバッチ API へのチャンク直列で、呼び出し回数がアイテム数に比例しないため per-item の同型構造に当たらず対象外とする。
`backend/internal/aws/cloudwatchlogs.go` の `FilterLogEvents` のロググループ直列ループは、リソース一覧 API ではなくログ検索の経路のため対象外とする。
CLI 専用経路の同型ループ (`ListIAMUserInfos`、`runEC2List` の `--global` など) は、API サーバの応答時間に影響しないため本 issue では扱わない。
`GetSession` が毎回 `config.LoadDefaultConfig` を呼ぶ問題は、SSO 再ログインとの安全性検証が要るため docs/issues/pending/0083 に分離した。

## 目的

5 箇所に残る同型の直列構造を、S3 / Cloud Run と同じ並列イディオムに揃え、リソース件数が多いプロファイルでの一覧 API の応答時間 (キャッシュ MISS 時) の短縮余地を潰す。

## 計測手段

docs/issues/closed/0041 と同様に、対象の各 `ListXResources` へ `log/slog` の区間計測 (クライアント生成、一覧収集、詳細取得ループ、全体) を追加し、実環境で before / after を採取する。
クライアント生成 (`GetSession`) の区間は docs/issues/pending/0083 の調査の入力を兼ねる。
計測ログは docs/issues/closed/0041 の前例に合わせ `slog.Info` とする。
対象関数は CLI からも呼ばれるため CLI 実行時にも stderr に出るが、docs/issues/closed/0041 (Cloud Run) と同じ挙動として許容する。
実環境で採取できない場合は、docs/issues/closed/0067 の前例 (docs/issues/closed/0043 と同型の構造という根拠による並列化) に従って並列化を行い、区間計測ログは残して数値の採取は実環境の利用時に行う。
どちらで close したかを本 issue に記録する。
なお同じ TODO から分割した docs/issues/0080 は候補ごとの採否を計測で判定するが、本 issue の並列化は仮説扱いにしない。
N 本の直列呼び出しは件数に比例して確実に遅く、同型構造の実測 (docs/issues/closed/0043) と前例 (docs/issues/closed/0067) が既にあるため。

## 設計判断

- 並列化は `backend/internal/aws/s3.go` の `ListS3Resources` のイディオムに揃える: `errgroup.WithContext` + `SetLimit`、結果は先に全件を収集してから事前確保したスライスへのインデックス書き込みとし、共有スライスへの `append` や共有マップへの書き込みを行わない。この構造により返却順は並列化前と同じに保たれる (構造はコードレビューで確認する)。事前確保方式により 0 件時の返り値は nil スライスから空スライスに変わり、JSON レスポンスが `null` から `[]` に変わるが、frontend は `apiGetList` (`frontend/src/api/client.ts`) が `?? []` で正規化済みで、CLI は空ループになるため実害はないと判断して許容する。
- エラー伝播の意味論は背景の括弧書きの分類 (全体エラー / 無視) を維持する。並列化により、全体エラーになる場合に「どのアイテムのエラーが返るか」は非決定的になるが、これは許容する。
  - `errgroup` のコンテキスト (`gctx`) がキャンセルされた場合は全体エラーとして返し、詳細が欠けた結果を `serveCached` のキャッシュに書き込ませない。失敗を無視する呼び出しでも、キャンセル起因の欠損は「タグ空」などの正常データと区別できないままキャッシュ TTL の 1 時間表示され続けるため、この扱いは 5 箇所すべてで共通にする。
  - iam の `iamFromUser` / `iamFromRole` は `error` を返すシグネチャだが実際は常に nil を返すデッドコードになっている。再構成時に「エラーを返さない」実態に合わせてシグネチャを整理する (挙動は変えない)。
  - 無視される失敗には `slog.Warn` を追加する。スロットリングが発生したとき、現状はエラーではなく「タグ空」「MFA なし」などの欠損データとして表示に出てしまい、観測する手段がないため。Warn を出すかの判定は純関数 `shouldWarnIgnoredErr(err error) bool` に切り出し、`context.Canceled` / `context.DeadlineExceeded` を除外する (`errgroup` のキャンセル連鎖で 1 リクエストに大量の Warn が出るのを避ける)。
- 並列度はサービスごとのファイル内定数にする (`s3BucketConcurrency = 30` と同形式)。初期値は kinesis / dynamo / sqs を 30、waf / iam を 10 とする。wafv2 と IAM のマネジメント系 API は他よりレート制限が厳しいため控えめに始め、スロットリング (slog.Warn の観測) に応じて調整する。リトライは SDK 標準のリトライに任せる。
  - 却下案: 全サービス共通の並列度設定。サービスごとにレート制限が異なり、個別に調整できる方が安全なため却下。
- WAF の 2 スコープ (REGIONAL / CLOUDFRONT) の取得も並列にする。クライアントは現状どおり 2 つ生成する (`aws.Config` の共有は docs/issues/pending/0083 の調査に委ねる)。
- ページング (`NextToken` / `NextMarker` を辿るループ) 自体は並列化しない。トークンが前ページの結果に依存するため本質的に直列である。
- テストの扱いは docs/issues/closed/0067 に合わせる: 並列パスを検証する新規テストは追加せず、正しさは「事前確保スライスへのインデックス書き込み」の構造 (コードレビューで確認) と `-race` 付きテストの通過で担保する。`-race` は既存テストが並列パスを実行する場合にしか効かないため、担保の主体はコードレビューである。`shouldWarnIgnoredErr` は純関数のためテーブル駆動テストを書く。
- 「ID 収集、詳細取得、変換」構造の共通ヘルパー化 (docs/issues/0078 から送られた論点) は、並列化で全対象を同一イディオムに揃えた結果として重複が明白に残る場合のみ検討し、採用する場合は本 issue では実装せず別 issue を起票する。採否の判断を本 issue に記録する。

## 完了条件

- kinesis / dynamo / sqs / waf / iam の 5 箇所それぞれについて、詳細取得が `errgroup.WithContext` + `SetLimit` で並列化され、並列度がファイル内定数で定義され、各 goroutine が事前確保されたスライスの自分のインデックスにのみ書き込む構造であることをコードレビューで確認し、結果をサービスごとに本 issue に記録する。0 件時のレスポンスが `null` から `[]` に変わること (許容の判断は設計判断に記載) も同レビューの確認項目に含める。
- WAF の REGIONAL / CLOUDFRONT の 2 スコープが並列に取得される。
- 詳細呼び出しの失敗時の挙動が背景の分類 (全体エラー / 無視) と一致する。どのアイテムのエラーが返るかが非決定的になることは許容する。
- `shouldWarnIgnoredErr` にテーブル駆動テストがあり、`context.Canceled` / `context.DeadlineExceeded` が対象外になることを検証している。
- `shouldWarnIgnoredErr` が真のとき `slog.Warn` を出す配線をコードレビューで確認し、本 issue に記録する。
- 対象の各 `ListXResources` に slog の区間計測 (クライアント生成、一覧収集、詳細取得ループ、全体) が追加されている。
- 実環境で計測した場合は before / after の区間ごとの数値 (クライアント生成、一覧収集、詳細取得ループ、全体) がサービスごとに本 issue に記録されている。計測できなかった場合は、その旨と同型構造を根拠としたことが記録されている。
- 共通ヘルパー化 (docs/issues/0078 からの申し送り) の採否と理由が本 issue に記録されている。採用する場合は別 issue が起票されている。
- iam を別 issue に切り出した場合は、切り出した issue の番号と理由が本 issue に記録され、上記の完了条件は残る 4 箇所に適用される。
- CLI 専用経路の直列ループと `GetSession` のキャッシュ化 (docs/issues/pending/0083) は本 issue では扱わない。
- `CHANGES.md` の `## develop` に `[UPDATE]` エントリと担当者行が記載されている。
- `mise run check` が通過する (`mise run backend:test` は `-race` を含む)。

## 未確定論点

- 並列度の初期値 (kinesis / dynamo / sqs は 30、waf / iam は 10) は `s3BucketConcurrency = 30` と API のレート制限の相対比較から置いた出発点で、実測の裏付けはない。実装後にスロットリング (`slog.Warn`) の観測に応じて調整する。

## 関連

- docs/issues/0078 / docs/issues/0080: 同じ TODO 項目からの分割。本 issue は backend の応答時間のみを扱う。
- docs/issues/0074 / docs/issues/0075: どちらも `backend/internal/aws/waf.go` を触る。番号順 (docs/issues/0074 → docs/issues/0075 → 本 issue) に実装すれば衝突しない。
- docs/issues/pending/0083: `GetSession` の `aws.Config` キャッシュ化の調査。本 issue から分離した。本 issue の区間計測 (クライアント生成) が調査の入力になる。
- docs/issues/closed/0041, 0043, 0067: 区間計測と並列化の前例。イディオムと計測手法の出典。

## 解決方法

5 箇所すべてを `ListS3Resources` と同一の並列イディオム (`errgroup.WithContext` + `SetLimit` + 事前確保スライスへのインデックス書き込み) に収斂させた。iam の別 issue への切り出しは行わなかった (再構成は「ページング内の詳細呼び出しを、先に全件収集してから詳細を取る形に分離する」だけで、他の 4 箇所と独立した設計判断は発生しなかったため)。

### 共通基盤 (backend/internal/aws/errors.go)

- `shouldWarnIgnoredErr(err error) bool` を純関数として追加。nil と `context.Canceled` / `context.DeadlineExceeded` (ラップ含む、`errors.Is` で判定) を対象外とする。`backend/internal/aws/errors_test.go` の `TestShouldWarnIgnoredErr` にテーブル駆動テストを追加 (nil / canceled / deadline exceeded / `%w` ラップ 2 種 / 通常エラー / API エラーの 7 ケース)。
- `handleIgnoredErr(err, msg, attrs...)` を追加し、「失敗を無視する」詳細呼び出しの処理を 1 箇所に集約した。キャンセル起因のエラーはそのまま返して全体エラーとして伝播させ (欠損データのキャッシュ書き込み防止)、それ以外は `slog.Warn(msg, ..., "err", err)` を出して nil を返す。`shouldWarnIgnoredErr` が真のとき Warn を出す配線はこの関数内にあり、全呼び出し箇所 (dynamo タグ、sqs タグ、waf の ListResourcesForWebACL とタグ、iam の 4 詳細呼び出し) がこの経路を通ることをレビューで確認した。

### サービスごとのコードレビュー結果

- kinesis (`kinesisStreamConcurrency = 30`): `DescribeStreamSummary` を並列化。失敗は全体エラー (従来どおり)。事前確保 `make([]KinesisResource, len(names))` に `resources[i]` で書き込み、共有 append なし。返却順は一覧順を維持。0 件時は `[]` になる (許容済み)。
- dynamo (`dynamoTableConcurrency = 30`): `DescribeTable` (全体エラー) + `ListTagsOfResource` (無視、`handleIgnoredErr` 経由) を並列化。インデックス書き込み・返却順維持・0 件時 `[]` を確認。
- sqs (`sqsQueueConcurrency = 30`): `GetQueueAttributes` (全体エラー) + `ListQueueTags` (無視、`handleIgnoredErr` 経由) を並列化。インデックス書き込み・返却順維持・0 件時 `[]` を確認。
- waf (`wafACLConcurrency = 10`): `listWAFACLs` 内で `GetWebACL` (全体エラー)、`ListResourcesForWebACL` (REGIONAL のみ、無視)、`ListTagsForResource` (無視) を並列化。さらに `ListWAFResources` で REGIONAL / CLOUDFRONT の 2 スコープを errgroup で並列取得し、結果はスコープ別の変数に受けて REGIONAL → CLOUDFRONT の順で結合するため返却順は従来と同じ。クライアントは従来どおり 2 つ生成 (aws.Config 共有は docs/issues/pending/0083 に委ねる)。0 件時 `[]` を確認。
- iam (`iamDetailConcurrency = 10`): ページング内で詳細を呼ぶ構造を「ユーザー・ロールの全件をページングで収集 → `make([]IAMResource, len(users)+len(roles))` へユーザーは `resources[i]`、ロールは `resources[len(users)+i]` で並列書き込み」に再構成。返却順 (ユーザー一覧順 → ロール一覧順) を維持。4 本の詳細呼び出し (`ListMFADevices` / `ListGroupsForUser` / `ListAttachedUserPolicies` / `ListAttachedRolePolicies`) はすべて `handleIgnoredErr` 経由の無視 (従来の `err == nil` ガードと同じ意味論 + Warn 観測)。0 件時 `[]` を確認。

### 設計判断の記録

- iamFromUser / iamFromRole のシグネチャ整理 (設計判断に記載した「常に nil を返すデッドコードの整理」) は行わず、`(IAMResource, error)` を維持した。キャンセル起因の失敗を全体エラーとして伝播させる方針 (5 箇所共通) により error 戻り値が生きたコードになったため、前提が消滅した。この判断は設計判断のエラー伝播の項 (キャンセルは全体エラー) を優先した結果である。
- 計測: 実環境 (AWS 認証) は本セッションで利用できないため、docs/issues/closed/0067 の前例に従い同型構造 (docs/issues/closed/0043 で実測 44913ms → 6317ms) を根拠として並列化した。5 箇所すべてに slog.Info の区間計測 (クライアント生成 / 一覧収集 / 詳細取得ループ (concurrency 属性付き) / 全体) を追加済みで、数値の採取は実環境の利用時に行える。
- 共通ヘルパー化 (docs/issues/0078 からの申し送り): 不採用。並列化後も各サービスの詳細取得は「呼ぶ API の本数 (1〜4 本)、エラー分類 (全体/無視) の組み合わせ、結果型」がすべて異なり、共通化すると型パラメータとコールバックの層が増えて可読性を損なう。共通化に値する重複 (無視エラーの処理) は `handleIgnoredErr` として切り出し済みで、残る重複は errgroup の 6 行の定型のみであり、ヘルパー化のコストに見合わない。別 issue の起票もしない。
- `CHANGES.md` の `## develop` に `[UPDATE]` エントリと担当者行を追記した。
- `mise run check` (backend:test は `-race` 付き) の通過を確認した。
