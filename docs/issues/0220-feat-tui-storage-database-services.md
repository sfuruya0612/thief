# TUI の一覧と Drawer に S3、RDS、DynamoDB、ElastiCache を足す

Created: 2026-10-25
Model: Claude Fable 5.1

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の一覧と Drawer を EC2 以外の 18 サービス (Lambda、ECR、ECS、S3、RDS、DynamoDB、ElastiCache、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS、IAM、WAF、Secrets Manager、Parameter Store、CloudFormation) にも足したい (docs/prd/thief.md の FR-29。`backend/internal/cli/tui.go` の `Deps.Services` には `tui.NewEC2Service` しか無く、列と Overview は Web UI の `frontend/src/components/tables/columns.tsx` と `frontend/src/components/Drawer/overviewRows.tsx` に合わせる)

18 サービスは Web UI のサイドバーの分類 (`frontend/src/lib/serviceMeta.ts` の `SERVICES` の `group`) に沿って 4 つの issue に分け、タグの取得の仕組みが他と違う CloudFormation はさらに 1 つの issue に分ける。
この issue は `storage` の S3 と `database` の RDS、DynamoDB、ElastiCache を扱う。
Lambda、ECR、ECS は issue 0219、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS は issue 0221、IAM、WAF、Secrets Manager、Parameter Store は issue 0222、CloudFormation は issue 0223 で扱う。

TUI の 1 サービスは `backend/internal/tui/service.go` の `Service` と `Row` で表し、実装の形と `Deps` の組み立ては issue 0218 と 0219 が定めたものに従う。
手本は `backend/internal/tui/ec2.go` の `NewEC2Service` と `ec2Row` である。
`Row.Tags()` は `map[string]string` を返し、Drawer の描画 (`backend/internal/tui/model.go` `refreshDrawer`) は空のとき `No tags` と表示する。

4 サービスの Web UI 側の事実は次のとおり。
一覧関数は Web UI の一覧のハンドラ (`backend/internal/api/handlers_aws.go`) が呼ぶものである。

| ID | 一覧関数 (`backend/internal/aws/`) | 列 (`columns.tsx`) | Overview (`overviewRows.tsx`) | Tags |
| --- | --- | --- | --- | --- |
| `s3` | `ListS3Resources(ctx, profile, _)` (リージョンを無視し `ListBuckets` を us-east-1 で呼ぶ) | `s3Columns`: Bucket、State、Region、Created、Public、Encryption | `s3OverviewRows`: Resource ID、Region、Created、Public access、Encryption | `S3Resource` に Tags フィールドが無い |
| `rds` | `ListRDSResources(ctx, profile, region)` | `rdsColumns`: Identifier、State、Engine、Engine Version、Cluster、Class、Region、MultiAZ、Endpoint、Uptime | `rdsOverviewRows`: Resource ID、Engine、Engine version、Cluster、Class、Multi-AZ、Endpoint、Port、VPC、Region、Uptime、Launched | `RDSResource.Tags` |
| `dynamo` | `ListDynamoResources(ctx, profile, region)` | `dynamoColumns`: Table、State、Mode、Items、Size、GSI、Region | `dynamoOverviewRows`: Resource ID、Capacity mode、Item count、Size、Global secondary indexes、Region | `DynamoResource.Tags` と `TagsFetchFailed` |
| `cache` | `ListElastiCacheResources(ctx, profile, region)` | `cacheColumns`: Cluster、State、Engine、Replication Group、Node type、Region、AZs、Nodes、Endpoint | `cacheOverviewRows`: Resource ID、Engine、Engine version、Replication group、Node type、Nodes、AZs、Endpoint、Port、Region | `ElastiCacheResource` に Tags フィールドが無い |

