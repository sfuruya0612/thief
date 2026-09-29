# 0031. govulncheck は check タスクと backend:vuln で実行し、pre-commit フックでは実行しない

Created: 2026-09-29
Model: Claude Fable 5.1
Status: Accepted
Decided: 2026-09-29

## 状況

ADR 0022 は、`mise run check` が fmt、lint、test を実行し、pre-commit フックが同じ検査をコミットの前に実行すると決めた。
backend の Lint は `go vet`、`staticcheck`、`govulncheck` の 3 つで、`backend:lint` が順に実行していた。
issue 0216 で pre-commit フックの backend の検査の所要時間を計測した (2026-09-29、warm キャッシュ、11 コアの Mac、Go 1.26.6)。

| 処理 | 所要時間 |
| --- | --- |
| `go vet ./...` | 3.7 秒 |
| staticcheck | 5.4 秒 |
| govulncheck | 12 秒 (CPU 33 秒) |
| `go test -race -cover ./...` (テスト結果がキャッシュ済み) | 7.7 秒 |
| 同 `-vet=off` | 4.4 秒 |

govulncheck は `backend:lint` の 21 秒のうち半分以上を占める。
govulncheck の検出結果は、依存かツールチェインの更新、脆弱性データベースの更新、脆弱なシンボルへの到達可能性を変えるコード変更で変わる。
通常のコード変更のコミットで変わることは少ない。
`go test` はテスト対象のパッケージに対して vet の一部 (atomic、bool、buildtags、directive、errorsas、ifaceassert、nilfunc、printf、stringintconv、tests) を自動で実行する。
`backend:lint` の `go vet ./...` は既定の全 analyzer を実行するため、この自動 vet の上位互換であり、pre-commit と `mise run check` では同じ検査の二重実行になっていた。

## 決定

- govulncheck は `backend:vuln` タスクに分け、`check` タスクの `depends` に加える。
  `mise run check` (PR 提出前の最終確認) と `mise run backend:vuln` で実行し、pre-commit フックが呼ぶ `mise run lint` では実行しない。
- `backend:test` は `go test -race -cover -vet=off ./...` とし、`go test` の自動 vet を止める。
  vet は `backend:lint` の `go vet ./...` に一本化する。
- ADR 0022 の他の決定 (golangci-lint を使わない、backend のテストは標準の `testing` でモックは `go.uber.org/mock` か手書き、frontend のテストは vitest の `vmThreads`、`mise.toml` の Go と `backend/go.mod` の `toolchain` 行の同時更新) はそのまま引き継ぐ。

## 検討した代替案

- govulncheck を廃止する。
  依存とツールチェインの脆弱性検査は残す (issue 0144 で標準ライブラリ由来の脆弱性を検出した実績がある)。
  実行の場を `check` と専用タスクに移すだけにした。
- `go test` の自動 vet を残す。
  `backend:lint` と同じ検査の二重実行で、warm で 3 秒、cold で vet の facts 計算に CPU 150 秒を毎回払う。
- `backend:lint` の `go vet ./...` を外して自動 vet に寄せる。
  自動 vet は analyzer の一部だけを実行するため、検査の範囲が狭くなる。
- govulncheck を `.pre-commit-config.yaml` の別フック (`stages` で `pre-push` などに限定) にする。
  フックの定義と mise のタスクの二重管理になり、`mise run check` との対応も崩れる。

## 結果

- pre-commit フックを通るコミットは govulncheck を経ない。
  到達可能性が変わるコード変更の検出は `mise run check` に依存する。
- `mise run backend:test` を単独で実行したときは vet が走らない。
  vet の結果が要るときは `backend:lint` を併用する。
- 変更後の所要時間 (issue 0216 の計測、warm キャッシュ、各 2 回計測した 2 回目の値) は `mise run backend:lint` が 14.52 秒 (変更前 21.28 秒)、テスト結果がキャッシュ済みの `mise run backend:test` が 4.99 秒 (変更前 7.86 秒) である。
  変更前は issue 0215 の `CGO_ENABLED=0` の固定より前の状態 (5202e60) で計測した。
  「状況」の表は issue 0216 の起票時にコマンド単体で計測した値で、この項の値は mise の起動と `backend:tools` の検査を含むタスク全体の値である。
- CI が無いこと、品質ゲートが各開発者の端末の pre-commit フックに依存することは ADR 0022 の結果のとおりで変わらない。

## 根拠資料

- `docs/issues/closed/0216`、`docs/issues/closed/0144`
- ADR 0022
- `mise.toml`、`.pre-commit-config.yaml`
- Go 1.26.6 の `cmd/go/internal/test/test.go` (自動 vet の説明と対象の一覧)、`golang.org/x/vuln` の `cmd/govulncheck/doc.go` (到達可能性による絞り込み)
