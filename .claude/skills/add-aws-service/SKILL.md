---
name: add-aws-service
description: thief に新しい AWS サービスのリソース一覧機能を追加する手順。backend (API サーバ・CLI) と frontend (React) の両方に横断する変更を、既存サービス(ECR 等)のパターンに揃えて一貫して行う。
---

# 新 AWS サービス追加手順

新しい AWS サービスのリソース一覧を thief に追加するときの標準手順。既存実装(ECR)のファイルをテンプレートとして参照し、命名パターンをそのまま踏襲すること。

前提: このリポジトリの正の実装は `backend/` と `frontend/` の 2 ディレクトリ。ルート直下の `cmd/` `internal/` `go.mod` は別系統の残骸であり、本手順の対象外(混同しないこと)。

## 1. backend: AWS SDK 呼び出し層

`backend/internal/aws/<service>.go` を新規作成する。

参照テンプレート: `backend/internal/aws/ecr.go`

パターン:
- `<Service>Resource` 構造体を定義し、JSON タグは snake_case。
- `ResourceID() string` / `ResourceName() string` / `ResourceState() string` / `ServiceName() string` を実装(resource インターフェース、`backend/internal/aws/resource.go` を参照)。
- `List<Service>Resources(ctx context.Context, profile, region string) ([]<Service>Resource, error)` を実装。`NewClient(ctx, profile, region, func(cfg aws.Config) *<sdk>.Client {...})` で SDK クライアントを取得し、ページネーションは `<sdk>.New<Op>Paginator` を使う。
- SDK 型 → Resource 型への変換はプライベートヘルパー関数(`<service>FromXxx`)に分離する。
- エラーは `fmt.Errorf("... : %w", err)` でラップ。コメントは日本語、エラーメッセージは英語。

## 2. backend: CLI コマンド層

`backend/internal/cli/<service>.go` を新規作成する。

参照テンプレート: `backend/internal/cli/ecr.go`

パターン:
- `new<Service>Cmd() *cobra.Command` 関数を定義し、`RunE` 内で `runList(cmd, columns, fetchFunc)` を呼ぶ(`runList` は `backend/internal/cli/helper.go`)。
- `backend/internal/cli/root.go` の `NewRootCmd()` にサブコマンドとして追加する。

## 3. backend: API ハンドラ層

`backend/internal/api/handlers_aws.go` にハンドラを追加し、`backend/internal/api/routes.go` にルートを追加する。

参照テンプレート: `handlers_aws.go` の `handleECR` / `handleECRImages`、`routes.go` の `/api/aws/profiles/{profile}/ecr` 行。

パターン:
- `func (s *Server) handle<Service>(w http.ResponseWriter, r *http.Request)` を追加。
- `s.profileAndRegion(r)` で profile/region を取得。
- `cacheKey("<service>", profile, region)` → `s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) { return awsinternal.List<Service>Resources(r.Context(), profile, region) })` の形でキャッシュ経由取得。
- エラー時は `writeAWSError(w, err)`、成功時は `writeCacheHeaders` + `writeJSON(w, entry.Value)`。
- ルートは `s.mux.HandleFunc("GET /api/aws/profiles/{profile}/<service>", s.handle<Service>)`。

## 4. frontend: 型定義

`frontend/src/types/aws.ts` に Raw(バックエンド snake_case JSON)と Row(UI camelCase)の 2 型を追加する。

参照テンプレート: 同ファイル内の `ECRRepoRaw` / `ECRRepoRow`。

## 5. frontend: 正規化関数

`frontend/src/lib/normalize.ts` に `<service>FromRaw(raw: <Service>Raw, region: string): <Service>Row` 純関数を追加する。

参照テンプレート: `frontend/src/lib/normalize.ts:176` の `ecrFromRaw`。

## 6. frontend: 列定義

`frontend/src/components/tables/columns.tsx` に `<service>Columns: ColumnDef<<Service>Row>[]` を追加する。

参照テンプレート: 同ファイルの `ecrColumns`(:298 付近)。既存ヘルパー(`Dash()`, `formatBytes()` 等)を再利用すること。

## 7. frontend: Drawer 詳細表示

`frontend/src/components/Drawer/overviewRows.tsx` に `<service>OverviewRows(r: <Service>Row): OverviewEntry[]` を追加する。

参照テンプレート: 同ファイルの `ecrOverviewRows`(:106 付近)。

## 8. frontend: サービスメタ情報とパスマッピング

`frontend/src/lib/serviceMeta.ts` の `SERVICES` 配列にエントリ(`key`/`name`/`sub`/`color`/`group`)を追加し、`SERVICE_TO_PATH` にサービスキー → バックエンド URL パスセグメントの対応を追加する。

## 9. frontend: API エンドポイント(サービス固有の追加操作がある場合のみ)

一覧取得だけなら `frontend/src/api/endpoints.ts` の汎用 `getResources<TRaw>(service, profile, region)` で足りる(`SERVICE_TO_PATH` 経由で自動解決される)。ECR の images のようにサブリソース取得が必要な場合のみ、専用関数を追加する。

## 10. frontend: AccountView への分岐追加

`frontend/src/views/AccountView.tsx` に import(normalizer/columns/overviewRows)を追加し、`activeService === '<service>'` の分岐で `ServicePanel<TRaw, TRow>` に `normalizer`/`columns`/`overviewRows` を渡す。

参照テンプレート: 同ファイルの `activeService === 'ecr'` ブロック。

## 11. frontend: サービスアイコン

`frontend/src/components/icons/AwsIcons.tsx` の `AWS_ICON_FILES` にサービスキー → `<service>.svg` の対応を追加する。

対応する AWS 公式 Architecture Icons(32px)のファイル名を `frontend/scripts/fetch-aws-icons.mjs` の `ICON_FILENAMES` にも追加する(このファイル名は AWS 公式アイコンパッケージ zip 内のファイル名と一致させること)。SVG 本体はライセンス上リポジトリにコミットしない(`.gitignore` 対象、`mise run frontend:fetch-icons <zip>` で取得)。該当する Architecture Icon が存在しない場合のみ Resource Icons を代替として使う(natgw の実例を参照)。

## 検証

- `mise run backend:build`
- `mise run backend:test`
- `mise run frontend:lint`
- `mise run frontend:test`

いずれかが失敗したら、テンプレートにした ECR 実装とのシグネチャ差分を確認する。
