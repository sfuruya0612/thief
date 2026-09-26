# Go ツールチェイン更新後の go: 管理ツールの再ビルドを lint 前に自動化する

Created: 2026-08-24
Model: Claude Fable 5
Completed: 2026-08-24

## 背景

docs/issues/TODO.md の次の項目に由来する。issue 0144 のレビューで「完了条件を超える恒久的な仕組み作り」として却下され TODO へ送られたもので、経緯は docs/issues/closed/0144-bug-govulncheck-stdlib-toolchain-outdated.md の「却下した指摘」にある。

> mise.toml の go バージョンを上げるたびに go: 管理下のツール (staticcheck 等) を自動で再ビルドする仕組みを追加したい (issue 0144 のスコープ外として送り)
>     旧ツールチェインでビルドされたバイナリが残ると、go vet は通るのに staticcheck だけ「file requires newer Go version」で失敗する原因不明な事象になる

`mise.toml` の `[tools]` は Go 本体 (`go = "1.26.6"`、mise.toml:5) に加えて、`go:` 形式で 4 つの Go 製ツールを管理している (mise.toml:8-11)。

- `go:github.com/vektra/mockery/v2`
- `go:golang.org/x/tools/cmd/goimports`
- `go:honnef.co/go/tools/cmd/staticcheck`
- `go:golang.org/x/vuln/cmd/govulncheck`

これらのツールはインストール時点でアクティブな Go ツールチェインでビルドされ、その後 Go 本体のバージョンを上げても再ビルドされない。issue 0144 の実装時には、ツールチェインを 1.25.12 から 1.26.6 に切り替えた直後に `mise run backend:lint` が失敗し、切り分けの結果、旧ツールチェインでビルドされた `staticcheck` が `file requires newer Go version go1.26 (application built with go1.25)` を出していたことが判明した (staticcheck はソース解析時に自身がビルドされた Go バージョンを言語バージョンの上限として扱うため、`go.mod` の `go 1.26` を読み込めなかった)。このとき `go vet` は通るため、タスク出力上は何が失敗したのか分かりにくい「原因不明な事象」に見える。復旧は `mise install --force "go:...@latest"` による手動再ビルドだった (詳細は 0144 の「実装時の乖離の記録」)。

現状、この再ビルドを自動で行う仕組みは無い。

- `mise.toml` に `[hooks]` セクションは無い。
- `mise run setup` (mise.toml:14-16) は `backend:setup` (`go mod download && go mod tidy`) と `frontend:setup` (`npm install`) に依存するだけで、go: ツールの導入や再ビルドを明示的に行うタスクは無い。
- `mise run backend:lint` (mise.toml:55-62) は `go vet` → `staticcheck` → `govulncheck` を直接実行するだけで、ツールのビルドバージョンを検査しない。

再ビルドの要否は機械的に判定できる。`mise which <tool>` でツールバイナリの実体パスを解決でき (例: `staticcheck` → `~/.local/share/mise/installs/go-honnef-co-go-tools-cmd-staticcheck/latest/bin/staticcheck`)、`go version -m <binary>` でそのバイナリをビルドした Go バージョンを取得できる (現状は 4 ツールとも go1.26.6 で、アクティブな Go 本体と一致していることを確認済み)。

## 対応方針

判定と再ビルドを行う mise タスク `backend:tools` を追加し、`backend:lint` の `depends` に載せる。タスクの内容は次のとおり。

1. `go:` 管理下の各ツールについて、`mise which <tool>` で実体パスを解決し、`go version -m` の出力からビルド時 Go バージョンを取り出す。ツールが未導入で `mise which` が失敗した場合 (初回セットアップ直後など) は、そのツールを再ビルド対象とせずスキップし、導入自体は mise の `[tools]` 宣言による自動プロビジョニングに任せる。`go version -m` の出力がパースできない場合は、スキップ扱いにせず明示的にエラーで止める (`go version -m` の出力形式は安定した契約ではないため、フォーマット変更時に誤判定のまま進ませない)。
2. アクティブな Go 本体のバージョン (`go env GOVERSION`) と比較する。
3. 不一致のツールだけを `mise install --force "go:<module>@latest"` で再ビルドする。一致していれば再ビルドしない。
4. ツールごとに再ビルドしたのかスキップしたのかを標準出力に 1 行ずつ出す。「再ビルドが実行されないこと」をタスクの出力で確認できるようにするためである。

