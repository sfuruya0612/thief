# pre-commit の backend チェックを短縮する (govulncheck の分離と go test の自動 vet の停止)

Created: 2026-09-29
Model: Claude Fable 5.1
Completed: 2026-09-29

## 背景

`docs/issues/TODO.md` の「pre-commit の backend チェック (lint / test) が遅いので、govulncheck を専用タスクに分けて go test の自動 vet を止めたい」に対応する。

`.pre-commit-config.yaml` は全コミットで `mise run fmt` / `mise run lint` / `mise run test` を実行する (`always_run: true`)。
backend 側は `backend:lint` (`go vet ./...`、staticcheck、govulncheck) と `backend:test` (`go test -race -cover ./...`) である。
2026-09-29 に warm キャッシュで計測した所要時間は次のとおり (11 コアの Mac、Go 1.26.6)。

| 処理 | 所要時間 |
| --- | --- |
| `go vet ./...` | 3.7 秒 |
| staticcheck | 5.4 秒 |
| govulncheck | 12 秒 (CPU 33 秒) |
| `go test -race -cover ./...` (テスト結果がキャッシュ済み) | 7.7 秒 |
| 同 `-vet=off` | 4.4 秒 |
| `go test -race -cover -count=1 ./...` (テストを実行) | 25 秒 |

govulncheck はモジュールの依存グラフ全体を解析し、脆弱性データベース (`-db` の既定値 `https://vuln.go.dev`) に接続する。
コード変更の無いコミットでも毎回 12 秒掛かり、`backend:lint` の 21 秒のうち半分以上を占める。
govulncheck は脆弱性データベースとの照合に加えて、呼び出しグラフの到達可能性で報告を絞る (`golang.org/x/vuln` の `cmd/govulncheck/doc.go`)。
このため検出結果は、依存かツールチェインを更新したとき (issue 0144) と脆弱性データベースに項目が増えたときのほか、脆弱なシンボルへの到達可能性が変わるコード変更 (require 済みのモジュールの別パッケージを新たに import する、脆弱な関数を新たに呼ぶ) でも変わりうる。
ただし通常のコード変更のコミットで変わることは少なく、変わった場合も `mise run check` で検出できるため、コミットのたびに検査する必要は無い。

`go test` はテスト対象のパッケージに対して vet の一部 (atomic、bool、buildtags、directive、errorsas、ifaceassert、nilfunc、printf、stringintconv、tests。`go help test` 参照) を自動で実行する。
`backend:lint` の `go vet ./...` は既定の全 analyzer を実行するため、この自動 vet の上位互換である。
pre-commit と `mise run check` はどちらも lint と test の両方を実行するため、自動 vet は同じ検査の二重実行になっている。
自動 vet の所要時間は warm で 3 秒、cold では `-debug-actiongraph` の集計で vet の facts 計算に CPU 150 秒を使う (`go test -race -cover ./...` の cold の CPU 435 秒のうち)。

## 対応方針

- `mise.toml` に `backend:vuln` タスク (`description` を持つ。`"$(mise which govulncheck)" ./...`、`dir = "backend"`、`depends = ["backend:tools"]`) を新設し、`backend:lint` の `run` から govulncheck の行を外す。
- ルートの `check` タスクの `depends` に `backend:vuln` を加え、`mise run check` (PR 提出前の最終確認) では従来どおり govulncheck が走るようにする。
  `check` の `description` (`PR 提出前の最終確認 (fmt + lint + test)`) を `fmt + lint + vuln + test` に直す。
  pre-commit フックが呼ぶ 3 タスク (`mise run fmt` / `mise run lint` / `mise run test`) のうち govulncheck を含むのは `lint` だけなので、`.pre-commit-config.yaml` を変えなくても govulncheck はフックから外れ、`mise run check` と `mise run backend:vuln` でだけ走る。
- `backend:test` の `run` を `go test -race -cover -vet=off ./...` にし、コメントで `backend:lint` の `go vet ./...` が自動 vet の上位互換であることを書く。
  `mise run backend:test` を単独で実行したときは vet が走らないため、vet の結果が要るときは `backend:lint` を併用する。
