# 0025 S3 / GCS オブジェクトのブラウザプレビューを追加する

Created: 2026-07-17
Completed: 2026-07-18
Model: Claude Fable 5 claude-fable-5

## 背景 / 根拠

S3 / GCS のオブジェクトブラウザ (共通コンポーネント `frontend/src/components/Drawer/DrawerObjectBrowser.tsx`) からはダウンロードしかできない。
中身をひと目確認したいだけでも、ローカルに保存してエディタで開く手間がかかる。
csv / txt / json はログ、設定、データ交換の主要形式であり、ブラウザ内でそのまま読めれば調査が速くなる。

オブジェクトの中身取得は backend に実装済みで (`GetS3Object` `backend/internal/aws/s3_object.go:57`、GCS の `GetObject` `backend/internal/gcp/gcs.go:103`)、現状の利用者はダウンロードハンドラのみ。
プレビューに足りないのは、サイズ上限つき読み込み、拡張子ガード、テキスト表示 UI の 3 点に絞られる。

## 対応内容

- オブジェクトブラウザの各行にプレビューアクションを追加する (S3 / GCS 共通)
- プレビュー可能なのは拡張子が csv / txt / json (大文字小文字を区別しない) かつサイズ 5 MB 未満のオブジェクトに限る。5 MB 以上、対象外拡張子の行はアクションを無効化し、理由を表示する
- txt / json はテキスト表示 (json は整形表示)、csv はテーブル表示する

## 実装方針

### 方式: 専用プレビュー API (JSON エンベロープ)

既存のダウンロード URL をフロントで fetch する方式ではなく、専用のプレビュー API を新設する。理由は 2 つある。

- サイズ上限を backend で強制できる。フロントの `row.size` 判定だけでは、一覧取得後に肥大したオブジェクトや API 直叩きに対して 5 MB 超のボディを丸ごと読んでしまう
- レスポンスを JSON エンベロープ (`{"content": "...", "content_type": "...", "size": N}`) にすれば、既存の `apiGet` (JSON 固定、`frontend/src/api/client.ts:81`) と `ApiError` のエラーハンドリングにそのまま乗る。生テキスト応答だと `apiGetText` の新設が必要になる

### backend

S3 / GCS で対称に実装する。

- ルート (`backend/internal/api/routes.go`):
  - `GET /api/aws/profiles/{profile}/s3/{bucket}/objects/preview?key=&region=` → `handleS3ObjectPreview` (`backend/internal/api/handlers_s3_object.go`)
  - `GET /api/gcp/gcs/{bucket}/objects/preview?key=&project_id=` → `handleGCPGCSObjectPreview` (`backend/internal/api/handlers_gcp.go`)
- 共通の検証ロジックはヘルパーに切り出し、両ハンドラで共有する
  1. 拡張子ガード: key 末尾が `.csv` / `.txt` / `.json` (case-insensitive) でなければ 400 (エラーコード例 `PREVIEW_UNSUPPORTED_TYPE`)
  2. サイズ事前判定: `GetObjectOutput.ContentLength` (S3) / `ObjectReader.Size` (GCS) が 5 MB 以上なら Body を閉じて 413 (エラーコード例 `PREVIEW_TOO_LARGE`)。5 MB 以上を転送してから捨てるのではなく、読み込み前に弾いて転送量を抑える
  3. 読み込み防御: 実読み込みは `io.LimitReader(body, maxPreviewSize+1)` で行い、上限超過を検出したら同じく 413。サイズメタデータと実体が食い違うケース (返却された ContentLength が信頼できない場合) への防御で、アップロード側の `readS3UploadBody` (`handlers_s3_object.go:78`) と同じパターン
  4. テキスト判定: 読み込んだバイト列が `utf8.Valid` でなければ 422 (エラーコード例 `PREVIEW_NOT_TEXT`)。拡張子が .txt でも実体がバイナリのオブジェクトを文字化けのまま返さないため
- 定数は `maxPreviewSize = 5 << 20` として 1 箇所で定義する。TODO の要件「5 MB 以上のものはプレビュー不可」に合わせ、判定は「5 MB 未満のみ許可」とする
- csv のパースは行わず、content をそのまま返す (パースは frontend の責務。backend はバイト列の取得と制限に徹する)
- ダウンロードハンドラと同様にキャッシュ (`serveCached`) は通さない
- 新エラーコードを追加する場合は `backend/internal/api/errors.go` のライタ群 (`writeError` 系) の形式に合わせる

### frontend

- `frontend/src/components/Drawer/DrawerObjectBrowser.tsx`: Download 列 (`:81` 付近) の隣に Preview アクションを追加する。共通コンポーネントのため S3 / GCS 同時に対応できる
  - 活性判定は行データで行う: `row.size < 5 MB` かつ key / name の末尾が対応拡張子。不可の行はボタンを無効化し、`title` 属性で理由 (サイズ超過 / 非対応形式) を示す
  - S3 の一覧は Content-Type を持たない (`S3ObjectResource` に ContentType フィールドがない) が、判定を拡張子ベースに統一することで S3 / GCS の差異を吸収する
- 新規 `frontend/src/components/Drawer/DrawerObjectPreview.tsx`: プレビュー本体
  - txt: 等幅の `<pre>` 表示。体裁は `frontend/src/app.css:1709` の `.logbox` (現在未使用) を流用する
  - json: `JSON.parse` して 2 スペースインデントで整形表示、パース失敗時は生テキストにフォールバックする
  - csv: パースして既存の `ResultTable` (`frontend/src/components/query/ResultTable.tsx`、ソートとページング付き) で表示する
  - 閉じるボタンでオブジェクト一覧へ戻る
