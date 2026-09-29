# backend のビルドとテストを CGO_ENABLED=0 に固定する

Created: 2026-09-29
Model: Claude Fable 5.1
Completed: 2026-09-29

## 背景

`docs/issues/TODO.md` の「backend のビルドとリンクが遅いので、CGO_ENABLED=0 で cgo (DataDog/zstd) を外してリンクを速くしたい」に対応する。

`mise.toml` の backend タスク (`backend:build` / `backend:install` / `backend:test` / `backend:lint`) は `CGO_ENABLED` を設定せず、Go の既定でビルドしている。
既定は `$(go env GOROOT)/src/cmd/go/internal/cfg/cfg.go` の判定 (`$CGO_ENABLED` があればそれに従い、無ければクロスコンパイルなら 0、ネイティブなら `platform.CgoSupported`) で決まり、darwin/arm64 のネイティブビルドでは 1 になる。
`go env CGO_ENABLED` は 1 で、`go version -m "$(go env GOPATH)/bin/thief"` も `build CGO_ENABLED=1` を示す。
`AGENTS.md` の「backend ビルドと CI」は「バイナリビルドは `CGO_ENABLED=0` を基本とする」と定めているが、タスクはそれを実現していない。

2026-09-29 に 11 コアの Mac (Go 1.26.6 darwin/arm64、依存を含め 990 パッケージ) で計測した結果は次のとおり。

| 操作 | CGO_ENABLED=1 (現状) | CGO_ENABLED=0 |
| --- | --- | --- |
| `go install ./cmd/thief` のリンク (warm キャッシュ、main の再リンクだけ) | 16.7〜20.9 秒 | 5.3 秒 |
| バイナリサイズ | 204 MB | 204 MB |
| `go build ./...` (cold キャッシュ) | 83 秒 (CPU 340 秒、うち `DataDog/zstd` の C コンパイル 42 秒) | 77 秒 |
| `go test -race -cover ./...` のテストバイナリ 18 本の再リンク (warm) | 27 秒 | 24 秒 |

リンクが遅い原因は cgo である。
`backend/go.mod` が依存する `github.com/DataDog/datadog-api-client-go/v2` v2.65.0 の `api/datadog` パッケージは、`zstd.go` (`//go:build cgo`) で cgo パッケージ `github.com/DataDog/zstd` (C ソースを同梱) を import する。
標準ライブラリ以外の cgo パッケージが依存グラフにあると Go のリンカは外部リンク (clang) に切り替わり、darwin では `dsymutil` と `strip` も走る。
この外部リンクがリンク時間の大半を占める。
依存グラフで cgo ファイルを持つパッケージは `runtime/cgo` と `github.com/DataDog/zstd` の 2 つだけである (`backend/` で `go list -deps -f '{{if .CgoFiles}}{{.ImportPath}}{{end}}' ./cmd/thief` を実行して確認)。
`os/user` は AWS SDK の `internal/shareddefaults` や Google の認証ライブラリを経由して依存グラフに含まれるが、darwin では cgo の有無にかかわらず syscall 版 (`os/user/cgo_lookup_syscall.go` の `//go:build !osusergo && darwin`) が使われるため、cgo ファイルにならず、`CGO_ENABLED` の切替で挙動も変わらない。

`CGO_ENABLED=0` にすると `api/datadog` は `no_zstd.go` (`//go:build !cgo`) に切り替わり、`compressZstd` は常にエラーを返す。
この関数は `client.go` (399〜400 行) がリクエストヘッダ `Content-Encoding` を `zstd1` にしたときだけ呼ぶ。
thief が `backend/internal/datadog/` から呼ぶ API は `GetDashboard` / `ListDashboards` / `QueryMetrics` / `ListOrgs` / `GetEstimatedCostByOrg` / `GetHistoricalCostByOrg` の 6 つで、いずれもこのヘッダを設定しない (`backend/internal/datadog/` に `Content-Encoding` も `zstd` も出てこない)。
したがって Datadog の呼び出しの挙動は変わらない。

