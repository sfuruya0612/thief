# S3 へのアップロードが 411 MissingContentLength で失敗する

Created: 2026-07-10
Completed: 2026-07-10
Model: Claude Sonnet 5

## 再現手順

1. `mise run backend:run` / `mise run frontend:run` (または docker compose) で起動する。
2. S3 バケットの Drawer を開き、Objects タブでファイルをアップロードする。

## 現象

```
put s3 object <bucket>/<key>: operation error S3: PutObject, https response error StatusCode: 411, RequestID: ..., HostID: ...,
api error MissingContentLength: You must provide the Content-Length HTTP header.
```

## 原因

`backend/internal/api/handlers_s3_object.go` の `handleS3ObjectUpload` は、multipart の `file` パート (`*multipart.Part`、`io.Seeker` を実装しない `io.Reader`) をそのまま `awsinternal.PutS3Object` の body として渡し、`contentLength` には固定で `0` を渡していた。

`backend/internal/aws/s3_object.go` の `PutS3Object` は `contentLength > 0` の場合にのみ `s3.PutObjectInput.ContentLength` をセットする実装だったため、`0` を渡すケースでは `ContentLength` が未設定のまま `PutObject` が呼ばれていた。AWS SDK for Go v2 の S3 `PutObject` はシークできない Body かつ `ContentLength` 未設定の場合、リクエストに `Content-Length` ヘッダーを付与できず、そのまま S3 に送信する。S3 の `PutObject` API は `Content-Length` を必須とするため、S3 側が `411 MissingContentLength` を返す。

## 影響

- S3 バケットへのファイルアップロードが常に失敗する。

## 解決方法

`multipart.Part` は事前にサイズが分からないため、アップロード前にメモリへ読み込んで `Content-Length` を確定させる方式にした。

- `backend/internal/api/handlers_s3_object.go` に `maxS3UploadSize` (100MiB) の上限と `readS3UploadBody()` を追加した。`file` パートを `io.LimitReader` で上限+1 バイトまで読み込み、上限を超えた場合は `errS3UploadTooLarge` を返す (無制限にメモリへ読み込むとメモリを圧迫するため)。
- `handleS3ObjectUpload` は読み込んだ `[]byte` を `bytes.NewReader` でラップし、`len(body)` を `contentLength` として `PutS3Object` に渡すように変更した。
- `backend/internal/aws/s3_object.go` の `PutS3Object` は `contentLength` を必須引数とし、常に `ContentLength` をセットするように変更した (条件分岐を削除)。
- `backend/internal/api/handlers_s3_object_test.go` に `TestReadS3UploadBody` を追加し、空・小サイズ・上限ちょうど・上限超過の各ケースを検証した。

検討した代替案:
- `aws-sdk-go-v2/feature/s3/manager` の `Uploader` へ切り替える案を最初に試したが、`manager.NewUploader`/`Upload` が deprecated (`feature/s3/transfermanager` への移行を推奨) であり、`go vet` の deprecated 検出 (SA1019) で `mise run backend:lint` が失敗するため不採用とした。後継の `transfermanager` は `v0.3.1` で pre-1.0 (API 破壊的変更の可能性あり) のため、堅牢性優先の方針の下では時期尚早と判断した。

`mise run backend:fmt` / `mise run backend:lint` / `mise run backend:test` がすべて成功することを確認した。
