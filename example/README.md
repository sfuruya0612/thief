# example: floci によるローカル動作確認環境

[floci](https://floci.io/floci/) (MIT ライセンスの AWS エミュレータ、単一コンテナで S3 / DynamoDB / SQS / SSM など多数のサービスをエミュレートする) を使い、実 AWS アカウントなしで thief の動作確認を行うための環境。

## 起動

リポジトリルートから実行する。

```sh
mise run example:up
```

ルートの `compose.yaml` に `example/compose.yaml` を override として重ねる。

- `floci` サービス (`floci/floci:latest`、ポート 4566) が追加で起動する
- `backend` サービスの `~/.aws` マウントが `./example/aws` (floci 専用プロファイルのみを含むダミー設定) に差し替わる
- `backend` サービスに `THIEF_S3_PATH_STYLE=true` が設定される (floci は S3 を path-style でのみ提供するため)

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

`mise run example:up` でコンテナが起動した後に実行すること。`seed.sh` はホストから実行する前提のため、エンドポイントはコンテナ名 (`floci`) ではなく `localhost:4566` を使う。

## 確認手順

1. `mise run example:up`
2. `mise run example:seed`
3. ブラウザで http://localhost:8088 を開く
4. プロファイル選択で `floci` を選ぶ
5. S3 (バケット一覧、オブジェクトブラウザ)、DynamoDB (テーブル一覧、schema / items タブ)、SQS、SSM Parameter Store、Secrets Manager、CloudFormation (スタック一覧、Events / Resources の Drawer) の一覧表示を確認する

## 動作確認できない機能

以下はエミュレータ経由での確認ができない。実アカウント + 実プロファイルで確認すること。

- SSO ログイン (`aws sso login` の子プロセス起動。`floci` プロファイルは access_key 認証のためこの経路自体を通らない)
- EC2 Start Session / ECS Exec のターミナル (session-manager-plugin のサブプロセスと実エージェント接続が前提)
- Cost Explorer / 請求系

## 注意

- `backend` サービスの `~/.aws` マウントが `./example/aws` に差し替わるため、`example` 環境ではホストの実 `~/.aws` (実アカウントのプロファイル・SSO キャッシュ) が一切見えなくなる。これは実アカウントからローカル確認環境を隔離する意図どおりの挙動。
- `example` 環境を終えて通常の `mise run docker:up` に戻る場合、`mise run example:down` で停止してから `docker:up` を実行すること (同じコンテナ名・ネットワークを使い回すため)。
