# TUI の一覧と Drawer に Lambda、ECR、ECS を足す

Created: 2026-10-25
Model: Claude Fable 5.1
Completed: 2026-10-25

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

## 動作確認

- 一覧関数の配線の読み合わせ: `backend/internal/cli/tui.go` の `newTUIDeps` が `tui.NewLambdaService(awsinternal.ListLambdaResources)`、`tui.NewECRService(awsinternal.ListECRResources)`、`tui.NewECSService(awsinternal.ListECSResources)` を渡す。`backend/internal/api/handlers_aws.go` の Web UI のハンドラが呼ぶ関数は Lambda が `awsinternal.ListLambdaResources` (130 行目)、ECS が `awsinternal.ListECSResources` (137 行目)、ECR が `awsinternal.ListECRResources` (178 行目) で、3 つとも同じ関数である。`Services` の並び (`ec2`、`lambda`、`ecr`、`ecs`) と重複の無さは `backend/internal/cli/tui_test.go` の `TestNewTUIDeps_ServicesFollowWebUIOrder` で確かめた。
- floci (ローカルの AWS エミュレータ、2026-10-25): `mise run backend:install` で `thief` を導入し、`mise run example:down`、`docker compose -f example/compose.yaml up -d`、`mise run example:seed` (この issue で足した Lambda / ECR / ECS の投入を含めて終了コード 0) の後、pty でキーを 1 文字ずつ送るスクリプトで `HOME="$(pwd)/example/home" thief tui -p floci` を動かした。19 項目すべてが期待どおりだった。
  1. 起動直後は `EC2 (0)` (seed は EC2 インスタンスを作らない)。
  2. `S` の選択肢は `ec2 EC2`、`lambda Lambda`、`ecr ECR`、`ecs ECS` の順。
  3. `lambda` と Enter: タイトル `Lambda (1)`、列 `Function State Runtime Region Memory Timeout`、行 `thief-example-hello active python3.12 ap-northeast-1 256 MB 30s`。Drawer の Overview は `Resource ID`、`Runtime python3.12`、`Memory 256 MB`、`Timeout 30s`、`Handler index.handler`、`Role arn:aws:iam::000000000000:role/thief-example-lamb…` (Drawer の幅で切り詰め)、`Region ap-northeast-1`。`2` の Tags は `No tags`。
  4. `S`、`ecr`、Enter: タイトル `ECR (2)`、列 `Repository URI Tag mutability Scan on push Created`、行 `thief-example/web … IMMUTABLE ✓ 2026-10-25T12:39:18Z` と `thief-example/batch … MUTABLE — 2026-10-25T12:39:18Z`。`thief-example/web` の Overview は `Repository ARN arn:aws:ecr:ap-northeast-1:000000000000:reposit…` (Drawer の幅で切り詰め)、`URI 000000000000.dkr.ecr.ap-northeast-1.localhost:4…` (同)、`Tag mutability IMMUTABLE`、`Scan on push enabled`、`Created 2026-10-25T12:39:18Z`、`Region ap-northeast-1`。`thief-example/batch` は `Tag mutability MUTABLE`、`Scan on push disabled`。`2` の Tags は `No tags`。
  5. `S`、`ecs`、Enter: タイトル `ECS (1)`、列 `Cluster State Active svc Running Pending Registered EC2 Region`、行 `thief-example-cluster active 0 0 0 0 ap-northeast-1` (State は `running` に正規化されず `active` のまま)。Overview は `Active services 0`、`Running tasks 0`、`Pending tasks 0`、`Registered EC2 0`、`Region ap-northeast-1`。`2` の Tags は `Env example`。`s` で通知 `ECS does not support sessions` が出て、キー操作のヒント (`q quit` など) は残る。
  6. `q` で終了コード 0。
  - スクリプトの 19 項目の内訳: (1) 起動直後が `EC2 (0)`、(2) `S` の選択肢の並び、(3) `Lambda (1)` に `thief-example-hello`、(4) Lambda の列の見出し、(5) Lambda の行の `active python3.12 256 MB 30s`、(6) Lambda の Overview の `Handler` と `Role`、(7) Lambda の Tags が `No tags`、(8) `ECR (2)` に 2 つのリポジトリ、(9) ECR の列の見出し、(10) ECR の行の `IMMUTABLE ✓` と `MUTABLE —`、(11) ECR の Overview の `Repository ARN` と `Scan on push enabled` / `disabled`、(12) ECR の Tags が `No tags`、(13) `ECS (1)` に `thief-example-cluster`、(14) ECS の列の見出し、(15) ECS の State が `active`、(16) ECS の Overview の 4 つの件数が `0`、(17) ECS の Tags が `Env example`、(18) ECS の `s` の通知とヒントの残存、(19) `q` の終了コード 0。
