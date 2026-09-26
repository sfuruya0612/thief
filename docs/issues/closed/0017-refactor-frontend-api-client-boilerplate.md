# API クライアント層の null 正規化とリクエスト定型を共通化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

1. `frontend/src/api/endpoints.ts` で `apiGet<T[] | null>(path, params).then((v) => v ?? [])` という「null レスポンスを空配列へ正規化する」定型が 22 箇所に反復している (getProfiles / getResources / getCost / getRegions / getS3Objects / ECS 3 種 / getECRImages / ELB 4 種 / getDynamoItems / BQ 3 種 / Datadog 2 種 / TiDB 3 種 / getGcpProjects / getGcpResources / getGcsObjects)。

2. `frontend/src/api/client.ts` 内部で以下が重複している。

   - クエリパラメータ直列化ブロック: `apiGet` (40-45 行) と `apiPostForm` (103-108 行) で完全同一
   - fetch の try/catch → `ApiError(0, 'network_error', ...)` 正規化: `apiGet` / `apiPost` / `apiPostForm` の 3 箇所で同一
   - `if (!res.ok) await throwApiError(res)`: 3 箇所
   - 202/204 の空ボディ処理: `apiPost` と `apiPostForm` で同一

## 対応方針

- `client.ts` に `apiGetList<T>(path, params): Promise<T[]>` を追加し (内部で `?? []`)、`endpoints.ts` のリスト系呼び出しを置き換える。
- `client.ts` 内部に `buildUrl(path, params)` と「fetch 実行 + ネットワークエラー正規化 + 非 2xx の ApiError 変換」を担う内部関数を切り出し、apiGet / apiPost / apiPostForm から共用する。202/204 の空ボディ処理は現状どおり POST 系のみに適用する (apiGet の挙動を変えない)。

## 画面表示への影響

なし。リクエスト URL・ヘッダ・エラー変換・返却値は同一。

## 解決方法

- `client.ts` に内部ヘルパを切り出した。
  - `buildUrl(path, params)`: クエリパラメータ直列化 (undefined スキップ、boolean 文字列化) を一元化。
  - `doFetch(url, init)`: fetch 実行 + ネットワーク到達不能の `ApiError(0, 'network_error')` 正規化 + 非 2xx の `throwApiError` 変換を一元化。
  - `parsePostResponse<T>(res)`: 202/204 の空ボディ処理を POST 系 (apiPost / apiPostForm) のみに適用。apiGet の挙動は変えていない。
- `apiGetList<T>(path, params): Promise<T[]>` を追加し、内部で `?? []` の空配列正規化を行うようにした。
- `endpoints.ts` の `apiGet<T[] | null>(...).then((v) => v ?? [])` 22 箇所を `apiGetList<T>(...)` に置換した。単一オブジェクトを返す `getProfileIdentity` / `getDynamoSchema` は apiGet のまま。
- `mise run check` 通過 (frontend: 0 errors / 105 tests passed) を確認した。
