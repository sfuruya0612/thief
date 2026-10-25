# TUI の一覧と Drawer に CloudFormation を足し、タグはスタックを選んだときに取得する

Created: 2026-10-25
Model: Claude Fable 5.1

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の一覧と Drawer を EC2 以外の 18 サービス (Lambda、ECR、ECS、S3、RDS、DynamoDB、ElastiCache、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS、IAM、WAF、Secrets Manager、Parameter Store、CloudFormation) にも足したい (docs/prd/thief.md の FR-29。`backend/internal/cli/tui.go` の `Deps.Services` には `tui.NewEC2Service` しか無く、列と Overview は Web UI の `frontend/src/components/tables/columns.tsx` と `frontend/src/components/Drawer/overviewRows.tsx` に合わせる)

18 サービスは Web UI のサイドバーの分類 (`frontend/src/lib/serviceMeta.ts` の `SERVICES` の `group`) に沿って 4 つの issue に分け、タグの取得の仕組みが他と違う CloudFormation はさらに 1 つの issue に分ける。
この issue は `management` の CloudFormation を扱い、18 サービスを完了させる。
Lambda、ECR、ECS は issue 0219、S3、RDS、DynamoDB、ElastiCache は issue 0220、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS は issue 0221、IAM、WAF、Secrets Manager、Parameter Store は issue 0222 で扱う。

TUI の 1 サービスは `backend/internal/tui/service.go` の `Service` と `Row` で表す。
実装の形、`Deps` の組み立て、`State()` と空の値の表記の規則、タグの取得失敗の表示 (`TagsFetchFailer` と `Tags (fetch failed)` の文字列) は issue 0218、0219、0220 が定めたものに従う。
`Row.Tags()` は `context.Context` を受け取らず値を返すだけで、行を選んだときの追加の取得を表す口は無い。
Drawer の描画 (`backend/internal/tui/model.go` `refreshDrawer`) は、選択中の行が変わったとき、タブを切り替えたとき、一覧を読み直したときに呼ばれ、Overview の先頭に `Name` と `State` を置き、Tags タブは `Row.Tags()` の値をその場で描画する。

CloudFormation の Web UI 側の事実は次のとおり。
一覧関数は Web UI の一覧のハンドラ (`backend/internal/api/handlers_aws.go`) が呼ぶものである。

| ID | 一覧関数 (`backend/internal/aws/`) | 列 (`columns.tsx`) | Overview (`overviewRows.tsx`) | Tags |
| --- | --- | --- | --- | --- |
| `cfn` | `ListCFNStacks(ctx, profile, region)` (CLI だけが使う `ListCfnStackSummaries` ではない) | `cfnColumns`: Stack、State、Drift、Created、Updated | `cfnOverviewRows`: Stack ID、State、Drift status、Created、Updated、Region | 一覧では取れない。Web UI は `frontend/src/components/Drawer/DrawerCFNTags.tsx` が `useCFNStackDetail` で `GET /api/aws/profiles/{profile}/cfn/stacks/{stack}` (`handleCFNStackDetail`) を呼び、`backend/internal/aws/cfn.go` の `DescribeCFNStackDetail(ctx, profile, region, stackName) (*CFNStackDetail, error)` の `Tags` を表示する |

- State は `CREATE_COMPLETE` のような StackStatus の生の値で、`NormalizeState` は変換しない。Drift の既定は `NOT_CHECKED` である。`ListCFNStacks` の `cfnFromSummary` は Tags を設定しない。
- `cfnOverviewRows` は `State` を含む。`refreshDrawer` は `Name` と `State` を先頭に置くため、そのまま写すと Drawer に `State` が 2 回出る。PRD の FR-29 の受け入れ基準は、`Row.Overview()` のキーの並びが Web UI の項目名から `Name` と `State` を除いた並びと一致すると定め、CloudFormation のタグは `Row.Tags()` ではなく行を選んだときに取得するタグが Web UI の Tags タブと一致すると定めている。
- Web UI が空のとき `Dash` か `dash` を描画する項目は、列と Overview の Created と Updated である。
- `DescribeCFNStackDetail` は `DescribeStacks` を 1 スタックに対して呼び、`CFNStackDetail` (`StackName`、`Status`、`DriftStatus`、`Tags` など) を返す。

## 目的

`thief tui` のサービスの選択肢に CloudFormation が並び、Web UI と同じ列の一覧と、同じ項目の Overview を Drawer で見られる。
タグは Web UI と同じくスタックを選んだときに取得して Drawer の Tags タブに表示する。
これで PRD の FR-29 の 19 サービスが揃う。

