# 0037 GCP API 由来の 4xx エラーを 500 に丸めず適切なステータスとメッセージで返す

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8 claude-opus-4-8

## 背景 / 根拠

Google Cloud の IAM / Service Account を開いた際に、次のエラーで画面が 500 表示になる。

```
500 INTERNAL_ERROR get iam policy for example-project-b: googleapi: Error 403: Cloud Resource Manager API has not been used in project example-project-b before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/cloudresourcemanager.googleapis.com/overview?project=example-project-b then retry. ... reason: SERVICE_DISABLED ... accessNotConfigured
```

根本原因はプロジェクト側で Cloud Resource Manager API が無効 (`SERVICE_DISABLED`) なことであり、これは GCP コンソールでの有効化操作でしか解消できない環境要因である。
問題は、GCP が 403 で返したこのエラーを backend が一律 500 INTERNAL_ERROR に丸めていることにある。

- `backend/internal/api/handlers_gcp.go` の GCP 系ハンドラは `serveCached` のエラー writer に `writeInternalFromError` を渡しており、`writeInternalFromError` は `errors.go` で無条件に 500 INTERNAL_ERROR を書き込む。
- `handleGCPLoggingEntries` および GCS の `GetObject` エラーパスも `writeInternalError(w, err.Error())` で一律 500 にしている。

この結果、利用者から見ると「有効化すれば直る 4xx の設定不備」がサーバ内部エラー (5xx) に見え、かつ本来アクション可能な有効化 URL を含むメッセージが 500 の生ダンプに埋もれる。
BigQuery ハンドラには既に `writeBQError` (`handlers_bigquery.go`) が `*googleapi.Error` の 4xx を透過するパターンを持っており、GCP 系全体で同じ扱いに揃っていない。

## 再現手順

1. Cloud Resource Manager API が無効なプロジェクト (例: `example-project-b`) を選択する。
2. Google Cloud の IAM または Service Account を開く。
3. 画面に `500 INTERNAL_ERROR ...` が表示される。

## 期待する挙動

- API 未有効化 (`SERVICE_DISABLED` / `accessNotConfigured`) は 403 で返し、有効化を促すメッセージ (有効化 URL を含む) をそのまま表示する。
- その他の Google API クライアントエラー (4xx) は当該ステータスで返す。
- サーバ内部起因でない事象を 5xx として扱わない。

## 対応方針

- `backend/internal/api/errors.go` に GCP 系エラーを HTTP ステータスへマップする `writeGCPError` を追加する。
  - `*googleapi.Error` を `errors.As` で取り出し、API 未有効化なら 403 `GCP_API_DISABLED`、その他 4xx なら当該ステータス `GCP_ERROR`、それ以外は 500 `INTERNAL_ERROR` とする。
  - API 未有効化の判定は、旧形式 `Errors[].Reason == "accessNotConfigured"` と新形式 `Details[].reason == "SERVICE_DISABLED"` の双方を検査する。
- `handlers_gcp.go` の GCP 系ハンドラ (Projects / CloudRun / GCS / GCS Objects / IAM / ServiceAccounts / GCS GetObject / Logging Entries) のエラー writer を `writeGCPError` に差し替える。
- フロントは `GcpView` が全エラーを `ErrorBanner` で表示するため変更不要 (backend が返す status / code / message をそのまま表示する)。

## スコープ外

- Cloud Resource Manager API の有効化自体 (GCP コンソール操作)。
- 新規依存の追加 (`google.golang.org/api/googleapi` は BigQuery ハンドラで既に利用中)。

## 検証

- `writeGCPError` のユニットテスト (テーブル駆動) で、SERVICE_DISABLED / accessNotConfigured / 一般 4xx / 5xx / googleapi 以外の error の各ケースを検証する。
- `mise run check` が通ること。

## 解決方法

`backend/internal/api/errors.go` に `writeGCPError` と補助関数 `gcpAPIDisabled` を追加した。

- `writeGCPError` は `errors.As` で `*googleapi.Error` を取り出し、API 未有効化なら 403 `GCP_API_DISABLED` (メッセージは `googleapi.Error.Message`、空なら `err.Error()` にフォールバック)、その他 4xx なら当該ステータス `GCP_ERROR`、いずれにも当たらなければ 500 `INTERNAL_ERROR` を書き込む。
- `gcpAPIDisabled` は旧形式 `Errors[].Reason == "accessNotConfigured"` と新形式 `Details[].reason == "SERVICE_DISABLED"` の双方を検査する。

`backend/internal/api/handlers_gcp.go` の GCP 系ハンドラ計 10 箇所のエラー writer を `writeGCPError` に差し替えた (Projects / CloudRun / GCS バケット / GCS オブジェクト一覧 / IAM / ServiceAccounts の `serveCached` writer と、GCS GetObject (download / preview) / Logging Entries / GCS PutObject (upload) の非キャッシュ経路)。`config.Dir()` のローカル FS エラー (line 42) はサーバ内部起因のため 500 のまま残した。

フロントは `GcpView` が全エラーを `ErrorBanner` (status / code / message 表示) で描画するため変更不要。API 未有効化時は `403 GCP_API_DISABLED` と有効化 URL を含むメッセージが表示される。

`backend/internal/api/errors_test.go` に `TestWriteGCPError` (テーブル駆動 7 ケース) を追加し、`mise run check` (fmt / lint / test) が通ることを確認した。