- 最初の floci の確認で見つけて直した不具合 (どちらも issue 0218 のプロトタイプの `backend/internal/tui/model.go` の `applyFilter` にあり、この issue の Drawer の確認はサービスの切替を経るため直さないと満たせない):
  1. Lambda (6 列) から ECR (5 列) へ切り替えると `github.com/charmbracelet/bubbles/table.(*Model).renderRow` が `runtime error: index out of range [5] with length 5` で panic し、TUI が終了コード 1 で終わった。`applyFilter` が旧いサービスの行を table に残したまま `SetColumns` を呼び、bubbles の `SetColumns` が旧い行 (6 セル) を新しい列 (5 列) で描き直すため。列を入れ替える前に `SetRows(nil)` で行を空にして直した。
  2. サービスを切り替えた後、一覧に行があるのに Drawer が `No row selected` のままだった。`reloadForNewTarget` が行を空にして `applyFilter` を呼ぶと `SetCursor(0)` が bubbles の `clamp` で `-1` になり、次に行が届いても `Cursor() >= len(trows)` の判定に掛からず負のまま残るため。負のカーソルも先頭に戻すように直した。取得の失敗や 0 件の後に `r` で行が届く経路も同じ不具合に当たる。
  - 修正後に同じスクリプトを再実行し、19 項目すべてが期待どおりになった。回帰テストは `backend/internal/tui/model_test.go` の `TestModel_ServiceSwitchToFewerColumnsSelectsFirstRow` と `TestModel_RowsAfterEmptyListSelectFirstRow`。
- Lambda の State は `backend/internal/aws/lambda.go` の `lambdaFromFunction` が `active`、`pending`、`inactive`、`failed` の小文字にするため、TUI も Web UI と同じ `active` と出る。
- Drawer の長い値 (Lambda の Role の ARN など) は Drawer の幅で切り詰められる。issue 0218 の Drawer の既存の振る舞いで、この issue では変えない。
- 実 AWS: 未確認。この実行環境に実 AWS の認証情報が無く、非対話の実行で外部リソースに触れない規約のため、開発者の確認として残す。確認する項目は、3 サービスの一覧と、Drawer の Overview と Tags (ECS はタグのあるクラスタで表示、ECR は `No tags`)。
- 完了条件の読み替え (承認が要る): 完了条件の「開発者が実 AWS で … 確認し、結果を「## 動作確認」に記録する」の行は、この実行では満たしていない。issue 0218 と issue 0220 から 0222 の同じ行にある「確認できなかった項目は、その旨と理由を書く」の扱いをこの issue にも当てはめ、未確認の事実と理由を記録して close した。多観点レビューの 3 観点すべてがこの読み替えを「開発者の承認が要る」と指摘しており、承認は得ていない。承認しない場合はこの issue を reopen し、実 AWS の確認結果をこの節に追記してから再 close する。

## 解決方法

issue 0218 の `Service` と `Row` の形に合わせて 3 サービスを足し、`Deps.Services` に登録した。

