# 0033 S3 / GCS プレビュー対象をバイナリ以外に拡張しプレビュー不可の行をグレーアウトする

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8 claude-opus-4-8

## 背景 / 根拠

現状の S3 / GCS オブジェクトプレビューは、拡張子が csv / txt / json のいずれか、かつ 5 MB 未満のオブジェクトのみを対象としている。
実際にはログ (`.log`)、Markdown (`.md`)、YAML (`.yaml`)、各種ソースコードなど、テキストであればプレビューできると便利なファイルが多数ある。
拡張子ホワイトリストを 3 種類に固定していると、これらが一律にプレビュー不可になる。

そこで、プレビュー可否の基準を「限定した拡張子のみ許可」から「バイナリ形式でなければ許可」に反転させる。
あわせて、プレビュー不可のオブジェクトは Preview ボタンを無効化するだけでなく、一覧行をグレーアウトして視認できるようにする。

## 要件

- バイナリ形式でないオブジェクトはプレビューできるようにする (TODO の「バイナリ以外はできるようにしたい」)。
- プレビュー不可のオブジェクトは、Preview ボタンを押下できないだけでなく一覧行をグレーアウトする (TODO の「グレーアウトして欲しい」)。
- 5 MB 以上をプレビュー不可とする既存の上限は維持する (本 issue のスコープ外)。

## 方針

### バイナリ判定

「バイナリか否か」を確実に判定するにはオブジェクトの中身が要るが、一覧の時点では中身を取得しない (S3 の一覧レスポンスは Content-Type すら持たない)。
そのため二段構えにする。

- 一覧行のプレビュー可否 (ボタン活性・グレーアウト) は、拡張子の**バイナリ拡張子デナイリスト**で判定する。既知のバイナリ拡張子 (画像・動画・音声・アーカイブ・実行ファイル・バイナリ文書・フォント・シリアライズ形式など) を持つオブジェクトのみをプレビュー不可とし、それ以外 (拡張子なしを含む) はプレビュー可能候補として扱う。判定材料が拡張子とサイズのみで、backend / frontend の双方が同じ結論を出せる。
- 実際のプレビュー実行時は backend が中身を読み、UTF-8 として妥当か、かつ NUL バイトを含まないかを検査する。これを最終権威とし、拡張子では見抜けないバイナリ (テキスト拡張子を持つバイナリ、拡張子なしのバイナリ) を確実に弾く。既存の `utf8.Valid` 判定に NUL バイト検査を追加する (UTF-16 やバイナリは NUL を含むが NUL 自体は妥当な UTF-8 のため、`utf8.Valid` だけでは見抜けない)。

一覧行が「プレビュー可能候補」と判定したオブジェクトでも、中身がバイナリなら実行時に 422 で拒否され、フロントはエラー文言を表示する。
これは一覧時点で中身を持たない S3 の制約上避けられない挙動であり、許容する。

### グレーアウト

- 汎用テーブル (`DataTable`) は行ごとの追加クラスを受け取れないため、`rowClassName?: (row) => string | undefined` プロパティを追加する (既存の利用箇所は未指定で従来どおり)。
- オブジェクトブラウザ (`DrawerObjectBrowser`) は、プレビュー不可の行に `preview-ineligible` クラスを付与する。
- CSS では、グレーアウト対象行のメタデータ列を淡色化しつつ、Actions 列 (Download / Preview) は淡色化しない。プレビュー不可でもダウンロードは可能なため、Download を誤って無効に見せない。無効化された Preview ボタンには `.btn:disabled` のスタイルを与える (現状未定義)。

## 変更対象

### backend

- `backend/internal/api/handlers_object_preview.go`
  - `previewAllowedExtensions` (ホワイトリスト) を `previewBinaryExtensions` (デナイリスト) に置き換える。
  - `previewExtensionAllowed` を「拡張子が既知バイナリでなければ true」に変更する。
  - `buildPreviewResponse` の UTF-8 判定に NUL バイト検査を追加する。
  - `writePreviewUnsupportedType` / `errPreviewNotText` の文言を実態に合わせる。

### frontend

- `frontend/src/lib/objectPreview.ts`: `PREVIEW_ALLOWED_EXTENSIONS` を `PREVIEW_BINARY_EXTENSIONS` に置き換え、`isPreviewEligible` / `previewDisabledReason` をデナイリスト基準にする。
- `frontend/src/components/DataTable.tsx`: `rowClassName` プロパティを追加する。
- `frontend/src/components/Drawer/DrawerObjectBrowser.tsx`: プレビュー不可の行に `preview-ineligible` を付与する。
- `frontend/src/app.css`: `table.dt tbody tr.preview-ineligible` のグレーアウトと `.btn:disabled` のスタイルを追加する。

### テスト / ドキュメント

