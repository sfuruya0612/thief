# example: floci によるローカル動作確認環境

[floci](https://floci.io/floci/) (MIT ライセンスの AWS エミュレータ、単一コンテナで S3 / DynamoDB / SQS / SSM など多数のサービスをエミュレートする) を使い、実 AWS アカウントなしで thief の動作確認を行うための環境。

## 起動

リポジトリルートから実行する。

```sh
mise run example:up
```

`floci/floci:latest` (ポート 4566) が単体で起動する。
thief 本体はネイティブ起動のため、別ターミナルで次のように起動する。

```sh
export HOME="$(pwd)/example/home"
export THIEF_S3_PATH_STYLE=true
mise run backend:run
```

- `HOME` を `./example/home` (floci 専用プロファイルのみを含む `.aws/config` / `.aws/credentials` を配置したダミー home) に差し替え、ホストの実 `~/.aws` (実アカウントのプロファイル・SSO キャッシュ) から隔離する。
- `THIEF_S3_PATH_STYLE=true` は floci が S3 を path-style でのみ提供するために必要。

frontend は別ターミナルで、`HOME` を差し替えずに通常どおり起動する (frontend は `~/.aws` を参照しない)。

```sh
mise run frontend:run
```

停止は次のとおり。

```sh
mise run example:down
```

## サンプルリソースの投入

```sh
mise run example:seed
```

`example/seed.sh` が aws CLI (エンドポイント `http://localhost:4566`、認証情報はダミー値) で以下を投入する。

- S3: バケット 2 つ (`thief-example-data` / `thief-example-logs`)、csv / txt / json のオブジェクト
- DynamoDB: テーブル `thief-example-users` + アイテム 3 件
- SQS: 標準キュー 2 本 + FIFO キュー 1 本
- SSM Parameter Store: パラメータ 3 件 (`String` x2、`SecureString` x1)
- Secrets Manager: シークレット 1 件
- CloudFormation: 最小スタック `thief-example-stack` (S3 バケット 1 個を作成するテンプレート)

`mise run example:up` で floci が起動した後に実行すること。

## 確認手順

1. `mise run example:up`
2. `mise run example:seed`
3. 上記の `HOME` / `THIEF_S3_PATH_STYLE` を設定した状態で `mise run backend:run` と `mise run frontend:run` を起動する
4. ブラウザで http://localhost:8082 を開く
5. プロファイル選択で `floci` を選ぶ
6. S3 (バケット一覧、オブジェクトブラウザ)、DynamoDB (テーブル一覧、schema / items タブ)、SQS、SSM Parameter Store、Secrets Manager、CloudFormation (スタック一覧、Events / Resources の Drawer) の一覧表示を確認する

## 動作確認できない機能

以下はエミュレータ経由での確認ができない。実アカウント + 実プロファイルで確認すること。

- SSO ログイン (`aws sso login` の子プロセス起動。`floci` プロファイルは access_key 認証のためこの経路自体を通らない)
- EC2 Start Session / ECS Exec のターミナル (session-manager-plugin のサブプロセスと実エージェント接続が前提)
- Cost Explorer / 請求系

## 注意

- backend の `HOME` を `./example/home` に差し替えるため、`example` 環境で起動した backend からはホストの実 `~/.aws` (実アカウントのプロファイル・SSO キャッシュ) が一切見えなくなる。これは実アカウントからローカル確認環境を隔離する意図どおりの挙動。
- `HOME` の差し替えはそのシェルセッション内でのみ有効。通常の thief 起動に戻るときは、`HOME` を差し替えていない別のターミナルで `mise run backend:run` を実行すること。
