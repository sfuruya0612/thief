# TUI の一覧と Drawer に ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS を足す

Created: 2026-10-25
Model: Claude Fable 5.1

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の一覧と Drawer を EC2 以外の 18 サービス (Lambda、ECR、ECS、S3、RDS、DynamoDB、ElastiCache、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS、IAM、WAF、Secrets Manager、Parameter Store、CloudFormation) にも足したい (docs/prd/thief.md の FR-29。`backend/internal/cli/tui.go` の `Deps.Services` には `tui.NewEC2Service` しか無く、列と Overview は Web UI の `frontend/src/components/tables/columns.tsx` と `frontend/src/components/Drawer/overviewRows.tsx` に合わせる)

18 サービスは Web UI のサイドバーの分類 (`frontend/src/lib/serviceMeta.ts` の `SERVICES` の `group`) に沿って 4 つの issue に分け、タグの取得の仕組みが他と違う CloudFormation はさらに 1 つの issue に分ける。
この issue は `networking` の ELB、CloudFront、API Gateway、NAT Gateway、`analytics` の Kinesis、`integration` の SQS を扱う。
Lambda、ECR、ECS は issue 0219、S3、RDS、DynamoDB、ElastiCache は issue 0220、IAM、WAF、Secrets Manager、Parameter Store は issue 0222、CloudFormation は issue 0223 で扱う。

TUI の 1 サービスは `backend/internal/tui/service.go` の `Service` と `Row` で表す。
実装の形、`Deps` の組み立て、数値の換算 (`backend/internal/tui/format.go` の `formatCount`)、タグの取得失敗の表示 (`TagsFetchFailer`) は issue 0218、0219、0220 が定めたものに従う。

6 サービスの Web UI 側の事実は次のとおり。
一覧関数は Web UI の一覧のハンドラ (`backend/internal/api/handlers_aws.go`) が呼ぶものである。

| ID | 一覧関数 (`backend/internal/aws/`) | 列 (`columns.tsx`) | Overview (`overviewRows.tsx`) | Tags |
| --- | --- | --- | --- | --- |
| `elb` | `ListELBResources(ctx, profile, region)` | `elbColumns`: Name、State、Type、Scheme、Region、DNS name、AZs | `elbOverviewRows`: Resource ID、Type、Scheme、DNS name、VPC、AZs、Region | `ELBResource` に Tags フィールドが無い |
| `cloudfront` | `ListCloudFrontResources(ctx, profile, _)` (リージョンを無視し us-east-1 で呼ぶ) | `cloudfrontColumns`: Distribution、State、Domain、Alternate domains、Origins | `cloudfrontOverviewRows`: Resource ID、Domain、Alternate domains、Origins、Enabled、Price class | `CloudFrontResource` に Tags フィールドが無い |
| `apigw` | `ListAPIGatewayResources(ctx, profile, region)` | `apigwColumns`: API、State、Type、Stage、Endpoint、Region | `apigwOverviewRows`: Resource ID、Type、Stage、Endpoint、Region | `APIGatewayResource.Tags` |
| `natgw` | `ListNATGatewayResources(ctx, profile, region)` | `natgwColumns`: Name、State、Gateway ID、VPC、Elastic IP、Region、Uptime | `natgwOverviewRows`: Resource ID、VPC、Subnet、Elastic IP、Region、Uptime、Launched | `NATGatewayResource.Tags` |
| `kinesis` | `ListKinesisResources(ctx, profile, region)` | `kinesisColumns`: Stream、State、Mode、Shards、Retention、Encryption、Region | `kinesisOverviewRows`: Resource ID、Capacity mode、Shards、Retention、Encryption、Region | `KinesisResource.Tags` はあるが `ListKinesisResources` が設定せず常に空 |
| `sqs` | `ListSQSResources(ctx, profile, region)` | `sqsColumns`: Queue、State、Type、Available、In flight、Retention、Region | `sqsOverviewRows`: Resource ID、Type、Messages available、Messages in flight、Retention、Region | `SQSResource.Tags` と `TagsFetchFailed` |

