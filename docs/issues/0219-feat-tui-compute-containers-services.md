# TUI の一覧と Drawer に Lambda、ECR、ECS を足す

Created: 2026-10-25
Model: Claude Fable 5.1

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の一覧と Drawer を EC2 以外の 18 サービス (Lambda、ECR、ECS、S3、RDS、DynamoDB、ElastiCache、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS、IAM、WAF、Secrets Manager、Parameter Store、CloudFormation) にも足したい (docs/prd/thief.md の FR-29。`backend/internal/cli/tui.go` の `Deps.Services` には `tui.NewEC2Service` しか無く、列と Overview は Web UI の `frontend/src/components/tables/columns.tsx` と `frontend/src/components/Drawer/overviewRows.tsx` に合わせる)

18 サービスは Web UI のサイドバーの分類 (`frontend/src/lib/serviceMeta.ts` の `SERVICES` の `group`) に沿って 4 つの issue に分け、タグの取得の仕組みが他と違う CloudFormation はさらに 1 つの issue に分ける。
この issue は `compute` の Lambda と `containers` の ECR、ECS を扱う。
S3、RDS、DynamoDB、ElastiCache は issue 0220、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS は issue 0221、IAM、WAF、Secrets Manager、Parameter Store は issue 0222、CloudFormation は issue 0223 で扱う。

TUI の 1 サービスは `backend/internal/tui/service.go` の `Service` (`ID`、`Title`、`Columns`、`Load`、`SupportsSession`) と `Row` (`ID`、`Name`、`State`、`Cells`、`Overview`、`Tags`) で表す。
実装は `backend/internal/tui/ec2.go` の `NewEC2Service` と `ec2Row` だけで、`backend/internal/cli/tui.go` の `tui.Deps` の `Services` は `tui.NewEC2Service(awsinternal.ListEC2Resources)` の 1 要素である (issue 0218 がこの組み立てを `runTUI` から関数に切り出す)。
`ec2.go` は `Columns()` を Web UI の `ec2Columns` の `header` の順に、`Overview()` を `ec2OverviewRows` の項目名の順に書き写し、`Load` は注入された一覧関数の失敗を `fmt.Errorf("list EC2 instances: %w", err)` でラップする。
`backend/internal/tui/ec2_test.go` が `Cells()` と `Overview()` の完全一致、`Cells()` と `Columns()` の個数の一致、偽の一覧関数を渡した `Load` の呼び出しとエラーのラップを確認している。
`Row.State()` と State の列は Web UI の `StatusBadge` が表示する値 (資源の `State` フィールドの生の値) を返し、Drawer の色付けだけが `aws.NormalizeState` を使う (issue 0218 で改めた契約)。
`Cells()` と `Overview()` で空の値を `—` にするのは、Web UI の同じセルか項目が `Dash` か `dash` を描画する項目に限る (issue 0218 の `ec2.go` の修正で定めた規則)。

Drawer の描画 (`backend/internal/tui/model.go` `refreshDrawer`) は Overview の先頭に `Name` と `State` を置いてから `Row.Overview()` を並べる。
タブは `Overview` と `Tags` の 2 つに固定され (`drawerTabTitles`)、`Row.Tags()` が空のときは `No tags` と表示する。

3 サービスの Web UI 側の事実は次のとおり。
一覧関数は Web UI の一覧のハンドラ (`backend/internal/api/handlers_aws.go`) が呼ぶものである。

| ID | 一覧関数 (`backend/internal/aws/`) | 列 (`columns.tsx`) | Overview (`overviewRows.tsx`) | Tags |
| --- | --- | --- | --- | --- |
| `lambda` | `ListLambdaResources(ctx, profile, region)` | `lambdaColumns`: Function、State、Runtime、Region、Memory、Timeout | `lambdaOverviewRows`: Resource ID、Runtime、Memory、Timeout、Handler、Role、Region | `LambdaResource.Tags` はあるが `ListLambdaResources` が設定せず常に空 |
| `ecr` | `ListECRResources(ctx, profile, region)` | `ecrColumns`: Repository、URI、Tag mutability、Scan on push、Created | `ecrOverviewRows`: Repository ARN、URI、Tag mutability、Scan on push、Created、Region | `ECRRepoResource` に Tags フィールドが無い。Web UI の Drawer のタブは `DRAWER_TABS.ecr = ['Overview', 'Images']` で Tags タブが無い |
| `ecs` | `ListECSResources(ctx, profile, region)` | `ecsColumns`: Cluster、State、Active svc、Running、Pending、Registered EC2、Region | `ecsOverviewRows`: Resource ID、Active services、Running tasks、Pending tasks、Registered EC2、Region | `ECSResource.Tags` (`DescribeClusters` に `ClusterFieldTags` を渡して取得) |