darwin では `-race` に cgo が要らない (`$(go env GOROOT)/src/cmd/go/internal/work/init.go` の 193〜195 行の判定は darwin を除外する)。
`go test -race -cover ./...` が `CGO_ENABLED=0` でも全パッケージ通ることを確認した。
名前解決も darwin では cgo 無しで libc の syscall を使う (`net/cgo_unix_syscall.go` の `//go:build !netgo && darwin`) ため、挙動は変わらない。

## 対応方針

`mise.toml` の `[env]` に `CGO_ENABLED = "0"` を追加する。

- `[env]` は mise の全タスクと、`mise activate` したシェルでこのリポジトリのディレクトリにいる間の環境に効く。
  `go build` や `go test` を直接叩いたときも同じ値になり、タスクと手打ちでビルドキャッシュが cgo の有無で 2 系統に分かれない。
- mise 2026.9.12 で確認したところ、`[env]` の値はシェルで先に設定した同名の変数より優先される (`[env]` に `PROBE_FLAG = "0"` を書いた設定で `PROBE_FLAG=1 mise env` を実行しても 0 が返る)。
  cgo を有効にして試したいときは、`env CGO_ENABLED=1 go build ./...` のようにコマンド単位で上書きする。
- 切替後の初回は、cgo 無しの依存を再コンパイルするため cold 相当の時間が掛かる (cold の計測値は `go build ./...` が `CGO_ENABLED=0` で 77 秒、`go test -race -cover ./...` が `CGO_ENABLED=1` で 124 秒。後者は `CGO_ENABLED=0` では未計測)。
  2 回目からはキャッシュが効く。
- `AGENTS.md` の「backend ビルドと CI」に、`CGO_ENABLED=0` を `mise.toml` の `[env]` で固定していることと、Linux では `-race` に cgo が要るためこの設定のままでは `backend:test` を実行できない (`init.go` の判定は darwin 以外で `-race requires cgo` として終了する。Linux で実行するときは `backend/` で `env CGO_ENABLED=1 go test -race -cover ./...` を使う) ことを追記する。
  「並行処理」節の「テストおよび CI では `go test -race ./...` を必ず実行する」の行にも、Linux では `env CGO_ENABLED=1` が要る旨を添える (同じ制約を 2 つの節から引けるようにする)。

採らなかった案。

- `backend:build` / `backend:install` / `backend:run` だけに task 単位の `env` で設定する: `backend:test` と `backend:lint` が cgo のままになり、`api/datadog` と `DataDog/zstd` のコンパイル結果がビルド用とテスト用でキャッシュに 2 系統でき、zstd の C コンパイルも残る。
  テストバイナリの再リンクの短縮 (27 秒から 24 秒) も得られない。
- `-ldflags='-s -w'` の追加: リンク時間は 5.5 秒で変わらず、バイナリが 204 MB から 139 MB に減るだけである。
  時間の短縮を目的とするこの issue では扱わない。
- Datadog クライアントの依存を外す、または zstd を使わないクライアントに差し替える: Datadog の機能そのものを変える変更で、この issue の範囲を超える。
- `os()` の条件で Linux だけ `CGO_ENABLED=1` にするテンプレート: このリポジトリに Linux でビルドする経路が無い (CI の設定が無く、`example/` の docker は floci だけを動かす)。
  存在しない経路のために条件分岐を持ち込まず、Linux の制約は `AGENTS.md` に書く。

## 完了条件

- `mise.toml` の `[env]` に `CGO_ENABLED = "0"` があり、コメントで理由 (`DataDog/zstd` の cgo が外部リンクを強制する) を書く。
- `mise run backend:install` が成功し、`go version -m "$(go env GOPATH)/bin/thief"` の出力に `build CGO_ENABLED=0` がある。
- `mise run backend:test` が darwin で成功する (`-race` を外さない)。
- `$(go env GOPATH)/bin/thief` を削除してから `mise run backend:install` を実行する所要時間を、変更前 (`CGO_ENABLED=1`) と変更後 (`CGO_ENABLED=0`) で各 1 回計測して「## 解決方法」に記録し、変更後の所要時間が変更前より短い。
  各計測の前に同じ設定で 1 回ビルドしてキャッシュを温め、main の再リンクだけが計測に入るようにする。
  変更前の計測は `mise.toml` を変える前に行う。
  変更後に測り直すときは、`[env]` がシェルの変数より優先されるため `mise run` では `CGO_ENABLED=1` にできないので、`backend/` で `env CGO_ENABLED=1 go install ./cmd/thief` を実行する。