- ELB の AZs は `, ` で結合した文字列、Type は `application`、`network`、`gateway` の生の値、`ID` はロードバランサの ARN である。
- CloudFront は列にも Overview にも Region が無い。`Name` は `Comment` で、空なら `Id` である。Alternate domains と Origins は `, ` で結合する。
- API Gateway の Type は `REST`、`HTTP`、`WebSocket` で、REST の Endpoint は `https://<id>.execute-api.<region>.amazonaws.com` を組み立てた文字列である。どちらも一覧関数が作る。
- NAT Gateway の `Name` はタグ `Name` の値 (無ければ空) で、Uptime と Launched は `LaunchTime` から算出する (`ec2.go` の `formatUptime` と `formatLaunched`)。
- Kinesis の Mode は `on-demand` か `provisioned` で、Retention は列と Overview のどちらも `24h` のような時間表記である。
- SQS の Queue 名はキューの URL の末尾、Available は `toLocaleString()` の桁区切りで、Retention は列では `4d`、Overview では `4 days` と表記が違う。タグは一覧関数の内部で `ListQueueTags` を呼んで取り、失敗したときは `TagsFetchFailed` を真にする。
- Web UI が空のとき `Dash` か `dash` を描画する項目は、ELB が列と Overview の AZs、CloudFront が列と Overview の Alternate domains と Origins、NAT Gateway が列の Uptime と Overview の Uptime と Launched である。API Gateway、Kinesis、SQS には無い。ELB の DNS name、API Gateway の Endpoint と Stage、NAT Gateway の VPC と Elastic IP、CloudFront の Domain、Kinesis の Encryption は空のとき空文字列である。CloudFront の Overview の Enabled は `yes` か `no` である。
- ELB の `active`、NAT Gateway の `available`、Kinesis の `active` (`kinesisFromSummary` が `DisplayState` で小文字にする) のような State の生の値は `NormalizeState` で `running` になるため、`ResourceState()` の値を表示すると Web UI と食い違う。

## 目的

`thief tui` のサービスの選択肢に ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS が並び、Web UI と同じ列の一覧と、同じ項目の Overview と Tags を Drawer で見られる。
SQS のタグの取得に失敗した行では、Web UI と同じくその旨が Drawer に出る。

## 設計判断

- サービスごとに `backend/internal/tui/elb.go`、`cloudfront.go`、`apigw.go`、`natgw.go`、`kinesis.go`、`sqs.go` を置き、`New<Service>Service(list)` が一覧関数を受け取る。`Columns()`、`Overview()`、`Cells()`、`Load` の失敗のラップ、`Deps.Services` の並び (`serviceMeta.ts` の順: issue 0220 までの 8 個に `elb`、`cloudfront`、`apigw`、`natgw`、`kinesis`、`sqs` を続ける) は issue 0219 と 0220 と同じ方針に従う。`State()` と State の列は資源の `State` フィールドの生の値を返し、空の値と真偽値の表記も issue 0219 の規則に従う。
- SQS の列の Retention (`4d`) と Overview の Retention (`4 days`) は、Web UI の `sqsColumns` と `sqsOverviewRows` がそれぞれ描画する文字列に合わせて別々に作る。1 つの表記に統一する案は、Web UI との一致 (FR-29 の受け入れ基準) から外れるので採らない。
- SQS の行は issue 0220 の `TagsFetchFailer` を実装し、`SQSResource.TagsFetchFailed` を返す。Available は `formatCount(int64(r.AvailableMessages))` で桁区切りにする (`SQSResource.AvailableMessages` は `int`)。
- ELB と CloudFront は `Row.Tags()` が `nil` を返し、Drawer は `No tags` と表示する。Kinesis の `Row.Tags()` は `KinesisResource.Tags` をそのまま返す (一覧関数が設定しないため常に `No tags` になる)。Web UI の Tags タブも同じく空であり、この issue は Web UI との一致を目的とするので、一覧関数にタグの取得を足すことはしない。
- CloudFront の一覧関数はリージョンを無視するが、TUI は他のサービスと同じく選択中のリージョンを渡す。CloudFront の列と Overview に Region を足す案は、Web UI に無い項目を増やすので採らない。
- 追加の API 呼び出しと権限は無い。TUI は Web UI の一覧のハンドラと同じ `ListELBResources`、`ListCloudFrontResources`、`ListAPIGatewayResources`、`ListNATGatewayResources`、`ListKinesisResources`、`ListSQSResources` を使う。
- floci での確認: `example/seed.sh` は SQS のキューを 3 つ (`thief-example-queue`、`thief-example-dlq`、`thief-example-fifo.fifo`) 作り、`example/README.md` の「確認手順」は SQS の一覧を Web UI で確認している。SQS は floci で確認し、README の「確認手順」に TUI での確認を足す。他の 5 サービスは README の確認済みの一覧に無いので実 AWS で確認し、floci でも試して動いたものは seed と README に足す。
- `CHANGES.md` の `## develop` に、この issue の 6 サービスを足した [ADD] のエントリを 1 つ書く (issue 0219 の方針)。

