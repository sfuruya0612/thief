# TUI の一覧と Drawer に IAM、WAF、Secrets Manager、Parameter Store を足す

Created: 2026-10-25
Model: Claude Fable 5.1
Completed: 2026-10-26

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の一覧と Drawer を EC2 以外の 18 サービス (Lambda、ECR、ECS、S3、RDS、DynamoDB、ElastiCache、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS、IAM、WAF、Secrets Manager、Parameter Store、CloudFormation) にも足したい (docs/prd/thief.md の FR-29。`backend/internal/cli/tui.go` の `Deps.Services` には `tui.NewEC2Service` しか無く、列と Overview は Web UI の `frontend/src/components/tables/columns.tsx` と `frontend/src/components/Drawer/overviewRows.tsx` に合わせる)

18 サービスは Web UI のサイドバーの分類 (`frontend/src/lib/serviceMeta.ts` の `SERVICES` の `group`) に沿って 4 つの issue に分け、タグの取得の仕組みが他と違う CloudFormation はさらに 1 つの issue に分ける。
この issue は `security` の IAM、WAF、Secrets Manager と `management` の Parameter Store を扱う。
Lambda、ECR、ECS は issue 0219、S3、RDS、DynamoDB、ElastiCache は issue 0220、ELB、CloudFront、API Gateway、NAT Gateway、Kinesis、SQS は issue 0221、CloudFormation は issue 0223 で扱う。

TUI の 1 サービスは `backend/internal/tui/service.go` の `Service` と `Row` で表す。
実装の形、`Deps` の組み立て、`State()` と空の値と真偽値の表記の規則、タグの取得失敗の表示 (`TagsFetchFailer`) は issue 0218、0219、0220 が定めたものに従う。
Drawer の描画 (`backend/internal/tui/model.go` `refreshDrawer`) は Overview の先頭に `Name` と `State` を置く。

4 サービスの Web UI 側の事実は次のとおり。
一覧関数は Web UI の一覧のハンドラ (`backend/internal/api/handlers_aws.go`) が呼ぶものである。

| ID | 一覧関数 (`backend/internal/aws/`) | 列 (`columns.tsx`) | Overview (`overviewRows.tsx`) | Tags |
| --- | --- | --- | --- | --- |
| `iam` | `ListIAMResources(ctx, profile, _)` (リージョンを無視する) | `iamColumns`: Name、Kind、MFA、Activity、Last active、Policies、Groups | `iamOverviewRows`: Resource ID、ARN、Kind、MFA、Last active、Policies attached、Groups | `IAMResource` に Tags フィールドが無い |
| `waf` | `ListWAFResources(ctx, profile, region)` (REGIONAL と CLOUDFRONT を内部で並列に取得する) | `wafColumns`: Web ACL、Description、State、Scope、Region、Rules、Associated | `wafOverviewRows`: Resource ID、Scope、Description、Rules、Associated resources、Region | `WAFResource.Tags` と `TagsFetchFailed` |
| `secrets` | `ListSecretResources(ctx, profile, region)` | `secretColumns`: Name、Description、Last changed | `secretOverviewRows`: Name、Description、Last changed、Region | `SecretResource` に Tags フィールドが無い |
| `ssm` | `ListSSMParameters(ctx, profile, region)` (CLI だけが使う `ListSSMParameterInfos` ではない) | `ssmColumns`: Name、Type、Version、Last modified | `ssmOverviewRows`: Name、Type、Tier、Version、Last modified、Region | `SSMParameterResource` に Tags フィールドが無い |