## 設計判断

- `backend/internal/tui/cfn.go` に `NewCFNService(list, detail)` を置き、一覧関数に加えて `DescribeCFNStackDetail` と同じシグネチャの関数を受け取る。`cfnRow` は `profile` と `region` を保持する。`Columns()`、`Overview()`、`Cells()`、`Load` の失敗のラップ、`Deps.Services` の並び (`serviceMeta.ts` の順: issue 0222 までの 18 個の最後に `cfn` を続ける) は issue 0219 から 0222 と同じ方針に従う。`State()` と State の列は `CFNStackResource.State` の生の値を返し、空の値の表記も issue 0219 の規則に従う。
- `Row.Overview()` は `cfnOverviewRows` から `State` を除いた並び (Stack ID、Drift status、Created、Updated、Region) にする。理由と却下した案は issue 0222 の Secrets Manager と Parameter Store の `Name` と同じである。
- `cfnRow.Tags()` は `nil` を返す。タグは `backend/internal/tui/service.go` に足す任意のインターフェース `TagFetcher` (`FetchTags(ctx context.Context) (map[string]string, error)`) で取り、`cfnRow` がそれを実装して `detail` を呼び `CFNStackDetail.Tags` を返す。
- `model.go` は、行が `TagFetcher` を実装するとき、選択中の行が変わったとき、Tags タブに切り替えたとき、一覧の読み込みが終わったときに (そのとき選択中の行。`applyFilter` はカーソルが範囲外のときだけ先頭に戻す)、その行のタグが未取得なら `tea.Cmd` で `FetchTags` を呼ぶ。結果が届くまで Tags タブに `Loading tags...` と表示し、成功したらそのタグを (タグが空なら `refreshDrawer` の既存の分岐と同じ `No tags` を)、失敗したら `Tags (fetch failed)` (issue 0220 と同じ文字列) を表示する。取得したタグは一覧の読み込みごとに行の `ID()` をキーにして保持し、`r` の再取得 (`startLoad`) で捨てる。結果のメッセージは読み込みの世代 (`loadSeq`) と行の `ID()` を持ち、届いたときに世代が古ければ捨て、世代が同じならその行のタグとして保持する (選択中の行と違えば表示だけしない)。`Load` の中で全スタックの `DescribeCFNStackDetail` を呼ぶ案は、スタックの数だけ API を呼んで一覧が遅くなり、Web UI も一覧ではタグを取らないので採らない。`Row.Tags()` のシグネチャに `ctx` を足す案は、残りの 18 サービスの行がすべて変わるので採らない。行が変わるたびに取り直して保持しない案は、同じスタックに戻るたびに `DescribeStacks` を呼ぶので採らない。
- 新しい権限は要らないが、同じ一覧の読み込みの中で、行 (スタック) ごとに最初に選んだときに `DescribeStacks` (`DescribeCFNStackDetail`) を 1 回呼ぶ (読み込み全体で 1 回ではない)。Web UI の CloudFormation の Drawer の Tags タブと同じ呼び出しである。一覧は Web UI の一覧のハンドラと同じ `ListCFNStacks` を使う。
- floci での確認: `example/seed.sh` は CloudFormation のスタック `thief-example-stack` を作り、`example/README.md` の「確認手順」は CloudFormation の一覧を Web UI で確認している。seed の `create-stack` にはタグが無いので、`--tags Key=Name,Value=thief-example-stack` を足して Tags の取得を floci で確認できるようにし、README の「確認手順」に TUI での確認を足す。
- PRD の「成功指標」の「TUI の AWS サービスの網羅」の「2026-10-23 時点は `ec2` の 1 個である」と FR-29 の「残りの 18 サービスは第 1 段階の残りとして後続の issue で足す」の記述は、この issue で 19 サービスが揃った事実に合わせて更新する。
- `CHANGES.md` の `## develop` に、CloudFormation を足してタグを選んだときに取得する [ADD] のエントリを 1 つ書く (issue 0219 の方針)。

## 完了条件

