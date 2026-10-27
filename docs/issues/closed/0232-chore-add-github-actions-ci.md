# GitHub Actions で mise run check を実行する CI を追加する

Created: 2026-10-27
Model: DeepSeek V4.1 Flash 1017

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> GitHub Actions で `mise run check` を実行する CI を置きたい (docs/prd/thief.md の「運用」と「決定した論点」。`.github/workflows/` が無く、品質ゲートが各開発者の pre-commit フックに依存している。Linux では `go test -race` に cgo が要るため `CGO_ENABLED=1` でテストを実行する)

`docs/prd/thief.md` の「運用」は、CI を GitHub Actions に置いて `mise run check` を実行することと、Linux では `CGO_ENABLED=1` でテストを実行することを定めている。`docs/prd/thief.md` の「決定した論点」の CI の行も「GitHub Actions で `mise run check` を実行する」と決定している。しかし、`.github/workflows/` は空であり、追跡されたワークフローは無い。

自動で走る品質ゲートは `.pre-commit-config.yaml` の prek フックだけであり、`mise run fmt`、`mise run lint`、`mise run test` を実行するが、`backend:vuln` を含まない。`mise run check` を手元で実行する経路はあるが、自動では走らない。フックは各開発者のローカル環境でしか動かない。この状態は ADR 0022 と ADR 0031 の「結果」に記録されている。

mise は `[env]` で `CGO_ENABLED = "0"` を固定しており、`[env]` の値はシェルで先に設定した同名の変数より優先される。そのため、ワークフローで `CGO_ENABLED=1 mise run check` としても、`backend:test` が実行する `go test -race` には `CGO_ENABLED=0` が渡り、Linux では `-race requires cgo` で失敗する。`backend:test` のタスク内でコマンド単位に `CGO_ENABLED=1` を上書きする必要がある。

`mise run check` の `depends` は `["fmt", "lint", "backend:vuln", "test"]` であり、mise は `depends` を並列に実行する。`fmt` はファイルを書き換え、`lint` はファイルを読む。ローカルでも CI でも、両者が同時に走ると lint が書き換え前の内容を読む可能性がある。

2026-10-27 の時点で `mise run backend:vuln` は 12 件の脆弱性を報告して失敗している。脆弱性の解消は issue 0231 で行う。macOS の `mise run check` は 0231 の完了後に通り、CI の Linux の `mise run check` は本 issue の `backend:test` の cgo 対応を入れた後に通る。

## 対応方針