- IAM の State に当たる列の見出しは `Activity` で、値は `IAMResource.State` の生の値である。`IAMResource.State` は `newIAMUserResource` と `newIAMRoleResource` が設定しないため空で、Web UI の `Activity` は空の `StatusBadge` を表示する (`ResourceState()` は固定の `active` を返すが、これは Web UI が表示しない)。IAM は列にも Overview にも Region が無い。`SecretResource.State` と `SSMParameterResource.State` も同じく空である。MFA、Policies、Groups は一覧関数の内部で追加の API を呼んで取り、失敗したときは `MFAEnabledFetchFailed`、`PoliciesFetchFailed`、`GroupsFetchFailed` (JSON の `mfa_enabled_fetch_failed`、`policies_fetch_failed`、`groups_fetch_failed`) を真にする。Web UI の `iamColumns` は、MFA の取得失敗では値の代わりに警告アイコン (`frontend/src/components/primitives/FetchFailedWarning.tsx`) を出し、Policies と Groups では件数の横に警告アイコンを出す。`iamOverviewRows` は MFA、Policies attached、Groups のいずれも値の横に警告アイコンを出す。
- IAM の MFA は列が `✓` と `✕`、Overview が `enabled` と `disabled` である。
- WAF の Scope は `REGIONAL` か `CLOUDFRONT` で、CLOUDFRONT の行も Region の列は引数のリージョンのままである。Associated は `AssociatedCount` と `AssociatedCountFetchFailed` (`associated_count_fetch_failed`) を持ち、`wafColumns` と `wafOverviewRows` は失敗のとき件数の横に警告アイコンを出す。タグは `ListTagsForResource` で取り、失敗したときは `TagsFetchFailed` を真にする。
- Secrets Manager と Parameter Store は列に State が無い。値は一覧に含めない (Web UI の Value タブは別の取得)。`secretOverviewRows` と `ssmOverviewRows` は `Name` を含む。`refreshDrawer` は `Name` と `State` を先頭に置くため、そのまま写すと Drawer に `Name` が 2 回出る。PRD の FR-29 の受け入れ基準は、`Row.Overview()` のキーの並びが Web UI の項目名から `Name` と `State` を除いた並びと一致すると定めている。
- Web UI が空のとき `Dash` か `dash` を描画する項目は、IAM が列と Overview の Last active、WAF が列と Overview の Description、Secrets Manager が列と Overview の Description と Last changed、Parameter Store が列と Overview の Last modified である。

## 目的

`thief tui` のサービスの選択肢に IAM、WAF、Secrets Manager、Parameter Store が並び、Web UI と同じ列の一覧と、同じ項目の Overview と Tags を Drawer で見られる。
IAM と WAF の項目ごとの取得失敗と、WAF のタグの取得失敗は、Web UI と同じくその旨が Drawer と一覧に出る。

## 設計判断

