# mise.toml の Go バージョンと go.mod の toolchain 行を揃える運用ルールを AGENTS.md に明文化する

Created: 2026-08-24
Model: Claude Fable 5
Completed: 2026-08-24

## 背景

docs/issues/TODO.md の次の項目に由来する。issue 0144 のレビューで「完了条件を超える恒久的な仕組み作り」として却下され TODO へ送られたもので、経緯は docs/issues/closed/0144-bug-govulncheck-stdlib-toolchain-outdated.md の「却下した指摘」にある。

> mise.toml の [tools].go と backend/go.mod の toolchain 行を、将来のパッチ更新時に揃える運用ルールを明文化したい (issue 0144 のスコープ外として送り)
>     揃え忘れると mise 未経由のシェル (CI 等) で GOTOOLCHAIN=auto が古いパッチバージョンを解決し、govulncheck の脆弱性検知が再発しうる

Go ツールチェインのバージョンは現在 2 箇所で宣言されている。

- `mise.toml` の `[tools]` の `go = "1.26.6"` (mise.toml:5)。mise を経由するシェルとタスク実行が使うバージョンを決める。
- `backend/go.mod` の `toolchain go1.26.6` (backend/go.mod:5)。mise を経由しないシェルで `GOTOOLCHAIN=auto` (Go の既定値) が解決するバージョンを決める。この行は issue 0144 の対応 (コミット ef08f97) で追加された。

この 2 箇所は独立に編集できるため、パッチ更新時に片方だけ上げる揃え忘れが起こりうる。`mise.toml` だけを上げた場合の症状は issue 0144 の多観点レビューで実測されている: `toolchain` 行が無い状態 (揃っていない状態と同じ症状) で `mise activate` を経由しないシェルから `govulncheck ./...` を実行すると、`GOTOOLCHAIN=auto` が `go 1.26` ディレクティブに対して古いパッチバージョン (実測で go1.26.0) を解決し、修正済みのはずの標準ライブラリ脆弱性が再検出された (0144 の記録では 16 件以上)。逆に `toolchain` 行だけを上げた場合は、Go の `GOTOOLCHAIN=auto` が `toolchain` 行のバージョンを下限として自動取得するため、mise 経由のシェルでも `go` コマンドは `toolchain` 行のバージョンで動き、`mise.toml` の pin が実際に使われるバージョンを表さなくなる。

一方、この運用ルールはどこにも書かれていない。`AGENTS.md` の「backend (Go)」節は `go.mod` の `go` ディレクティブの方針 (「`go 1.26` を基本とし、リポジトリ既存値が古い場合のみそれに揃える」) には触れているが、`toolchain` 行と `mise.toml` の同期には触れていない (AGENTS.md 内に toolchain / GOTOOLCHAIN の記述は無い)。リポジトリに運用手順を書く他のドキュメントは存在しない (`docs/` ディレクトリは無く、`CLAUDE.md` は AGENTS.md への委譲のみ)。

なお、現時点でこのリポジトリに CI ワークフローは存在しない (`.github/workflows` は無い)。TODO 文中の「CI 等」が現状で指すのは mise 未経由のシェル全般 (`mise activate` を設定していないシェル、`mise` 未設定の他端末、将来導入されうる CI) である。唯一の自動化は `.pre-commit-config.yaml` の `mise run fmt` / `mise run lint` / `mise run test` であり、これらは mise 経由で実行される。

## 対応方針

`AGENTS.md` の「backend (Go)」節に、Go ツールチェインのパッチ更新の運用ルールを追記する。記載する内容は次のとおり。

- `mise.toml` の `[tools]` の `go` と `backend/go.mod` の `toolchain` 行は、同一バージョンへ同時に更新する。
- 揃え忘れたときの症状: mise を経由しないシェルでは `GOTOOLCHAIN=auto` が `go.mod` の `toolchain` 行を基準にバージョンを解決するため、`mise.toml` だけを上げても govulncheck が旧ツールチェインの標準ライブラリを検査し、修正済みの脆弱性が再検出される (issue 0144 で実測)。
- 更新後の確認手順: `mise install` でツールチェインを導入し、`mise run check` を通す。

追記は「backend (Go)」節の「backend ビルドと CI」に置く。ビルドに使うツールチェインの取り扱いはビルド節の関心事であり、「基本方針」はバージョン系列 (1.26 系) の方針だけを述べる場所として保つ。

