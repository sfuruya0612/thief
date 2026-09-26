# Secrets Manager / Parameter Store の登録内容を更新できるようにする

Created: 2026-07-22
Completed: 2026-07-22
Model: Claude Opus 4.8

## 背景

`docs/issues/TODO.md` に「Secret Manager, Parameter Store の登録内容を更新できるようにしたい」という要望がある。

現状は Secrets Manager のシークレットと SSM Parameter Store のパラメータは Web UI・API・CLI いずれも参照 (一覧・値取得) のみで、値を更新する手段がない。

## 目的

Secrets Manager のシークレット値と SSM Parameter Store のパラメータ値を、Web UI・API・CLI から更新できるようにする。

## 設計判断

- UI: Drawer にサービス別タブを増やす既存方針 (S3 Objects / ECS Terminal 等) に揃え、secrets / ssm の Drawer に「Edit」タブを追加する。Overview 汎用描画への特殊対応は避ける (ユーザー確認済み)。
- スコープ: Web UI + API + CLI の 3 経路に対応する (ユーザー確認済み)。
- HTTP メソッド: 既存の書き込み系 (snippet 保存・CloudFront invalidation 等) が全て POST のため POST に統一する。name は階層名 (例 `/app/prod/db`) を含みうるため URL パスではなく JSON ボディで受け取り、パス・クエリのエンコード問題を回避する。
- Secrets Manager: `PutSecretValue` で新しいバージョンを作成する。説明・タグ・暗号化キーは保持される。
- SSM: `PutParameter` を `Overwrite=true` かつ Type / KeyId を指定せずに呼び、既存の型・KMS キーを保持したまま値だけを更新する。
- CLI の値入力: `--value` フラグ、または省略時は stdin から読む (末尾の改行 1 つを除去)。機密値をシェル履歴に残さず渡せるようにするため。
- CLI の Secrets Manager 一覧はメタデータのみ (値を含めない) とし、平文値を端末・シェル履歴に残さない。値の取得は Web UI / API に限定する。

## 完了条件

- backend: `aws.PutSecretValue` / `aws.PutSSMParameter` を追加する。
- backend: `POST /api/aws/profiles/{profile}/secretsmanager` / `POST /api/aws/profiles/{profile}/ssm/parameters` を追加し、更新後に一覧キャッシュを無効化する。
- CLI: `secretsmanager put <name>` / `ssm param put <name>` を追加する。Secrets Manager 一覧 `secretsmanager ls` も追加する。
- frontend: secrets / ssm の Drawer に「Edit」タブを追加し、現在値の編集・保存 (上書き確認あり) をできるようにする。
- 日本語・英語の UI 文字列を i18n (drawerAws 名前空間) に追加する。
- `mise run check` が全て通過する。

## 解決方法

### backend (aws サービス層)

- `internal/aws/secretsmanager.go`: `PutSecretValue` を追加。CLI 一覧用に値を含まない `SecretInfo` と `ListSecretInfos` を追加。
- `internal/aws/ssm.go`: `PutSSMParameter` を追加 (`Overwrite=true`、Type / KeyId 未指定で型・KMS キーを保持)。

### backend (API 層)

- `internal/api/models.go`: 値更新リクエストの DTO `ValueUpdateRequest` を追加。
- `internal/api/handlers_secrets_ssm.go` (新規): ボディ検証の純粋関数 `parseValueUpdate` と `handleSecretsPut` / `handleSSMPut` を追加。更新後に該当一覧キャッシュを `Invalidate` する。
- `internal/api/routes.go`: `POST .../secretsmanager` / `POST .../ssm/parameters` を登録。
- `internal/api/handlers_secrets_ssm_test.go` (新規): `parseValueUpdate` のテーブル駆動テスト。

### backend (CLI)

- `internal/cli/helper.go`: 値入力ヘルパー `readUpdateValue` と純粋関数 `stripOneTrailingNewline` を追加。
- `internal/cli/ssm.go`: `ssm param put <name>` を追加。
- `internal/cli/secretsmanager.go` (新規): `secretsmanager` コマンド (`ls` / `put`) を追加。
- `internal/cli/root.go`: `newSecretsManagerCmd` を登録。
- `internal/cli/helper_test.go` (新規): `stripOneTrailingNewline` のテーブル駆動テスト。

### frontend

- `api/endpoints.ts`: `updateSecretValue` / `updateSSMParameter` を追加。
- `api/queries.ts`: `useSecretUpdate` / `useSSMUpdate` ミューテーションを追加 (成功時に一覧クエリを invalidate)。
- `components/Drawer/DrawerValueEditor.tsx` (新規): 値編集の presentational コンポーネント。現在値を読めない場合は保存不可 (盲目的な上書きを防ぐ)。
- `components/Drawer/DrawerSecretEdit.tsx` / `DrawerSSMEdit.tsx` (新規): 一覧クエリから現在値を取得し `DrawerValueEditor` へ渡すタブコンポーネント。
- `components/Drawer/Drawer.tsx`: secrets / ssm のタブに `Edit` を追加し描画分岐を追加。
- `i18n/locales/{ja,en}/drawerAws.json`: `valueEditor` セクションを追加。
- `components/Drawer/DrawerValueEditor.test.tsx` (新規): 編集・保存・上書き確認・エラー表示・現在値未取得時の保存不可のテスト。

### 検証

- `mise run backend:build`: 成功。
- `mise run backend:test` (`go test -race -cover`): 全パッケージ pass。
- `mise run backend:lint` (vet / staticcheck / govulncheck / golangci-lint): 0 件。
- `mise run frontend:lint`: 0 errors、警告は既存のみ (新規ファイルからの警告なし)。
- `mise run frontend:test`: 59 ファイル / 533 テスト全て pass。
