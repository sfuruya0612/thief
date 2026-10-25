# TUI の一覧と Drawer に IAM、WAF、Secrets Manager、Parameter Store を足す

Created: 2026-10-25
Model: Claude Fable 5.1

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
