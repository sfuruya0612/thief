# CLI に Secrets Manager の値取得コマンドを追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-18

## 背景

Web の Secrets 一覧は Drawer の Value タブから Secret の値を参照・編集できる。backend には復号済みの値を取得する `GetSecretValue` (`backend/internal/aws/secretsmanager.go:52`) と API ハンドラ `handleSecretValue` (`backend/internal/api/handlers_secrets_ssm.go`) がある。

しかし CLI は `secretsmanager ls` と `secretsmanager put` だけで、`backend/internal/cli/secretsmanager.go` に値取得のサブコマンドが無い。利用者は CLI から Secret の値を確認できない。

## 目的

`thief secretsmanager get <name>` で Secret の復号済みの値を表示できるようにする。

## 設計判断

- 取得は既存の `GetSecretValue` をそのまま呼ぶ。新規 API・権限は不要。
- 出力は SSM の `ssm param get` (`backend/internal/cli/ssm.go:57`) に揃え、`Name` / `Value` の 2 列を `printRowsOrGroupBy` で出力する。これにより `-o csv` と `--no-header` が使える。
  - 却下案：値をそのまま 1 行で印字する案。SSM の `get` と出力形式が食い違い、CSV 出力もできなくなるため却下。
- 値は機密情報のため、エラーメッセージやログには含めない (`GetSecretValue` の既存の方針を踏襲)。
- CLI 用の `ToRow()` を持つ型は `internal/aws` に `SecretValue` を追加する (SSM の `SSMParameterValue` と同型)。

## 完了条件

- `thief secretsmanager get <name>` が Name / Value の 2 列を tab 出力する。
- `-o csv` と `--no-header` が使える。
- Secret 名に該当が無い場合、AWS のエラーが `errors.Is` で辿れる形で伝播する。
- `internal/cli/columns_torow_test.go` に `SecretValue` のケースを追加する。
- `mise run check` が通過する。

## 解決方法

- `backend/internal/aws/secretsmanager.go` に `SecretValue` とその `ToRow()`、既存の `GetSecretValue` をラップする `GetSecretValueDetail` を追加した。値はログ・エラーメッセージに含めない方針を維持している。
- `backend/internal/cli/secretsmanager.go` に `secretsmanager get <name>` を追加し、`secretGetColumns` (Name / Value) で `printRowsOrGroupBy` する。`ls` / `put` と同列に並べた。
- `columns_torow_test.go` に `SecretValue` のケースを追加した。
- `mise run check` (fmt / lint / test) が通過することを確認した。
