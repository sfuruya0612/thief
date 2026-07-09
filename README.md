# thief

[![Go Report Card](https://goreportcard.com/badge/github.com/sfuruya0612/thief)](https://goreportcard.com/report/github.com/sfuruya0612/thief)
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat)](LICENSE)

## セットアップ

タスクランナーは [mise](https://mise.jdx.dev/) で統一している。詳細タスク一覧は `AGENTS.md` を参照。

1. [mise](https://mise.jdx.dev/getting-started.html) を導入する。
2. 依存関係を取得する。

   ```sh
   mise run setup
   ```

3. AWS SSO でログインする (ローカル / Docker 起動の両方で必要)。

   ```sh
   aws sso login --profile <プロファイル名>
   ```

## ローカルで起動する

backend と frontend をそれぞれ別ターミナルで起動する。

```sh
mise run backend:run   # API サーバー起動 (http://127.0.0.1:8080)
mise run frontend:run  # Vite dev server 起動 (http://localhost:8082)
```

ブラウザで `http://localhost:8082` を開く。

## Docker で起動する

```sh
mise run docker:up     # docker compose up --build と同義
```

- frontend: `http://localhost:8088` (nginx が `dist` を静的配信し、`/api/` 配下は backend へリバースプロキシする)。
- backend: `http://localhost:8089` (通常は frontend 経由でアクセスするため直接開く必要はない)。
- AWS 認証はホストの `~/.aws` (config/credentials/SSO cache) を読み取り専用でマウントするため、事前にホスト側で `aws sso login` を実行しておくこと。
- 停止する場合は `mise run docker:down` を実行する。

### カスタムドメイン (`thief.local`) でアクセスする

`localhost` 以外の任意のホスト名で frontend/backend にアクセスしたい場合、`/etc/hosts` に以下を追記する (要 sudo)。

```sh
echo "127.0.0.1 thief.local" | sudo tee -a /etc/hosts
```

追記後、`docker compose up --build` (または `mise run docker:up`) で起動し、ブラウザで `http://thief.local:8088` を開く。frontend の nginx が `/api/` 配下を backend へ同一オリジンでプロキシするため、追加のビルド設定変更は不要。

## License

[MIT License](./LICENSE)
