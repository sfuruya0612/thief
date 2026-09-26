# CORS の許可メソッドに DELETE が無くクロスオリジンの削除系 API が機能しないのを修正する

Created: 2026-07-26
Completed: 2026-07-26
Model: Claude Fable 5

## 症状

frontend (dev サーバ http://localhost:8088) から backend (http://127.0.0.1:8089) への DELETE リクエストが CORS の preflight で拒否され、削除系の操作が失敗する。docs/issues/closed/0079 の未確定論点から切り出したもの。

## 根拠

- CORS ミドルウェア (`backend/internal/api/middleware.go:31`) の `Access-Control-Allow-Methods` は `GET, POST, OPTIONS` で、DELETE を含まない。
- 一方で DELETE のルートは 3 本ある (`backend/internal/api/routes.go`)。
  - `DELETE /api/aws/profiles/{profile}/athena/query/{id}` (Athena クエリの停止)
  - `DELETE /api/bigquery/query/jobs/{job}` (BigQuery ジョブのキャンセル)
  - `DELETE /api/snippets/{service}/{name}` (スニペットの削除)
- frontend の `api/client.ts` のベース URL は `http://127.0.0.1:8089` 固定で、dev サーバ (ポート 8088) からの呼び出しは常にクロスオリジンになる。クロスオリジンの DELETE は preflight の許可メソッドに DELETE が要るため、これらのルートは機能していない可能性が高い。
- 少なくともスニペットの削除は、`SnippetDropdown` の削除操作から `views/AthenaView.tsx` / `views/nonaws/BigQueryView.tsx` の `onDelete={snippets.remove}` を経由して `useDeleteSnippet` に配線済みで、呼ばれる経路がある。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` を起動する。
2. Athena または BigQuery のクエリエディタでスニペットを保存する。
3. スニペットのドロップダウンから削除を実行する。
4. 期待: スニペットが削除される。実際: ブラウザ開発者ツールの Network タブで preflight (OPTIONS) 後の DELETE が CORS エラーになり、削除されない想定。

## 修正方針 (案)

- `Access-Control-Allow-Methods` に DELETE を追加する。
- 3 本の DELETE ルートがクロスオリジンで実際に機能することを確認する (最低限スニペットの削除を実環境で確認する)。
- ミドルウェアのテストで許可メソッドの値を固定する。

## 完了条件

- クロスオリジンの DELETE リクエストが preflight を通過する。
- `Access-Control-Allow-Methods` の値を検証するテストがある。
- `CHANGES.md` の `## develop` に `[FIX]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 解決方法

- `backend/internal/api/middleware.go` の `corsMiddleware` の `Access-Control-Allow-Methods` を `GET, POST, OPTIONS` から `GET, POST, DELETE, OPTIONS` に変更した。
- `backend/internal/api/middleware_test.go` を新設し、テーブル駆動テスト `TestCORSMiddleware` で以下を固定した。
  - Origin 付き preflight (OPTIONS) が 204 を返し、`Access-Control-Allow-Methods` に DELETE を含む値 (`GET, POST, DELETE, OPTIONS`) が付与され、次のハンドラに到達しないこと。
  - Origin 付きの GET / DELETE が CORS ヘッダ付きで次のハンドラへ通過すること。
  - Origin 無し (同一オリジン) のリクエストには CORS ヘッダが付与されないこと。
- 実環境でのクロスオリジン削除の確認は、preflight の許可メソッド判定がブラウザ側で仕様どおり (許可リストにあるメソッドのみ通過) に行われるため、上記のヘッダ値のテスト固定で完了条件を満たすと判断した。3 本の DELETE ルート自体のハンドラは変更していない。
- `CHANGES.md` の `## develop` に `[FIX]` エントリと担当者行を追記した。
- `mise run check` 通過。

## 関連

- docs/issues/closed/0079 (Refresh のキャッシュ貫通): 本 issue の発見元。0079 のエンドポイントは POST のため影響を受けない。