- csv パーサ: 外部依存を増やさず、`frontend/src/lib/parseCsv.ts` に RFC 4180 準拠の純関数を新設する (ダブルクォートで囲まれたフィールド内のカンマ、改行、`""` エスケープに対応)。`String.prototype.split(',')` の素朴な実装ではこれらを壊すため採用しない
- API 層:
  - `frontend/src/api/endpoints.ts`: `getS3ObjectPreview` / `getGcsObjectPreview` (既存 `s3DownloadUrl` `:155` / `gcsDownloadUrl` `:637` の隣)
  - `frontend/src/api/queries.ts`: `useS3ObjectPreview` / `useGcsObjectPreview` (`enabled: !!previewKey` でプレビュー対象確定時のみ取得)
  - 型: プレビューレスポンスの Raw 型 (`{content, content_type, size}`) を S3 / GCS 共通で定義する

### テスト

- backend: 拡張子ガード (境界: `.JSON` 大文字、`.json.gz` のような多重拡張子は不可)、サイズ境界 (5 MB ちょうどは不可、5 MB - 1 byte は可)、`ContentLength` と実体が食い違う場合の LimitReader 検出、UTF-8 判定、をテーブル駆動でテストする
- frontend: `parseCsv` のユニットテストを厚くする (クォート内カンマ、クォート内改行、`""` エスケープ、末尾改行、空フィールド、CRLF)。活性判定 (サイズ / 拡張子) のユニットテスト、`DrawerObjectPreview` の json 整形 / フォールバックのコンポーネントテスト

## スコープ外

- csv / txt / json 以外の形式 (画像、parquet、gzip 等) のプレビュー
- 5 MB 以上のオブジェクトの部分プレビュー (先頭 N 行表示)
- プレビューからの編集、保存
- 文字コードの自動判定 (UTF-8 のみ対応。Shift_JIS 等はテキスト判定で弾かれる)

## 検証

- `mise run check` を通す
- 実環境 (または issue 0024 の floci 環境) で、csv / txt / json それぞれのプレビュー表示、5 MB 以上のオブジェクトと非対応拡張子でボタンが無効化されること、バイナリ実体の .txt がエラー表示になることを S3 / GCS 両方で確認する

## 解決方法

方針どおり JSON エンベロープの専用プレビュー API を新設した。

- backend: `handlers_object_preview.go` に共通ヘルパー (`previewExtensionAllowed`、`previewSizeAllowed`、`readPreviewBody`、`buildPreviewResponse`、エラーライタ群) を実装し、`handleS3ObjectPreview` (`handlers_s3_object.go`) / `handleGCPGCSObjectPreview` (`handlers_gcp.go`) の両方から共有した。サイズ判定は「メタデータでの事前弾き (`previewSizeAllowed`)」と「実読み込み時の `io.LimitReader` による防御 (`readPreviewBody`)」の 2 層に分け、それぞれ独立にテーブル駆動テストした (5 MB ちょうどの境界を含む)。UTF-8 判定は `utf8.Valid` で行い、失敗時は `PREVIEW_NOT_TEXT` (422) を返す。ルートは `GET /api/aws/profiles/{profile}/s3/{bucket}/objects/preview` と `GET /api/gcp/gcs/{bucket}/objects/preview` を追加した。
- frontend: `lib/objectPreview.ts` に `fileExtension` (Go の `path/filepath.Ext` と同等の判定。ディレクトリ名中のドットを拡張子と誤認しない) と `isPreviewEligible` / `previewDisabledReason` を実装し、`lib/parseCsv.ts` に RFC 4180 準拠の CSV パーサ (クォート内カンマ・改行、`""` エスケープ、CRLF 対応) を新設した。`components/Drawer/DrawerObjectPreview.tsx` で txt は `<pre>` 等幅表示、json は `JSON.parse` して 2 スペース整形 (失敗時は生テキストへフォールバック)、csv は `ResultTable` でテーブル表示する。`DrawerObjectBrowser.tsx` (S3 / GCS 共通コンポーネント) の Actions 列に Preview ボタンを追加し、`previewKeyOf` / `sizeOf` / `usePreview` を props として注入する形にしたため、`DrawerS3Objects.tsx` / `DrawerGCSObjects.tsx` の変更は薄いフック注入のみで済んだ。
- テスト: backend は `handlers_object_preview_test.go` (拡張子ガード 9 ケース、サイズ境界 4 ケース、LimitReader 挙動 5 ケース、UTF-8/サイズエラー 4 ケース) を追加し `go test -race ./...` 全通過。frontend は `lib/objectPreview.test.ts`、`lib/parseCsv.test.ts`、`components/Drawer/DrawerObjectPreview.test.tsx` を新設し、`DrawerS3Objects.test.tsx` に Preview ボタンの無効化判定・クリックからのプレビュー表示までを検証するテストを追加した。GCS 側は S3 と共通の `DrawerObjectBrowser` / `DrawerObjectPreview` を使うため個別テストは追加せず、共通コンポーネントのテストでカバーした。`mise run frontend:lint` / `mise run frontend:test` (309 件) / backend `go build` / `go vet` / `go test -race` (全パッケージ) が全て通過することを確認した。
