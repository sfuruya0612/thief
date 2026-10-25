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
- Lambda: 関数 `thief-example-hello` (python3.12、256 MB、30 秒、タグ `Env=example`。デプロイパッケージを作るため `zip` コマンドが要る)
- ECR: リポジトリ 2 つ (`thief-example/web` はタグ不変 + プッシュ時スキャン、`thief-example/batch` は既定の設定)
- ECS: クラスタ `thief-example-cluster` (タグ `Env=example`)

`mise run example:up` で floci が起動した後に実行すること。

## 確認手順

1. `mise run example:up`
2. `mise run example:seed`
3. 上記の `HOME` / `THIEF_S3_PATH_STYLE` を設定した状態で `mise run backend:run` と `mise run frontend:run` を起動する
4. ブラウザで http://localhost:8088 を開く
5. プロファイル選択で `floci` を選ぶ
6. S3 (バケット一覧、オブジェクトブラウザ)、DynamoDB (テーブル一覧、schema / items タブ)、SQS、SSM Parameter Store、Secrets Manager、CloudFormation (スタック一覧、Events / Resources の Drawer) の一覧表示を確認する

### TUI (`thief tui`) の EC2 一覧

`example/seed.sh` は EC2 インスタンスを作らないため、手で 2 台作ってから TUI を起動する。認証情報は `example/home/.aws/credentials` の `floci` プロファイル (access_key 認証) を使う。

1. `mise run backend:install` で `thief` を `$(go env GOPATH)/bin` に導入する
2. 別のターミナルで `mise run example:up` を実行する (前面で動き続ける)
3. `example/seed.sh` と同じダミーの認証情報とリージョンで EC2 インスタンスを 2 台作る

   ```sh
   export AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=ap-northeast-1
   aws --endpoint-url http://localhost:4566 ec2 run-instances --image-id ami-12345678 --instance-type t3.micro \
     --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=web-01}]'
   aws --endpoint-url http://localhost:4566 ec2 run-instances --image-id ami-12345678 --instance-type t3.micro \
     --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=batch-01}]'
   ```

4. リポジトリのルートで `HOME="$(pwd)/example/home" thief tui -p floci` を起動すると、一覧に `web-01` と `batch-01` の行が出る
5. `/` で `web` と打つと `web-01` の 1 行だけになる。選択中の行の右の Drawer に Overview (Resource ID、Type、AZ、Region など) が出て、`2` で Tags に切り替えると `Name` のタグが出る (`1` で Overview に戻る)
6. `q` で終了する

### TUI (`thief tui`) の Lambda / ECR / ECS 一覧

`example/seed.sh` が投入した資源で確かめる。EC2 インスタンスは要らない。

1. 上の EC2 一覧の 1 と 2 と同じく `thief` を導入し、`mise run example:up` を動かしておく
2. `mise run example:seed`
3. リポジトリのルートで `HOME="$(pwd)/example/home" thief tui -p floci` を起動し、`S` で開く選択肢に `EC2`、`Lambda`、`ECR`、`ECS` がこの順に出る
4. `lambda` と打って Enter を押すと、一覧に `thief-example-hello` の行 (State `active`、Runtime `python3.12`、Memory `256 MB`、Timeout `30s`) が出る。Drawer の Overview に Handler `index.handler` と Role が出て、`2` の Tags は `No tags` になる (一覧の取得がタグを取らないため。Web UI と同じ)
5. `S`、`ecr`、Enter で `thief-example/web` (Tag mutability `IMMUTABLE`、Scan on push `✓`) と `thief-example/batch` (`MUTABLE`、`—`) の 2 行が出る。Drawer の Overview の Scan on push は `enabled` / `disabled`、Tags は `No tags`
6. `S`、`ecs`、Enter で `thief-example-cluster` の行 (State `active`、件数はすべて `0`) が出る。`2` の Tags に `Env` が出る。`s` を押すと `ECS does not support sessions` の通知が出てセッションは始まらない
7. `q` で終了する

## 動作確認できない機能

以下はエミュレータ経由での確認ができない。実アカウント + 実プロファイルで確認すること。

- SSO ログイン (`aws sso login` の子プロセス起動。`floci` プロファイルは access_key 認証のためこの経路自体を通らない)
- EC2 Start Session / ECS Exec のターミナル (session-manager-plugin のサブプロセスと実エージェント接続が前提)
- Cost Explorer / 請求系

## 注意

- backend の `HOME` を `./example/home` に差し替えるため、`example` 環境で起動した backend からはホストの実 `~/.aws` (実アカウントのプロファイル・SSO キャッシュ) が一切見えなくなる。これは実アカウントからローカル確認環境を隔離する意図どおりの挙動。
- `HOME` の差し替えはそのシェルセッション内でのみ有効。通常の thief 起動に戻るときは、`HOME` を差し替えていない別のターミナルで `mise run backend:run` を実行すること。
