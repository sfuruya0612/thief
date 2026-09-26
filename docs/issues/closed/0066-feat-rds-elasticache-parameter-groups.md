# RDS / ElastiCache のパラメータグループの中身を参照できるようにする

Created: 2026-07-23
Completed: 2026-07-23
Model: Claude Opus 4.8

## 背景

`docs/issues/TODO.md` に「RDS, Elasticache の紐づいているパラメータグループの中身を参照できるようにしたい」という要望がある。

現状、RDS インスタンスと ElastiCache クラスタの一覧・詳細は参照できるが、各リソースに紐づくパラメータグループ (DB Parameter Group / Cache Parameter Group) の中身 (パラメータ名と値) を確認する手段がない。
パラメータグループ名すら一覧レスポンスに含まれていない。

## 目的

RDS インスタンスと ElastiCache クラスタに紐づくパラメータグループのパラメータ一覧 (名前や値、変更可否など) を Web UI と CLI から参照できるようにする。

## 設計判断

- UI: Drawer にサブリソースタブを増やす既存方針 (ECR Images / ELB Targets / CFN Resources) に揃え、rds / cache の Drawer に「Parameters」タブを追加する。
- パラメータグループ名は一覧取得時に取得できるため、`RDSResource` に `parameter_groups` (複数)、`ElastiCacheResource` に `parameter_group` (単一) を追加する。グループ一覧を取得する専用 API は設けない。
- RDS のインスタンスは複数の DB パラメータグループを持ちうる。ELB Targets タブと同じ二段選択 (グループを選ぶとそのグループのパラメータを表示する) とし、グループが 1 つのときは自動選択する。
- ElastiCache のクラスタは単一の Cache パラメータグループを持つ。グループ選択を挟まず直接パラメータを表示する。
- パラメータ取得 API はグループ名をクエリで受け取る (ELB target-health の `tg_arn` と同じ方式)。`GET .../rds/parameters?group=...` / `GET .../elasticache/parameters?group=...` とする。
- Drawer タブが現在リソースのパラメータグループ名を得る経路は、`DrawerSecretEdit` と同じく一覧クエリ (`useResources`) のキャッシュから該当行を引く方式とし、Drawer 本体に型キャストを持ち込まない。
- 参照専用タブのため、UI ラベルは ECR Images / ELB Targets タブと同様に英語のハードコードとし、i18n には載せない (0065 の双方向操作を伴う Edit タブとは扱いを分ける)。
- パラメータ本体の取得には `DescribeDBParameters` / `DescribeCacheParameters` のページネータを使い、全ページを取得する。追加の依存は不要 (AWS SDK for Go v2 に含まれる)。

## 完了条件

- backend: `aws.ListRDSParameters` / `aws.ListElastiCacheParameters` を追加し、`RDSResource` / `ElastiCacheResource` にパラメータグループ名フィールドを追加する。
- backend: `GET .../rds/parameters` / `GET .../elasticache/parameters` を追加する。
- CLI: `rds parameters --group <name>` / `elasticache parameters --group <name>` を追加する。
- frontend: rds / cache の Drawer に「Parameters」タブを追加し、パラメータの一覧を表示できるようにする。
- backend / frontend ともに変換関数のテーブル駆動テストを追加する。
- `mise run check` が全て通過する。

## 解決方法

### backend (aws サービス層)

- `internal/aws/rds.go`: `RDSResource` に `ParameterGroups` を追加し、`rdsFromInstance` で `DBParameterGroups` から名前を集める。`RDSParameter` 型と変換関数 `rdsParameterFromSDK`、`ListRDSParameters` (DescribeDBParameters ページネータ) を追加。CLI 用に `RDSParameterInfo` と `ListRDSParameterInfos` を追加。
- `internal/aws/elasticache.go`: `ElastiCacheResource` に `ParameterGroup` を追加し、`elastiCacheFromCluster` で `CacheParameterGroup` から名前を取る。`ElastiCacheParameter` 型と `cacheParameterFromSDK`、`ListElastiCacheParameters` を追加。CLI 用に `ElastiCacheParameterInfo` と `ListElastiCacheParameterInfos` を追加。
- `internal/aws/rds_test.go` / `internal/aws/elasticache_test.go` (新規): パラメータグループ抽出とパラメータ変換のテーブル駆動テスト。

### backend (API 層)

- `internal/api/routes.go`: `GET .../rds/parameters` / `GET .../elasticache/parameters` を登録。
- `internal/api/handlers_aws.go`: `handleRDSParameters` / `handleElastiCacheParameters` を追加 (group クエリ必須、serveCached でキャッシュ)。

### backend (CLI)

- `internal/cli/rds.go`: `rds parameters --group <name>` を追加。
- `internal/cli/elasticache.go`: `elasticache parameters --group <name>` を追加。

### frontend

- `types/aws.ts`: `RDSRaw`/`RDSRow` に `parameter_groups`/`parameterGroups`、`CacheRaw`/`CacheRow` に `parameter_group`/`parameterGroup` を追加。`RDSParameterRaw`/`RDSParameterRow`、`CacheParameterRaw`/`CacheParameterRow` を追加。
- `lib/normalize.ts`: `rdsFromRaw`/`cacheFromRaw` を更新し、`rdsParameterFromRaw`/`cacheParameterFromRaw` を追加。
- `api/endpoints.ts` / `api/queries.ts`: `getRDSParameters`/`getCacheParameters` と `useRDSParameters`/`useCacheParameters` を追加。
- `components/tables/columns.tsx`: `rdsParameterColumns`/`cacheParameterColumns` を追加。
- `components/Drawer/DrawerRDSParameters.tsx` / `DrawerCacheParameters.tsx` (新規): 一覧クエリ (`useResources`) のキャッシュからグループ名を引き、パラメータを表示する。RDS は複数グループをセグメント切り替え、ElastiCache は単一グループを直接表示する。
- `components/Drawer/Drawer.tsx`: rds / cache の Drawer に「Parameters」タブを追加。
- `lib/normalize.test.ts`: `rdsParameterFromRaw`/`cacheParameterFromRaw` のテストを追加。

### 検証

- `mise run check`: backend の build / test (-race) / lint、frontend の lint / test すべて通過。frontend は 535 テスト pass。
