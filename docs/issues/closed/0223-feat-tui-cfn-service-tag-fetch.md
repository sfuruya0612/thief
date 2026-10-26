# TUI の一覧と Drawer に CloudFormation を足し、タグはスタックを選んだときに取得する

Created: 2026-10-25
Model: Claude Fable 5.1
Completed: 2026-10-26

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

## 動作確認

- 一覧関数と detail の配線の読み合わせ: `backend/internal/cli/tui.go` の `newTUIDeps` は Parameter Store の次 (59 行目) に `tui.NewCFNService(awsinternal.ListCFNStacks, awsinternal.DescribeCFNStackDetail)` を置く。`backend/internal/api/handlers_aws.go` の CloudFormation の一覧のハンドラ `handleCFN` (238 行目) は 241 行目で `ListCFNStacks` を、Tags タブが使う `handleCFNStackDetail` (245 行目) は 249 行目で `DescribeCFNStackDetail` を呼ぶ。TUI に渡す 2 つの関数はこれらと同じである。`backend/internal/cli/tui_test.go` の `TestNewTUIDeps_ServicesFollowWebUIOrder` は、`Services` の `ID()` の並びが FR-29 の 19 個 (`cfn` が最後) と一致することを確かめる。
- floci の `DescribeStacks` はタグを返す (2026-10-26): seed の `create-stack` に `--tags Key=Name,Value=thief-example-stack` を足した。issue 0222 の確認に使った floci にはタグ無しのスタックが残っていたので、aws CLI で `delete-stack` してから seed と同じテンプレートとタグで `create-stack` し直し、`describe-stacks` が `Tags` に `Name=thief-example-stack` を返すことを確かめた。この時点では `mise run example:seed` を頭から流し直してはいなかった (他の資源が既にあり、S3 の `create-bucket` などで止まるため)。レビューの指摘を受け、同日に `mise run example:down` でコンテナを消してから `mise run example:up` と `mise run example:seed` を流し直した。seed は 19 の区切り (`==>`) をすべて通って終了コード 0 で終わり、`describe-stacks` は `CreationTime` が `2026-10-26T01:58:20Z` の `thief-example-stack` に `Tags` として `Name=thief-example-stack` を返した。下の pty の検証スクリプトもこのスタックで流し直し、8 項目すべて通った (行の Created は `2026-10-26T01:58:20Z`)。
- floci での確認 (2026-10-26): `HOME="$(pwd)/example/home" thief tui -p floci` を pty で起動する検証スクリプト (8 項目) を流し、すべて通った。観測した画面は次のとおり。
  1. 起動直後は `EC2 (0)`。
  2. `S` の選択肢は `ssm Parameter Store` の次に `cfn CloudFormation` が最後に出て、`Select a service (19/19)`。
  3. `cfn` で絞って Enter を押すと `CloudFormation (1)`。列は `Stack State Drift Created Updated`、行は `thief-example-stack CREATE_COMPLETE NOT_CHECKED 2026-10-26T00:54:12Z —` (floci の `ListStacks` は作り直したスタックの `LastUpdatedTime` を返さないので Updated は `—`)。Drawer の見出しは `Name thief-example-stack` と `State CREATE_COMPLETE` で、Overview は `Stack ID arn:aws:cloudformation:ap-northeast-1:000000000000:stack/thief-example-stack/088d6d53-...` (幅で切れる)、`Drift status NOT_CHECKED`、`Created 2026-10-26T00:54:12Z`、`Updated —`、`Region ap-northeast-1` の順。`State` は 1 回しか出ない。
  4. `2` の Tags タブは `Name thief-example-stack` で、`No tags` と `fetch failed` は出ない。`r` で読み直しても再び `Name thief-example-stack` が出る。
  5. `q` で終了コード 0。
  - `Loading tags...` の表示は、floci の応答が速く pty の取り込みの間に結果に置き換わるため画面では捉えていない。取得中の表示、空のタグの `No tags`、失敗の `Tags (fetch failed)` と理由、旧い世代の破棄、別の行へ移った後の保持、同じ行の再選択と `r` の後の取り直しは `backend/internal/tui/model_test.go` の `TestModel_TagFetcher*` (レビューの反映後は 10 本) で確かめる。