- `backend/internal/tui/lambda.go`: `NewLambdaService` (`ID()` は `lambda`、`Title()` は `Lambda`)、`lambdaService`、`lambdaRow`。列は Web UI の `lambdaColumns` と同じ `Function`、`State`、`Runtime`、`Region`、`Memory`、`Timeout`。Overview は `lambdaOverviewRows` と同じ `Resource ID`、`Runtime`、`Memory`、`Timeout`、`Handler`、`Role`、`Region`。Memory は `formatMemoryMB` で `<n> MB`、Timeout は `formatTimeoutSec` で `<n>s` にする。Web UI に dash を描く項目が無いため、空の値は空文字列のまま表示する。`State()` は `LambdaResource.State` の生の値、`Tags()` は `LambdaResource.Tags`、`SupportsSession()` は false。`Load` の失敗は `list Lambda functions: %w` で包む。
- `backend/internal/tui/ecr.go`: `NewECRService` (`ecr`、`ECR`)、`ecrService`、`ecrRow`。列は `ecrColumns` と同じ `Repository`、`URI`、`Tag mutability`、`Scan on push`、`Created`。Scan on push は一覧で `check` (`✓`) か `dash`、Overview で `enabled` か `disabled` にする。Created は空なら `orDash` で `—` にする。Overview は `ecrOverviewRows` と同じ `Repository ARN`、`URI`、`Tag mutability`、`Scan on push`、`Created`、`Region`。`State()` は `ECRRepoResource.State` の生の値とする (`ListECRResources` は設定しないため空になる。`ResourceState()` が返す固定の `active` は使わない)。`Tags()` は nil、`SupportsSession()` は false。失敗は `list ECR repositories: %w` で包む。
- `backend/internal/tui/ecs.go`: `NewECSService` (`ecs`、`ECS`)、`ecsService`、`ecsRow`。列は `ecsColumns` と同じ `Cluster`、`State`、`Active svc`、`Running`、`Pending`、`Registered EC2`、`Region`。件数は `formatCount` で 10 進にする。Overview は `ecsOverviewRows` と同じ `Resource ID`、`Active services`、`Running tasks`、`Pending tasks`、`Registered EC2`、`Region`。`State()` は `ECSResource.State` の生の値とする (`active` のままにし、`NormalizeState` の `running` にしない)。`Tags()` は `ECSResource.Tags`、`SupportsSession()` は false (ECS Exec は issue 0224 で足す)。失敗は `list ECS clusters: %w` で包む。
- `backend/internal/cli/tui.go`: `newTUIDeps` の `Services` を `ec2`、`lambda`、`ecr`、`ecs` の順にし、Web UI のハンドラと同じ `awsinternal.ListLambdaResources`、`ListECRResources`、`ListECSResources` を渡す。`backend/internal/cli/tui_test.go` の `TestNewTUIDeps_ServicesAreEC2Only` を `TestNewTUIDeps_ServicesFollowWebUIOrder` に改め、`ID()` の並びがこの順の 4 個で、重複が無いことを確かめる。
- テスト: `backend/internal/tui/lambda_test.go`、`ecr_test.go`、`ecs_test.go` のそれぞれに 4 つのテストを書いた。`ID()` と `Title()` と `SupportsSession()` と `Columns()` の見出しの並び (`Test<Id>Service_IDTitleAndColumns`)。すべての項目に値がある資源の `Cells()` と `Overview()` と `Tags()` の値、および `Cells()` の数が `Columns()` の数と一致すること (`Test<Id>Row_CellsAndOverview`)。空の値と偽の値 (ECR の Scan on push と Created、Lambda の Runtime と Handler と Role、ECS の State と件数) とタグの無い資源の表示 (`Test<Id>Row_EmptyValuesFollowWebUI`、ECR は `TestECRRow_EmptyAndFalseValuesFollowWebUI`)。偽の一覧関数を渡した `Load` の配線とエラーの包み方 (`Test<Id>Service_LoadWrapsErrorAndSetsRegion`)。ECR の `State()` が空のままであることは `TestECRRow_CellsAndOverview`、ECS の `State()` が `active` のままであることは `TestECSRow_CellsAndOverview` と `TestECSService_LoadWrapsErrorAndSetsRegion` で確かめる。
- `backend/internal/tui/model.go` の `applyFilter`: floci の確認で見つけた issue 0218 の 2 つの不具合を直した (「## 動作確認」)。この issue の設計判断には無い変更である。列数の少ないサービスへ切り替えると bubbles の `renderRow` が panic するため、列を入れ替える前に `SetRows(nil)` で table の行を空にする。行が無いときの `SetCursor(0)` は `-1` に丸められ、切替の後に行が届いても Drawer が `No row selected` のままになるため、カーソルが負のときも `SetCursor(0)` で先頭に戻す。この issue の完了条件 (3 サービスの Drawer の確認) はサービスの切替を経るため、別の issue に送らずここで直した。回帰テストは `backend/internal/tui/model_test.go` の `TestModel_ServiceSwitchToFewerColumnsSelectsFirstRow` (列数の少ないサービスへの切替で panic せず先頭の行が選ばれる)、`TestModel_RowsAfterEmptyListSelectFirstRow` (0 件の後に行が届くと先頭の行が選ばれる)、`TestModel_RowsAfterLoadErrorSelectFirstRow` (取得の失敗の後に `r` で行が届くと先頭の行が選ばれる)、`TestModel_ReloadKeepsCursorOnSelectedRow` (2 行目を選んだまま `r` で取り直しても 2 行目の選択が保たれる)。
- `example/seed.sh`: Lambda 関数 `thief-example-hello` (python3.12、256 MB、30 秒、タグ `Env=example`。デプロイパッケージは `zip` で作る)、ECR リポジトリ `thief-example/web` (IMMUTABLE、scanOnPush) と `thief-example/batch`、ECS クラスタ `thief-example-cluster` (タグ `Env=example`) の投入を足した。`zip` が無い環境で途中まで投入して止まらないよう、資源を作る前に `command -v zip` で確かめて `zip is required to build the Lambda deployment package` で終了する。`example/README.md` の投入の一覧と「確認手順」に、「TUI (`thief tui`) の Lambda / ECR / ECS 一覧」を足した。
- `CHANGES.md` の `## develop` に、この issue の [ADD] のエントリと、`applyFilter` の修正の [FIX] のエントリを足した。
- 完了条件の検証: `NewLambdaService` などの `ID()` と `Title()` は `TestLambdaService_IDTitleAndColumns`、`TestECRService_IDTitleAndColumns`、`TestECSService_IDTitleAndColumns` で、列と Overview と 4 種の資源の表示は `TestLambdaRow_CellsAndOverview`、`TestECRRow_CellsAndOverview`、`TestECSRow_CellsAndOverview` と `TestLambdaRow_EmptyValuesFollowWebUI`、`TestECRRow_EmptyAndFalseValuesFollowWebUI`、`TestECSRow_EmptyValuesFollowWebUI` で、`Tags()` は 3 つの `Test<Id>Row_CellsAndOverview` (タグの無い資源は `Test<Id>Row_EmptyValuesFollowWebUI`) で、ECS の `SupportsSession()` は `TestECSService_IDTitleAndColumns` で確かめた。`Deps.Services` の並びと一覧関数の読み合わせは `TestNewTUIDeps_ServicesFollowWebUIOrder` と「## 動作確認」で確かめた。`AGENTS.md` の「サービスの追加」の記述は、3 ファイルの型と `cli/tui.go` の登録で満たす。実 AWS の確認の行は満たしていない。issue 0218 と issue 0220 から 0222 の同じ行の「確認できなかった項目は、その旨と理由を書く」の扱いを当てはめ、未確認の事実と理由を「## 動作確認」に書いて close した。この読み替えは開発者の承認を得ていない (「## 動作確認」の「完了条件の読み替え」)。floci で動いた 3 サービスは `example/seed.sh` と `example/README.md` に足した。
- `mise run check` の通過は、新たな失敗が無いことと読み替えた。`mise run backend:vuln` (govulncheck) は issue 0218 と同じ Go 1.26.6 の標準ライブラリと `golang.org/x/net` v0.59.0 の既知の 12 件 (GO-2026-6809、6617、6613、6612、6611、6610、6608、6607、6605、6603、6600、6599) で失敗するが、この issue の変更は依存を変えない。それ以外 (gofmt、goimports、`backend:lint`、`backend:test`、prettier、`frontend:lint` (警告 9 件は既存)、`frontend:test`) は通る。
- レビューの反映: 多観点レビュー (完了条件、テスト、規約の 3 観点) の指摘で次を直した。`applyFilter` の回帰テストに `TestModel_ReloadKeepsCursorOnSelectedRow` と `TestModel_RowsAfterLoadErrorSelectFirstRow` を足した。`example/seed.sh` に `zip` の事前チェックを足した。`CHANGES.md` の [FIX] のエントリを `## develop` の先頭のブロックの [FIX] 群の末尾 (種別の順を保つ位置) へ移した。「## 動作確認」の ECR の Overview に表示された値と、floci のスクリプトの 19 項目の内訳を足した。この節の完了条件の検証をテスト名で書き分けた。却下した指摘は 3 つ。`mise run check` の行を「新たな失敗が無い」と読む判定は implement-issues の Step 5 が定める判定で、issue 0218 と同じ扱いにした。`backend/internal/tui/lambda.go` の `list Lambda functions: %w` が `backend/internal/aws/lambda.go` の `list lambda functions: %w` と重なる指摘は、設計判断の文言どおりで 4 サービスの TUI 層の文言を揃えるため変えなかった (エラーの判定は `errors.Is` で文言に依らない)。`applyFilter` の修正が設計判断に無い範囲の拡大だという指摘は、3 観点のうち 2 観点がこの issue の検証に必須の修正として範囲内と判定し、「## 動作確認」と `CHANGES.md` の [FIX] に記録してあるため、独立したバグ issue は作らなかった。実 AWS の行の読み替えと合わせて、開発者の承認は Step 12 の報告で求める。
