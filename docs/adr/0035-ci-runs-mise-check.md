# 0035. CI で mise run check を実行する

Created: 2026-10-27
Model: DeepSeek V4.1 Flash 1017
Status: Accepted
Decided: 2026-10-27

## 状況

ADR 0022 は品質ゲートを `mise run check` と pre-commit フックに置き、CI を持たないことを前提としていた。ADR 0031 は govulncheck を `check` タスクと `backend:vuln` に移し、pre-commit フックでは実行しないことを決めた。どちらも CI が無く、品質ゲートが各開発者の端末の pre-commit フックに依存する状態を記録している。

`docs/prd/thief.md` の「決定した論点」は、CI を GitHub Actions に置き、`mise run check` を実行することを決めた。Linux では `go test -race` に cgo が要る。`mise.toml` の `[env]` は `CGO_ENABLED = "0"` を固定し、シェルで先に設定した同名の変数より優先する。`check` の `depends` は並列に実行され、`fmt` の書き込みと `lint` の読み取りが競合しうる。

## 決定

- `.github/workflows/ci.yml` に CI を置く。実行契機は `pull_request`、`main` への `push`、毎週月曜 0:00 UTC の `schedule` とする。
- ランナーは `ubuntu-latest` とし、`actions/checkout` と `jdx/mise-action` を使う。`uses` はタグではなくコミット SHA で固定し、行末のコメントに更新時の目安となるバージョンを書く。権限は `contents: read` だけにし、checkout の資格情報は保存しない。
- `mise run ci:setup` で `go mod download` と `npm ci` を実行し、`mise run check` を実行する。`check` の後に `git status --porcelain` で差分と未追跡ファイルを検査する。
- `mise.toml` の `backend:test` は、`go env GOOS` が `darwin` 以外のとき `CGO_ENABLED=1` をコマンド単位で上書きする。
- `mise run check` は `fmt` の完了後に `lint`、`backend:vuln`、`test` を並列で実行する。`fmt` と `lint` の競合を避けるためである。
- ADR 0031 の「govulncheck を `backend:vuln` タスクに分け、`check` タスクの `depends` に加える」という決定のうち、`check` の `depends` の並びだけをこの ADR で置き換える。govulncheck を pre-commit フックで実行しないことと、`backend:test` の `-vet=off` は維持する。

## 検討した代替案

- ワークフローで `CGO_ENABLED=1 mise run check` とする。
  mise の `[env]` がシェルの値を上書きするため、`backend:test` には `CGO_ENABLED=0` が渡り、Linux で失敗する。
- CI 専用の `backend:test:ci` タスクを追加する。
  `check` の依存を `backend:test:ci` に差し替えると、CI 専用のタスク名をローカルの品質ゲートに持ち込むことになる。`go env GOOS` で分岐する 1 つの `backend:test` にすれば、ローカルと CI で同じタスクを使い続けられる。
- `mise.ci.toml` に `CGO_ENABLED = "1"` を置き、`MISE_ENV=ci` で実行する。
  設定ファイルが増え、ローカルの `check` と CI の `check` で参照する設定が分岐する。
- `mise run setup` を CI で使う。
  `go mod tidy` と `npm install` が `go.mod`、`go.sum`、`package-lock.json` を書き換えうる。DuckDB Wasm の拡張の取得も、テストでは使わないため不要である。
- `git diff --exit-code` で差分を検査する。
  未追跡ファイルを検出できない。
- `check` の `depends` を現状の並列のままにする。
  `fmt` の書き込みと `lint` の読み取りが競合しうる。
- frontend と backend のビルドも CI で検証する。
  PRD の CI の決定は `mise run check` の実行であり、ビルドの検証は別の判断を要する。

## 結果

- CI が緑になるには、Go ツールチェインと `golang.org/x/net` の更新 (issue 0231) が必要である。
- CI の初回の実行は、ユーザーが push した後に確認する。
- ADR 0022 と ADR 0031 の「CI が無い」という記述は、この ADR より前の状態を表す。
- ADR 0031 のうち、`check` の `depends` の並びだけをこの ADR で置き換える。govulncheck を pre-commit フックで実行しないことと、`backend:test` の `-vet=off` は維持する。このため ADR 0031 の Status を `Superseded by 0035` にし、ADR 0022 の Status は `Superseded by 0031` のまま変更しない。

## 根拠資料

- `docs/prd/thief.md` の「運用」と「決定した論点」
- `docs/issues/0231`、`docs/issues/0232`
- `mise.toml`、`AGENTS.md`
- `actions/checkout`、`jdx/mise-action`
