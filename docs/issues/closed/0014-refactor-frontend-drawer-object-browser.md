# DrawerS3Objects と DrawerGCSObjects の重複を共通コンポーネント化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`frontend/src/components/Drawer/DrawerS3Objects.tsx` と `DrawerGCSObjects.tsx` は約 95% 同一。

- `stripLeadingSlashes` / `normalizeUploadPrefix` が完全一致 (コメントまで同一)
- rows の useMemo、Download 列を末尾に足す columns の useMemo、`onUpload` / `pickFile`、return の JSX 全体 (prefix 入力、アップロード dropzone、Loading / error / DataTable 分岐) が構造・クラス名・文言まで一致

差分は実質 4 点のみ:

- キーフィールド: S3 は `r.key`、GCS は `r.name`
- id 射影: S3 は `{ ...r, id: r.key, state: '' }`、GCS は `{ ...r, state: '' }`
- フック: `useS3Objects` / `useS3Upload` vs `useGcsObjects` / `useGcsUpload`
- ダウンロード URL: `s3DownloadUrl(profile, region, bucket, key)` vs `gcsDownloadUrl(projectId, bucket, name)`

オブジェクトストレージ対応 (例: OCI Object Storage) を追加するたびに 150 行が複製される。

## 対応方針

共通コンポーネント `DrawerObjectBrowser<TRow>` を切り出す。

- prefix 入力・選択ファイル・dragOver の state と JSX を共通側に持つ。
- 差分は props 化する: `objects` (取得済みの data / isLoading / error)、`useUpload(uploadPrefix)` (アップロード先 prefix が内部 state 由来のためフックを props で受ける)、`keyOf(row)`、`toTableRow(row)`、`baseColumns`、`downloadHref(row)`。
- 既存 2 コンポーネントは薄いラッパとして残す (呼び出し側は無変更)。

## 画面表示への影響

なし。マークアップ・クラス名・文言を同一に保つ。既存の `DrawerS3Objects.test.tsx` を無変更で通すことを完了条件とする。

## 解決方法

- `frontend/src/components/Drawer/DrawerObjectBrowser.tsx` を新規追加した。prefix 入力・選択ファイル・dragOver の state、`stripLeadingSlashes` / `normalizeUploadPrefix`、prefix 前方一致フィルタ、Download 列の付与、アップロード UI、Loading / エラー / DataTable の分岐をすべて共通側に持つ。
- ストレージ差分は props で注入する: `data` / `isLoading` / `error` (取得結果)、`keyOf` / `toTableRow` (キー射影)、`baseColumns`、`downloadHref`、`useUpload` (アップロード先 prefix が内部 state 由来のためカスタムフックとして受け取り、描画ごとに必ず 1 回呼ぶ)。
- `DrawerS3Objects.tsx` / `DrawerGCSObjects.tsx` は S3 / GCS 固有の注入のみを行う薄いラッパにした (props インターフェースは無変更)。
- 既存の `DrawerS3Objects.test.tsx` (一覧表示 / アップロード POST / prefix クライアントサイドフィルタ / prefix 付きアップロード) を無変更で全通過。
- `mise run check` 全通過を確認した。
