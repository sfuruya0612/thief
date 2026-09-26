# 0024 floci による AWS ローカル動作確認環境を example/ に整備する

Created: 2026-07-17
Completed: 2026-07-18
Model: Claude Fable 5 claude-fable-5

## 背景 / 根拠

thief の動作確認は実 AWS アカウントを前提としており、次の不便がある。

- 書き込み系機能 (S3 アップロード、CloudFront invalidation など) の試行錯誤を実アカウントに向けるのはリスクがある
- 「リソースが 0 件」「多様な状態のリソースが混在」といった表示条件を実アカウントで再現しにくい
- SSO ログインが切れていると一切の確認ができない

[floci](https://floci.io/floci/) は MIT ライセンスの AWS エミュレータで、単一コンテナ (`floci/floci:latest`、ポート 4566) で S3 / DynamoDB / SQS / SSM など 68 サービスをエミュレートし、アカウント登録なしで利用できる。
これを使ったローカル動作確認環境を `example/` ディレクトリに整備する。

実現可能性は調査済みで、thief 側の対応はほぼ設定のみで済む。

- クライアント生成は `GetSession` (`backend/internal/aws/session.go:13`) の素の `config.LoadDefaultConfig` と各サービスの `NewFromConfig(cfg)` (オプション無指定) に集約されており、依存している `aws-sdk-go-v2/config v1.32.29` は環境変数 `AWS_ENDPOINT_URL` と `~/.aws/config` の `endpoint_url` 設定によるエンドポイント上書きをネイティブサポートしている (config パッケージの `resolveBaseEndpoint` が `LoadDefaultConfig` の既定チェーンに含まれ、各サービスの `NewFromConfig` が `cfg.BaseEndpoint` を `Options.BaseEndpoint` へ伝播することをソースで確認済み)。したがってエンドポイント切り替え自体にコード変更は不要
- 唯一の必須コード変更は S3 の path-style 対応 (後述)

## 対応内容

- `example/` ディレクトリを新規作成し、floci 起動と thief からの接続に必要な一式を配置する
  - `example/compose.yaml`: floci サービス定義と、backend への設定注入 (compose の複数ファイル指定で既存 `compose.yaml` に重ねる override)
  - `example/aws/config` / `example/aws/credentials`: floci 向けプロファイル定義
  - `example/seed.sh`: サンプルリソースの投入スクリプト
  - `example/README.md`: 起動から確認までの手順
- backend に S3 の path-style オプトインを追加する
- `mise.toml` に example 環境の起動 / 停止 / シードのタスクを追加する

## 実装方針

### プロファイル方式の選定

backend への接続設定は、環境変数 `AWS_ENDPOINT_URL` を渡す方式ではなく、`~/.aws` マウントを `example/aws/` に差し替えるプロファイル方式を採る。
理由は 2 つある。

- thief の UI はプロファイル選択が前提で、プロファイル一覧は `~/.aws/config` の自前パース (`backend/internal/aws/profiles.go` の `parseAWSConfig`) で作られる。環境変数だけではピッカーに何も現れない
- `AWS_ENDPOINT_URL` はプロセス全体に効くため、実プロファイルと floci プロファイルの共存ができない。プロファイル単位の `endpoint_url` なら floci プロファイルだけをエミュレータに向けられる

配置する設定は次の形にする。

`example/aws/config`:

```ini
[profile floci]
region = ap-northeast-1
endpoint_url = http://floci:4566
```

`example/aws/credentials`:

```ini
[floci]
aws_access_key_id = test
aws_secret_access_key = test
```

floci はアカウント登録なしで利用できるエミュレータであり、実在の AWS 認証情報を必要としないため、ダミー値を置く (値の要件は実装時に floci の挙動で確認する)。
この形式なら `resolveAuthType` (`profiles.go:308`) が `access_key` と判定し、プロファイル一覧に SSO バッジなしで表示される。

`example/compose.yaml` では backend の volumes を `./example/aws:/root/.aws:ro` に差し替える。
ホストの実 `~/.aws` が見えなくなるが、これはローカル確認環境を実アカウントから分離する意図どおりの挙動であり、README にその旨を明記する。

### S3 path-style 対応 (唯一のコード変更)

floci を含むエミュレータは S3 を path-style (`http://host:4566/bucket/key`) で提供する。virtual-hosted style (`bucket.host:4566`) はバケット名のサブドメインを名前解決できないため接続に失敗する。
SDK には path-style を切り替える環境変数が存在せず (`s3.Options.UsePathStyle` はコード内オプションのみ)、thief 側の対応が必要になる。

- `backend/internal/config/config.go` の `applyEnv` に `THIEF_S3_PATH_STYLE` (bool) を追加する
- S3 クライアント生成は `newS3Client` (`backend/internal/aws/s3.go:151`) の 1 箇所に集約されている (バケット一覧もオブジェクトブラウザの `newS3ClientForBucket` も最終的にここを通る) ため、この関数に `func(o *s3.Options) { o.UsePathStyle = true }` を渡す分岐を 1 箇所入れれば S3 全機能に波及する
- 既定は false とし、既存の実アカウント動作に影響を与えない。`example/compose.yaml` で `THIEF_S3_PATH_STYLE=true` を設定する

代案として「`aws.Config.BaseEndpoint` が設定されていたら自動で path-style にする」判定も考えられるが、プロファイルの `endpoint_url` はサービス別解決のため `cfg.BaseEndpoint` に現れるとは限らず、判定が不確実になる。明示的な環境変数オプトインの方が堅牢である。

### seed スクリプト

`example/seed.sh` は aws CLI でサンプルリソースを投入する。
各コマンドに `--endpoint-url http://localhost:4566` を付け、認証情報はスクリプト内で環境変数 (`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`) として渡す。
ホストから実行するため、エンドポイントはコンテナ名 (`floci`) ではなく `localhost:4566` を使う点に注意する。

投入対象は thief の表示確認に効くものから始める。

- S3: バケット 2 つ + csv / txt / json のオブジェクト (オブジェクトブラウザとプレビュー機能 (issue 0025) の確認を兼ねる)
- DynamoDB: テーブル + 数アイテム (schema / items タブの確認)
- SQS: キュー数本 (属性表示の確認)
- SSM Parameter Store: 数パラメータ
- Secrets Manager: シークレット 1 件
- CloudFormation: 小さなスタック 1 つ (実装済みの Events / Resources の Drawer 表示 (issue 0023) の確認を兼ねる)

EC2 / ECS / RDS などのコンピュート系は floci 側のエミュレーション精度に依存するため、動作したものから順次 seed に加える。
floci が対応しないサービス (Cost Explorer など) は README の制約一覧に記載する。

### 動作確認の対象外となる機能

以下はエミュレータ経由での確認ができないことを README に明記する。

- SSO ログイン (`aws sso login` の子プロセス起動。floci プロファイルは access_key のため経路自体を通らない)
- EC2 Start Session / ECS Exec のターミナル (session-manager-plugin のサブプロセスと実エージェント接続が前提)
- Cost Explorer / 請求系

### プロファイル選択時の前提確認

プロファイルを開いた直後に呼ばれる identity (`sts GetCallerIdentity`) と regions (`ec2 DescribeRegions`、us-east-1 固定 `backend/internal/aws/regions.go:73`) が floci で応答することを確認する。
どちらかが失敗すると UI がリソース表示まで進めない可能性があるため、失敗する場合は挙動を確認して README の制約に記載し、必要なら別 issue を切る。

### mise タスク

既存の `docker:up` / `docker:down` (`mise.toml:143` 付近、`dir` 指定なしでリポジトリルート実行) に倣い、以下を追加する。

```toml
[tasks."example:up"]
description = "floci 込みのローカル動作確認環境を起動"
run = "docker compose -f compose.yaml -f example/compose.yaml up --build"

[tasks."example:down"]
run = "docker compose -f compose.yaml -f example/compose.yaml down"

[tasks."example:seed"]
run = "./example/seed.sh"
```

## スコープ外

- CI への組み込み (エミュレータ前提の統合テスト化は別 issue)
- 全 20 サービスのシード網羅 (floci 側の対応状況を見ながら追加する)
- GCP / Datadog / TiDB のローカルエミュレーション

## 検証

- `mise run check` を通す (S3 path-style 分岐のユニットテストを含む)
- `mise run example:up` + `mise run example:seed` の後、http://localhost:8088 で floci プロファイルを開き、S3 (バケット一覧、オブジェクトブラウザ、アップロード)、DynamoDB (テーブル / schema / items)、SQS、SSM、Secrets Manager、CloudFormation の一覧表示を確認する
- `THIEF_S3_PATH_STYLE` 未設定時に実アカウントの S3 動作が変わらないことを確認する

## 解決方法

方針どおり実装した。

- backend: `internal/config/config.go` に `Config.S3PathStyle` フィールドと `THIEF_S3_PATH_STYLE` の解釈を追加した。実際の分岐は `internal/aws/s3.go` の `newS3Client` が `THIEF_S3_PATH_STYLE` を直接 `os.Getenv` で参照する方式 (`s3PathStyleEnabled`/`s3PathStyleOption`) にした。理由は、S3 クライアント生成が `newS3Client` の 1 箇所に集約されている一方 `config.Config` はこの関数の呼び出し元 (handler 層) までしか到達しておらず、bool を橋渡しするには `NewClient` ジェネリック関数や全呼び出し元シグネチャへの変更が波及するため。影響範囲を S3 のみに閉じる局所的な opt-in とし、判断理由をコード内コメントに残した (`config.Config.S3PathStyle` は値の保持・可視化目的で残す)。
- `example/` 一式 (`compose.yaml`、`aws/config`、`aws/credentials`、`seed.sh`、`README.md`) を issue の設計どおり作成し、`mise.toml` に `example:up`/`example:down`/`example:seed` タスクを追加した。
- テスト: `internal/aws/s3_test.go` を新設し、`THIEF_S3_PATH_STYLE` あり/なしでの `s3.Options.UsePathStyle` 反映をテーブル駆動でテストした。`mise run check` (backend fmt/lint/test, frontend fmt/lint/test) が全て通過することを確認した。

### 実機検証

Docker と外部ネットワークが利用できる環境だったため、実際に `docker compose -f compose.yaml -f example/compose.yaml up --build -d` → `./example/seed.sh` → backend API (`curl`) 経由での疎通確認まで実施した。

- `docker pull floci/floci:latest` に成功し、`sts get-caller-identity` / `ec2 describe-regions --region us-east-1` / `s3api list-buckets` が floci から正常応答することを確認した (issue の「プロファイル選択時の前提確認」項目)。
- `docker compose ... config` でマージ結果を確認し、`~/.aws:/root/.aws:ro` (ルート compose.yaml) が `./example/aws:/root/.aws:ro` (override) に正しく置き換わり (同一マウント先は override が優先)、`depends_on: floci` と `THIEF_S3_PATH_STYLE=true` が backend サービスに正しくマージされていることを確認した。
- `example/seed.sh` 実行後、backend API (`GET /api/aws/profiles`、`.../identity`、`.../regions`、`.../s3`、`.../s3/{bucket}/objects`、`.../dynamo`、`.../sqs`、`.../ssm/parameters`、`.../secretsmanager`、`.../cfn/stacks`) で seed 済みリソースが全て正しく返ることを実データで確認した。`floci` プロファイルは `auth_type: access_key` で表示され (SSO バッジなし)、想定どおりの挙動だった。
- 検証後は `docker compose ... down` でコンテナを、`docker rmi` でビルドしたイメージ (`thief-backend`/`thief-frontend`/`floci/floci`) を削除し、後片付け済み。

ブラウザ (`http://localhost:8088`) での目視確認は headless 環境のため未実施 (backend API 経由の確認で代替)。SSO ログイン、EC2 Start Session / ECS Exec ターミナル、Cost Explorer / 請求系は issue の設計どおり README に「動作確認できない機能」として明記した (未検証)。
