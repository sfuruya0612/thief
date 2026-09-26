# 0003. backend と frontend を 1 つのリポジトリに置き、frontend はブラウザ専用の SPA にする

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

thief の Web クライアントは、かつて Flutter で macOS デスクトップにも対応していた (`AGENTS.md` の frontend 「基本方針」に「Flutter 時代の macOS Desktop 対応は廃止済み」とある)。
Flutter から React への移行と、デスクトップ対応の廃止の経緯を記録した issue とコミットは、リポジトリに無い。

## 決定

- backend (`backend/`、Go) と frontend (`frontend/`、Vite + React + TypeScript) を 1 つのリポジトリに置く (コミット 2aac22f 「backend/frontend モノレポ構成へ移行し…」)。
- frontend のビルド対象は Web ブラウザだけとする。
- frontend は backend の API (`http://127.0.0.1:8089`、`VITE_API_BASE` で変更可) を呼ぶ SPA とする。

## 検討した代替案

記録なし。
Flutter の継続とデスクトップ対応の維持を比べた記録は見つかっていない。

## 結果

- backend と frontend の型の食い違いを、同じリポジトリ内の検査で見つけられる (ADR 0007)。
- ブラウザの制約 (CORS、WebSocket、OPFS) の中で機能を作る。
  ターミナルは WebSocket で中継し (ADR 0012)、オブジェクトの SQL 検索は DuckDB Wasm で動かす (ADR 0018)。
- タスクは `mise run backend:*` と `mise run frontend:*` に分け、ルートの `mise run check` でまとめて検査する (ADR 0021)。

## 根拠資料

- コミット 2aac22f (2026-07-08)
- `AGENTS.md` の「リポジトリ概要」と frontend 「基本方針」
