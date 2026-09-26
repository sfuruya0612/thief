# S3 / GCS のプレビュー可能オブジェクトを編集して保存できるようにする

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8

## 背景

S3 / GCS のオブジェクトプレビューは閲覧専用である。
プレビュー可能なテキスト系オブジェクトを thief 上で修正したい場合でも、外部でダウンロード・編集・再アップロードする必要がある。

書き込み経路自体は既に存在する。
S3 は `internal/aws/s3_object.go` の `PutS3Object` と、`POST /api/aws/profiles/{profile}/s3/{bucket}/objects/upload` (`handleS3ObjectUpload`) がある。
GCS は `POST /api/gcp/gcs/{bucket}/objects/upload` (`handleGCPGCSObjectUpload`) がある。
frontend にも `useS3Upload` / `useGcsUpload` (`api/queries.ts`) が存在する。
upload エンドポイントは multipart/form-data の `file` パートを受け取る (`handleS3ObjectUpload`) ため、編集テキストは Blob / File に変換して送る。
プレビューは内容を切り詰めない。`handleS3ObjectPreview` は `maxPreviewSize` (5 MiB) 以上を `PREVIEW_TOO_LARGE` で、非テキストを拒否し、条件を満たすオブジェクトは全内容を返す (`handlers_object_preview.go`)。

## 目的

プレビュー可能なオブジェクトをブラウザ上で編集し、既存の書き込み経路を用いて S3 / GCS へ書き戻せるようにする。

## 対応内容

### backend

- 既存の upload エンドポイントで編集後の内容を書き戻せるかを確認し、不足があれば補う
- 競合検出を行う場合、プレビュー応答に ETag / 世代を含め、`PutS3Object` などの書き込みへ条件付き更新 (S3 の IfMatch、GCS の Conditions) を追加する

### frontend

- プレビュー画面に編集モードと保存操作を追加する
- 保存前に上書きの確認を求める
- 保存後にプレビュー内容と一覧を再取得する (既存の invalidateQueries を利用する)

## 設計上の論点

- 保存経路の上限とプレビュー条件の整合を確認する必要がある。編集可能なのは 5 MiB 未満のテキストに限られるが、upload 側の上限 (`handleS3ObjectUpload` の受理サイズ) がこれと一致しているかを確認する。
- 書き込みは破壊的操作である。誤操作による上書きを防ぐ確認フローを必須とする。
- 取得後に第三者が更新した場合の競合をどう扱うか (ETag / 世代による条件付き更新で弾くか、警告のみか) を決める必要がある。現状 ETag は一覧の `S3ObjectResource` にはあるがプレビュー応答には無く、GCS の `ObjectInfo` は ETag / 世代を持たないため、条件付き更新を行うならこれらを編集フローへ露出させる前段作業が要る。
- 書き込み権限を持たないプロファイルでの挙動 (保存操作の非活性化、エラー表示) を定義する必要がある。
- 保存時の Content-Type をどう決定するか (取得時の値を維持するか、拡張子から再判定するか) を決める必要がある。

## 完了条件

- プレビュー可能なオブジェクトを編集して保存できる
- 保存前に上書き確認が表示される
- 保存後にプレビューと一覧へ最新内容が反映される
- 書き込み不可の場合にユーザへ理由が提示される

## 解決方法

設計上の論点は以下の方針とした (ユーザ判断)。

- 競合検出 (ETag / 世代) は導入しない。最後に書いた者勝ちで上書きする。GCS の `ObjectInfo` に
  世代がなく S3/GCS で非対称になること、上書き確認ダイアログで誤操作は別途予防できることから、
  実装コストに見合わないと判断した。
- 保存時の Content-Type はプレビュー取得時の値を維持し、拡張子からの再判定は行わない。

### backend

- 既存の upload エンドポイント (`handleS3ObjectUpload` / `handleGCPGCSObjectUpload`) をそのまま
  書き戻し経路として利用した。編集内容は 5 MiB 未満のテキストに限られ、upload 側の上限
  (`maxS3UploadSize` 100 MiB) を十分下回るため変更不要。
- 書き込み権限を持たないプロファイルでの挙動は、既存の `writeAWSError` がクラウド API のエラー
  (AccessDenied 等) をそのまま HTTP エラーとして返す経路に委ねた。プレビュー時点での権限事前
  チェックは行わない。

### frontend

- `components/Drawer/DrawerObjectPreview.tsx` に編集モードを追加した。編集は表示形式 (CSV
  テーブル / JSON 整形) に関わらず常に生テキストを直接編集する。
- 保存前に `window.confirm` で上書き確認を行う。既存のモーダル/ダイアログ実装が無いため、
  1 箇所だけの確認のために専用コンポーネントを新設せず標準の `confirm` を使った。
- `components/Drawer/DrawerObjectBrowser.tsx` に、フィルタ入力由来の `uploadPrefix` を付けない
  保存専用の `useUpload(undefined)` インスタンスを追加し、プレビュー中のフルキーへそのまま
  upload するようにした。
- `api/queries.ts` の `useS3Upload` / `useGcsUpload` の `onSuccess` に、対応する
  `*-object-preview` クエリキーの invalidate を追加した (既存の invalidateQueries を利用)。

### 完了条件の確認

- `DrawerObjectPreview.test.tsx` / `DrawerS3Objects.test.tsx` に編集 → 上書き確認 → 保存 →
  upload API 呼び出し、保存失敗時のエラー表示と編集モード維持のテストを追加し、全て通過を確認した。