## 完了条件

- `backend/internal/tui/elb.go`、`cloudfront.go`、`apigw.go`、`natgw.go`、`kinesis.go`、`sqs.go` に `NewELBService`、`NewCloudFrontService`、`NewAPIGatewayService`、`NewNATGatewayService`、`NewKinesisService`、`NewSQSService` があり、`ID()` がそれぞれ `elb`、`cloudfront`、`apigw`、`natgw`、`kinesis`、`sqs`、`Title()` が `serviceMeta.ts` の `SERVICES` の `name` (`ELB`、`CloudFront`、`API Gateway`、`NAT Gateway`、`Kinesis`、`SQS`) と一致する。
- 6 サービスそれぞれの `backend/internal/tui/<id>_test.go` が、issue 0219 と同じ項目 (`Columns()` の見出しと `<id>Columns` の `header` の一致、`Overview()` の項目名と `<id>OverviewRows` の一致、issue 0219 の 4 種の資源のうち該当するもの (すべてに値がある、任意の項目が空、タグが無い、の 3 種は 6 サービスすべて。真偽値が偽は CloudFront の Enabled だけ) の `Cells()` と `Overview()` の文字列の一致、`Cells()` と `Columns()` の個数の一致、偽の一覧関数を渡した `Load` の配線とエラーのラップ) を確認する。空の項目は、ELB の AZs、CloudFront の Alternate domains と Origins、NAT Gateway の列の Uptime と Overview の Uptime と Launched だけ `—` になり、他 (ELB の DNS name、API Gateway の Endpoint、NAT Gateway の Elastic IP など) は空文字列になる。CloudFront の Enabled が偽の資源で Overview が `no` になる。ELB の `State()` と State の列は、`ELBResource.State` が `active` の資源で `active` のままである。SQS のテストは、列の Retention が `4d`、Overview の Retention が `4 days` になる資源を含む。NAT Gateway のテストは、`Name` タグの無い資源で `Name()` が空になることを含む。
- SQS の行が `TagsFetchFailer` を実装し `SQSResource.TagsFetchFailed` を返すこと、ELB と CloudFront の `Row.Tags()` が `nil` を返すこと、API Gateway、NAT Gateway、Kinesis の `Row.Tags()` が資源の `Tags` を返すことをテストで確認する。
- `backend/internal/cli/tui.go` の `tui.Deps` を組み立てる関数の `Services` が `serviceMeta.ts` の順で並び、その `ID()` の集合が issue 0220 までの 8 個にこの 6 個を足した 14 個であることをテストで確認する。各サービスに渡す一覧関数が `handlers_aws.go` の同じサービスのハンドラが呼ぶ関数と同じであることをコードの読み合わせで確認し、結果を「## 動作確認」に書く。
- floci (`mise run example:up` と `mise run example:seed`) で `HOME="$(pwd)/example/home" thief tui -p floci` を起動し、SQS の一覧に `thief-example-queue`、`thief-example-dlq`、`thief-example-fifo.fifo` が表示され、`thief-example-fifo.fifo` の Type が Web UI と同じ表記であることを確認して「## 動作確認」に記録する。`example/README.md` の「確認手順」に TUI で SQS を確認する項がある。
- `CHANGES.md` の `## develop` に、この issue の [ADD] のエントリが 1 つある。
- 開発者が実 AWS で ELB、CloudFront、API Gateway、NAT Gateway、Kinesis の一覧と Drawer を確認し、結果を「## 動作確認」に記録する。確認できなかった項目は、その旨と理由を書く。
- `mise run check` が通る。
- 扱わない範囲: ELB の Listeners と Targets タブ、CloudFront の Behaviors タブ (第 1 段階では扱わない。扱うかは PRD の未確定論点「TUI の Drawer のサービス固有のタブ」で決める)、`ListELBResources`、`ListCloudFrontResources`、`ListKinesisResources` へのタグの取得の追加、他の 12 サービス (issue 0219、0220、0222、0223)。

## 関連

- docs/prd/thief.md の FR-29
- issue 0218: `Service` と `Row` のインターフェース、`Deps` の組み立ての関数、`Load` の配線のテストの形
- issue 0219: サービスのファイルの形、`Deps.Services` の並び、`Cells()` の文字列の方針
- issue 0220: `format.go` の `formatCount` と `TagsFetchFailer` をこの issue の SQS が使う
- issue 0222 と 0223: 同じ TODO の項目から分けた残りの 5 サービス
