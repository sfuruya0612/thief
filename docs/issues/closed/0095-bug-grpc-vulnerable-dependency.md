# grpc の既知脆弱性 GO-2026-6061 により mise run check が失敗する

Created: 2026-07-28
Completed: 2026-07-30
Model: Claude Sonnet 5

## 症状

リポジトリルートで `mise run check` を実行すると、`backend:lint` の `govulncheck` 呼び出しが脆弱性を検出し、`ERROR task failed` で `mise run check` 全体が exit code 3 で終了する。

```
[backend:lint] === Symbol Results ===
[backend:lint]
[backend:lint] Vulnerability #1: GO-2026-6061
[backend:lint]     Vulnerabilities in the xDS RBAC authorization engine and the HTTP/2
[backend:lint]     transport server implementation in google.golang.org/grpc
[backend:lint]   More info: https://pkg.go.dev/vuln/GO-2026-6061
[backend:lint]   Module: google.golang.org/grpc
[backend:lint]     Found in: google.golang.org/grpc@v1.82.0
[backend:lint]     Fixed in: google.golang.org/grpc@v1.82.1
[backend:lint] Your code is affected by 1 vulnerability from 1 module.
[backend:lint] ERROR task failed
```

`go test -race -cover ./...` と `npm run test` (vitest) 自体は全パッケージ・全テストファイルが通過しており、この失敗は `govulncheck` によるものだけである。

## 再現手順

1. リポジトリルートで `mise run check` を実行する。
2. `backend:lint` の `govulncheck` ステップで `Vulnerability #1: GO-2026-6061` が報告される。
3. `mise run check` が exit code 3 で終了する。期待される結果は exit code 0 での全体通過だが、実際は govulncheck の検出により失敗する。

## 原因

`backend/go.mod` が `google.golang.org/grpc v1.82.0` に固定されており、この版は GO-2026-6061 (xDS RBAC authorization engine および HTTP/2 transport server implementation の脆弱性) の対象になっている。修正版は `google.golang.org/grpc v1.82.1` 以降。

govulncheck のコールパス解析では、次の呼び出し経路が脆弱性のあるコードパスへ実際に到達していると報告されている。

- `internal/gcp/logging_tail.go` の `TailLogEntries` (37 行) / `runTailLogEntries` (55, 64 行) が `loggingpb.loggingServiceV2TailLogEntriesClient` の `Send`/`Recv` を経由して `transport.ClientStream`/`transport.http2Client` の脆弱なメソッドを呼び出す。
- `internal/aws/cloudwatchlogs.go` の `StartLiveTail` (193 行) が `cloudwatchlogs.StartLiveTailEventStream.Close` を経由して `transport.NewHTTP2Client` を呼び出す。

このリポジトリのコード自体に脆弱性はないが、依存版が固定されているため govulncheck が失敗を報告する。

### 訂正

対応時に依存グラフを検証した結果、上記の記述に 2 点の誤りが判明した。

1. AWS 側の経路は存在しない。`go list -deps github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs` と `go list -deps ./internal/aws` のいずれにも `google.golang.org/grpc` は 1 件も含まれない。`google.golang.org/grpc/internal/transport` は Go の internal パッケージ可視性ルールにより `google.golang.org/grpc/...` 配下からのみ import 可能であり、AWS SDK for Go v2 から `transport.NewHTTP2Client` を呼び出すことは構造的に不可能である。AWS SDK for Go v2 の `StartLiveTail` は `vnd.amazon.eventstream` を用いた AWS 独自のストリーミング実装であり grpc-go を使わない。脆弱なコードパスに到達するのは `internal/gcp/logging_tail.go` の Cloud Logging Live Tail の経路のみである。
2. `google.golang.org/grpc` は間接依存ではなく direct 依存である。`internal/api/errors.go` が `google.golang.org/grpc/codes` と `google.golang.org/grpc/status` を import しており、`backend/go.mod` の require ブロックに `// indirect` なしで記載されている。