- S3 の Region の列と Overview の Region はバケット固有の `S3Resource.Region` である (`frontend/src/lib/normalize.ts` の `s3FromRaw` が `raw.region` を優先する)。`S3Resource.State` は `s3FromBucket` が設定しないため空で、Web UI の `s3Columns` の State は空の `StatusBadge` を表示する (`ResourceState()` は固定の `active` を返すが、これは Web UI が表示しない)。
- RDS の Uptime と Launched は EC2 と同じく起動時刻から算出する (`ec2.go` の `formatUptime` と `formatLaunched` と同じ形)。
- DynamoDB の Items は `toLocaleString()` の桁区切り (`1,234`)、Size は `columns.tsx` の `formatBytes` (`1024` 未満は `<n> B`、以上は `1024` で割って小数 1 桁と `KB`、`MB`、`GB`、`TB`) で描画する。backend にはこの換算の関数が無い。
- DynamoDB のタグは一覧関数の内部で `ListTagsOfResource` を呼んで取り、失敗したときは一覧全体を失敗にせず `TagsFetchFailed` を真にする (PRD の FR-4)。Web UI の Drawer は `frontend/src/components/Drawer/DrawerTags.tsx` が `fetchFailed` のとき見出しを `Tags (fetch failed)` (`frontend/src/i18n/locales/en/drawerAws.json` の `drawerTags.fetchFailedHeading`) にして警告アイコンを出す。TUI の `Row` にはこれを表す口が無い。
- ElastiCache の `ElastiCacheResource.ServiceName()` は `elasticache` を返すが、Web UI の ID (`serviceMeta.ts` の `SERVICE_TO_PATH` のキー、PRD の FR-29 の ID) は `cache` である。
- Web UI が空のとき `Dash` か `dash` を描画する項目は、S3 が Overview の Created と Encryption、RDS が列の Cluster と Uptime と Overview の Cluster、Uptime、Launched、ElastiCache が列の Replication Group と AZs と Overview の Replication group と AZs である。DynamoDB には無い。S3 の列の Created と Encryption、RDS と ElastiCache の Endpoint は空のとき空文字列である。
- 真偽値の表記は列と Overview で違う。RDS の MultiAZ は列が `✓` と `—`、Overview の Multi-AZ が `yes` と `no`、S3 の Public は列が `public` と `private`、Overview の Public access が `allowed` と `blocked` である。
- RDS と ElastiCache の State は `available` のような生の値で、`NormalizeState` は `available` を `running` に変えるため、`ResourceState()` の値を表示すると Web UI と食い違う。

## 目的

`thief tui` のサービスの選択肢に S3、RDS、DynamoDB、ElastiCache が並び、Web UI と同じ列の一覧と、同じ項目の Overview と Tags を Drawer で見られる。
DynamoDB のタグの取得に失敗した行では、Web UI と同じくその旨が Drawer に出る。

## 設計判断

