# mise.toml が pin する Go ツールチェインの標準ライブラリに govulncheck が 7 件の脆弱性を報告する

Created: 2026-08-15
Model: Claude Sonnet 5
Completed: 2026-08-15

## 症状

`mise run backend:lint` (`mise run check` 経由でも同様) が `govulncheck ./...` の実行で次のとおり失敗し、`ERROR task failed` で終了する。

```
Your code is affected by 7 vulnerabilities from the Go standard library.
This scan also found 0 vulnerabilities in packages you import and 1
vulnerability in modules you require, but your code doesn't appear to call these
vulnerabilities.
Use '-show verbose' for more details.
```

`mise run check` は `fmt` → `lint` → `test` の順に依存するため、この失敗により `mise run check` 全体が非 0 exit で終了する。

## 再現手順

1. `cd backend && govulncheck ./...` を実行する。
2. 期待される結果: 到達可能な脆弱性 0 件で終了する。
3. 実際の結果: 標準ライブラリ由来の 7 件の脆弱性が到達可能として報告され、非 0 exit で終了する。次の 7 件である。

   | Vulnerability | 内容 | Found in | Fixed in |
   | --- | --- | --- | --- |
   | GO-2026-6218 | net/url の resolvePath の二次計算量 | net/url@go1.25.12 | net/url@go1.25.13 |
   | GO-2026-6091 | html/template の JavaScript regexp context tracking | html/template@go1.25.12 | html/template@go1.25.13 |
   | GO-2026-6090 | crypto/tls のハンドシェイク後メッセージ受理上限 | crypto/tls@go1.25.12 | crypto/tls@go1.25.13 |
   | GO-2026-6089 | net/http の非暗号化 HTTP/2 チェック時の ReadHeaderTimeout | net/http@go1.25.12 | net/http@go1.25.13 |
   | GO-2026-6088 | encoding/xml のデコード時の再帰深度ガード | encoding/xml@go1.25.12 | encoding/xml@go1.25.13 |
   | GO-2026-5972 | encoding/asn1 の最大再帰深度 | encoding/asn1@go1.25.12 | encoding/asn1@go1.25.13 |
   | GO-2026-5026 | golang.org/x/net/idna 由来、ASCII のみの Punycode ラベルの拒否漏れ (net/http 経由) | net/http@go1.25.12 | net/http@go1.25.13 |

   到達経路の例 (govulncheck の出力から抜粋):
   - GO-2026-6218: `internal/tidb/client.go:61:24: tidb.Client.get calls http.Client.Do, which eventually calls url.URL.Parse`
   - GO-2026-6091: `internal/cli/server.go:42:37: cli.newServerCmd calls http.Server.ListenAndServe, which eventually calls template.Template.Execute`
   - GO-2026-6090: `internal/aws/cloudwatchlogs.go:201:2: aws.StartLiveTail calls cloudwatchlogs.StartLiveTailEventStream.Close, which eventually calls tls.Conn.Handshake`
   - GO-2026-6089: `internal/cli/server.go:42:37: cli.newServerCmd calls http.Server.ListenAndServe`
   - GO-2026-6088: `internal/aws/s3_object.go:87:30: aws.GetS3Object calls s3.Client.GetObject, which eventually calls xml.Decoder.Decode`
   - GO-2026-5972: `internal/aws/cloudwatchlogs.go:201:2: aws.StartLiveTail calls cloudwatchlogs.StartLiveTailEventStream.Close, which eventually calls asn1.Unmarshal`
   - GO-2026-5026: `internal/gcp/gcs.go:137:2: gcp.PutObject calls storage.Client.Close, which eventually calls http.Client.CloseIdleConnections`

同時に報告される「1 vulnerability in modules you require」は `-show verbose` で確認すると GO-2026-5932 (`golang.org/x/crypto/openpgp` が unmaintained、Fixed in: N/A) であり、これは docs/issues/closed/0108 で既に把握済みでスコープ外とされているものと同一である。本 issue はこの 1 件を対象にしない。

## 原因

`mise.toml` の `[tools]` セクション (`mise.toml:5`) が `go = "1.25.12"` を pin している。

```
[tools]
go = "1.25.12"
```

この時点で導入される Go ツールチェインの標準ライブラリに、go1.25.13 で修正済みの上記 7 件の脆弱性が含まれる。`govulncheck` はこの pin されたツールチェインの標準ライブラリのシンボルへの到達を検出するため、`go.mod` の依存 (`go get` で更新できる direct / indirect モジュール) を変更しても解消しない。`mise ls-remote go` で `1.25.13` が導入可能なバージョンとして存在することを確認済みである。

