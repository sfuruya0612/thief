# 0002. 1 つの Go バイナリで API サーバと CLI を提供する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-12

## 状況

thief は、Web UI 用の API サーバと、端末で使う CLI の両方を提供する。
どちらもクラウドのリソースを取得して一覧にする処理を持つ。
2026-07-08 のモノレポ移行 (コミット 2aac22f) では、API サーバの `backend/cmd/server` と CLI の `backend/cmd/thief` が別のエントリポイントとして並んでいた。

## 決定

エントリポイントを `backend/cmd/thief/main.go` の 1 つにする。
API サーバは `thief server` サブコマンドとして起動する (コミット 7497b0c 「cmd/server を廃止して thief server サブコマンドに統一する」)。
リソースの取得層 (`backend/internal/aws/`、`internal/gcp/`、`internal/bigquery/`、`internal/datadog/`、`internal/tidb/`) を、CLI (`internal/cli/`) と API (`internal/api/`) で共有する。

## 検討した代替案

記録なし。

## 結果

- リソースの取得を CLI と API で二重に実装しない。
- `internal/api` は `internal/cli` を import できない。
  両方が使う処理は独立したパッケージに切り出す。
  例として、SSO のデバイス認可を `internal/ssoauth` に切り出した (ADR 0013)。
  `backend/internal/config/config.go` にも、循環 import を避けるための定数の配置がある。
- 配布物は 1 つのバイナリになる (`mise run backend:install`)。

## 根拠資料

- コミット 7497b0c (2026-07-12)
- `AGENTS.md` の backend 「プロジェクト構造」
- `backend/cmd/thief/main.go`、`backend/internal/cli/root.go` `NewRootCmd`、`backend/internal/cli/server.go`
