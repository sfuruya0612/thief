# 0018. S3 と GCS のオブジェクトの SQL 検索を、ブラウザ内の DuckDB Wasm で行う

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-09-25

## 状況

オブジェクトのプレビューは 5 MiB 未満のテキストに限られ、parquet を読めず、1 GB 級のオブジェクトには使えない。
利用者は CSV、JSON、parquet のオブジェクトを SQL で絞り込みたい (`docs/issues/TODO.md`)。

## 決定

- `@duckdb/duckdb-wasm` (1.32.0 に固定) と `apache-arrow` を frontend に追加する。
- オブジェクトを Worker が OPFS に書き、`obj` ビューとして SQL を実行する。
- 対象は csv、tsv、json、jsonl、ndjson とそれぞれの gzip 圧縮、および parquet の 11 形式とする。
- 大きさの上限は既定 1 GiB とし、`THIEF_OBJECT_QUERY_MAX_BYTES` で変える。
  上限は `GET /api/config` で frontend に渡す。
- DuckDB の拡張は、実行時に外部の配布元 (extensions.duckdb.org) から取得せず、同じオリジンから配信する (`frontend/public/assets/duckdb-extensions`、`mise run frontend:fetch-duckdb-extensions`)。

## 検討した代替案

issue 0196 は次の案を採らなかった。

- backend に Go の DuckDB バインディングを組み込む。
  CGO が必須で、`CGO_ENABLED=0` のビルド (`AGENTS.md`) に反するため。
- sql.js や alasql などの JavaScript の SQL エンジン。
  parquet を読めないため。

## 結果

- SQL の解析と実行はブラウザの中で完結し、backend の CPU とメモリの負荷は増えない。
  オブジェクトの本体は既存のダウンロード API を通って届くため、backend はオブジェクトの大きさに比例した転送を中継する (`backend/internal/api/handlers_s3_object.go`、`backend/internal/api/handlers_gcp.go`)。
- 依存が 2 つ増えた。
  依存を最小にする方針 (ADR 0004) の例外であり、標準の API に SQL エンジンが無いことを理由とした。
- セットアップに DuckDB の拡張の取得が加わった。

## 根拠資料

- `docs/issues/closed/0196`、`0197`
- `frontend/src/lib/duckdb.ts`、`frontend/src/lib/objectQuery.ts`
- `backend/internal/api/handlers_config.go`