- `.github/workflows/ci.yml` を追加する。ジョブは `check` の 1 つだけとし、`runs-on: ubuntu-latest`、`timeout-minutes: 30` とする。
- 実行契機は `pull_request`、`main` への `push`、`schedule` の `cron: '0 0 * * 1'` (毎週月曜 0:00 UTC) とする。スケジュール実行は、2026-10-27 の会話で利用者が週 1 回の実行を求めたため加える。依存の脆弱性データベースの更新を継続的に検出する目的である。
- `permissions` は `contents: read` だけにする。追加の API 呼び出しと権限は無く、AWS、Google Cloud、Datadog の認証情報は使わない。
- `concurrency` の `group` を `ci-${{ github.workflow }}-${{ github.event_name }}-${{ github.ref }}` とし、`cancel-in-progress: true` で同じイベントと ref の古い実行をキャンセルする。`github.event_name` を含めるのは、週次の `schedule` が `main` への `push` の実行をキャンセルしないようにするためである。
- `actions/checkout@v7` でリポジトリを取得する。`persist-credentials: false` を指定し、後のステップの npm のインストールスクリプトから GitHub のトークンを読めないようにする。`jdx/mise-action@v5` で `mise.toml` の `[tools]` を導入する。`mise-action` のキャッシュは既定で有効なため、そのまま使う。
- `mise run ci:setup` で依存を取得してから、`mise run check` を実行する。`ci:setup` は `backend:ci` と `frontend:ci` に依存する。`backend:ci` は `go mod download`、`frontend:ci` は `npm ci` を実行する。`mise run setup` は `go mod tidy` と `npm install` と `frontend:fetch-duckdb-extensions` を含むため、CI では使わない。`npm install` は `package-lock.json` を書き換えうる。`fetch-duckdb-extensions` は、frontend のテストが DuckDB Wasm のエンジンを差し替えるか `@duckdb/duckdb-wasm` をモックしており、拡張ファイルを読まないため不要である。
- `mise run check` の後に `test -z "$(git status --porcelain)"` を実行し、`fmt` がファイルを書き換えていないことと、依存取得が意図しないファイルを追加していないことを検証する。`git diff --exit-code` は未追跡ファイルを検出しないため使わない。
- `mise.toml` の `backend:test` をシェルスクリプトに変える。`go env GOOS` が `darwin` 以外のときは `CGO_ENABLED=1 go test -race -cover -vet=off ./...` を、`darwin` のときは `go test -race -cover -vet=off ./...` を実行する。
- ワークフローから `CGO_ENABLED=1` を渡す案は採らない。mise の `[env]` が優先されるためである。
- CI 専用の `backend:test:ci` タスクを追加する案は採らない。`check` の依存を `backend:test:ci` に差し替えると、CI 専用のタスク名をローカルの品質ゲートに持ち込むことになる。`go env GOOS` で分岐する 1 つの `backend:test` にすれば、ローカルと CI で同じタスクを使い続けられる。
- `mise.ci.toml` に `CGO_ENABLED = "1"` を置き、ワークフローで `MISE_ENV=ci mise run check` を実行する案は採らない。設定ファイルが増え、ローカルの check と CI の check で参照する設定が分岐するためである。
- `mise run check` の `depends` を `["fmt"]` に、`depends_post` を `["lint", "backend:vuln", "test"]` に変える。`fmt` の完了後に lint、vuln、test を並列で実行する。現状の並列のままにする案は、`fmt` の書き込みと `lint` の読み取りが競合するため採らない。
- `docs/adr/0035-ci-runs-mise-check.md` を追加し、`docs/adr/README.md` の索引に加える。ADR 0035 は `Created`、`Model`、`Status`、`Decided` のヘッダを持ち、「状況、決定、検討した代替案、結果、根拠資料」の節をこの順に置く。ADR 0031 の「govulncheck を `backend:vuln` タスクに分け、`check` タスクの `depends` に加える」という決定のうち、`check` の `depends` の並びだけを置き換える。govulncheck を pre-commit フックで実行しないことと、`backend:test` の `-vet=off` は維持する。ADR 0031 の Status を `Superseded by 0035` にする。ADR 0022 の Status は `Superseded by 0031` のまま変更しない。
- `frontend:build` と `backend:build` の CI、リリースワークフロー、Dependabot、CodeQL、カバレッジの送信は、この issue では扱わない。PRD が CI について定めているのは `mise run check` の実行であり、ビルドの検証は別の判断を要するためである。スケジュール実行、`git status --porcelain` による差分の検証、`check` の順序の変更は、TODO の文言には無いが、利用者の要望と CI を安定させるために必要である。

## 完了条件