併せて、`mise.toml` の `go = "1.26.6"` の行に「backend/go.mod の toolchain 行と同一バージョンに揃える (AGENTS.md 参照)」という趣旨のコメントを付け、`backend/go.mod` の `toolchain` 行にも同趣旨のコメントを付ける。編集する人が最初に開くのはルールではなく値のあるファイルなので、値の近傍に相互参照を置く。要望本体は AGENTS.md への明文化であり、この 2 箇所のコメントは要望に無い付随的な補足である。

採らなかった案は次のとおり。

- 両ファイルの値の一致を機械的に検証するチェック (lint タスクへの比較スクリプト追加など) は、本 issue では行わない。TODO の要望は運用ルールの明文化であり、チェックの実装形 (比較スクリプトの言語、どのタスクに載せるか、`toolchain` 行が意図的に先行するケースの扱い) は別の設計判断を要する。必要になれば別 issue として起票する。
- `mise.toml` 内コメントのみで済ませる案は採らない。コメントは値の近傍の注意書きとしては有効だが、なぜ揃えるのかの根拠 (GOTOOLCHAIN=auto の解決順序と issue 0144 の実測) を書き切る場所ではなく、リポジトリの規約の正式な置き場は AGENTS.md である。

変更対象はドキュメントと設定ファイルのコメントのみで、ビルド成果物の挙動は変わらない。

## 完了条件

- `AGENTS.md` の「backend (Go)」節の「backend ビルドと CI」に、次の 3 点を含む運用ルールが追記されている: (1) `mise.toml` の `[tools].go` と `backend/go.mod` の `toolchain` 行を同一バージョンへ同時に更新すること、(2) 揃え忘れたときに mise 未経由のシェルで govulncheck の検知が再発するという根拠、(3) 更新後に `mise install` と `mise run check` で確認する手順。
- `mise.toml` の `go` の行の近傍に、`backend/go.mod` の `toolchain` 行と揃える旨のコメントがある。
- `backend/go.mod` の `toolchain` 行の近傍に、`mise.toml` の `[tools].go` と揃える旨のコメントがある。
- 両ファイルの値の一致を機械的に検証する仕組みの追加は、本 issue の対象外である。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0144-bug-govulncheck-stdlib-toolchain-outdated.md: 本 issue の分割元。0144 は `toolchain` 行の追加と脆弱性解消そのものを行い、本 issue は将来の更新時の運用ルールの明文化を行う。
- docs/issues/0146-chore-rebuild-go-tools-on-toolchain-update.md: 同じく 0144 のレビューから TODO へ送られた項目。0146 はツールチェイン更新時の go: 管理ツールの再ビルドの自動化を扱い、本 issue が明文化する「2 箇所のバージョンを揃える」ルールとは独立に実装、検証、close できる。なお 0146 の仕組みは本ルールの遵守 (2 箇所の一致) を検知するものではなく、揃え忘れを機械的に検出する手段は両 issue を実装しても存在しない (本 issue の「採らなかった案」のとおり、検証の仕組みは必要になれば別途起票する)。

## 解決方法

- `AGENTS.md` の「backend ビルドと CI」に、`mise.toml` の `[tools].go` と `backend/go.mod` の `toolchain` 行を同一バージョンへ同時に更新する運用ルールを追記した。揃え忘れの症状 (mise 未経由のシェルで `GOTOOLCHAIN=auto` が `toolchain` 行を基準に解決し govulncheck の検知が再発する。issue 0144 で実測) と、`toolchain` 行だけを上げた場合の症状、更新後の確認手順 (`mise install` と `mise run check`) を含む。
- `mise.toml` の `go = "1.26.6"` の直前の行に、`backend/go.mod` の `toolchain` 行と揃える旨のコメントを追加した。
- `backend/go.mod` の `toolchain go1.26.6` の直前の行に、`mise.toml` の `[tools].go` と揃える旨のコメントを追加した。
- 両ファイルの値の一致を機械的に検証する仕組みは追加していない (完了条件の対象外)。
- 検証: `mise run check` が通過し、ベースラインからの新たな失敗は無い (実装前後とも失敗ゼロ)。
- レビューで却下した指摘: 観点 5 の「AGENTS.md が gitignore 対象の issue 0144 を実測根拠として参照しており、clone 先で参照を検証できない」は却下した。コミット対象ファイルから issue を参照する慣習は既にあり (AGENTS.md の多言語対応の項の docs/issues/closed/0066 参照、CHANGES.md の issue 0131 / 0135 への本文参照)、本 issue の対応方針が「(issue 0144 で実測)」の文言を明示的に指定しているうえ、実測の中身 (症状と再発条件) は追記文の本文に要約済みで参照が無くても検証できるため。