- サービスごとに `backend/internal/tui/s3.go`、`rds.go`、`dynamo.go`、`cache.go` を置き、`New<Service>Service(list)` が一覧関数を受け取る。`Columns()`、`Overview()`、`Cells()`、`Load` の失敗のラップ、`Deps.Services` の並び (`serviceMeta.ts` の順: `ec2`、`lambda`、`ecr`、`ecs`、`s3`、`rds`、`dynamo`、`cache`) は issue 0219 と同じ方針に従う。`State()` と State の列は資源の `State` フィールドの生の値を返し、空の値と真偽値の表記も issue 0219 の規則 (Web UI が `Dash` か `dash` を描画する項目だけ `—`、真偽値は列と Overview で別々に写す) に従う。
- ElastiCache の `Service.ID()` は `cache` にする。`ServiceName()` の `elasticache` を使う案は、PRD の FR-29 の ID の一覧、`handlers_aws.go` のパス、issue 0218 の `Deps` のテストの期待値 (PRD の 19 個の ID) と食い違うので採らない。
- 数値と容量の換算は `backend/internal/tui/format.go` に `formatCount(n int64) string` (3 桁ごとの `,` 区切り) と `formatBytes(n int64) string` (`columns.tsx` の `formatBytes` と同じ規則) を置き、`format_test.go` でテストする。`toLocaleString()` はロケールに依存するが、Web UI は `en` と `ja` のどちらでも `,` 区切りになるので `,` に固定する。DynamoDB のファイルにだけ置く案は、issue 0221 の SQS (Available) でも使うので採らない。`ec2.go` の `formatUptime` と `formatLaunched` は RDS でそのまま使う。
- タグの取得失敗は、`Row` のインターフェースを変えずに示す。`backend/internal/tui/service.go` に任意のインターフェース `TagsFetchFailer` (`TagsFetchFailed() bool`) を足し、`refreshDrawer` が `Row` を型アサーションして真なら Tags タブの本文の先頭を `Tags (fetch failed)` にする (Web UI の `drawerTags.fetchFailedHeading` と同じ英語の文字列。Drawer の AWS 由来の文言は英語のままにするという docs/issues/closed/0066 の方針に従う)。`Row` に `TagsFetchFailed()` を必須で足す案は、EC2 など取得失敗を持たない 16 サービスにも空の実装を強いるので採らない。Overview の項目として `Tags: fetch failed` を出す案は、Web UI が Tags タブで示すものを Overview に移すことになるので採らない。issue 0221 の SQS と issue 0222 の WAF はこの `TagsFetchFailer` を使う。
- S3 と ElastiCache は `Row.Tags()` が `nil` を返し、Drawer は `No tags` と表示する。Web UI の Tags タブも資源に Tags フィールドが無いため空であり、この issue は Web UI との一致を目的とするので、一覧関数にタグの取得を足すことはしない。
- 追加の API 呼び出しと権限は無い。TUI は Web UI の一覧のハンドラと同じ `ListS3Resources`、`ListRDSResources`、`ListDynamoResources`、`ListElastiCacheResources` を使う。S3 の一覧関数はリージョンを無視するが、TUI は他のサービスと同じく選択中のリージョンを渡す。
- floci での確認: `example/seed.sh` は S3 のバケット 2 つと DynamoDB のテーブル 1 つを作り、`example/README.md` の「確認手順」は S3 と DynamoDB の一覧を Web UI で確認している。この 2 サービスは floci で確認する。S3 は README が backend に求めるのと同じく `THIEF_S3_PATH_STYLE=true` を `thief tui` にも設定する必要があり、README の「確認手順」に TUI での確認を足すときにこの環境変数を書く。RDS と ElastiCache は README の確認済みの一覧に無いので実 AWS で確認し、floci でも試して動いたものは seed と README に足す。
- `CHANGES.md` の `## develop` に、この issue の 4 サービスを足した [ADD] のエントリを 1 つ書く (issue 0219 の方針)。

## 完了条件