- Lambda の Memory のセルは `<memoryMb> MB` のように単位を付けた文字列である (`lambdaColumns` の `cell`)。列と Overview に `Dash` を使う項目は無く、Runtime、Handler、Role が空のときは空文字列である。
- ECR の列に State は無く、`ECRRepoResource.State` は `ecrRepoFromRepo` が設定しないため空である (Web UI の Drawer の見出しの `StatusBadge` も空の値を表示する。`ResourceState()` は固定の `active` を返すが、これは Web UI が表示しない)。`ID` はリポジトリの ARN で、Created は RFC3339 の文字列をそのまま表示する。列の Scan on push は真なら `✓`、偽なら `—`、Overview の Scan on push は `enabled` か `disabled` である。列の Created と Overview の Created は空のとき `—` で、URI と Tag mutability は空のとき空文字列である。
- ECS の一覧はクラスタの行で、`Active svc`、`Running`、`Pending`、`Registered EC2` は件数である。State は `ECSResource.State` の生の値 (`ecsFromCluster` が `DisplayState` で小文字にした `active` など) で、`NormalizeState` は `active` を `running` に変えるため、`ResourceState()` の値を表示すると Web UI と食い違う。ECS Exec の接続は issue 0224 で扱う。

## 目的

`thief tui` のサービスの選択肢に Lambda、ECR、ECS が並び、Web UI と同じ列の一覧と、同じ項目の Overview と Tags を Drawer で見られる。

## 設計判断

- サービスごとに `backend/internal/tui/<id>.go` (`lambda.go`、`ecr.go`、`ecs.go`) を置き、`ec2.go` と同じ形で `New<Service>Service(list)` が一覧関数を受け取って `Service` を返す。`Load` の失敗は `list Lambda functions: %w` のようにサービス名を含めてラップする。`Row.State()` と `Cells()` の State は資源の `State` フィールドの生の値を返す (issue 0218 の契約)。`ResourceState()` に委譲する案は、ECS の `active` が `running` になり、ECR の空の State が `active` になり Web UI の `StatusBadge` の表示と食い違うので採らない。1 ファイルに 3 サービスをまとめる案は、`ec2.go` との対称性が崩れ、サービスの追加のたびにファイルが伸びるので採らない。
- `Columns()` の見出しは Web UI の `<id>Columns` の `header` と同じ文字列と順序にし、`Overview()` の項目名は `<id>OverviewRows` と同じ文字列と順序にする。`Cells()` と `Overview()` の値は Web UI の `cell` と `value` が描画する文字列 (Lambda の `128 MB`、ECR の RFC3339 の Created、ECS の件数の 10 進表記) に合わせる。空の値を `ec2.go` の `orDash` で `—` にするのは、Web UI の同じセルか項目が `Dash` か `dash` を描画する項目 (ECR の列の Created と Overview の Created) に限り、他の項目は空文字列のままにする。すべての空の値を `—` にする案は、Web UI が空文字列を描画する Lambda の Runtime や ECR の URI で表示が食い違うので採らない。真偽値は列と Overview で Web UI の文字列をそれぞれ写す (ECR の Scan on push は列が `✓` と `—`、Overview が `enabled` と `disabled`)。
- `Deps.Services` の並びは `serviceMeta.ts` の `SERVICES` の並び (`ec2`、`lambda`、`ecr`、`ecs`) にする。サービスの ID の辞書順に並べる案は、Web UI のサイドバーと順序が違い、PRD の概要の「TUI は Web UI に相当する操作を端末の中で行うことを目指し」から外れるので採らない。
- ECR は `Row.Tags()` が `nil` を返し、Drawer の Tags タブは `No tags` と表示する。Web UI の ECR には Tags タブが無いため、`Service` にタブの一覧を持たせて ECR だけ Tags タブを消す案もあるが、1 サービスのために `Service` のインターフェースを変え、`model.go` のタブの描画を可変にすることになるので採らない。サービス固有のタブ (ECR の Images など) を TUI に置くかは PRD の未確定論点「TUI の Drawer のサービス固有のタブ」で決め、そのときに合わせて決める。
- Lambda の `Row.Tags()` は `LambdaResource.Tags` をそのまま返す。`ListLambdaResources` がタグを設定しないため常に `No tags` になるが、Web UI の Tags タブも同じく空であり、この issue は Web UI との一致を目的とするので、`ListLambdaResources` にタグの取得を足すことはしない (backend 側の欠落として「扱わない範囲」に挙げる)。
- ECS の `SupportsSession()` はこの issue では `false` にする。ECS Exec (タスクとコンテナの選択、`StartECSExec`) は issue 0224 が足す。
- 偽の一覧関数を渡した `Load` のテストと、`Deps` の組み立ての関数の `ID()` の集合のテストは issue 0218 が定めた形に従う。
- 追加の API 呼び出しと権限は無い。TUI は Web UI の一覧のハンドラと同じ `ListLambdaResources`、`ListECRResources`、`ListECSResources` を使う。
- floci での確認: `example/README.md` の「確認手順」に Lambda、ECR、ECS は無く、floci がこれらをエミュレートするかはリポジトリに記録が無い。この issue では 3 サービスの一覧を実 AWS で確認し、floci でも試して動いたサービスは `example/seed.sh` と README の「確認手順」に足す。EC2 の `run-instances` は seed に足さない (issue 0218 の判断に従う)。
- `CHANGES.md` の `## develop` に、この issue の 3 サービスを足した [ADD] のエントリを 1 つ書く。issue 0218 の TUI の [ADD] に追記する案は、issue ごとに 1 コミットする運用でエントリの境目が追えなくなるので採らない。issue 0220 から 0224 も同じく issue ごとに 1 エントリとする。

