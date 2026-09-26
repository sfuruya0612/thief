# 0031 Docker によるアプリ起動を廃止し関連ファイルを削除する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8 claude-opus-4-8

## 背景 / 根拠

thief 本体 (backend / frontend) を Docker 上で起動する構成は要件から外れた。
使われない起動経路のファイルと記述を残すと、セットアップ手順が二重化し、保守対象と混乱の元になる。
本 issue では Docker によるアプリ起動に関連するファイルと記述を削除する。

通常の起動は `mise run backend:run` (ネイティブ) と `mise run frontend:run` (Vite dev server) に一本化する。

## 削除対象

Docker でアプリを起動するためだけに存在するファイルを削除する。

- `compose.yaml` (ルート。backend / frontend サービス定義と `custom_ca` シークレット)
- `backend/Dockerfile`
- `backend/.dockerignore`
- `frontend/Dockerfile`
- `frontend/.dockerignore`
- `frontend/nginx.conf` (dist の静的配信と `/api/` リバースプロキシ。Docker 実行時のみ使用)

`mise.toml` から以下のタスクを削除する。

- `docker:up` (`mise.toml` L142-145)
- `docker:down` (`mise.toml` L147-149)

企業ネットワークの CA を build 時に注入する `custom_ca` シークレットと `CUSTOM_CA_CERT` の仕組み (`compose.yaml` L5-6, L28-29, L35-41 および各 Dockerfile 側の受け口) は Docker build 専用のため、上記ファイル削除に伴い消える。

## example/ (floci ローカル動作確認環境) の扱い

example/ は残す。
ただし現状の `example/compose.yaml` はルートの `compose.yaml` に定義された backend / frontend サービス (Docker ビルド) をオーバーレイして動く前提であり、上記の削除対象をそのまま消すと `mise run example:up` が機能しなくなる。

そこで example/ を floci コンテナ単体を起動する構成に作り替える。

- `example/compose.yaml` を、`floci/floci:latest` (ポート 4566) のみを起動する standalone な compose に変更する (ルート `compose.yaml` への依存を断つ)。
- thief 本体はネイティブ起動 (`mise run backend:run` / `mise run frontend:run`) とし、backend に `THIEF_S3_PATH_STYLE=true` と floci 専用プロファイル (`example/aws`) を与えて floci のエンドポイント (`http://localhost:4566`) を向ける。
- `mise` の `example:up` / `example:down` (`mise.toml` L152-158) は floci 単体 compose を起動 / 停止する内容に更新する。
- `example:seed` (`mise.toml` L160-162) と `example/seed.sh`、`example/aws/config`、`example/aws/credentials` は現状どおり利用する。
- floci 単体構成に切り替えた手順に合わせて `example/README.md` を更新する (起動手順、アクセス先ポート、注意書き)。

floci は外部の AWS エミュレータであり単一コンテナとしてのみ配布される。
本 issue で廃止するのは thief 本体を Docker で起動する経路であって、floci コンテナの利用は対象外である。

## コードへの影響

以下のコードは Docker 専用ではないため残す。

- backend の設定 env (`THIEF_LISTEN_ADDR` / `THIEF_WEB_ORIGINS` / `THIEF_SNIPPETS_DIR` / `THIEF_S3_PATH_STYLE`) とデフォルト値 (`backend/internal/config/config.go`)。compose がこれらを上書きしていただけで、デフォルト値はローカル実行向けであり変更不要。
- backend の S3 path-style opt-in (`backend/internal/aws/s3.go`)。floci を含む S3 互換エミュレータ向けの汎用機能。
- frontend の `VITE_API_BASE` 参照 (`frontend/src/api/client.ts` L3-11, `frontend/src/vite-env.d.ts`)。Vite の一般的なビルド時 env。

`client.ts` L9-10 の「空文字列のとき `window.location.origin` を使う」分岐は nginx リバースプロキシ (同一オリジン配信) のために導入した経路であり、Docker 廃止後は使われなくなる。
残しても無害だが、この機会に削除して分岐を単純化してよい (実装時に判断)。

## ドキュメント更新