この方式を採る理由は、Go 本体のバージョンがどの経路で変わっても (mise.toml の編集、`mise install go@x.y.z` の直接実行)、ツールを使う時点で不整合を検出して自己修復できることと、判定が `go version -m` の出力という決定的な事実に基づくことである。

採らなかった案は次のとおり。

- 判定を省き、毎回 `mise install --force` で全ツールを再インストールする案。lint のたびに全ツールのビルドとモジュール取得が走るため採らない。
- `[hooks].postinstall` で再ビルドを起動する案。mise 公式ドキュメント (hooks ページ) によれば、`postinstall` フックはツールのインストール後に発火し、インストールされたツールの一覧を環境変数 `MISE_INSTALLED_TOOLS` (JSON) で受け取れ、何もインストールされない `mise install` でも発火する。この仕様の下では、フック内の `mise install --force` が再帰的に postinstall を発火させうるため停止条件の管理が要る。いずれの挙動もローカルの mise (2026.6.12) では検証していない。使う時点で検査する lint 前段のタスクの方が、発火経路に依存せず単純である。
- AGENTS.md に手動の再ビルド手順を明文化するだけの案。TODO の要望は「自動で再ビルドする仕組み」であり、手動手順は実行し忘れの余地を残すため、0144 で実際に起きた「原因不明な失敗に見える」問題の再発を防ぎきれない。
- `backend:lint` の run 冒頭に判定処理をインライン展開する案。単体で実行できるタスクに切り出す方が、ツールチェイン更新の直後に lint を経由せず再ビルドだけを行う使い方ができ、テスト (手動での動作確認) もしやすい。

追加の API 呼び出しや権限は不要である。ネットワークアクセスは再ビルドが必要になったときの `mise install --force` (モジュールの取得) のみで発生する。

## 未確定論点

- ビルドバージョン不一致状態の再現手段。検証には「旧ツールチェインでビルドされたツールバイナリが残っている」状態を作る必要があるが、その手段 (例: `mise exec go@1.25.12` を経由した `mise install --force` の実行) は起票時点で試していない。`mise install --force` は現環境のツールバイナリを置き換えるため、確認だけのために実行すると復旧作業が要る。再現手段は実装時に確定し、issue の close 時に実施した手順を記録する。
  - 解消済み (実装時に確定): `GOTOOLCHAIN=go1.25.12` を付けた `mise install --force "go:honnef.co/go/tools/cmd/staticcheck@latest"` は、latest が解決する v0.8.1 の go.mod が `go >= 1.26.0` を要求するためビルドできず失敗する (mise のインストールは失敗し、導入済みの latest ディレクトリは変更されない)。確定した再現手段は、`GOTOOLCHAIN=go1.25.12 GOBIN="$(dirname "$(mise which staticcheck)")" go install honnef.co/go/tools/cmd/staticcheck@v0.7.0` で mise の latest ディレクトリのバイナリを直接上書きする方法 (v0.7.0 の go.mod は `go 1.25.0` を要求するため go1.25.12 でビルドできる)。この状態の staticcheck は `internal error in importing "internal/byteorder" (unsupported version: 2)` のような標準ライブラリの読み込みエラーで失敗することを確認した。復旧は本 issue で追加した `backend:tools` の再ビルドが行うため手動復旧は不要である。
- 初回セットアップ直後 (go: ツール未導入) の実行順序。mise が `[tools]` 宣言のツールをタスク実行前に自動導入するタイミングと、`backend:tools` 内の `mise which` の実行タイミングの前後関係は未検証である。実装時に go: ツール未導入相当の状態から `mise run backend:lint` を実行して検証し、未導入ツールのスキップ (対応方針の 1) で足りるかを確認する。
  - 解消済み (実装時に実測): `mise uninstall "go:github.com/vektra/mockery/v2"` で未導入状態を作って `mise run backend:tools` を実行したところ、mise (2026.6.12) はタスク実行前に `[tools]` 宣言のツールを自動導入し (タスク出力より先に `[1/3] install` が出る)、タスク内の `mise which` は導入済みのバイナリを解決した。したがって通常の実行経路で未導入スキップは発火しないが、自動導入を無効化した環境 (`MISE_TASK_AUTO_INSTALL` 等) 向けの防御としてスキップ分岐は残す。