- `.github/workflows/ci.yml` が存在し、`pull_request`、`main` への `push`、`schedule` の `cron: '0 0 * * 1'` のトリガーを定義している。
- ワークフローは `ubuntu-latest` で `actions/checkout` と `jdx/mise-action` を使い、`uses` はタグではなくコミット SHA で固定されている (行末のコメントにバージョンを書く)。`actions/checkout` は `persist-credentials: false` を指定し、`permissions` は `contents: read` だけである。
- ワークフローは `mise run ci:setup`、`mise run check`、`test -z "$(git status --porcelain)"` を順に実行する。
- `mise.toml` に `ci:setup`、`backend:ci`、`frontend:ci` があり、`ci:setup` が `backend:ci` と `frontend:ci` に依存し、`backend:ci` が `go mod download`、`frontend:ci` が `npm ci` を実行する。
- `mise.toml` の `backend:test` が、`go env GOOS` が `darwin` 以外のとき `CGO_ENABLED=1` をコマンド単位で上書きする。
- `mise run check` の `depends` が `fmt` だけ、`depends_post` が `lint`、`backend:vuln`、`test` である。
- macOS 上で `mise run ci:setup`、`mise run check` を順に実行し、その後に `git status --porcelain` が空である。
- `golang:1.26.10` の公式イメージ (gcc を含む) の Linux コンテナで mise を導入して `mise trust` を実行し、`mise run backend:test` を実行して通る。この確認で `go env GOOS` が `linux` を返す分岐と `CGO_ENABLED=1` の上書きを実際に踏む。
- `AGENTS.md` の次の記述が更新されている。ルートタスク表の `mise run check` の説明が「`fmt` の後に `lint`、`vuln`、`test` を並列で実行」である。ルートタスク表に `ci:setup`、backend タスク表に `backend:ci` と `backend:test` の Linux の上書き、frontend タスク表に `frontend:ci` がある。「pre-commit」の節に、CI が `.github/workflows/ci.yml` で `mise run check` を実行することが書かれている。「backend ビルドと CI」の `CGO_ENABLED=0` の説明と Linux の `-race` の記述が、`backend:test` が `go env GOOS` を見て `CGO_ENABLED=1` を上書きすることを説明している。「並行処理」の節の「データ競合検出のため、テストおよび CI では `go test -race ./...` を必ず実行する」の Linux の説明も同じ内容になっている。
- `docs/prd/thief.md` の「運用」の CI の項目が、`.github/workflows/ci.yml`、実行契機 (`pull_request`、`main` への `push`、`schedule` の `cron: '0 0 * * 1'`)、`mise run ci:setup`、`mise run backend:test` が `go env GOOS` を見て `CGO_ENABLED=1` を上書きすること、`git status --porcelain` による差分の検証を説明している。「ワークフローは未作成である (`.github/workflows/` が無い。「決定した論点」)。」の文が残っていない。「決定した論点」の CI の行の後続の作業が「無い (issue 0232 で追加)」になっている。
- `docs/adr/0035-ci-runs-mise-check.md` が `Created`、`Model`、`Status`、`Decided` のヘッダを持ち、「状況、決定、検討した代替案、結果、根拠資料」の節をこの順に持つ。「検討した代替案」に、ワークフローからの `CGO_ENABLED=1`、`backend:test:ci`、`mise.ci.toml`、`check` の並列維持の却下理由が書かれている。「結果」に、ADR 0022 と ADR 0031 の「CI が無い」という記述は当時の状態の記録であり、ADR 0031 のうち `check` の `depends` の並びだけを置き換え、govulncheck を pre-commit で実行しないことと `backend:test` の `-vet=off` は維持することが書かれている。`docs/adr/0031-vuln-check-in-check-task-not-precommit.md` の Status が `Superseded by 0035` であり、`docs/adr/0022-quality-gates.md` の Status は `Superseded by 0031` のままである。`docs/adr/README.md` の索引に 0035 が追加されている。
- `CHANGES.md` の `## develop` の `### misc` に、CI の追加を説明するエントリを 1 つ追加し、変更内容の次の行に 2 文字分インデントした担当者行 `  - @sfuruya0612` を置く。
- `docs/issues/TODO.md` の該当行を `[x]` にし、`(docs/issues/closed/0232)` を追記する。
- 扱わない範囲: `frontend:build` と `backend:build` の CI、リリースワークフロー、Dependabot、CodeQL、カバレッジの送信、ブランチ保護の設定、CI の初回実行の確認 (ユーザーが push した後に確認する)。

## 関連

- issue 0231 (Go ツールチェインと `golang.org/x/net` の更新)。0231 で脆弱性を解消し、本 issue の Linux の cgo 対応と合わせて CI の `mise run check` が通る。0231 を先に実装する。

## 解決方法

- `.github/workflows/ci.yml` を追加した。`pull_request`、`main` への `push`、`schedule` の `cron: '0 0 * * 1'` で起動し、`ubuntu-latest` で `actions/checkout` (`3d3c42e5aac5ba805825da76410c181273ba90b1`、v7.0.1) と `jdx/mise-action` (`82a612c5ab8bcda7cc5db6b7e81a8e7584c3dc6e`、v5.2.3) をコミット SHA で固定して使う。`persist-credentials: false` と `permissions: contents: read` を指定する。
- ワークフローは `mise run ci:setup`、`mise run check`、`test -z "$(git status --porcelain)"` を順に実行する。
- `mise.toml` に `ci:setup`、`backend:ci`、`frontend:ci` を追加した。`ci:setup` は `go mod download` と `npm ci` を実行する。
- `mise.toml` の `backend:test` を、`go env GOOS` が `darwin` 以外のとき `CGO_ENABLED=1` をコマンド単位で上書きするシェルスクリプトに変えた。
- `mise run check` の `depends` を `fmt` だけにし、`depends_post` で `lint`、`backend:vuln`、`test` を並列に実行するように変えた。
- macOS 上で `mise run ci:setup`、`mise run check` を順に実行し、その後に `git status --porcelain` が空であることを確認した。
- `golang:1.26.10` の Linux コンテナで mise を導入し、`mise run backend:test` を実行して、`go env GOOS` が `linux` を返す分岐と `CGO_ENABLED=1` の上書きを確認した。
- `AGENTS.md` の pre-commit の節、ルート / backend / frontend のタスク表、並行処理の節、backend ビルドと CI の節を更新した。
- `docs/prd/thief.md` の「運用」の CI の項目と「決定した論点」の CI の行を更新した。
- `docs/adr/0035-ci-runs-mise-check.md` を追加し、`docs/adr/README.md` の索引に加え、ADR 0031 の Status を `Superseded by 0035` にした。ADR 0022 の Status は `Superseded by 0031` のまま変更していない。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追加し、`docs/issues/TODO.md` の該当行を `[x]` にした。