- `backend/internal/api/handlers_object_preview_test.go`: 拡張子判定 (バイナリ拡張子・拡張子なし・テキスト拡張子) と NUL バイト検査のケースを更新・追加する。
- `frontend/src/lib/objectPreview.test.ts`: デナイリスト基準に更新する。
- `frontend/src/components/Drawer/DrawerS3Objects.test.tsx`: Preview ボタンの無効化理由文言とグレーアウト行の検証を更新・追加する。
- `CHANGES.md` の `## develop` に追記する。

## スコープ外

- 5 MB のサイズ上限の変更。
- 非 csv / json のテキストを整形して表示すること (csv はテーブル、json は整形、その他テキストは生テキスト表示のまま。Markdown レンダリング等は行わない)。
- S3 一覧レスポンスへの Content-Type 追加 (オブジェクト毎の HeadObject が必要でコストが高いため行わない)。

## 検証

- テキスト系拡張子 (`.log` / `.md` / `.yaml` など) と拡張子なしのテキストがプレビューできることを確認する。
- 画像などバイナリ拡張子のオブジェクトが一覧でグレーアウトされ、Preview ボタンが無効になり、Download は可能なことを確認する。
- テキスト拡張子を持つがバイナリ内容のオブジェクトが、プレビュー実行時に 422 で拒否されエラー表示されることを確認する。
- floci (`example/`) の S3 に投入したサンプルオブジェクトで一覧・プレビュー・グレーアウトを実機確認する。
- `mise run check` が全通過することを確認する。

## 解決方法

### backend (`handlers_object_preview.go`)

- 許可拡張子ホワイトリスト `previewAllowedExtensions` (csv/txt/json) を、既知バイナリ拡張子のデナイリスト `previewBinaryExtensions` (画像・動画・音声・アーカイブ・実行ファイル・バイナリ文書・フォント・シリアライズ形式・ディスクイメージ/DB) に置き換えた。
- `previewExtensionAllowed` を「拡張子が既知バイナリでなければ true (拡張子なしを含む)」に反転した。
- `buildPreviewResponse` のテキスト判定を `utf8.Valid` に加え「NUL バイトを含まない」条件を追加した (NUL は妥当な UTF-8 のため utf8.Valid だけでは UTF-16 やバイナリを見抜けない)。`bytes.IndexByte(data, 0) >= 0` で検出する。
- `writePreviewUnsupportedType` / `errPreviewNotText` / `writePreviewNotText` の文言を実態 (バイナリ判定) に合わせた。
- S3 / GCS ハンドラ (`handlers_s3_object.go` / `handlers_gcp.go`) の doc コメントを更新した。

### frontend

- `lib/objectPreview.ts`: `PREVIEW_ALLOWED_EXTENSIONS` を backend と同じ集合の `PREVIEW_BINARY_EXTENSIONS` に置き換え、`isBinaryExtension` を追加。`isPreviewEligible` / `previewDisabledReason` をデナイリスト基準にした (不可理由を「バイナリファイルはプレビューできません」に変更)。
- `components/DataTable.tsx`: 行ごとの追加クラスを返す `rowClassName?: (row) => string | undefined` プロパティを追加した (既存の利用箇所は未指定で従来どおり)。
- `components/Drawer/DrawerObjectBrowser.tsx`: プレビュー不可の行に `preview-ineligible` クラスを付与した。
- `app.css`: `table.dt tbody tr.preview-ineligible` でメタデータ列を淡色化し、Actions 列 (最終列) は淡色化しない (Download は使用可能なため)。`.btn:disabled` のスタイルを追加した。

### テスト

- `handlers_object_preview_test.go`: 拡張子判定 (バイナリ拡張子・拡張子なし・各種テキスト拡張子・多重拡張子) と NUL バイト検査のケースを更新・追加した。
- `lib/objectPreview.test.ts`: デナイリスト基準に更新し `isBinaryExtension` のテストを追加した。
- `components/Drawer/DrawerS3Objects.test.tsx`: Preview ボタン無効化理由の文言とグレーアウト行 (`preview-ineligible` クラス) の検証を追加した。

### 実機検証 (floci)

`example/` の floci に `preview-test` バケットを作り、テキスト系 (`.log` / `.yaml` / `.md` / `.go` / 拡張子なし `Dockerfile`)、バイナリ拡張子 (`.png`)、テキスト拡張子だが NUL 混入 (`tricky.txt`) を投入して確認した。

- backend API: `.log` はプレビュー内容を返し、`.png` は 400、`tricky.txt` は 422 `PREVIEW_NOT_TEXT` を返すことを確認した。
- ブラウザ (Playwright + システム Chrome): テキスト系 5 種の Preview ボタンが有効で行がグレーアウトされないこと、`.png` の行がグレーアウトされ Preview ボタンが無効 (title「バイナリファイルはプレビューできません」) で Download は使用可能なこと、`.log` のプレビューでテキストが表示されること、`tricky.txt` のプレビューでバイナリ判定エラーが表示されることを目視確認した。

### 確認

- `mise run check` が全通過することを確認した (lint は既存 warning のみ、frontend テスト 337 件パス、backend テストパス)。