- `README.md` の Docker 起動節 (L54-64) と `thief.local` 節 (L66-74)、および L17 の「ローカル / Docker 起動の両方で必要」の文言から Docker 表記を除く。
- `AGENTS.md` の `## Docker (ローカル開発)` 節 (L61-76) を削除する。
- `CHANGES.md` は変更履歴のため過去エントリ (L35, L49) は残し、本変更を `## develop` に追記する。

## スコープ外

- 起動ポートの変更 (issue 0032 で扱う)。
- example/ の floci 以外への移行や動作確認範囲の拡張。

## 検証

- `mise run backend:run` と `mise run frontend:run` で起動でき、ブラウザから通常どおり操作できることを確認する。
- `mise run example:up` (floci 単体) + 本体ネイティブ起動 + `mise run example:seed` で、S3 / DynamoDB / SQS / SSM / Secrets Manager / CloudFormation の一覧表示が確認できることを確認する。
- `docker` / `compose` / `nginx` を参照する記述がリポジトリに残っていないこと (example/ の floci 分を除く) を確認する。
- `mise run check` が全通過することを確認する。

## 解決方法

- Docker 専用ファイルを削除した: `compose.yaml`、`backend/Dockerfile`、`backend/.dockerignore`、`frontend/Dockerfile`、`frontend/.dockerignore`、`frontend/nginx.conf`。
- `mise.toml` から `docker:up` / `docker:down` タスクを削除した。
- `example/` を floci コンテナ単体の standalone 構成に作り替えた。
  - `example/compose.yaml` を `floci/floci:latest` (ポート 4566) のみの compose に変更した (ルート `compose.yaml` への依存を断った)。
  - `example/aws/` を `example/home/.aws/` に再配置した。従来は Docker の `~/.aws` ボリュームマウントで隔離していたが、ネイティブ起動では thief の `ListProfiles` (`backend/internal/aws/profiles.go`) が `os.UserHomeDir()` を直接読むため `AWS_CONFIG_FILE` 等の環境変数では隔離できないことが実機検証で判明した。`HOME` 環境変数そのものを `example/home` に差し替えることで、`os.UserHomeDir()` の解決先を丸ごと切り替え、Docker のボリュームマウントと同じ隔離を実現した。
  - `example/home/.aws/config` の `endpoint_url` をコンテナ内 DNS 名 `http://floci:4566` から、ホストから直接アクセスする `http://localhost:4566` に変更した (ネイティブ起動では backend がホスト上で動くため)。
  - `mise.toml` の `example:up` / `example:down` を `docker compose -f example/compose.yaml up/down` に更新した (ルート compose とのオーバーレイ指定を削除)。
  - `example/README.md` を standalone 構成の起動手順 (`HOME` / `THIEF_S3_PATH_STYLE` を設定して `mise run backend:run` / `frontend:run` を実行する) に書き換えた。
- 実機検証: `mise run example:up` → `mise run example:seed` → `HOME=$(pwd)/example/home THIEF_S3_PATH_STYLE=true go run ./cmd/thief server` で起動し、`/api/aws/profiles` が `floci` のみを返すこと (実 `~/.aws` から隔離されていること)、`/api/aws/profiles/floci/s3` と `/api/aws/profiles/floci/dynamo` が seed 済みのバケット・テーブルを正しく返すことを確認した。
- `frontend/src/api/client.ts` の `VITE_API_BASE` が空文字のとき `window.location.origin` を使う分岐 (nginx リバースプロキシ専用) を削除し、`BASE_URL` の解決を単純化した。テストでこの分岐への依存はなかったため変更のみで対応した。
- ドキュメントを更新した: `README.md` の Docker 起動節・`thief.local` 節と「ローカル / Docker 起動の両方で必要」の文言を削除し、`AGENTS.md` の `## Docker (ローカル開発)` 節を削除した。`CHANGES.md` の `## develop` に `[CHANGE]` エントリを追記した (過去の Docker 追加時のエントリは履歴としてそのまま残した)。
- `mise run backend:run` / `mise run frontend:run` によるネイティブ起動が変更後も問題なく動作することを確認した。
- `docker` / `Docker` / `compose.yaml` / `nginx` の残存参照を全文検索し、`mise.toml` の `example:up`/`down` タスク定義 (floci 自体の起動に docker compose を使うため意図的に残る) と `CHANGES.md` の過去エントリ以外に残っていないことを確認した。
- `mise run check` が全通過することを確認した。
