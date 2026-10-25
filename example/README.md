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
- RDS: DB インスタンス `thief-example-db` (mysql、db.t3.micro、タグ `Env=example`)
- ElastiCache: クラスタ `thief-example-cache` (memcached。floci の `CreateCacheCluster` は memcached だけを受け付けるため redis にしない)
- Kinesis: ストリーム `thief-example-stream` (シャード 2、保持 24 時間)
- API Gateway: REST API `thief-example-api` と HTTP API `thief-example-http`
- NAT Gateway: Elastic IP を 1 つ確保し、既定 VPC のサブネット `subnet-default-ap-northeast-1-a` にゲートウェイ 1 つ (タグ `Name=thief-example-nat`)
- ELB: Application Load Balancer `thief-example-alb` (internet-facing、サブネットは `subnet-default-ap-northeast-1-a` と `-c`)
- CloudFront: ディストリビューション 1 つ (コメント `thief example`、オリジンは `thief-example-data.s3.amazonaws.com`。ID は floci が割り当てる)

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

### TUI (`thief tui`) の S3 / RDS / DynamoDB / ElastiCache 一覧

`example/seed.sh` が投入した資源で確かめる。S3 は backend と同じく `THIEF_S3_PATH_STYLE=true` が要る (floci が path-style でのみ S3 を提供するため)。

1. 上の EC2 一覧の 1 と 2 と同じく `thief` を導入し、`mise run example:up` を動かしておく
2. `mise run example:seed`
3. リポジトリのルートで `HOME="$(pwd)/example/home" THIEF_S3_PATH_STYLE=true thief tui -p floci` を起動し、`S` で開く選択肢に `EC2`、`Lambda`、`ECR`、`ECS`、`S3`、`RDS`、`DynamoDB`、`ElastiCache` がこの順に出る
4. `s3` と打って Enter を押すと、一覧に `thief-example-data` と `thief-example-logs` の行 (State は空、Region `ap-northeast-1`、Public `private`) が出る (seed 以外に CloudFormation のスタックが作るバケットと floci が Lambda 用に作るバケットも出る)。Drawer の Overview は Resource ID、Region、Created、Public access (`blocked`)、Encryption の順で、`2` の Tags は `No tags` になる (S3 の資源にタグが無いため。Web UI と同じ)
5. `S`、`rds`、Enter で `thief-example-db` の行 (State `available`、Engine `mysql`、Cluster `—`、Class `db.t3.micro`、MultiAZ `—`) が出る。Drawer の Overview は Resource ID、Engine、Engine version、Cluster、Class、Multi-AZ (`no`)、Endpoint、Port、VPC、Region、Uptime、Launched の順で、floci は作成時刻を返さないため Uptime と Launched は `—` になる。`2` の Tags に `Env` が出る
6. `S`、`dynamo`、Enter で `thief-example-users` の行 (State `active`、Mode `on-demand`、Items `3`) が出る。Drawer の Overview は Resource ID、Capacity mode、Item count、Size、Global secondary indexes、Region の順
7. `S`、`cache`、Enter で `thief-example-cache` の行 (State `available`、Engine `memcached`、Replication Group `—`、AZs `—`) が出る。Drawer の Overview は Resource ID、Engine、Engine version、Replication group、Node type、Nodes、AZs、Endpoint、Port、Region の順で、floci はノードの情報を返さないため Node type は空、Nodes は `0`、AZs は `—` になる。`2` の Tags は `No tags`
8. `q` で終了する

### TUI (`thief tui`) の ELB / CloudFront / API Gateway / NAT Gateway / Kinesis / SQS 一覧

`example/seed.sh` が投入した資源で確かめる。

