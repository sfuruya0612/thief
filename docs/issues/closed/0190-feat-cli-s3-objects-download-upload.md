# CLI に S3 オブジェクトの一覧・ダウンロード・アップロードを追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-18

## 背景

Web の S3 はバケット一覧に加え、Drawer の Objects タブからオブジェクトの一覧・プレビュー・ダウンロード・アップロード・編集ができる。backend の取得層には次の関数が揃っている。

- `ListS3Objects` (`backend/internal/aws/s3_object.go:37`)
- `GetS3Object` (`backend/internal/aws/s3_object.go:82`)
- `PutS3Object` (`backend/internal/aws/s3_object.go:100`)

一方 CLI は `s3 ls` (バケット一覧のみ、`backend/internal/cli/s3.go`) しかなく、オブジェクトを操作できない。Object Storage の操作が CLI から行えないのは運用上の主要な欠落である。

## 目的

CLI から S3 オブジェクトの一覧・ダウンロード・アップロードを行えるようにする。

## 設計判断

- サブコマンドを `gcp gcs` の並び (`backend/internal/cli/gcp.go`) に合わせて追加する。
  - `thief s3 objects <bucket> [--prefix]` : 一覧 (最大 1000 件、打ち切り時は stderr に警告)
  - `thief s3 download <bucket> <key> [--output-file <path>]` : ダウンロード
  - `thief s3 upload <bucket> <key> <file>` : アップロード
- ダウンロード先は `--output-file` 省略時に key の basename をカレントディレクトリに使う。既存ファイルは上書きする (`aws s3 cp` と同じ)。key が `/` 終わりの場合は basename が決まらないため `--output-file` を必須にする。
  - 却下案：省略時に標準出力へ書く案。バイナリを端末に流す事故になるため却下。
  - 却下案：既存ファイルがある場合にプロンプトで確認する案。非対話のスクリプト用途を妨げるため却下。
- アップロードの Content-Type は拡張子から `mime.TypeByExtension` で推定し、不明なら空のまま `PutS3Object` に渡す (`PutS3Object` は空なら ContentType を付けない)。
- ダウンロードの出力先解決と Content-Type 推定は純関数に切り出し、単体テストを書く。AWS 呼び出し本体は薄いコマンドに留める。
- 追加の API・権限は不要 (`GetObject` / `PutObject` の権限は把握済みの Web 機能と同一)。

## 完了条件

- `thief s3 objects <bucket>` が Name / Size / LastModified / StorageClass / ETag の列で一覧を tab 出力し、`--prefix` で絞り込める。CSV 出力もできる。
- `thief s3 download <bucket> <key>` が `--output-file` のパス (省略時は key の basename) にオブジェクトを書き出し、書き出し先のパスを stdout に表示する。
- `thief s3 download <bucket> <key-with-trailing-slash>` で `--output-file` 省略時はエラーになる。
- `thief s3 upload <bucket> <key> <file>` がファイルをアップロードし、Content-Type を拡張子から推定する。
- 出力先解決と Content-Type 推定の純関数に単体テストがある。
- `mise run check` が通過する。

## 解決方法

- `backend/internal/cli/s3.go` に `objects` / `download` / `upload` サブコマンドを追加した。`objects` は既存の `ListS3Objects` を呼び、1000 件打ち切り時は stderr に警告する。`download` は `GetS3Object` のストリームをローカルファイルへコピーし、`upload` は `PutS3Object` にファイルと `info.Size()` を渡す。
- 出力先解決 `resolveDownloadPath` と Content-Type 推定 `contentTypeForFile` を純関数として切り出し、`backend/internal/cli/s3_test.go` に単体テストを追加した。key が `/` 終わりのとき `--output-file` 省略はエラーになる。
- `mise run check` (fmt / lint / test) が通過することを確認した。