- `backend/internal/tui/cfn.go` に `NewCFNService(list, detail)` があり、`ID()` が `cfn`、`Title()` が `serviceMeta.ts` の `SERVICES` の `name` (`CloudFormation`) と一致する。
- `backend/internal/tui/cfn_test.go` が、issue 0219 と同じ項目 (`Columns()` の見出しと `cfnColumns` の `header` の一致、`Overview()` の項目名と `cfnOverviewRows` から `State` を除いた並びの一致、issue 0219 と同じ 4 種の資源のうち該当する 3 種 (すべてに値がある、Created と Updated が空、タグが無い) の `Cells()` と `Overview()` の文字列の一致、`Cells()` と `Columns()` の個数の一致、偽の一覧関数を渡した `Load` の配線とエラーのラップ) を確認する。空の Created と Updated は列と Overview のどちらも `—` になる。`State()` と State の列は `CFNStackResource.State` が `CREATE_COMPLETE` の資源で `CREATE_COMPLETE` のままである。
- `cfn_test.go` で、`cfnRow.Tags()` が `nil` を返すこと、`NewCFNService` に偽の `detail` 関数を渡した行の `FetchTags` が、行の `profile`、`region`、スタック名でその関数を呼び、`CFNStackDetail.Tags` を返し、失敗をラップして返すことを確認する。
- `backend/internal/tui/service.go` に `TagFetcher` がある。`model_test.go` で次を確認する: `TagFetcher` を実装する行を選ぶと `FetchTags` が呼ばれ、結果が届くまで Tags タブに `Loading tags...` が出る。成功でそのタグが出る。成功でタグが空のとき `No tags` が出る。失敗で `Tags (fetch failed)` が出る。`r` の再取得より前の世代の結果は表示されず保持もされない。別の行に移ってから届いた結果は表示されないが保持され、その行に戻ったとき `FetchTags` を呼び直さずそのタグが出る。同じ行に戻ったとき `FetchTags` を呼び直さず保持したタグが出る。`r` の後は同じ行でも `FetchTags` を呼び直す。Overview タブのまま行を選んでから Tags タブに切り替えたときにも取得が始まる。
- `backend/internal/cli/tui.go` の `tui.Deps` を組み立てる関数の `Services` が `serviceMeta.ts` の順で並び、その `ID()` の集合が PRD の FR-29 の 19 個 (`ec2`、`lambda`、`ecr`、`ecs`、`s3`、`rds`、`dynamo`、`cache`、`elb`、`cloudfront`、`apigw`、`natgw`、`kinesis`、`sqs`、`iam`、`waf`、`secrets`、`ssm`、`cfn`) と一致することをテストで確認する。CloudFormation に渡す一覧関数が `handlers_aws.go` の CloudFormation のハンドラが呼ぶ `ListCFNStacks` で、`detail` が `DescribeCFNStackDetail` であることをコードの読み合わせで確認し、結果を「## 動作確認」に書く。
- floci (`mise run example:up` と `mise run example:seed`) で `HOME="$(pwd)/example/home" thief tui -p floci` を起動し、CloudFormation の一覧に `thief-example-stack` が表示され、Drawer の Overview の先頭が `Name` と `State` で `State` が重複せず、Tags タブに `Name` と `thief-example-stack` が表示されることを確認して「## 動作確認」に記録する。`example/seed.sh` の `create-stack` に `--tags` があり、`example/README.md` の「確認手順」に TUI で CloudFormation を確認する項がある。floci の `DescribeStacks` がタグを返さない場合は、その旨と理由を「## 動作確認」に書き、Tags タブの表示は実 AWS で確認する。
- PRD の「成功指標」の「TUI の AWS サービスの網羅」の「2026-10-23 時点は `ec2` の 1 個である」と FR-29 の「残りの 18 サービスは第 1 段階の残りとして後続の issue で足す」が、19 サービスが揃った事実に合わせて書き換えられている。
- `CHANGES.md` の `## develop` に、この issue の [ADD] のエントリが 1 つある。
- `mise run check` が通る。
- 扱わない範囲: CloudFormation の Events と Resources タブ (第 1 段階では扱わない。扱うかは PRD の未確定論点「TUI の Drawer のサービス固有のタブ」で決める)、他の 17 サービス (issue 0219、0220、0221、0222)。

## 関連

- docs/prd/thief.md の FR-29 (Overview の `Name` と `State` の扱い、CloudFormation のタグの取得)、「成功指標」の「TUI の AWS サービスの網羅」
- issue 0218: `Service` と `Row` のインターフェース、`refreshDrawer` の `Name` と `State` の扱い、`Deps` の組み立ての関数、`Load` の配線のテストの形
- issue 0219: サービスのファイルの形、`Deps.Services` の並び、`Cells()` の文字列の方針
- issue 0220: `Tags (fetch failed)` の文字列をこの issue の取得失敗の表示に使う。この issue は `service.go` に `TagFetcher` を足す
- issue 0222: 同じ TODO の項目から分けた 4 サービス。`Overview()` から `Name` を除く判断をこの issue の `State` に当てはめる