- 完了条件の「`mise run check` が通る」は、implement-issues の Step 5 の読み方 (既知の失敗に無い新たな失敗が無いこと) で判定した。`mise run backend:vuln` (govulncheck) は issue 0218 の検査から続く既知の 12 件 (GO-2026-6809、6617、6613、6612、6611、6610、6608、6607、6605、6603、6600、6599。ツールチェインと `golang.org/x/net` の版に由来し、この issue は依存を変えない) で終了コード 3 になり、字義どおりには通らない。これ以外の読み替えは無い。失敗の表示の理由の 1 行 (次の文) は設計判断に無い表示の追加で、方式を変える乖離ではない。失敗の表示は `Tags (fetch failed)` の見出しの下に失敗の理由 (エラーの文字列) を 1 行足した。Web UI の `DrawerCFNTags` が `DrawerError` でエラーの内容を出すのに合わせたもので、方式は変えていない (Step 4 の乖離の記録には当たらない)。
- 「Created と Updated が空」の資源の確認は `cfn_test.go` の `TestCFNRow_EmptyValuesFollowWebUI` で行う (floci の seed のスタックは Created を持つ)。
- 多観点レビュー (2026-10-26) の反映:
  - `r` の直後に Tags タブへ旧い世代のタグが残っていた (一覧が届くまで Drawer を組み直していなかった)。`startLoad` が 2 つの map を空にした後に `refreshDrawer` を呼ぶようにし、`TestModel_TagFetcherReloadClearsShownTags` で `r` の直後に旧いタグが消えて `Loading tags...` に戻ることを確かめる。
  - 別の行の結果 (`tagsFetchedMsg`) でも Drawer を組み直していたため、`refreshDrawer` の `GotoTop` で表示中の Drawer のスクロール位置が先頭に戻っていた。選択中の行の結果のときだけ組み直すようにし、`TestModel_TagFetcherOtherRowResultKeepsDrawerScroll` で確かめる。
  - 絞り込みの入力と `esc` の経路の取得がテストに無かった。`TestModel_TagFetcherFilterStartsFetch` を足した。
  - 失敗した結果も世代の中で保持し、同じ行を選び直しても取り直さず `r` の後に取り直す (設計判断の「保持」は成功と失敗を区別しない)。`TestModel_TagFetcherFailedResultIsNotRefetchedUntilReload` で固定した。
  - `cfnRow.FetchTags` は `detail` が nil のとき、または `detail` がエラー無しで nil を返したとき、`tea.Cmd` の goroutine で panic していた。どちらも `errCFNDetailUnavailable` を包んだエラーにして `Tags (fetch failed)` の表示に落とし、`TestCFNRow_FetchTagsWithoutDetailFails` で確かめる。0 件の `Load` と nil の `Tags` の確認も `cfn_test.go` に足した。
  - `TagFetcher.FetchTags` に godoc を足した。`backend/internal/cli/tui_test.go` の「第 1 段階の残りのサービスを足したら」のコメントを、19 サービスが揃った現状に合わせた。
  - 却下した指摘: 「`docs/issues/TODO.md` の 155 行目は既に `docs/issues/closed/0223` を含む」は事実と異なる (155 行目の参照は `docs/issues/0223` で、ドラフト 4 の書き換えが要る)。「`TestModel_TagFetcherTagsTabSwitchStartsFetch` が `tagsPending` を直接空にして実運用で起こらない状態を作っている」は、タブ切替の呼び出しが行の選択時の取得の安全網であることをテストのコメントが明記しており、利用者の挙動は行の選択時の取得のテストが固定しているため、そのままにした。
- レビューの反映後の再検査 (2026-10-26): 段ごとの検査 (gofmt、goimports、`mise run backend:lint`、`mise run backend:vuln`、`mise run backend:test`、`prettier --check`、`mise run frontend:lint`、`mise run frontend:test`) を流し直し、govulncheck の既知の 12 件以外に失敗は無い (新たな失敗 0 件、backend のテストは 19 パッケージすべて ok、frontend は 1350 件通過)。`mise run backend:install` で入れ直したバイナリで floci の検証スクリプト (8 項目) を流し直し、すべて通った。

## 解決方法

issue 0218 の `Service` と `Row` のインターフェース (`backend/internal/tui/service.go`) に合わせ、CloudFormation の一覧と Drawer を `backend/internal/tui/cfn.go` に足し、スタックを選んだときにタグを取る仕組みを `service.go` と `model.go` に足した。これで第 1 段階の 19 サービスが揃った。