- `AGENTS.md` の「タスクランナー」の表に `backend:vuln` の行を追加し、`check` / `backend:lint` / `backend:test` の行を新しい内容に合わせる。
  「pre-commit」節の「まとめて確認するなら `mise run check`」に、`check` はフックが実行しない `backend:vuln` (govulncheck) も実行することを添える。
- `docs/prd/thief.md` の品質ゲートの 2 箇所 (成功指標の表の「fmt、lint、test がすべて成功する」と「運用」の「`mise run check` が fmt、lint、test をまとめて実行する」) を `fmt、lint、vuln、test` に直す。
- ADR 0022 (`docs/adr/0022-quality-gates.md`) の決定のうち「`mise run check` が fmt、lint、test を順に実行し、pre-commit フックが同じ検査をコミットの前に実行する」と「backend の Lint は `go vet`、`staticcheck`、`govulncheck`」の 2 つは、この変更で覆る。
  ADR 0001 の運用 (判断を覆すときは元の ADR を書き換えず新しい ADR を書き、元の `Status` を `Superseded by NNNN` にする) に従い、`docs/adr/0031-vuln-check-in-check-task-not-precommit.md` を新設し、ADR 0022 の `Status` を `Superseded by 0031` に変える。
  ADR の番号は実装時に `docs/adr/README.md` の表の最大番号 + 1 を採る (起票時点の空き番号は 0031。issue 0200 も ADR の新設を完了条件に持つため、実装順で番号が変わりうる)。
  この issue で 0031 と書く箇所はその番号に読み替える。
  ADR 0031 には、状況 (本 issue の計測値)、決定 (govulncheck は `backend:vuln` と `check` で実行し pre-commit では実行しない。`go test` の自動 vet を止め、vet は `backend:lint` に一本化する。ADR 0022 の他の決定 (golangci-lint を使わない、テストは標準 `testing`、frontend は `vmThreads`、Go と `toolchain` の同時更新) はそのまま引き継ぐ)、検討した代替案 (下の採らなかった案)、結果 (フックを通るコミットは govulncheck を経ない。到達可能性が変わるコード変更の検出は `mise run check` に依存する) を書き、`docs/adr/README.md` の表に 0031 の行を足して 0022 の Status を更新する。

採らなかった案。

- govulncheck を廃止する: 依存とツールチェインの脆弱性検査は残す (issue 0144 で標準ライブラリの脆弱性を検出した実績がある)。
  実行の場を `check` と専用タスクに移すだけにする。
- 自動 vet を残す: `backend:lint` と同じ検査の二重実行で、warm 3 秒と cold の CPU 150 秒を毎回払う。
- `backend:lint` の `go vet ./...` を外して自動 vet に寄せる: 自動 vet は analyzer の一部だけを実行するため、検査の範囲が狭くなる。
- govulncheck を `.pre-commit-config.yaml` の別フック (`stages` で `pre-push` などに限定) にする: フックの定義と mise のタスクの二重管理になり、`mise run check` との対応も崩れる。

## 完了条件

- `mise.toml` に `backend:vuln` があり、`backend:lint` の `run` に govulncheck が無く、`check` の `depends` に `backend:vuln` が含まれ、`backend:test` の `run` が `go test -race -cover -vet=off ./...` である。
- `mise run backend:vuln` が govulncheck を実行する (出力に govulncheck の結果が含まれる)。
- `mise run check` の出力に govulncheck の実行が含まれる。
- `mise run backend:lint` と `mise run backend:test` (テスト結果がキャッシュ済みの状態) の所要時間を変更前後で各 1 回計測して「## 解決方法」に記録し、どちらも変更後の所要時間が変更前より短い。
  変更前の計測は `mise.toml` を変える前に行う。
- `.pre-commit-config.yaml` を変えない。
- `backend:vuln` に `description` があり、`check` の `description` が `fmt + lint + vuln + test` を含む。
- `AGENTS.md` の「タスクランナー」の表と「pre-commit」節を対応方針のとおり更新する。
- `docs/prd/thief.md` の品質ゲートの 2 箇所を対応方針のとおり更新する。
- `docs/adr/0031-vuln-check-in-check-task-not-precommit.md` があり、ADR 0001 の形式 (ヘッダの `Created` / `Model` / `Status` / `Decided`、状況、決定、検討した代替案、結果、根拠資料) を満たし、`docs/adr/0022-quality-gates.md` の `Status` が `Superseded by 0031` で、`docs/adr/README.md` の表に 0031 の行があり 0022 の行の Status が `Superseded by 0031` である。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追加する。
- `mise run check` が通る。