## 修正方針

`mise.toml` の `[tools]` の `go` を、`mise ls-remote go` で導入可能な Go 1.26 系の最新版 `1.26.6` に更新する。7 件の脆弱性すべての Fixed in (最小要求 `1.25.13`) を上回り、かつリポジトリ `AGENTS.md` が定める「対象言語/バージョン: Go 1.26 系を前提とする (現行最新は 1.26.x)」の方針にも一致する。

`backend/go.mod` の `go` ディレクティブを現在の `1.25.0` から `1.26` に更新する。`AGENTS.md` は「`go.mod` の `go` ディレクティブは `go 1.26` を基本とし、リポジトリ既存値が古い場合のみそれに揃える」と定めており、現在値は 1.26 系より古いため揃える。

`mise install go@1.26.6` でツールチェインを導入し、`mise run check` を実行して govulncheck の標準ライブラリ由来の指摘が 0 件になることを確認する。

### 実装時の乖離の記録

ツールチェインを 1.26.6 に切り替えた直後、`mise run backend:lint` が `go vet ./...` の失敗として報告される事象が発生した。調査の結果、実際に失敗していたのは `go vet` ではなく後続の `staticcheck ./...` であり (`mise run` のタスクスクリプトへ一時的にステップ区切りのデバッグ出力を挿入し、`go vet` の終了コードが `0` であること、`staticcheck` の実行中に `file requires newer Go version go1.26 (application built with go1.25)` が発生することを確認して切り分けた)、原因は `mise.toml` の `[tools]` の `"go:honnef.co/go/tools/cmd/staticcheck" = "latest"` が go1.26.6 への切り替え以前 (旧ツールチェインの下) にビルドされたバイナリのままだったことである。`staticcheck` はソースを解析する際に自身がビルドされた Go バージョンを上限として言語バージョンを解釈するため、`go.mod` の `go 1.26` を読み込めなかった。

`mise install --force "go:honnef.co/go/tools/cmd/staticcheck@latest"` で go1.26.6 の下で再ビルドすることで解消した。同時に `mise install --force` で `go:golang.org/x/tools/cmd/goimports@latest`、`go:golang.org/x/vuln/cmd/govulncheck@latest`、`go:github.com/vektra/mockery/v2@latest` も再ビルドした (`mise.toml` が `go:` 形式で管理する Go 製ツールは全て、ツールチェイン切り替え後に再ビルドが必要という同種の再発を防ぐため)。

この対応は方式そのものの変更ではなく、「`mise install go@1.26.6` でツールチェインを導入し `mise run check` を通す」という既存の修正方針を実現するための実装詳細である。

### レビュー指摘による追加対応

多観点レビューの観点 1 (完了条件の充足) で、`backend/go.mod` に `toolchain` 行が無いため、`mise activate` を経由しないシェル (新しいターミナルタブ、CI、`mise` 未設定の他端末など) で `PATH` 上に旧バージョンの `go` 実体 (`go1.25.12` など) が先に解決される場合、`GOTOOLCHAIN=auto` が `go 1.26` ディレクティブに対して `1.26.6` 未満の別バージョン (実測で `go1.26.0`) を自動取得してしまい、`govulncheck ./...` が `mise.toml` の pin と無関係に別バージョンで実行され、標準ライブラリ由来の脆弱性が 0 件にならない (実測で GO-2026-5856、GO-2026-5039 等 16 件以上を検出) ことが判明した。

この指摘を受け、`backend/go.mod` に `toolchain go1.26.6` を追記した。`env -i` で `PATH` の先頭に旧 `go1.25.12` を置いた最小環境でも `go version` が `go: downloading go1.26.6` の後に `go1.26.6` を正しく解決することを確認し、`mise run backend:lint` と `mise run backend:build` が引き続き問題なく通過することも確認した。方式を保ったままの実装詳細の追加であり、方針そのものの変更ではない。

### 却下した指摘

観点 3 (堅牢性) のレビューで次の 2 点が指摘されたが、いずれも本 issue の完了条件 (govulncheck の標準ライブラリ由来脆弱性を 0 件にする) を超える恒久的な運用の仕組み作りであり、設計判断を要するため本 issue のスコープには含めない。

