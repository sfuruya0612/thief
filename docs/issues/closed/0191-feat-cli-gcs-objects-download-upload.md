# CLI に GCS オブジェクトのダウンロード・アップロードを追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-18

## 背景

Web の GCS は Drawer の Objects タブからオブジェクトの一覧・プレビュー・ダウンロード・アップロード・編集ができる。backend の取得層には次の関数が揃っている。

- `ListObjects` (`backend/internal/gcp/gcs.go:66`) : CLI の `gcp gcs objects` は実装済み
- `GetObject` (`backend/internal/gcp/gcs.go:110`)
- `PutObject` (`backend/internal/gcp/gcs.go:132`)

一方 CLI は `gcp gcs objects <bucket>` (一覧のみ、`backend/internal/cli/gcp.go:349`) しかなく、S3 と同じく Object Storage の操作が CLI から行えない。

## 目的

CLI から GCS オブジェクトのダウンロード・アップロードを行えるようにする。

## 設計判断

- S3 と対称なサブコマンドを `gcp gcs` 配下に追加する。
  - `thief gcp gcs download <bucket> <key> [--output-file <path>]`
  - `thief gcp gcs upload <bucket> <key> <file>`
- ダウンロード先と Content-Type の扱いは issue 0190 の S3 と同じ規則にする。純関数は S3 のものを共有する (`internal/cli` 内の共通ヘルパー)。
  - 却下案：GCS 専用の純関数を別に作る案。同じ規則を二重管理するため却下。
- 追加の API・権限は不要。

## 完了条件

- `thief gcp gcs download <bucket> <key>` が `--output-file` のパス (省略時は key の basename) にオブジェクトを書き出し、書き出し先のパスを stdout に表示する。
- `thief gcp gcs upload <bucket> <key> <file>` がファイルをアップロードし、Content-Type を拡張子から推定する。
- 出力先解決と Content-Type 推定は S3 と共有する純関数を使う (単体テストは issue 0190 で追加するものを共有する)。
- `mise run check` が通過する。

## 解決方法

- `backend/internal/cli/gcp.go` の `gcp gcs` 配下に `download` / `upload` サブコマンドを追加した。`download` は `gcp.GetObject` の `ObjectReader` をローカルファイルへコピーし、`upload` は `gcp.PutObject` にファイルを渡す。
- 出力先解決と Content-Type 推定は issue 0190 で追加した `resolveDownloadPath` / `contentTypeForFile` を共有する。
- `mise run check` (fmt / lint / test) が通過することを確認した。
