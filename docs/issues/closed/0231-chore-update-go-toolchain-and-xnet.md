# Go ツールチェインを go1.26.10 に、golang.org/x/net を v0.60.0 に更新する

Created: 2026-10-27
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-10-27

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> Go のツールチェインを go1.26.10 以上に、golang.org/x/net を v0.60.0 以上に更新したい (issue 0218 のスコープ外として送り)

2026-10-25 の `govulncheck` は 12 件の脆弱性を報告し、`mise run backend:vuln` と `mise run check` が失敗する。2026-10-27 に `mise run backend:vuln` を実行し、12 件を報告して終了コード 3 で失敗することを確認した。

`govulncheck` は 12 件を標準ライブラリ由来と集計している。既定の出力の `Found in` は crypto/x509、net/http、mime/multipart、crypto/tls、html/template の 5 パッケージである。このうち 5 件 (GO-2026-6617、GO-2026-6612、GO-2026-6611、GO-2026-6610、GO-2026-6603) は `golang.org/x/net` v0.59.0 のモジュールにも該当する。GO-2026-6608 の影響範囲は net/textproto と mime/multipart で、既定の出力の `Found in` は mime/multipart である。GO-2026-6605 の影響範囲には net/http/httputil も含まれるが、このリポジトリの `Found in` は net/http である。

GO-2026-6809 の修正版は go1.26.10、それ以外の標準ライブラリの修正版は go1.26.9、x/net の 5 件の修正版は v0.60.0 である。したがって、go と x/net の両方を更新する必要がある。

現在のバージョンは、`mise.toml` の `[tools].go` が `1.26.6`、`backend/go.mod` の `toolchain` 行が `go1.26.6`、`golang.org/x/net` が `v0.59.0` である。`golang.org/x/net` は backend から直接 import されておらず、`internal/gcp`、`cloud.google.com/go/logging/apiv2`、`google.golang.org/api/transport/http` を経由して使われる間接依存である。

`AGENTS.md` の「backend ビルドと CI」は、`mise.toml` の `[tools].go` と `backend/go.mod` の `toolchain` 行を同一バージョンへ同時に更新することを定めている (issue 0145)。

## 対応方針

- `mise.toml` の `[tools].go` を `1.26.10` に、`backend/go.mod` の `toolchain` 行を `go1.26.10` に同時に更新する。
- `backend` で `go get golang.org/x/net@v0.60.0` を実行し、`go mod tidy` で `go.mod` と `go.sum` を整理する。x/net は `// indirect` のままにする。
- `mise install` で go1.26.10 を導入し、`mise run backend:vuln` を実行して、影響する脆弱性が 0 件で終了コード 0 になることを確認する。修正版が無い GO-2026-5932 (`golang.org/x/crypto/openpgp`、`Fixed: N/A`) はモジュール単位の指摘として残りうるが、コードから到達しないため終了コードには影響しない。
- `mise run check` を macOS 上で実行して通ることを確認する。Linux では `mise.toml` の `[env] CGO_ENABLED = "0"` により `backend:test` の `go test -race` が失敗するが、その対応は issue 0232 で行う。
- go1.26.9 に留める案は採らない。GO-2026-6809 の修正が go1.26.10 だけに含まれ、go1.26.9 では 1 件が残るためである。
- go1.26.10 だけを更新し、`golang.org/x/net` を v0.59.0 のままにする案は採らない。x/net の 5 件が残り、`mise run backend:vuln` が失敗し続けるためである。
- `golang.org/x/net` だけを更新する案は採らない。標準ライブラリ由来の 12 件が残り、`mise run check` が失敗し続けるためである。
- go1.27 系へ上げる案は採らない。2026-10-27 時点の最新安定版は go1.27.3 だが、`AGENTS.md` は backend を Go 1.26 系とすることを前提としている。1.27 系への更新は別の判断とし、本 issue では 1.26 系の最新パッチに留める。あわせて `AGENTS.md` の「現行最新は 1.26.x」という記述を削除する。
- `golang.org/x/net` を v0.61.0 にする案は採らない。v0.61.0 は x/crypto v0.58.0、x/sys v0.49.0、x/text v0.43.0 を要求し、更新範囲が広がるためである。TODO の最低ラインである v0.60.0 に固定する。

## 完了条件

- `mise.toml` の `[tools].go` が `1.26.10` である。
- `backend/go.mod` の `toolchain` 行が `go1.26.10` である。
- `backend/go.mod` の `golang.org/x/net` が `v0.60.0` で `// indirect` のままである。
- `mise run backend:vuln` の終了コードが 0 で、影響する脆弱性が 0 件と報告される。GO-2026-5932 (`golang.org/x/crypto/openpgp`、`Fixed: N/A`) はモジュール単位の指摘として残ってよい。
- macOS 上で `mise run check` が通る。
- `AGENTS.md` の「backend (Go)」の「対象言語/バージョン」から「(現行最新は 1.26.x)」が削除されている。
- `CHANGES.md` の `## develop` の `### misc` に、Go ツールチェインと `golang.org/x/net` の更新を説明するエントリを 1 つ追加し、変更内容の次の行に 2 文字分インデントした担当者行 `  - @sfuruya0612` を置く。
- `docs/issues/TODO.md` の該当行を `[x]` にし、`(docs/issues/closed/0231)` を追記する。
- 扱わない範囲: go1.27 系への更新、`golang.org/x/net` 以外の依存の更新、`backend/go.mod` の `go` ディレクティブの変更、Linux の `CGO_ENABLED=1` 対応 (issue 0232)、CHANGES.md の既存エントリの並びの修正。

## 解決方法

- `mise.toml` の `[tools].go` を `1.26.6` から `1.26.10` に、`backend/go.mod` の `toolchain` 行を `go1.26.6` から `go1.26.10` に同時に更新した。
- `backend` で `go get golang.org/x/net@v0.60.0` を実行し、`go mod tidy` で `go.mod` と `go.sum` を整理した。`golang.org/x/net` は `v0.60.0 // indirect` のままである。
- `mise run backend:vuln` を実行し、影響する脆弱性が 0 件で終了コード 0 になることを確認した。モジュール単位の GO-2026-5932 (`Fixed: N/A`) は残る。
- macOS 上で `mise run check` を実行し、通ることを確認した。
- `AGENTS.md` の「backend (Go)」の「対象言語/バージョン」から「(現行最新は 1.26.x)」を削除した。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追加し、`docs/issues/TODO.md` の該当行を `[x]` にした。