- `mise.toml` の `[tools].go` と `backend/go.mod` の `toolchain` 行を将来のパッチ更新時に揃える運用ルールの明文化。
- `mise.toml` の go バージョンを上げるたびに `go:` 管理下のツール (staticcheck 等) を自動で再ビルドする仕組みの追加。

いずれも `docs/issues/TODO.md` に将来の改善項目として追記し、必要になった時点で別途 issue 化するかを判断する。

## 完了条件

- `mise.toml` の `[tools]` の `go` を `1.25.13` 以上 (7 件すべてが Fixed in として示す最小バージョン以上) に更新する。
- 更新後のバージョンで `mise install go` によりツールチェインを導入し、`cd backend && govulncheck ./...` を実行して、標準ライブラリ由来の到達可能な脆弱性が 0 件になることを確認する。
- GO-2026-5932 (`golang.org/x/crypto/openpgp`、Fixed in: N/A) は本 issue の対象外のまま残る。
- `backend/go.mod` の `go` ディレクティブ (現在 `go 1.25.0`) は、更新後のツールチェインバージョンと矛盾しないことを確認する。ディレクティブ自体の変更が必要かはツールチェイン更新後に判断する。
- `mise run check` が通る。

## 解決方法

`mise.toml` の `[tools].go` を `1.25.12` から `1.26.6` に更新し、`mise install go@1.26.6` でツールチェインを導入した。`backend/go.mod` の `go` ディレクティブを `1.25.0` から `1.26` に更新した (AGENTS.md が定める既定値に合わせる)。

実装中、ツールチェイン切り替え直後に `mise run backend:lint` が失敗する事象が発生した。一時的にタスクスクリプトへステップ区切りのデバッグ出力を挿入して切り分けた結果、実際に失敗していたのは `go vet` ではなく後続の `staticcheck ./...` であり、原因は `mise.toml` が `go:` 形式で管理する `staticcheck` (`honnef.co/go/tools/cmd/staticcheck`) が go1.26.6 への切り替え以前の旧ツールチェインでビルドされたバイナリのまま残っていたことだった。`mise install --force` で `staticcheck`、`goimports`、`govulncheck`、`mockery` を go1.26.6 の下で再ビルドして解消した (詳細は本文「### 実装時の乖離の記録」)。

多観点レビューの観点 1 (完了条件の充足) で、`backend/go.mod` に `toolchain` 行が無いため、`mise activate` を経由しないシェルでは `GOTOOLCHAIN=auto` が `mise.toml` の pin と無関係な別バージョン (実測で `go1.26.0`) を自動取得し、govulncheck の脆弱性 0 件が再現しない場合があるという指摘を受けた。`backend/go.mod` に `toolchain go1.26.6` を追加し、`env -i` で旧 `go1.25.12` を PATH 先頭に置いた最小環境でも `go1.26.6` に正しく解決されることを確認して解消した (詳細は本文「### レビュー指摘による追加対応」)。

観点 3 (堅牢性) で、mise.toml と go.mod の toolchain 行を将来揃える運用ルールの明文化、および `go:` 管理下ツールの自動再ビルドの仕組みが無いことが指摘されたが、いずれも本 issue の完了条件を超える恒久的な仕組み作りであり設計判断を要するため、本 issue のスコープには含めず却下し、`docs/issues/TODO.md` に将来の改善項目として追記した (詳細は本文「### 却下した指摘」)。

観点 5 (回帰と整合) で、CHANGES.md エントリの種別案 `[FIX]` が、過去の govulncheck 対応による依存更新 (klauspost/compress、golang.org/x/text、google.golang.org/grpc) がいずれも `### misc` に記録されている前例と矛盾するという指摘を受け、`### misc` に分類し直した。

検証は次のとおり行った。

- `cd backend && govulncheck ./...` (mise 経由) で標準ライブラリ由来の脆弱性が 0 件になり、GO-2026-5932 (`golang.org/x/crypto/openpgp`、docs/issues/closed/0108 で既知、対象外) のみが残ることを確認した。
- `mise run check` (fmt + lint + test、backend + frontend) が全体を通過することを確認した (`go test -race -cover ./...` は全 16 パッケージ ok、race 検出なし。`mise run frontend:test` は 73 ファイル 732 テスト全通過)。
- `cd backend && go build ./cmd/...` と `go mod verify` が問題なく通ることを確認した。
- `backend/go.mod` の `go` ディレクティブ (`go 1.26`) は導入したツールチェイン (`1.26.6`) と矛盾しないことを確認した。