## 関連

- issue 0144: govulncheck が標準ライブラリの脆弱性を検出したときの記録。
  検査は `check` に残す。
- ADR 0022: 品質ゲートの決定。
  本 issue の ADR 0031 が supersede する。
- issue 0215: `CGO_ENABLED=0` への固定。
  `mise.toml` の `[env]` を変える。

## 解決方法

`mise.toml` に `backend:vuln` タスク (`description = "backend 脆弱性検査 (govulncheck)"`、`dir = "backend"`、`depends = ["backend:tools"]`、`"$(mise which govulncheck)" ./...`) を `backend:lint` の直後に新設し、`backend:lint` の `run` から govulncheck の行を外した。
`check` の `depends` を `["fmt", "lint", "backend:vuln", "test"]` にし、`description` を `PR 提出前の最終確認 (fmt + lint + vuln + test)` にした。
`backend:test` の `run` を `go test -race -cover -vet=off ./...` にし、`backend:lint` の `go vet ./...` が自動 vet の上位互換であることと、単独実行では vet が走らないことをコメントに書いた。
`.pre-commit-config.yaml` は変えていない (フックが呼ぶ `mise run lint` に govulncheck が含まれなくなる)。
`AGENTS.md` の「タスクランナー」の表に `backend:vuln` の行を足し、`check` / `backend:lint` / `backend:test` の行を直し、「pre-commit」節に `check` はフックが実行しない `backend:vuln` も実行することを添えた。
`docs/prd/thief.md` の品質ゲートの 2 箇所を `fmt、lint、vuln、test` にした。
ADR 0022 の決定のうち覆る 2 つを `docs/adr/0031-vuln-check-in-check-task-not-precommit.md` に記録し、ADR 0022 の `Status` を `Superseded by 0031` にし、`docs/adr/README.md` の表を更新した。

確認した結果 (2026-09-29、Go 1.26.6、darwin)。

- `mise run backend:vuln` が `[backend:vuln] $ "$(mise which govulncheck)" ./...` に続けて `No vulnerabilities found.` を出す。
- `mise run check` の出力に `[backend:vuln]` の govulncheck の実行が含まれる。
- フックが呼ぶ `mise run lint` の出力に govulncheck の実行は無い (`[backend:tools] skip govulncheck: built with go1.26.6 (matches active toolchain)` はツールのビルド時バージョンの検査であり、govulncheck の実行ではない)。
- `git diff -- .pre-commit-config.yaml` が空。
- `mise run backend:test` の出力の 1 行目が `go test -race -cover -vet=off ./...` で、18 パッケージすべて成功した。
- `mise run check` が通った。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追記した (種別タグ無しは `### misc` の既存の書式に合わせた)。

所要時間 (他の処理を走らせていない状態で `/usr/bin/time -p` の real。各タスクを続けて 2 回実行し、テスト結果がキャッシュ済みの 2 回目どうしを主な比較とする。変更前は `mise.toml` を変える前の HEAD 5202e60 で計測)。

| タスク | 変更前 | 変更後 |
| --- | --- | --- |
| `mise run backend:lint` | 21.86 秒 (2 回目 21.28 秒) | 14.58 秒 (2 回目 14.52 秒) |
| `mise run backend:test` (キャッシュ済み) | 16.02 秒 (2 回目 7.86 秒) | 5.53 秒 (2 回目 4.99 秒) |

どちらも変更後の所要時間が変更前より短い。
2 回目どうしの比較では `backend:lint` が 21.28 秒から 14.52 秒、`backend:test` が 7.86 秒から 4.99 秒に短くなり、issue 本文の計測 (自動 vet あり 7.7 秒、`-vet=off` 4.4 秒) と整合する。
変更前の計測は issue 0215 の `CGO_ENABLED=0` の固定より前の状態 (HEAD 5202e60) で取ったため、1 回目の `backend:test` の差 (16.02 秒から 5.53 秒) には 0215 の効果 (cgo 無しの再コンパイルと再リンク) が含まれ、この issue 単独の効果を表さない。
