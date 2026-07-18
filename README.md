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

3. AWS SSO でログインする。

   ```sh
   aws sso login --profile <プロファイル名>
   ```

4. (任意) AWS / Google Cloud 公式アイコンを展開する。

   `frontend/public/assets/aws-icons/` と `frontend/public/assets/gcp-icons/` はライセンス上リポジトリにコミットしていないため、各社公式サイトから Asset Package (zip) をダウンロードし、以下のコマンドで展開する。未展開でも起動・表示は可能 (アイコンが欠けるだけ)。

   ```sh
   mise run frontend:fetch-aws-icons <path-to-aws-icons.zip>
   mise run frontend:fetch-gcp-icons <path-to-gcp-icons.zip>
   ```

## ローカルで起動する

backend と frontend をそれぞれ別ターミナルで起動する。

```sh
mise run backend:run   # API サーバー起動 (http://127.0.0.1:8089)
mise run frontend:run  # Vite dev server 起動 (http://localhost:8088)
```

ブラウザで `http://localhost:8088` を開く。

## CLI として使う

`thief` は API サーバー / frontend を使わず、単体の CLI ツールとしても利用できる。

```sh
mise run backend:install   # go install ./cmd/thief で $GOPATH/bin に導入
thief ec2                  # 例: EC2 インスタンス一覧を表示
```

`$GOPATH/bin` (通常 `~/go/bin`) が `PATH` に含まれていることを確認すること。サブコマンド一覧は `thief --help` を参照。

## License

[MIT License](./LICENSE)
