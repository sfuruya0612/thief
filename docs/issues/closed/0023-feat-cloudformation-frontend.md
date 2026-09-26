# 0023 CloudFormation のスタック一覧と Events / リソースの Drawer 表示を追加する

Created: 2026-07-17
Completed: 2026-07-17
Model: Claude Fable 5 claude-fable-5

## 背景 / 根拠

backend には CloudFormation のスタック一覧 API が既に存在する (`GET /api/aws/profiles/{profile}/cfn/stacks`、`backend/internal/api/routes.go:32`、実装は `backend/internal/aws/cfn.go` の `ListCFNStacks`)。
CLI 向けにはスタック詳細 (`DescribeCfnStack`: Parameters / Outputs / Tags) と Change Set 表示も実装済みである。
しかし frontend の `serviceMeta.ts` / `AccountView.tsx` に cfn のエントリがなく、Web UI からスタックを確認できない。

スタックの調査で必要になるのは、デプロイの成否と失敗原因を追う Events と、スタックが管理するリソースの一覧である。
この 2 つは backend にも未実装 (cfn.go に `DescribeStackEvents` / `ListStackResources` の呼び出しがない) のため、backend / frontend 両方の追加が必要になる。

なお TODO.md には「CloudFormation の API 以外を実行しないと情報の取得ができない場合はユーザに相談すること」とあったが、本 issue の範囲 (スタック一覧、スタック詳細、Events、リソース一覧) は `ListStacks` / `DescribeStacks` / `DescribeStackEvents` / `ListStackResources` の 4 つ、すべて CloudFormation API のみで完結するため、相談が必要なケースは発生しない。
リソースの物理 ID から先の各サービス詳細 (例: 管理下の EC2 インスタンスの状態) を出す場合は各サービスの API が必要になるが、これはスコープ外とする。

## 対応内容

- frontend の AWS サービス一覧に CloudFormation (サービスキー `cfn`) を追加し、スタック一覧を表示する
- スタック行選択時の Drawer にタブ `Overview` / `Events` / `Resources` / `Tags` を設ける
  - Overview: 一覧フィールド (state / drift / created / updated) に加え、スタック詳細 API から Description / Parameters / Outputs を表示する
  - Events: スタックイベント (timestamp / logical id / resource type / status / status reason) を新しい順に表示する。失敗系 status (`CREATE_FAILED` や `ROLLBACK` 系) は色分けする
  - Resources: スタックが管理するリソース (logical id / physical id / resource type / status / last updated) を表示する
  - Tags: スタック詳細 API から取得したタグを表示する (一覧 API の元である `ListStacks` はタグを返さないため、既存の `CFNStackResource.Tags` は常に空になっている)

## 実装方針

### backend: AWS SDK 呼び出し層

`backend/internal/aws/cfn.go` に追加する。SDK (`github.com/aws/aws-sdk-go-v2/service/cloudformation` v1.71.7) は導入済み。

- `CFNStackDetail` (JSON タグ snake_case): 既存の CLI 用 `CfnStackDetail` は表示用の string フィールドで JSON タグを持たないため、API 用の DTO を新設し、`DescribeStacks` 呼び出しロジックを両者で共有する形にリファクタする
- `ListCFNStackEvents(ctx, profile, region, stackName string) ([]CFNStackEvent, error)`: `DescribeStackEvents` を `NewDescribeStackEventsPaginator` で呼ぶ。イベントは無限に遡れる (直近 90 日分が返る) ため、最新 100 件で打ち切る (定数 `maxCFNStackEvents`)。UI の用途は直近のデプロイ結果確認であり、全履歴は不要
- `ListCFNStackResources(ctx, profile, region, stackName string) ([]CFNStackResourceSummary, error)`: `ListStackResources` を paginator で全ページ取得する (1 スタックのリソース数は CloudFormation の上限 500 個で抑えられているため全件で問題ない)
- 型変換はプライベートヘルパー (`cfnEventFromSDK` / `cfnResourceFromSDK`) に分離し、エラーは `fmt.Errorf("...: %w", err)` でラップする

### backend: API ハンドラ層

`backend/internal/api/handlers_aws.go` にハンドラ、`backend/internal/api/routes.go` にルートを追加する。ECR images のサブリソースパターン (`handlers_aws.go:141`、`routes.go:21`) に倣う。

- `GET /api/aws/profiles/{profile}/cfn/stacks/{stack}` → `handleCFNStackDetail` (Overview / Tags 用)
- `GET /api/aws/profiles/{profile}/cfn/stacks/{stack}/events` → `handleCFNStackEvents`
- `GET /api/aws/profiles/{profile}/cfn/stacks/{stack}/resources` → `handleCFNStackResources`
- いずれも `serveCached` + `cacheKey("cfn-events", profile, region, stack)` 形式でキャッシュ経由にする。ただし Events はデプロイ進行中の確認が主用途で 1 時間の既定 TTL では古すぎるため、短い TTL (例: 30 秒) を渡す。エラーは `writeAWSError` に集約する

### backend: CLI

CLI には cfn コマンドが既にあり、今回の主目的は Web UI のため、CLI への events / resources サブコマンド追加は行わない (必要になったら別 issue とする)。

### frontend