- サービスごとに `backend/internal/tui/iam.go`、`waf.go`、`secrets.go`、`ssm.go` を置き、`New<Service>Service(list)` が一覧関数を受け取る。`Columns()`、`Overview()`、`Cells()`、`Load` の失敗のラップ、`Deps.Services` の並び (`serviceMeta.ts` の順: issue 0221 までの 14 個に `iam`、`waf`、`secrets`、`ssm` を続ける) は issue 0219 から 0221 と同じ方針に従う。`State()` と State の列 (IAM は `Activity`) は資源の `State` フィールドの生の値を返し、空の値と真偽値の表記も issue 0219 の規則に従う。
- Secrets Manager と Parameter Store の `Row.Overview()` は、`secretOverviewRows` と `ssmOverviewRows` から `Name` を除いた並びにする。Web UI の Overview と項目の集合は変わらず、`Name` の位置だけが先頭に移る。重複したまま写す案は、同じ項目が 2 回出て PRD の受け入れ基準を満たさないので採らない。`refreshDrawer` を変えて、`Row.Overview()` に `Name` や `State` があれば先頭に置かないようにする案は、Drawer の描画がサービスの事情を知ることになり、issue 0218 で確定した `refreshDrawer` の形を変えるので採らない。
- 項目ごとの取得失敗 (IAM の MFA、Policies、Groups と WAF の Associated) は、Web UI の警告アイコンに代えて値の文字列で示す。`backend/internal/tui/format.go` に `fetchFailed(value string) string` を足し、`value` が空なら `fetch failed`、空でなければ `<value> (fetch failed)` を返す。`Cells()` と `Overview()` は、その項目の取得失敗が真のときこの関数の結果を使う。IAM の MFA は、列では Web UI が値を出さないので `fetch failed`、Overview では Web UI が値の横にアイコンを出すので `enabled (fetch failed)` か `disabled (fetch failed)` になる。Policies、Groups、WAF の Associated は列と Overview のどちらも件数の後ろに付く。別の列や Overview の項目を足して失敗を示す案は、Web UI に無い項目を増やすので採らない。失敗を無視して値だけ出す案は、PRD の FR-4 の「その値が取得失敗であることを示す」から外れるので採らない。この表示は PRD の FR-29 の受け入れ基準「`Cells()` と `Overview()` の値が Web UI の同じセルと項目が描画する文字列と一致し」と食い違う (Web UI は値の文字列としては `disabled` や `2` だけを描画し、失敗をアイコンで示す) ため、この issue でその受け入れ基準に例外を足す。
- WAF の行は issue 0220 の `TagsFetchFailer` を実装し、`WAFResource.TagsFetchFailed` を返す。
- IAM、Secrets Manager、Parameter Store は `Row.Tags()` が `nil` を返し、Drawer は `No tags` と表示する。Web UI の Tags タブも資源に Tags フィールドが無いため空であり、この issue は Web UI との一致を目的とするので、一覧関数にタグの取得を足すことはしない。
- 追加の API 呼び出しと権限は無い。TUI は Web UI の一覧のハンドラと同じ `ListIAMResources`、`ListWAFResources`、`ListSecretResources`、`ListSSMParameters` を使う。IAM の一覧関数はリージョンを無視するが、TUI は他のサービスと同じく選択中のリージョンを渡す。
- floci での確認: `example/seed.sh` は Parameter Store のパラメータ 3 つ (`/thief/example/app-name`、`/thief/example/max-connections`、`/thief/example/db-password`) と Secrets Manager のシークレット `thief/example/db-credentials` を作り、`example/README.md` の「確認手順」はこの 2 サービスの一覧を Web UI で確認している。この 2 サービスは floci で確認し、README の「確認手順」に TUI での確認を足す。IAM と WAF は README の確認済みの一覧に無いので実 AWS で確認し、floci でも試して動いたものは seed と README に足す。
- `CHANGES.md` の `## develop` に、この issue の 4 サービスを足した [ADD] のエントリを 1 つ書く (issue 0219 の方針)。

## 完了条件