- `cfn.go` (`NewCFNService(list, detail)`、`cfnRow`): 列は Web UI の `cfnColumns` の `header` と同じ `Stack`、`State`、`Drift`、`Created`、`Updated` とし、Overview は `cfnOverviewRows` から `State` (`refreshDrawer` が先頭に置く) を除いた `Stack ID`、`Drift status`、`Created`、`Updated`、`Region` とした。Created と Updated は空のとき `—` にする。`State()` は `CFNStackResource.State` の値をそのまま返す。`Load` は一覧関数のエラーを `list CloudFormation stacks: %w` で包み、行に `profile` と `region` と `detail` を持たせる。`Tags()` は nil を返し、`FetchTags(ctx)` が `detail(ctx, profile, region, スタック名)` を呼んで `CFNStackDetail.Tags` を返す (失敗は `describe CloudFormation stack <name>: %w` で包む。`detail` が無いか nil の詳細を返したときも panic ではなく `errCFNDetailUnavailable` を包んだエラーにする)。`SupportsSession()` は偽を返す。
- `service.go`: 一部の行だけが実装するインターフェース `TagFetcher` (`FetchTags(ctx context.Context) (map[string]string, error)`) を足した。
- `model.go`: `Model` に `tags map[string]tagsResult` (行の `ID()` ごとの結果) と `tagsPending map[string]bool` (取得中の行) を足した。`ensureTags()` は、選択中の行が `TagFetcher` を実装し、その行のタグが未取得で取得中でもないときに `FetchTags` を呼ぶ `tea.Cmd` を返す。一覧の読み込み中は呼ばない。`ensureTags()` を呼ぶのは、一覧が届いたとき (`rowsLoadedMsg`)、`1` と `2` のタブの切替、一覧のカーソルの移動、絞り込みの入力の後、絞り込みの `esc` の後の 5 か所である。結果の `tagsFetchedMsg` は `seq` (取得を始めたときの `loadSeq`) と行の `ID()` を持ち、`seq` が今の `loadSeq` と違えば捨て、同じなら保持し、選択中の行の結果なら Drawer を描き直す (別の行の結果で描き直すとスクロール位置が先頭に戻るため)。`startLoad` (`r` と対象の切替で呼ぶ) は 2 つの map を空にしてから Drawer を描き直し、旧い世代のタグを一覧が届く前に消す。`refreshDrawer` の Tags タブは、`TagFetcher` を実装する行では保持した結果を使い、結果がまだ無ければ `Loading tags...` を、失敗なら `Tags (fetch failed)` の見出しと理由を、タグが空なら既存の `No tags` を出す。
- `backend/internal/cli/tui.go` の `newTUIDeps`: `Deps.Services` の最後に `tui.NewCFNService(awsinternal.ListCFNStacks, awsinternal.DescribeCFNStackDetail)` を足した (渡す 2 つの関数は `handlers_aws.go` の `handleCFN` と `handleCFNStackDetail` が呼ぶものと同じ)。
- `docs/prd/thief.md`: 「成功指標」の「TUI の AWS サービスの網羅」と FR-29 の現状の記述を、19 サービスが揃った事実に合わせて書き換え、`Updated` を 2026-10-26 にした。
- テスト: `cfn_test.go` に、`ID()` と `Title()` と `Columns()` の見出しを確かめる `TestCFNService_IDTitleAndColumns`、値のある資源と Created と Updated が空の資源について `Cells()` と `Overview()` の文字列、`Cells()` と `Columns()` の個数、`Tags()` が nil であることを確かめる `TestCFNRow_CellsAndOverview` と `TestCFNRow_EmptyValuesFollowWebUI`、`Load` の配線とエラーのラップと行が `TagFetcher` を実装することを確かめる `TestCFNService_LoadWrapsErrorAndSetsProfileAndRegion`、`FetchTags` が行の `profile` と `region` とスタック名で `detail` を呼び、タグと包んだ失敗を返すことを確かめる `TestCFNRow_FetchTagsCallsDetailWithStackName` を足した。`model_test.go` に、偽の `detail` を持つ CloudFormation の Service で Model を組む `cfnTestModel` と、`TestModel_TagFetcher*` の 6 本 (取得の開始と `Loading tags...` と結果の表示、空のときの `No tags` と失敗のときの `Tags (fetch failed)`、`r` より前の世代の破棄と `r` の後の取り直し、別の行へ移った後の保持と戻ったときの再利用、Tags タブへの切替による取得、読み込み中に選んだ行の取得) を足した。`cli/tui_test.go` の期待する並びに `cfn` を足した。
- `example/seed.sh` の `create-stack` に `--tags Key=Name,Value=thief-example-stack` を足し、`example/README.md` の「サンプルリソースの投入」の一覧と「確認手順」に TUI の CloudFormation の項を足した。
- `CHANGES.md` の `## develop` に [ADD] のエントリを 1 つ足した。

完了条件の検証は「## 動作確認」に記録した。floci の `DescribeStacks` はタグを返すため、Tags タブに `Name thief-example-stack` が出ることまで floci で確かめた。多観点レビューの反映で、`startLoad` の Drawer の描き直し、選択中の行の結果だけの描き直し、`FetchTags` の nil ガードを足し、`model_test.go` に 4 本、`cfn_test.go` に 1 本のテストを足した (反映の一覧は「## 動作確認」)。`mise run check` は `mise run backend:vuln` (govulncheck) が issue 0218 の検査から続く既知の 12 件 (この issue は依存を変えない) で終了コード 3 になり、他の段階 (gofmt、goimports、backend の lint と test、prettier、frontend の lint と test) は通った。既知の失敗を例外として close する (この例外の承認は開発者に委ねる)。