`add-aws-service` スキル (`.claude/skills/add-aws-service/SKILL.md`) の手順 4〜11 に従う。参照テンプレートは ECR。

- `frontend/src/types/aws.ts`: `CFNStackRaw` / `CFNStackRow` (一覧)、`CFNStackDetailRaw` / `CFNStackDetailRow`、`CFNStackEventRaw` / `CFNStackEventRow`、`CFNStackResourceRaw` / `CFNStackResourceRow` を追加する
- `frontend/src/lib/normalize.ts`: `cfnFromRaw` ほか対応する純関数を追加する
- `frontend/src/components/tables/columns.tsx`: `cfnColumns` (name / state / driftStatus / createdAt / updatedAt) を追加する
- `frontend/src/components/Drawer/overviewRows.tsx`: `cfnOverviewRows` を追加する
- `frontend/src/lib/serviceMeta.ts`: `SERVICES` にエントリを追加し、`SERVICE_TO_PATH` に `cfn: 'cfn/stacks'` を追加する (既存 backend ルートのパスセグメントが他サービスと違い 2 階層である点に注意。汎用 `getResources` がこのパスで解決できることを確認し、できない場合は `frontend/src/api/endpoints.ts` に専用関数を置く)
- `frontend/src/views/AccountView.tsx`: `activeService === 'cfn'` 分岐で `ServicePanel` に normalizer / columns / overviewRows を渡す
- `frontend/src/components/Drawer/Drawer.tsx`: `DRAWER_TABS` に `cfn: ['Overview', 'Events', 'Resources', 'Tags']` を追加し、描画分岐に新規タブコンポーネントを配線する
- 新規 `frontend/src/components/Drawer/DrawerCFNEvents.tsx` / `DrawerCFNResources.tsx`: Drawer 内サブリソーステーブルの既存例 `DrawerECSTasks.tsx` / `DrawerECSServices.tsx` に倣い、`useCFNStackEvents` / `useCFNStackResources` (TanStack Query、`enabled: !!stackName`) で遅延取得して `DataTable` 表示する。status 列は `StatusBadge` で色分けし、`ResourceStatusReason` は折り返し表示する (失敗原因の文章が入るため)
- `frontend/src/api/queries.ts` / `endpoints.ts`: 詳細 / events / resources の取得関数とフックを追加する
- `frontend/src/components/icons/AwsIcons.tsx` と `frontend/scripts/fetch-aws-icons.mjs` の `ICON_FILENAMES` に CloudFormation の公式 Architecture Icon を追加する

### テスト

- backend: `cfnEventFromSDK` / `cfnResourceFromSDK` の変換と、100 件打ち切りの境界をテーブル駆動でテストする (`backend/internal/aws/cfn_test.go` を新設。現状 cfn.go にはテストがない)
- frontend: normalize 系のユニットテストと、`DrawerCFNEvents` の失敗 status 色分けのコンポーネントテスト

## スコープ外

- テンプレート本文の表示 (`GetTemplate`)
- Change Sets の表示 (CLI には `DescribeCfnChangeSet` があるが Web UI は対象外)
- ドリフト検出の実行 (`DetectStackDrift`。一覧にある drift status の表示のみ行う)
- スタックの作成 / 更新 / 削除などの書き込み操作
- 管理下リソースの物理 ID から先の各サービス詳細表示 (CloudFormation 以外の API が必要になる領域)
- CLI への events / resources サブコマンド追加

## 検証

- `mise run check` を通す
- 実環境で、スタック一覧の表示、Drawer の 4 タブ表示、更新失敗歴のあるスタックで Events の失敗行が色分けされること、リソース数の多いスタック (100 超) で Resources が全件表示されることを確認する

## 解決方法

- `backend/internal/aws/cfn.go` に `CFNStackDetail` / `DescribeCFNStackDetail` (Overview/Tags 用)、`ListCFNStackEvents` (100 件打ち切り)、`ListCFNStackResources` (全件) を追加した。打ち切りロジックは `appendCFNStackEventsPage` として純関数に分離しテーブル駆動テストを追加した (`cfn_test.go`)
- `backend/internal/api/handlers_aws.go` / `routes.go` に `GET /api/aws/profiles/{profile}/cfn/stacks/{stack}` (Overview/Tags)、`/events`、`/resources` を追加した。Events は 30 秒 TTL、他は既定の 1 時間 TTL でキャッシュする
- frontend は `add-aws-service` スキルの手順に従い、CFN を通常サービス (`serviceMeta.ts` / `columns.tsx` / `overviewRows.tsx` / `AccountView.tsx`) として追加した上で、Drawer に `Overview` / `Events` / `Resources` / `Tags` の 4 タブを配線した
  - Overview はスタック一覧の情報に加え `DrawerCFNOverviewExtra` でスタック詳細 API の Description/Parameters/Outputs を補完表示する
  - Events は失敗系ステータス (`CREATE_FAILED` や `ROLLBACK` 系) を正規表現 `isCfnEventFailure` で判定し赤色表示する
  - Tags は一覧 API がタグを返さないため `DrawerCFNTags` でスタック詳細 API のタグを表示する
- 型変換 (`cfnFromRaw` 系) と Events の失敗色分けのユニットテスト/コンポーネントテストを追加した