- `backend/internal/tui/iam.go`、`waf.go`、`secrets.go`、`ssm.go` に `NewIAMService`、`NewWAFService`、`NewSecretsService`、`NewSSMService` があり、`ID()` がそれぞれ `iam`、`waf`、`secrets`、`ssm`、`Title()` が `serviceMeta.ts` の `SERVICES` の `name` (`IAM`、`WAF`、`Secrets Manager`、`Parameter Store`) と一致する。
- 4 サービスそれぞれの `backend/internal/tui/<id>_test.go` が、issue 0219 と同じ項目 (`Columns()` の見出しと `<id>Columns` の `header` の一致、`Overview()` の項目名と `<id>OverviewRows` の一致、issue 0219 の 4 種の資源のうち該当するもの (すべてに値がある、任意の項目が空、タグが無い、の 3 種は 4 サービスすべて。真偽値が偽は IAM の MFA だけ) の `Cells()` と `Overview()` の文字列の一致、`Cells()` と `Columns()` の個数の一致、偽の一覧関数を渡した `Load` の配線とエラーのラップ) を確認する。Secrets Manager と Parameter Store の `Overview()` に `Name` が無く、それ以外の項目が Web UI と同じ順であることを含む。空の項目は、IAM の Last active、WAF の Description、Secrets Manager の Description と Last changed、Parameter Store の Last modified だけ `—` になり、他は空文字列になる。IAM の MFA が偽の資源で列が `✕`、Overview が `disabled` になる。IAM、Secrets Manager、Parameter Store の `State()` は、`State` フィールドが空の資源で空文字列になる (`ResourceState()` の `active` ではない)。IAM の `Activity` の列も同じく空文字列になる。
- `backend/internal/tui/format.go` の `fetchFailed` が、空文字列で `fetch failed`、`3` で `3 (fetch failed)` を返すことを `format_test.go` で確認する。IAM のテストは、`MFAEnabledFetchFailed`、`PoliciesFetchFailed`、`GroupsFetchFailed` が真の資源で、`Cells()` の MFA が `fetch failed`、`Overview()` の MFA が `disabled (fetch failed)` (`ListIAMResources` は取得失敗のとき `MFAEnabled` を偽のままにする)、`Policies` が 2 件、`Groups` が 1 件の資源で列と Overview の Policies が `2 (fetch failed)`、Groups が `1 (fetch failed)` になることを含む。WAF のテストは、`AssociatedCountFetchFailed` が真で `AssociatedCount` が `3` の資源で列と Overview の Associated が `3 (fetch failed)` になることを含む。
- `docs/prd/thief.md` の FR-29 の受け入れ基準「`Cells()` と `Overview()` の値が Web UI の同じセルと項目が描画する文字列と一致し (空の値を `—` にするのは Web UI が `Dash` か `dash` を描画する項目に限る)」に、「項目ごとの取得失敗 (IAM の MFA、Policies、Groups と WAF の Associated) は、Web UI の警告アイコンに代えて `fetch failed` の文字列を値に付ける」という例外が足されている。
- WAF の行が `TagsFetchFailer` を実装し `WAFResource.TagsFetchFailed` を返すこと、IAM、Secrets Manager、Parameter Store の `Row.Tags()` が `nil` を返すことをテストで確認する。
- `backend/internal/cli/tui.go` の `tui.Deps` を組み立てる関数の `Services` が `serviceMeta.ts` の順で並び、その `ID()` の集合が issue 0221 までの 14 個にこの 4 個を足した 18 個であることをテストで確認する。各サービスに渡す一覧関数が `handlers_aws.go` の同じサービスのハンドラが呼ぶ関数と同じであることをコードの読み合わせで確認し、結果を「## 動作確認」に書く。
- floci (`mise run example:up` と `mise run example:seed`) で `HOME="$(pwd)/example/home" thief tui -p floci` を起動し、Parameter Store の一覧に seed の 3 つのパラメータ、Secrets Manager の一覧に `thief/example/db-credentials` が表示され、Drawer の Overview の先頭が `Name` で、その後に `Name` が重複しないことを確認して「## 動作確認」に記録する。`example/README.md` の「確認手順」に TUI でこの 2 サービスを確認する項がある。
- 開発者が実 AWS で IAM と WAF の一覧と Drawer を確認し、結果を「## 動作確認」に記録する。確認できなかった項目は、その旨と理由を書く。
- `CHANGES.md` の `## develop` に、この issue の [ADD] のエントリが 1 つある。
- `mise run check` が通る。
- 扱わない範囲: Secrets Manager と Parameter Store の Value タブ、WAF の Rules タブ (第 1 段階では扱わない。扱うかは PRD の未確定論点「TUI の Drawer のサービス固有のタブ」で決める)、`ListIAMResources`、`ListSecretResources`、`ListSSMParameters` へのタグの取得の追加、他の 14 サービス (issue 0219、0220、0221、0223)。

## 関連

- docs/prd/thief.md の FR-29 (Overview の `Name` と `State` の扱い、受け入れ基準に足す取得失敗の例外)、FR-4 (取得失敗の扱い)
- issue 0218: `Service` と `Row` のインターフェース、`refreshDrawer` の `Name` と `State` の扱い、`Deps` の組み立ての関数、`Load` の配線のテストの形
- issue 0219: サービスのファイルの形、`Deps.Services` の並び、`Cells()` の文字列の方針
- issue 0220: `format.go` と `TagsFetchFailer` をこの issue の WAF が使う。この issue は `format.go` に `fetchFailed` を足す
- issue 0221: 同じ TODO の項目から分けた 6 サービス
- issue 0223: 同じ TODO の項目から分けた CloudFormation。この issue の次に `Deps` の `ID()` の集合を 19 個にする

## 動作確認