- `AGENTS.md` の「backend ビルドと CI」に対応方針の 2 点を追記し、「並行処理」節の `-race` の行に Linux の注記を添える。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追加する。
- `mise run check` が通る。

## 関連

- issue 0196: Go 側の DuckDB バインディングを `CGO_ENABLED=0` の方針を理由に却下した。
  この issue はその方針をタスクで実現する。
- issue 0216: pre-commit の backend チェックの短縮。
  `mise.toml` の別の箇所 (`backend:lint` / `backend:test` / `check`) を変える。

## 解決方法

`mise.toml` の `[env]` に `CGO_ENABLED = "0"` を追加し、理由 (`github.com/DataDog/zstd` の cgo が Go のリンカを外部リンクに切り替えてリンク時間の大半を占める)、thief が使う Datadog API が zstd 圧縮の経路を通らないこと、`[env]` がシェルの変数より優先されるため cgo を有効にするときは `env CGO_ENABLED=1 go build ./...` のようにコマンド単位で上書きすることをコメントに書いた。
`AGENTS.md` の「backend ビルドと CI」に固定の事実と理由と上書きの方法、Linux では `-race` に cgo が要るため `backend/` で `env CGO_ENABLED=1 go test -race -cover ./...` を使うことを追記し、「並行処理」節の `-race` の行にも Linux の注記を添えた。

確認した結果 (2026-09-29、Go 1.26.6、darwin)。

- `mise env` の出力に `set -gx CGO_ENABLED 0` がある。
- `mise run backend:install` が成功し、`go version -m "$(go env GOPATH)/bin/thief"` に `build CGO_ENABLED=0` がある。
  `backend/` で `go list -deps -f '{{if .CgoFiles}}{{.ImportPath}}{{end}}' ./cmd/thief` は何も出さない (cgo を含むパッケージが依存グラフから消えた)。
- `mise run backend:test` (`go test -race -cover ./...`) が darwin で 18 パッケージすべて成功した。
- `mise run check` が通った。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追記した (種別タグ無しは `### misc` の既存の書式に合わせた)。

所要時間 (`$(go env GOPATH)/bin/thief` を削除してから `mise run backend:install` を実行する時間。各計測の前に同じ設定で 1 回ビルドしてキャッシュを温めた。他の処理を走らせていない状態で `/usr/bin/time -p` で計測)。

| 設定 | real | user | sys |
| --- | --- | --- | --- |
| 変更前 (`CGO_ENABLED=1`、`mise.toml` を変える前の HEAD 5202e60 で計測) | 17.40 秒 | 13.97 秒 | 2.30 秒 |
| 変更後 (`CGO_ENABLED=0`) | 11.90 秒 | 7.66 秒 | 2.94 秒 |

変更後の所要時間は変更前より短い。

「## 背景」の表の `CGO_ENABLED=0` のリンク 5.3 秒は、`backend/` で `CGO_ENABLED=0 go build -o <一時ファイル> ./cmd/thief` を直接実行した値であり、上の表とは手順が異なる。
上の表は完了条件の手順 (バイナリを削除してから `mise run backend:install`) の値で、mise の起動と、`go install` がリンクの前に行う依存パッケージの更新の要否の確認を含む。
バイナリが最新でリンクが走らない状態の `mise run backend:install` は real 4.76 秒 (2026-09-29 に同じ Mac で計測) で、この分が差の大半を占める。
同じ手順で測り直すと real 10.26 秒で、残りの差は実行ごとのばらつきの範囲にある。