## 完了条件

- `backend/go.mod` の `google.golang.org/grpc` (直接指定があれば直接、間接依存のみなら `go get` で) を `v1.82.1` 以上に更新する。
- `go mod tidy -v` (`mise run backend:tidy`) を実行し、`go.sum` の整合を取る。
- `mise run check` が全て通過する (govulncheck が GO-2026-6061 を報告しないことを含む)。

## 解決方法

### 変更内容

`backend/go.mod` の `google.golang.org/grpc` を `v1.82.0` から `v1.82.1` に更新した。差分は `backend/go.mod` 1 行、`backend/go.sum` 2 行 (h1 ハッシュと go.mod ハッシュ)、`CHANGES.md` の `### misc` エントリ 1 件のみで、Go のコードは変更していない。

`go.sum` の変更が grpc の 2 行に閉じており、他の依存の追随更新は発生していない。`/go.mod` のハッシュは v1.82.0 と v1.82.1 で同一であり、grpc 自身の go.mod がパッチ間で変わっていないことを示す。

### 検証結果

`mise run check` は fmt が作業ツリーを書き換えるため、各構成タスクを読み取り専用の形式で個別に実行した。

- `go build ./...` / `go vet ./...` : いずれも成功。
- `go test -race -cover ./...` : backend 全 14 パッケージが ok、FAIL なし。
- `npm run test` (vitest) : 71 ファイル / 638 テストが pass。`npm run lint` は exit 0。
- `govulncheck ./...` : `No vulnerabilities found`、exit 0。GO-2026-6061 は verbose 出力にも現れない。
- `go mod tidy -diff` : 出力なし (tidy 済み)。`go mod verify` : `all modules verified`。
- `gofmt -l .` / `goimports -l .` : 出力なし。

`mise run check` は本 issue の対応により exit 0 で全通過する状態になった。

### v1.82.1 の差分の確認

モジュールキャッシュ上の v1.82.0 と v1.82.1 を比較したところ、変更は `internal/envconfig`、`internal/transport` (`controlbuf.go` / `http2_client.go` / `http2_server.go`)、`internal/xds/rbac` (`matchers.go`)、`version.go` に限られる。いずれも grpc モジュール内の internal パッケージであり、公開 API に変更はない。このリポジトリが直接 import する `grpc/codes` と `grpc/status` には差分がない。

`controlbuf.go` の変更はスループット制限のデフォルト値を固定値 50 から `envconfig.ControlBufferThrottleLimit` (デフォルト 100) に置き換えるもので、Cloud Logging Live Tail のような長時間ストリーミングにとって制限が緩む方向であり退行しない。

### 対象外とした事項

`govulncheck` は本対応後も 2 件をモジュールレベルで報告するが、いずれもコードから到達せず exit code に影響しない。本 issue のスコープ (GO-2026-6061 の解消) 外として扱った。

- GO-2026-5932 (`golang.org/x/crypto/openpgp`) : `Fixed in: N/A` であり、バージョン更新では解消できない。
- GO-2026-5841 (`github.com/klauspost/compress/s2`) : `v1.18.7` で修正済み。追随更新は別 issue で扱う。

### 補足

レビューで指摘を受け、`CHANGES.md` のエントリから AWS 側の到達経路の記述を削除した。訂正の根拠は「## 原因」の「### 訂正」節に記載した。

検証中に `staticcheck` を `mise exec` を介さず直接実行すると compile エラーを出して exit 1 になる現象を観測したが、これはリポジトリの問題ではない。`PATH` 上の `~/go/bin/staticcheck` (2024.1.1、go1.22.4 ビルド) が go1.25 のモジュールを解析できないだけであり、`mise run backend:lint` が解決する mise 管理版 (2026.1 / v0.7.0) は backend 全パッケージを正常に解析して指摘ゼロで exit 0 を返す。`staticcheck -checks=all` でデフォルト無効の ST/QF 系の指摘が出力されることを確認し、解析が実行されていることを裏付けた。