- 一覧関数の配線の読み合わせ: `backend/internal/cli/tui.go` の `newTUIDeps` が `tui.NewIAMService(awsinternal.ListIAMResources)`、`tui.NewWAFService(awsinternal.ListWAFResources)`、`tui.NewSecretsService(awsinternal.ListSecretResources)`、`tui.NewSSMService(awsinternal.ListSSMParameters)` を SQS の次に並べる。これらは `backend/internal/api/handlers_aws.go` の `handleIAM` (197 行目、200 行目で `ListIAMResources`)、`handleSSMList` (211 行目、214 行目で `ListSSMParameters`)、`handleSecretsList` (231 行目、234 行目で `ListSecretResources`)、`handleWAF` (383 行目、386 行目で `ListWAFResources`) が呼ぶ関数と同じである。`Deps.Services` の 18 個の `ID()` と並びは `backend/internal/cli/tui_test.go` の `TestNewTUIDeps_ServicesFollowWebUIOrder` で確かめた。
- floci での IAM と WAF の試行 (2026-10-26): 設計判断の「floci でも試して動いたものは seed と README に足す」に従い、issue 0221 の確認に使った floci に aws CLI で、IAM のユーザー `thief-example-user` (グループ `thief-example-group` への追加と管理ポリシー `ReadOnlyAccess` の付与を含む) とロール `thief-example-role` (`AWSLambdaBasicExecutionRole` の付与を含む)、WAF の Web ACL `thief-example-waf` (REGIONAL、タグ `Env=example`) を作った。2 サービス 7 回の作成の呼び出しはすべて成功し、TUI の IAM の一覧に 2 行、WAF の一覧に 1 行が表示された。Web ACL を ALB `thief-example-alb` に関連付けると、TUI の Associated は `8` になった。これは floci の `ListResourcesForWebACL` が `ResourceType` を無視して同じ ALB の ARN を返し (`API_GATEWAY`、`APPSYNC`、`COGNITO_USER_POOL` を指定しても ALB の ARN が返ることを aws CLI で確かめた)、backend の `wafACLDetail` (`backend/internal/aws/waf.go`) が 8 種類の `ResourceType` の結果を合算するためで、TUI の変換の問題ではない (Web UI も同じ値を表示する)。この挙動を避けるため、`example/seed.sh` には関連付けを入れず、Associated が `0` になる形で IAM と WAF の投入を足し、`example/README.md` の「サンプルリソースの投入」の一覧と「確認手順」に 4 サービスの項を書いた。
- floci での確認 (2026-10-26): `mise run example:down` と `mise run example:up` で floci を作り直し、足した後の `mise run example:seed` を頭から流して、IAM と WAF の段を含めて終了コード 0 で終わることを確かめた。その後 `HOME="$(pwd)/example/home" thief tui -p floci` を pty で起動する検証スクリプト (22 項目) を流し、すべて通った。観測した画面は次のとおり。
  1. 起動直後は `EC2 (0)`。
  2. `S` の選択肢は `ec2 EC2` から `sqs SQS` までの 14 個に続けて `iam IAM`、`waf WAF`、`secrets Secrets Manager`、`ssm Parameter Store` の順で、`Select a service (18/18)`。
  3. Parameter Store: `Parameter Store (3)`。行は `/thief/example/app-name String 1 2026-10-25T22:35:35Z`、`/thief/example/db-password SecureString 1 2026-10-25T22:35:36Z`、`/thief/example/max-connections String 1 2026-10-25T22:35:36Z` (Last modified は投入時刻)。Drawer の見出しは `Name /thief/example/app-name` と `State` (空) で、Overview は `Type String`、`Tier` (空。floci は Tier を返さない)、`Version 1`、`Last modified 2026-10-25T22:35:35Z`、`Region ap-northeast-1` の順で、`Name` は繰り返されない。Tags タブは `No tags`。
  4. Secrets Manager: `Secrets Manager (1)`。行は `thief/example/db-credentials — 2026-10-25T22:35:37Z` (Description は seed が設定しないので `—`)。Drawer の見出しは `Name thief/example/db-credentials` と `State` (空) で、Overview は `Description —`、`Last changed 2026-10-25T22:35:37Z`、`Region ap-northeast-1` の順で、`Name` は繰り返されない。Tags タブは `No tags`。
  5. IAM: `IAM (2)`。行は `thief-example-user user ✕ — 1 1` と `thief-example-role role ✕ — 1 0` (Activity は空、Last active は `—`)。ユーザーの Overview は `Resource ID AIDA6L9R9VRSTIZFRZTD` (ID は floci が割り当てる)、`ARN arn:aws:iam::000000000000:user/thief-example-user`、`Kind user`、`MFA disabled`、`Last active —`、`Policies attached 1`、`Groups 1` の順。`/` で `role` を絞り込むと `IAM (1/2)` になり、ロールの Overview は `Resource ID AROA…`、`ARN arn:aws:iam::000000000000:role/thief-example-role`、`Kind role`、`MFA disabled`、`Last active —`、`Policies attached 1`、`Groups 0`。Tags タブは `No tags`。
  6. WAF: `WAF (1)`。行は `thief-example-waf thief example web ACL active REGIONAL ap-northeast-1 0 0`。Overview は `Resource ID 5e4954aa-61f9-4794-b8b7-6930518556c8` (Web ACL の ID は floci が割り当てる)、`Scope REGIONAL`、`Description thief example web ACL`、`Rules 0`、`Associated resources 0`、`Region ap-northeast-1` の順。Tags タブは `Env example` で、`fetch failed` の見出しは無い。
  7. `q` で終了コード 0。