1. 上の EC2 一覧の 1 と 2 と同じく `thief` を導入し、`mise run example:up` を動かしておく
2. `mise run example:seed`
3. リポジトリのルートで `HOME="$(pwd)/example/home" thief tui -p floci` を起動し、`S` で開く選択肢に `ELB`、`CloudFront`、`API Gateway`、`NAT Gateway`、`Kinesis`、`SQS` が `ElastiCache` の後にこの順に出る
4. `sqs` と打って Enter を押すと、一覧に `thief-example-queue`、`thief-example-dlq`、`thief-example-fifo.fifo` の行 (State `active`、Available `0`、In flight `0`、Retention `4d`、Region `ap-northeast-1`) が出て、Type は `thief-example-fifo.fifo` だけ `FIFO`、他は `Standard` になる (Web UI の SQS 一覧と同じ表記)。Drawer の Overview は Resource ID (キューの ARN)、Type、Messages available、Messages in flight、Retention (`4 days`)、Region の順で、`2` の Tags は `No tags` になる (seed のキューにタグが無いため)
5. `S`、`elb`、Enter で `thief-example-alb` の行 (State `active`。作成の直後は `provisioning` で、数分後に `active` になる。Type `application`、Scheme `internet-facing`、AZs `ap-northeast-1a, ap-northeast-1c`) が出る。Drawer の Overview は Resource ID (ロードバランサの ARN)、Type、Scheme、DNS name、VPC (`vpc-default-ap-northeast-1`)、AZs、Region の順で、`2` の Tags は `No tags` (Web UI と同じく ELB はタグを取得しない)
6. `S`、`cloudfront`、Enter で、ディストリビューション ID の行 (State `deployed`、Domain `<ID>.cloudfront.net`、Alternate domains `—`、Origins `thief-example-data.s3.amazonaws.com`) が出る。Drawer の見出しの Name はコメントの `thief example` で、Overview は Resource ID、Domain、Alternate domains、Origins、Enabled (`yes`)、Price class (`PriceClass_All`) の順。`2` の Tags は `No tags`
7. `S`、`api`、Enter で `thief-example-api` (Type `REST`) と `thief-example-http` (Type `HTTP`) の行 (State `active`、Stage は空、Endpoint は `https://<API ID>.execute-api.ap-northeast-1.amazonaws.com`) が出る。Drawer の Overview は Resource ID (API ID)、Type、Stage、Endpoint、Region の順で、`2` の Tags は `No tags`
8. `S`、`nat`、Enter で `thief-example-nat` の行 (State `available`、Gateway ID `nat-...`、VPC `vpc-default-ap-northeast-1`、Elastic IP は floci が割り当てた値、Uptime は作成からの経過) が出る。Drawer の Overview は Resource ID、VPC、Subnet (`subnet-default-ap-northeast-1-a`)、Elastic IP、Region、Uptime、Launched (作成日を UTC で表示) の順で、`2` の Tags に `Name` が出る
9. `S`、`kinesis`、Enter で `thief-example-stream` の行 (State `active`、Mode `provisioned`、Shards `2`、Retention `24h`、Encryption `NONE`) が出る。Drawer の Overview は Resource ID (ストリームの ARN)、Capacity mode、Shards、Retention、Encryption、Region の順で、`2` の Tags は `No tags` (Web UI と同じく Kinesis はタグを取得しない)
10. `q` で終了する

## 動作確認できない機能

以下はエミュレータ経由での確認ができない。実アカウント + 実プロファイルで確認すること。

- SSO ログイン (`aws sso login` の子プロセス起動。`floci` プロファイルは access_key 認証のためこの経路自体を通らない)
- EC2 Start Session / ECS Exec のターミナル (session-manager-plugin のサブプロセスと実エージェント接続が前提)
- Cost Explorer / 請求系

## 注意

- backend の `HOME` を `./example/home` に差し替えるため、`example` 環境で起動した backend からはホストの実 `~/.aws` (実アカウントのプロファイル・SSO キャッシュ) が一切見えなくなる。これは実アカウントからローカル確認環境を隔離する意図どおりの挙動。
- `HOME` の差し替えはそのシェルセッション内でのみ有効。通常の thief 起動に戻るときは、`HOME` を差し替えていない別のターミナルで `mise run backend:run` を実行すること。
