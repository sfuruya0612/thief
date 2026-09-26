# 0032 起動ポートを frontend 8088 / backend 8089 に変更する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8 claude-opus-4-8

## 背景 / 根拠

現状の通常起動ポートは backend が 8080、frontend (Vite dev server) が 8082 である。
8080 は多くのローカル開発ツールが使う定番ポートで、他プロセスと衝突しやすい。
他のプロセスと被りにくくするため、frontend を 8088、backend を 8089 に変更する。

これらは Docker 起動で使っていたポート番号だが、issue 0031 で Docker によるアプリ起動を廃止するため空く。
本 issue は issue 0031 の後に着手する (0031 完了までは 8088 / 8089 が Docker のホスト公開ポートと重複するため)。

## 変更対象

以下の 4 箇所は互いに整合させる必要があるため、まとめて変更する。
WebSocket 許可オリジンのデフォルトは frontend の origin と一致していなければならず、frontend の API フォールバック先は backend のリッスンポートと一致していなければならない。

1. backend リッスンアドレスのデフォルト: `backend/internal/config/config.go` L111 `127.0.0.1:8080` を `127.0.0.1:8089` にする。
2. frontend の API フォールバック先: `frontend/src/api/client.ts` L8 `http://127.0.0.1:8080` を `http://127.0.0.1:8089` にする (上記 1 と一致させる)。
3. frontend の Vite dev / preview ポート: `frontend/vite.config.ts` L8 (`server.port`) と L13 (`preview.port`) の `8082` を `8088` にする。
4. WebSocket 許可オリジンのデフォルト: `backend/internal/config/config.go` L104 `defaultWebOrigins` の `localhost:8082` / `127.0.0.1:8082` を `localhost:8088` / `127.0.0.1:8088` にする (上記 3 の frontend origin と一致させる)。

## 付随して更新する記述

ポート番号に言及するコメント / 説明 / ドキュメントを新しい値に合わせる。

- backend: `backend/internal/cli/server.go` L19、`backend/internal/api/server.go` L89、`backend/internal/config/config.go` L102-103 (defaultWebOrigins のコメント)、`backend/internal/api/handlers_session.go` L81。
- mise: `mise.toml` L89 (`backend:run` の説明)、L133 (`frontend:run`)、L138 (`frontend:serve`)。
- ドキュメント: `README.md` L37-38, L41、`AGENTS.md` L41, L52-53, L298。
- `CHANGES.md` の `## develop` に本変更を追記する。

## スコープ外

- `backend/internal/aws/elb_test.go` L198, L208 の `Port: 8080` は ELB ターゲットグループのフィクスチャ値でありアプリのリッスンポートとは無関係。変更しない。
- Docker 関連のポート記述 (compose.yaml / nginx.conf / Dockerfile) は issue 0031 で削除済みのため対象外。

## 検証

- `mise run backend:run` が `127.0.0.1:8089` で待ち受けることを確認する。
- `mise run frontend:run` が `http://localhost:8088` で起動し、backend (8089) と通信できることを確認する。
- EC2 Start Session / ECS Exec のブラウザターミナル (WebSocket) が 8088 の origin から接続でき、Origin チェックで弾かれないことを確認する。
- `VITE_API_BASE` 未設定時に frontend が `127.0.0.1:8089` を向くことを確認する。
- `mise run check` が全通過することを確認する。

## 解決方法

記載の 4 箇所と付随記述をすべて更新した。

- `backend/internal/config/config.go`: `ListenAddr` のデフォルトを `127.0.0.1:8089` に、`defaultWebOrigins` を `localhost:8088`/`127.0.0.1:8088` に変更した。
- `frontend/src/api/client.ts`: `VITE_API_BASE` 未設定時のフォールバックを `http://127.0.0.1:8089` に変更した。
- `frontend/vite.config.ts`: `server.port` / `preview.port` を `8088` に変更した。
- コメント・ドキュメントを更新した: `backend/internal/cli/server.go`、`backend/internal/api/server.go`、`backend/internal/api/handlers_session.go`、`mise.toml` (`backend:run`/`frontend:run`/`frontend:serve` の description)、`README.md`、`AGENTS.md`、`example/README.md` (確認手順のアクセス先ポート)。
- `CHANGES.md` の `## develop` に `[CHANGE]` エントリを追記した。
- `backend/internal/aws/elb_test.go` の `Port: 8080` は issue記載のとおりスコープ外として変更しなかった (ELB ターゲットグループのフィクスチャ値でアプリのリッスンポートとは無関係)。
- 実機検証: `mise run backend:run` / `mise run frontend:run` で起動し、`http://127.0.0.1:8089` (backend) と `http://localhost:8088` (frontend) が応答すること、旧ポート `8080`/`8082` には何も bind されていないことを確認した。ブラウザ (Playwright + システム Chrome) で `http://localhost:8088` を開き、コンソールエラーなしで AWS プロファイル・リソース一覧が表示されることを確認した。
- `mise run check` が全通過することを確認した。