- 実 AWS での IAM と WAF の確認は行っていない。実 AWS の読み取りには SSO のログイン (ブラウザでの対話) が要り、この作業は開発者が不在の非対話の実行で行っているため、ログインできない。読み取り自体を規約は禁じていないので、これは規約ではなく認証の制約である。完了条件の主文 (開発者が実 AWS で確認する) は満たしておらず、但し書き (確認できなかった項目はその旨と理由を書く) で close する。この読み替えの承認は開発者に委ねる。2 サービスの一覧と Drawer は floci で確認した (上の 5 と 6)。floci では観測できず、実 AWS でしか確認できない表示は、IAM の MFA の `✓` と `enabled`、Last active の値、複数のポリシーとグループの件数、MFA と Policies と Groups の `fetch failed`、WAF の `CLOUDFRONT` の Scope、1 以上の Rules と正しい Associated の件数、Associated の `fetch failed`、`Tags (fetch failed)` の見出しである。これらの変換は次のテストが確かめている。IAM の `✓` と `enabled` と 2 件のポリシーと 1 件のグループは `backend/internal/tui/iam_test.go` の `TestIAMRow_CellsAndOverview`、Last active の値は同じテスト (`LastActivity` のある資源)、`fetch failed` の 3 項目は `TestIAMRow_FetchFailedFollowsIssue0222` と、backend が取得失敗のときに作る形 (MFA は偽のまま、ポリシーとグループは nil) の `TestIAMRow_FetchFailedWithBackendShape` (`0 (fetch failed)` と `disabled (fetch failed)`)、MFA が真で取得失敗の `enabled (fetch failed)` は `TestIAMRow_MFAEnabledFetchFailed`。Activity は実 AWS でも floci でも常に空である。`ListIAMResources` (`backend/internal/aws/iam.go` の `newIAMUserResource` と `newIAMRoleResource`) が `State` を設定しないためで、`TestIAMRow_CellsAndOverview` は空文字列になることを確かめている。WAF の `CLOUDFRONT` は `waf_test.go` の `TestWAFRow_EmptyValuesFollowWebUI`、Rules と Associated の件数は `TestWAFRow_CellsAndOverview`、Associated の `fetch failed` は `TestWAFRow_AssociatedFetchFailedFollowsIssue0222`、`TagsFetchFailed` の伝播は `TestWAFRow_EmptyValuesFollowWebUI` (backend の `wafACLDetail` が失敗時に返す空の非 nil map を使う) で確かめ、`TagsFetchFailer` の行に見出しを出す描画は `model_test.go` の `dynamoRow` を使ったテストと、`wafRow` を Drawer に通す `TestModel_TagsTabFetchFailedHeadingForWAFRow` が確かめている。State と Scope が空のとき dash にならないことは `TestWAFRow_EmptyStateAndScopeStayEmpty` で確かめている。`fetchFailed` の 2 つの形は `format_test.go` の `TestFetchFailed` (`0 (fetch failed)` を含む) で確かめている。4 サービスの `Load` が 0 件の一覧でエラーにならないことと、Secrets Manager と Parameter Store の行が `TagsFetchFailer` を実装しないことも各テストで確かめている。
- 設計判断との乖離: 設計判断は「IAM と WAF は README の確認済みの一覧に無いので実 AWS で確認し、floci でも試して動いたものは seed と README に足す」と書いているが、前半の実 AWS での確認は上のとおり行えず、後半の floci での試行と seed と README への追加だけを行った。実装の方式は変えていない。確認の手段が設計判断の記述から外れた点を乖離として記録する。
- `mise run check` の通過は新たな失敗が無いことで判定し、既知の失敗を例外として close する。`mise run backend:vuln` には issue 0218 から 0221 と同じ既知の 12 件 (GO-2026-6809、6617、6613、6612、6611、6610、6608、6607、6605、6603、6600、6599) だけが出る。この issue の変更は依存を変えない。gofmt と goimports の差分は無く、`mise run backend:lint`、`mise run backend:test` (19 パッケージ ok)、prettier、`mise run frontend:lint` (警告 9 件は既存)、`mise run frontend:test` (119 ファイル 1350 件) は通った。frontend のファイルはこの issue では変えていない。