- `backend/internal/tui/s3.go`、`rds.go`、`dynamo.go`、`cache.go` に `NewS3Service`、`NewRDSService`、`NewDynamoService`、`NewCacheService` があり、`ID()` がそれぞれ `s3`、`rds`、`dynamo`、`cache`、`Title()` が `serviceMeta.ts` の `SERVICES` の `name` (`S3`、`RDS`、`DynamoDB`、`ElastiCache`) と一致する。
- 4 サービスそれぞれの `backend/internal/tui/<id>_test.go` が、issue 0219 と同じ項目 (`Columns()` の見出しと `<id>Columns` の `header` の一致、`Overview()` の項目名と `<id>OverviewRows` の一致、issue 0219 の 4 種の資源のうち該当するもの (すべてに値がある、任意の項目が空、タグが無い、の 3 種は 4 サービスすべて。真偽値が偽は RDS の MultiAZ と S3 の Public だけ) の `Cells()` と `Overview()` の文字列の一致、`Cells()` と `Columns()` の個数の一致、偽の一覧関数を渡した `Load` の配線とエラーのラップ) を確認する。空の項目は、S3 の Overview の Created と Encryption、RDS の列の Cluster と Uptime と Overview の Cluster、Uptime、Launched、ElastiCache の列の Replication Group と AZs と Overview の Replication group と AZs だけ `—` になり、他 (S3 の列の Created、RDS と ElastiCache の Endpoint など) は空文字列になる。RDS の MultiAZ が偽の資源で列が `—`、Overview が `no`、S3 の Public が偽の資源で列が `private`、Overview が `blocked` になる。RDS の `State()` と State の列は、`RDSResource.State` が `available` の資源で `available` のままである。S3 の `State()` と State の列は、`S3Resource.State` が空の資源で空文字列になる (`ResourceState()` の `active` ではない)。
- `backend/internal/tui/format.go` の `formatCount` と `formatBytes` が、`0`、`999`、`1000`、`1234567` と、`0`、`1023`、`1024`、`1536`、`1099511627776` を Web UI の `toLocaleString()` と `formatBytes` と同じ文字列 (`0`、`999`、`1,000`、`1,234,567` と `0 B`、`1023 B`、`1.0 KB`、`1.5 KB`、`1.0 TB`) にすることを `format_test.go` で確認する。
- `backend/internal/tui/service.go` に `TagsFetchFailer` があり、DynamoDB の行が `TagsFetchFailed` を `DynamoResource.TagsFetchFailed` から返す。`model_test.go` で、`TagsFetchFailed()` が真の行を選んだとき Drawer の Tags タブに `Tags (fetch failed)` が出ること、真でタグが空のとき `No tags` が出ないこと (Web UI の `DrawerTags` は取得失敗の見出しだけを出す)、偽でタグが空のとき `No tags` が出ることを確認する。
- S3 と ElastiCache の `Row.Tags()` が `nil` を返し、RDS の `Row.Tags()` が `RDSResource.Tags` を、DynamoDB の `Row.Tags()` が `DynamoResource.Tags` を返すことをテストで確認する。
- `backend/internal/cli/tui.go` の `tui.Deps` を組み立てる関数の `Services` が `ec2`、`lambda`、`ecr`、`ecs`、`s3`、`rds`、`dynamo`、`cache` の順で並び、その `ID()` の集合がこの 8 個であることをテストで確認する。各サービスに渡す一覧関数が `handlers_aws.go` の同じサービスのハンドラが呼ぶ関数と同じであることをコードの読み合わせで確認し、結果を「## 動作確認」に書く。
- floci (`mise run example:up` と `mise run example:seed`) で `HOME="$(pwd)/example/home" THIEF_S3_PATH_STYLE=true thief tui -p floci` を起動し、S3 の一覧に `thief-example-data` と `thief-example-logs`、DynamoDB の一覧に `thief-example-users` が表示され、Drawer の Overview の項目が Web UI と同じ並びであることを確認して「## 動作確認」に記録する。`example/README.md` の「確認手順」に TUI で S3 と DynamoDB を確認する項があり、`THIEF_S3_PATH_STYLE=true` を書いている。
- `CHANGES.md` の `## develop` に、この issue の [ADD] のエントリが 1 つある。
- 開発者が実 AWS で RDS と ElastiCache の一覧と Drawer を確認し、結果を「## 動作確認」に記録する。確認できなかった項目は、その旨と理由を書く。
- `mise run check` が通る。
- 扱わない範囲: S3 の Objects タブ、RDS と ElastiCache の Parameters タブ、DynamoDB の Items タブ (第 1 段階では扱わない。扱うかは PRD の未確定論点「TUI の Drawer のサービス固有のタブ」で決める)、`ListS3Resources` と `ListElastiCacheResources` へのタグの取得の追加、他の 14 サービス (issue 0219、0221、0222、0223)。

## 関連

- docs/prd/thief.md の FR-29、FR-4 (取得失敗の扱い)
- issue 0218: `Service` と `Row` のインターフェース、`Deps` の組み立ての関数、`Load` の配線のテストの形
- issue 0219: サービスのファイルの形、`Deps.Services` の並び、`Cells()` の文字列の方針。この issue はそれに `format.go` と `TagsFetchFailer` を足す
- issue 0221 (SQS): この issue の `TagsFetchFailer` と `formatCount` を使う。issue 0222 (WAF): この issue の `TagsFetchFailer` を使う