- 並行実行の安全性。mise はタスクを既定で最大 4 並列で実行するため、`backend:tools` の `mise install --force` がインストールディレクトリを書き換えている最中に、別タスクや別ターミナルの多重起動が同じバイナリを参照しうる。mise 自体のインストール処理がプロセス間で安全か (ロック機構の有無) を実装時に確認し、必要なら再ビルドを直列化する。
  - 解消済み (実装時に調査): mise のソース (main ブランチ、ローカルの 2026.6.12 と同系) では、ツールインストールの入口 `install_version` (src/backend/mod.rs) が最初に `install_state::lock_tool_version` (src/toolset/install_state.rs) を呼び、`fslock` クレートによる OS のプロセス間ファイルロックをツールバージョン単位で取得してからアンインストールと再インストールを行う。同一ツールへの並行インストールは mise 自身が直列化するため、タスク側の追加の直列化は不要と判断した。再ビルド中に別タスクが旧バイナリを実行する残余の競合窓は、再ビルドが発生するのは不一致 (旧バイナリのままでは失敗する) 状態のみであり、従来の手動 `mise install --force` 復旧と同じ窓であるため許容する。
- 対象ツール一覧の持ち方。タスク内に 4 ツールの一覧をハードコードすると、`mise.toml` の `[tools]` に go: ツールを追加したときに揃え忘れる。`mise ls --current --json` 等の出力から `go:` プレフィックスのツールを動的に列挙できれば重複管理を避けられるが、JSON のパースに jq 等の追加依存が要るかを含め、実装時に出力形式を確認して決める。動的列挙が追加依存なしに書けない場合は、タスク内の一覧に固定し、`mise.toml` の `[tools]` 側へ「go: ツールを追加したら backend:tools の一覧にも追加する」というコメントを置く。
  - 解消済み (実装時に確認): JSON を使わない `mise ls --current` の出力が「ツール名 バージョン ソース 要求バージョン」の空白区切りであることを確認し、awk で第 1 カラムが `^go:` の行を絞る動的列挙を採用した (jq 等の追加依存なし)。ハードコード案は不要になった。なお `mise ls --current` はグローバル設定 (`~/.tool-versions` や `~/.config/mise/config.toml`) 由来のツールも列挙するため、そこに go: ツールがあれば判定対象に含まれるが、現環境の go: ツールは本リポジトリの `mise.toml` の 4 つのみである (含まれても検査と再ビルドの意味は同じで実害は無い)。バイナリ名はモジュールパスの末尾セグメントから導出し、末尾がメジャーバージョン (mockery の `/v2` 等) の場合はその 1 つ手前のセグメントを使う。

## 完了条件

- `go:` 管理ツールのビルド時 Go バージョンをアクティブな Go 本体と比較し、不一致のツールだけを `mise install --force` で再ビルドする mise タスク `backend:tools` が追加され、`mise run backend:tools` で単体実行できる。
- `mise run backend:lint` が `backend:tools` に依存し、lint 本体 (`go vet` / `staticcheck` / `govulncheck`) の実行前に判定が走る。
- 全ツールのバージョンが一致している状態で `mise run backend:tools` を実行すると、全ツールについてスキップした旨の出力が出て、再ビルドした旨の出力が出ない (タスクはツールごとに再ビルド/スキップを標準出力に出す)。
- ビルドバージョン不一致を再現した状態から `mise run backend:lint` を実行すると、不一致のツールが再ビルドされ、lint が「file requires newer Go version」で失敗しない。不一致状態の再現手段は未確定論点に書いたとおり実装時に確定し、issue の close 時に実施した手順を記録する。
- `mise install` のプロセス間の安全性 (ロック機構の有無) を実装の最初に調査し、調査結果と、直列化が必要と判明した場合の対応を issue に記録する。
- 追加するタスクの判定対象に、`mise.toml` の `[tools]` で `go:` 形式で管理される全ツール (現時点で mockery、goimports、staticcheck、govulncheck の 4 つ) が含まれている。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0144-bug-govulncheck-stdlib-toolchain-outdated.md: 本 issue の分割元。0144 は手動の `mise install --force` で復旧し、その恒久化が本 issue である。
- docs/issues/0145-chore-document-toolchain-sync-rule.md: 同じく 0144 のレビューから TODO へ送られた項目。0145 は mise.toml と go.mod のバージョンを揃える運用ルールの明文化を扱い、本 issue の再ビルド自動化とは独立に実装、検証、close できる。本 issue の判定は「ツールのビルド時 Go バージョンとアクティブな Go 本体の一致」であり、0145 のルール (mise.toml と go.mod の 2 箇所の一致) の遵守を検知するものではない。