## 解決方法

issue 0218 の `Service` と `Row` のインターフェース (`backend/internal/tui/service.go`) に合わせ、4 サービスの一覧と Drawer を `backend/internal/tui/` に足した。いずれも `Columns()` の見出しは Web UI の `frontend/src/components/tables/columns.tsx` にある同じサービスの列定義の `header` と同じにし、`Overview()` の項目は `frontend/src/components/Drawer/overviewRows.tsx` の同じサービスの項目から `Name` と `State` (`refreshDrawer` が先頭に置く) を除いたものにした。`Load` は渡された一覧関数のエラーを `list <Service> ...: %w` で包み、`SupportsSession()` は 4 つとも偽を返す。

- `iam.go` (`NewIAMService`、`iamRow`): 列は `Name`、`Kind`、`MFA`、`Activity`、`Last active`、`Policies`、`Groups`、Overview は `Resource ID`、`ARN`、`Kind`、`MFA`、`Last active`、`Policies attached`、`Groups`。`ListIAMResources` は `State` を設定しないので、`State()` と Activity の列は空文字列になる。定数 `cross` (`✕`) を足し、列では `mfaCell()` が `✓` か `✕` を、Overview では `mfaOverview()` が `enabled` か `disabled` を返す。`policies()` と `groups()` は件数を文字列にして返す。`MFAEnabledFetchFailed`、`PoliciesFetchFailed`、`GroupsFetchFailed` が真の資源は、それぞれの値に `fetchFailed` を通す。`IAMResource` にタグが無いので `Tags()` は nil を返す。
- `waf.go` (`NewWAFService`、`wafRow`): 列は `Web ACL`、`Description`、`State`、`Scope`、`Region`、`Rules`、`Associated`、Overview は `Resource ID`、`Scope`、`Description`、`Rules`、`Associated resources`、`Region`。`associated()` は `AssociatedCountFetchFailed` が真のとき値に `fetchFailed` を通す。`Tags()` は `WAFResource.Tags` を、`TagsFetchFailed()` は `WAFResource.TagsFetchFailed` を返す (`TagsFetchFailer`)。後者が真のとき、Drawer の Tags タブの見出しは `Tags (fetch failed)` になる。
- `secrets.go` (`NewSecretsService`、`secretsRow`): 列は `Name`、`Description`、`Last changed`、Overview は `Description`、`Last changed`、`Region`。Web UI の `secretOverviewRows` の先頭にある `Name` は Drawer の見出しと重複するので省く。Description と Last changed は空のとき `—` にする。`Tags()` は nil を返す。
- `ssm.go` (`NewSSMService`、`ssmRow`): 列は `Name`、`Type`、`Version`、`Last modified`、Overview は `Type`、`Tier`、`Version`、`Last modified`、`Region`。`ssmOverviewRows` の先頭にある `Name` は同じ理由で省く。Last modified は空のとき `—` にする。`Tags()` は nil を返す。
- `format.go`: `fetchFailed(value string) string` を足した。空文字列には `fetch failed` を、それ以外には `<value> (fetch failed)` を返す。Web UI が警告アイコンで示す項目ごとの取得失敗を、TUI ではこの文字列で示す。
- `backend/internal/cli/tui.go` の `newTUIDeps`: `Deps.Services` の SQS の次に IAM、WAF、Secrets Manager、Parameter Store をこの順で足した。一覧関数には `awsinternal.ListIAMResources`、`ListWAFResources`、`ListSecretResources`、`ListSSMParameters` を渡す (`handlers_aws.go` の `handleIAM`、`handleWAF`、`handleSecretsList`、`handleSSMList` が呼ぶ関数と同じ)。これで 18 サービスになった。
- `docs/prd/thief.md` の FR-29 の受け入れ基準に、例外として「項目ごとの取得失敗 (IAM の MFA、Policies、Groups と WAF の Associated) は、Web UI の警告アイコンに代えて `fetch failed` の文字列を値に付ける」を足した。
- テスト: `iam_test.go`、`waf_test.go`、`secrets_test.go`、`ssm_test.go` に、`ID()` と `Title()` と `Columns()` の見出し、値のある資源と空の資源の `Cells()` と `Overview()` の文字列、`Cells()` と `Columns()` の個数の一致、`Load` の配線とエラーのラップのテストを足した。IAM では MFA が偽の資源の `✕` と `disabled` (`TestIAMRow_EmptyAndFalseValuesFollowWebUI`)、3 項目の取得失敗 (`TestIAMRow_FetchFailedFollowsIssue0222`)、backend が取得失敗のときに作る形での `0 (fetch failed)` と `disabled (fetch failed)` (`TestIAMRow_FetchFailedWithBackendShape`)、`enabled (fetch failed)` の分岐 (`TestIAMRow_MFAEnabledFetchFailed`) を、WAF では `TagsFetchFailed` の伝播 (`TestWAFRow_EmptyValuesFollowWebUI`)、Associated の取得失敗 (`TestWAFRow_AssociatedFetchFailedFollowsIssue0222`)、State と Scope が空のとき dash にならないこと (`TestWAFRow_EmptyStateAndScopeStayEmpty`) も確かめる。`model_test.go` の `TestModel_TagsTabFetchFailedHeadingForWAFRow` は `wafRow` を Drawer に通して見出しを確かめる。Secrets Manager と Parameter Store の行は `TagsFetchFailer` を実装しないことを確かめる。4 サービスの `Load` は 0 件の一覧でエラーにならないことを確かめる。`format_test.go` の `TestFetchFailed` が `fetchFailed` の 2 つの形を確かめる。`cli/tui_test.go` の `TestNewTUIDeps_ServicesFollowWebUIOrder` が期待する並びに 4 つを足した。
- `example/seed.sh` に IAM のユーザー (グループと管理ポリシー付き) とロール (管理ポリシー付き)、WAF の Web ACL (タグ付き) の投入を足し、`example/README.md` の「サンプルリソースの投入」の一覧と「確認手順」に TUI の 4 サービスの項を足した。floci の `ListResourcesForWebACL` が `ResourceType` を無視するため、Web ACL は ALB に関連付けない (詳細は seed のコメントと「## 動作確認」に書いた)。
- `CHANGES.md` の `## develop` に [ADD] のエントリを 1 つ足した。

完了条件の検証は「## 動作確認」に記録した。実 AWS での IAM と WAF の確認は認証の制約で行えなかったため、floci での確認とテストで代えており、この読み替えの承認は開発者に委ねる。`mise run check` に新たな失敗は無く、`mise run backend:vuln` の既知の 12 件だけが出る。
