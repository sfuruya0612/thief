# Node.js を 26.13.1 に、Go を 1.27.4 に更新し、backend と frontend の依存パッケージを更新する

Created: 2026-11-12
Model: DeepSeek V4.1 Flash 1017
Branch: feature/chore-update-nodejs-go-and-dependencies

## 背景

2026-11-11 に Node.js の security release が公開され、26.x、24.x、22.x の各系統に修正版が出た。
26 系統の最新版は v26.13.1 (2026-11-10 公開、security release) である。
本リポジトリは `mise.toml` の `[tools].node` で `26.4.0` を固定しており、`node:http2`、`node:https`、TLS の脆弱性 (CVE-2026-123863、CVE-2026-123864、CVE-2026-123873 など) の修正を受け取っていない。

Go は `mise.toml` の `[tools].go` が `1.26.10`、`backend/go.mod` の `toolchain` 行が `go1.26.10` で揃っている。
go1.27.4 は 2026-11-10 に公開され、security fixes を含む 1.27 系の最新パッチである。
docs/issues/closed/0231 は、go1.27 系への更新を別の判断として本 issue へ送った。

依存パッケージの現状は、`backend` の `go list -m -u all` が 156 モジュールの更新を報告し、`frontend` の `npm outdated` が 30 パッケージの更新を報告する。
`npm audit --omit=dev` は 0 件で、既知の脆弱性は無い。

## 対応方針

- `mise.toml` の `[tools].node` を `26.13.1` にする。
- `mise.toml` の `[tools].go` を `1.27.4` に、`backend/go.mod` の `toolchain` 行を `go1.27.4` に同時に更新する (docs/issues/closed/0145 の運用)。
- `backend/go.mod` の `go` ディレクティブを `go 1.27.0` に上げ、言語バージョンを toolchain と揃える。
- `AGENTS.md` の「対象言語/バージョン」が定める Go 1.26 系の前提を 1.27 系に書き換える。
- `AGENTS.md` の「frontend (React)」の対象スタックと `docs/prd/thief.md` の「制約と前提」にある Go と React のバージョンを、更新後の値に揃える。
- backend は `go get -u ./...` で直接依存と間接依存を同じメジャーバージョンの中で最新に上げ、`go mod tidy` で `go.mod` と `go.sum` を整理する。
- frontend は `npm update` で `package.json` の範囲内の更新を当てたうえで、メジャー更新を `package.json` に反映する。対象は `react` と `react-dom` の 19、`@types/react` と `@types/react-dom` の 19、`@testing-library/jest-dom` の 7、`@xterm/xterm` の 6 と `@xterm/addon-fit` の 0.11、`apache-arrow` の 21、`i18next` の 27 と `react-i18next` の 18、`jsdom` の 31、`typescript` の 7 である。
- `@duckdb/duckdb-wasm` は `1.32.0` に固定したままにする。npm の `latest` は `1.33.1-dev57.0` であり、安定版が無いため。
- メジャー更新で API の差異が出た箇所は、呼び出し側を追随させる。追随の結果、画面の見た目と振る舞いを変える修正が要る場合は、その場で直さずに選択肢を報告する。
- 依存の更新で `mise run check` が落ちた場合、落ちた原因を直してから完了とする。テストの削除、skip、lint 規則の緩和では通さない。

## 完了条件

- `mise.toml` の `[tools].node` が `26.13.1` で、`node --version` が `v26.13.1` を返す。
- `mise.toml` の `[tools].go` が `1.27.4`、`backend/go.mod` の `toolchain` 行が `go1.27.4`、`go` ディレクティブが `go 1.27.0` であり、`go version` が `go1.27.4` を返す。
- `frontend/package.json` と `frontend/package-lock.json` が上記のメジャー更新を反映し、`npm ls` が依存の不整合を報告しない。
- `npm audit --omit=dev` の既知の脆弱性が 0 件である。
- `backend/go.mod` と `backend/go.sum` が `go mod tidy` の結果と一致する。
- `mise run check` が通る。fmt の実行後に `git status --porcelain` が空か、差分が生じたファイルを整形済みにして残す。
- `mise run backend:vuln` が終了コード 0 で、影響する脆弱性が 0 件と報告される。
- `AGENTS.md` の「backend (Go)」の「対象言語/バージョン」が Go 1.27 系を前提とする記述になっている。
- `AGENTS.md` の「frontend (React)」の対象スタックと `docs/prd/thief.md` の「制約と前提」が、Go 1.27 系と React 19 を指している。
- `CHANGES.md` の `## develop` の `### misc` に、Node.js、Go、backend、frontend の更新を説明するエントリを追加し、担当者行 `  - @sfuruya0612` を持つ。
- 扱わない範囲: `@duckdb/duckdb-wasm` の開発ビルドへの更新、Go の `go` ディレクティブが要求する言語バージョンを超える書き換え、CI のジョブ構成の変更。