## 完了条件

- `backend/internal/tui/lambda.go`、`ecr.go`、`ecs.go` に `NewLambdaService`、`NewECRService`、`NewECSService` があり、`ID()` がそれぞれ `lambda`、`ecr`、`ecs`、`Title()` が `serviceMeta.ts` の `SERVICES` の `name` (`Lambda`、`ECR`、`ECS`) と一致する。
- 3 サービスそれぞれの `backend/internal/tui/<id>_test.go` が次を確認する。`Columns()` の見出しの並びが `columns.tsx` の `<id>Columns` の `header` の並びと一致する。`Overview()` の項目名の並びが `overviewRows.tsx` の `<id>OverviewRows` の項目名の並びと一致する (`Name` と `State` は含めない)。次の 4 種の資源のうち該当するもので `Cells()` と `Overview()` が Web UI の描画文字列と一致する: すべての項目に値がある資源、任意の項目 (ECR の Created、Lambda の Handler など) が空の資源、真偽値が偽の資源、タグの無い資源 (すべてに値がある、任意の項目が空、タグが無い、の 3 種は 3 サービスすべて。真偽値が偽は ECR の Scan on push だけで、Lambda と ECS には真偽値の列も Overview の項目も無い)。空の項目は、Web UI が `Dash` か `dash` を描画する項目 (ECR の列と Overview の Created) だけ `—` になり、他は空文字列になる。ECR の Scan on push は列が `✓` か `—`、Overview が `enabled` か `disabled` になる。ECS の `State()` と State の列は、`ECSResource.State` が `active` の資源で `active` のままである。ECR の `State()` は、`ECRRepoResource.State` が空の資源で空文字列になる (`ResourceState()` の `active` ではない)。`Cells()` の個数が `Columns()` の個数と一致する。`New<Service>Service` に偽の一覧関数を渡して `Load` を呼ぶと、渡した `profile` と `region` でその関数が呼ばれ、返した資源が行になり、失敗がサービス名を含む文言でラップされる。
- ECR の `Row.Tags()` が `nil` を返し、Lambda の `Row.Tags()` が `LambdaResource.Tags` を返し、ECS の `Row.Tags()` が `ECSResource.Tags` を返すことをテストで確認する。
- ECS の `SupportsSession()` が `false` を返すことを `ecs_test.go` で確認する。
- `backend/internal/cli/tui.go` の `tui.Deps` を組み立てる関数の `Services` が `ec2`、`lambda`、`ecr`、`ecs` の順で並び、その `ID()` の集合がこの 4 個であることを、issue 0218 のテストの期待値を広げて確認する。各サービスに渡す一覧関数が `backend/internal/api/handlers_aws.go` の同じサービスのハンドラが呼ぶ関数と同じであることをコードの読み合わせで確認し、結果を「## 動作確認」に書く。
- 開発者が実 AWS で、3 サービスの一覧と、Drawer の Overview と Tags (ECS はタグのあるクラスタで表示、ECR は `No tags`) を確認し、結果を「## 動作確認」に記録する。floci で動いたサービスがあれば `example/seed.sh` と `example/README.md` の「確認手順」に足す。
- `AGENTS.md` の `internal/tui/` の節の「サービスの追加」の記述のとおり、`tui/lambda.go`、`tui/ecr.go`、`tui/ecs.go` のそれぞれに `Service` と `Row` を満たす型があり、`cli/tui.go` の `Deps.Services` にその 3 つの `Service` が足されている。
- `CHANGES.md` の `## develop` に、この issue の [ADD] のエントリが 1 つある。
- `mise run check` が通る。
- 扱わない範囲: ECS Exec (issue 0224)、`ListLambdaResources` へのタグの取得の追加、ECR の Images タブと ECS の Services、Tasks、Instances タブ (第 1 段階では扱わない。扱うかは PRD の未確定論点「TUI の Drawer のサービス固有のタブ」で決める)、他の 15 サービス (issue 0220、0221、0222、0223)。

## 関連

- docs/prd/thief.md の FR-29 (サービスの一覧と Drawer の受け入れ基準)、成功指標「TUI の AWS サービスの網羅」
- issue 0218: `Service` と `Row` のインターフェース、`Deps` の組み立ての関数、`Load` の配線のテストの形をこの issue が使う
- issue 0220、0221、0222、0223: 同じ TODO の項目から分けた残りの 15 サービス。`Deps` の `Services` の並びと `ID()` の集合のテストの期待値を順に広げる
- issue 0224: ECS の `SupportsSession()` を `true` にし、ECS Exec を足す