## 解決方法

- `mise.toml` に `backend:tools` タスクを追加した。`mise ls --current` の出力から `go:` プレフィックスのツールを動的に列挙し、各ツールのバイナリを `mise which` で解決して `go version -m` の 1 行目の末尾フィールドからビルド時 Go バージョンを取り出し、`go env GOVERSION` と比較して不一致のツールだけを `mise install --force "go:<モジュール>@latest"` で再ビルドする。ツールごとに skip / rebuild を標準出力に 1 行ずつ出す。未導入で `mise which` が失敗したツールはスキップし、`mise ls` や `go version -m` 自体の失敗は set -e で即停止し、`go version -m` の 1 行目がパースできない場合はエラーメッセージを出して停止する。
- `backend:lint` の `depends` に `backend:tools` を追加し、lint 本体 (`go vet` / `staticcheck` / `govulncheck`) の前に判定が走るようにした。
- 未確定論点 4 件は「## 未確定論点」の各項目に追記したとおり解消した。
- 検証 (mise タスクのシェルスクリプトには自動テスト基盤が無いため、完了条件の各行は手動検証で確認した):
  - 全ツール一致状態の `mise run backend:tools`: 4 ツールすべて skip のみ (出力例: `skip mockery: built with go1.26.6 (matches active toolchain)`)。この出力で `go:github.com/vektra/mockery/v2` からバイナリ名 `mockery` への導出も確認できる。
  - 不一致の再現と自己修復: staticcheck (v0.7.0) と goimports (x/tools v0.49.0) を `GOTOOLCHAIN=go1.25.12 GOBIN="$(dirname "$(mise which <バイナリ>)")" go install <モジュール>@<バージョン>` で go1.25.12 ビルドに上書きし、`mise run backend:tools` の 1 回の実行で 2 ツールが `rebuild <バイナリ>: built with go1.25.12, active toolchain is go1.26.6` と出て再ビルドされ、両方 go1.26.6 ビルドに復元されることを確認した。staticcheck 単体の不一致からの `mise run backend:lint` でも、`backend:tools` の rebuild 出力の後に go vet / staticcheck / govulncheck がビルドバージョン起因のエラーなしで通過し、staticcheck が go1.26.6 ビルド (latest 解決で v0.7.0 から v0.8.1 に更新) に復元された。
  - 異常系 (タスクの run スクリプト本体を抽出し、PATH に置いたシムで実行): `mise which mockery` だけ失敗するシムでは `skip mockery: not installed` が出てループが続行した。`go version -m` が不正な 1 行を返す go のシムでは `error: cannot parse built Go version of goimports from ...` が標準エラー出力に出て終了コード 1 で停止した。`mise ls` を終了コード 3 で失敗させるシムでは検査を行わずに終了コード 3 で停止した (silent no-op にならない)。中身がテキストの壊れたバイナリを `mise which` が返すシムでは `go version -m` の代入が失敗して終了コード 1 で停止した (この場合 go1.26.6 の `go version -m` は診断を出力せず終了コードのみで失敗を示す)。空白を含むディレクトリに置いたバイナリでは、go1.26.6 ビルドは skip、go1.25.12 ビルドは rebuild と正しく判定された。go: ツールが 0 件の `mise ls` 出力では何もせず終了コード 0 で終了した。
  - `mise run check` が通過し、ベースライン (frontend 73 ファイル 732 テスト、backend 16 パッケージ) からの新たな失敗は無い (実装前後とも失敗ゼロ)。
- レビューで却下した指摘: (1) 観点 1 の「`mise which` にも `</dev/null` を付ける」は、`mise which` が標準入力を読まない照会コマンドであり、指摘自体も実害なしと明記していたため却下した (その後の書き換えで while ループ自体を for ループに変え、標準入力の共有問題は構造的に解消した)。(2) 観点 3 の「バイナリ名導出のバグと真の未導入を区別できない」は、skip 行がツール名付きで毎回出力され目視検知でき、タスク文脈では mise の自動導入により真の未導入がほぼ起きないことを実測済みのため、対応方針 1 の設計のまま許容した。(3) 観点 3 の「@latest 固定が pin されたツールの追加で食い違う」は、要求バージョンの動的解決は現状全ツール latest 指定の下では投機的一般化になるため実装せず、前提を明示するコメントの追加にとどめた。
