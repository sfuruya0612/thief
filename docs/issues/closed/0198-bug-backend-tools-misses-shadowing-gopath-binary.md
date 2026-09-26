# backend:tools が GOPATH/bin の旧 staticcheck による mise 管理ツールの隠蔽を検知しない

Created: 2026-09-24
Model: Claude Fable 5.1
Completed: 2026-09-26

## 症状

`mise run check` (および `mise run backend:lint`) が `staticcheck ./...` で次のエラーを出して失敗する。`go vet` と `go test -race -cover ./...` は通過する。

```
[backend:lint] -: internal error in importing "cmp" (unsupported version: 2); please report an issue (compile)
[backend:lint] -: internal error in importing "internal/byteorder" (unsupported version: 2); please report an issue (compile)
...
[backend:lint] -: module requires at least go1.26.0, but Staticcheck was built with go1.22.4 (compile)
[backend:lint] ERROR task failed
```

`backend:lint` が依存する `backend:tools` は「旧ツールチェインでビルドされた go: ツールを検知して再ビルドする」ためのタスクだが (`mise.toml` の `[tasks."backend:tools"]`、docs/issues/closed/0146)、この状況では `skip staticcheck: built with go1.26.6 (matches active toolchain)` と判定して何もせず、直後の `staticcheck ./...` が失敗する。

## 再現手順

1. mise 管理の staticcheck が現行の Go でビルドされていることを確認する。`mise which staticcheck` が `~/.local/share/mise/installs/go-honnef-co-go-tools-cmd-staticcheck/latest/bin/staticcheck` を返し、`go version -m "$(mise which staticcheck)"` の 1 行目が `go1.26.6` を示す。
2. 旧 Go でビルドされた staticcheck を `$(go env GOPATH)/bin/staticcheck` に置く (再現に使った実物は 2024-08-26 に `go install` された staticcheck 2024.1.1 (0.5.1)、`go version -m` は `go1.22.4`)。
3. リポジトリルートで `mise run backend:lint` を実行する。
4. 期待する結果: `backend:tools` が隠蔽を検知して失敗するか、mise 管理の staticcheck が実行されて lint が通過する。
5. 実際の結果: `backend:tools` は `skip staticcheck: built with go1.26.6 (matches active toolchain)` を出し、続く `staticcheck ./...` が症状のエラーで失敗する。
6. `mise exec -- which staticcheck` は `$(go env GOPATH)/bin/staticcheck` を返し、`mise exec -- sh -c 'echo $PATH'` の先頭は `~/go/bin`、mise 管理の staticcheck のディレクトリはその後ろにある。
7. `$(go env GOPATH)/bin/staticcheck` を退避 (改名) すると `mise exec -- which staticcheck` は mise 管理の実体を返し、`mise run check` が通過する (2026-09-24 に実測)。

## 原因

発見時に分かっている範囲は次のとおり。

- mise の go プラグインは `GOPATH/bin` (`~/go/bin`) を `[tools]` で宣言した go: ツールの bin ディレクトリより前に PATH へ載せる。`mise exec -- sh -c 'echo $PATH'` の 1 番目が `~/go/bin`、2 番目が `~/.local/share/mise/installs/go/1.26.6/bin`、go: ツールの bin はその後に並ぶ。このため `~/go/bin` に同名のバイナリがあると、mise 管理のツールがすべて隠蔽される (staticcheck のほか goimports も `~/go/bin/goimports` が使われている)。`GOBIN` や `GOPATH` の環境変数を与えても mise が `GOPATH` を上書きするため順序は変わらない (`go_set_gobin` は `false`)。
- `backend:tools` は `mise which "$bin"` でツールの実体を解決するが、`mise which` は mise が管理するインストール先を返すため、PATH 上で実際に実行される `~/go/bin/staticcheck` を見ない。検査対象と実行対象が食い違い、隠蔽が検知されない。
- `backend:lint` は `staticcheck ./...` を PATH 解決で実行する。

## 修正方針

implement-issues の Step 4 で確定した (2026-09-25、ユーザー確認済み)。完了条件の 2 案のうち「`backend:lint` が mise 管理の staticcheck を実行して通過する」を採る。

- `mise.toml` で go: 管理ツールを実行する 3 タスクを、PATH 解決に代えて `mise which <bin>` で解決した mise 管理の実体を実行する形にする。
  - `backend:lint`: `staticcheck ./...` を `"$(mise which staticcheck)" ./...` に、`govulncheck ./...` を `"$(mise which govulncheck)" ./...` にする。
  - `backend:fmt`: `goimports -w .` を `"$(mise which goimports)" -w .` にする。
  - `backend:mocks`: `mockery` を `"$(mise which mockery)"` にする。
- `backend:tools` は変更しない。同タスクが検査する `mise which` の解決先と、上記 3 タスクが実行する実体が一致するので、「## 原因」に書いた検査対象と実行対象の食い違いが消える。
- `mise which` が失敗する (未導入) 場合は `"$(...)"` が空文字列になりシェルが command not found で失敗する。mise は `[tools]` で宣言したツールをタスク実行前に自動導入するため、この経路は通常は通らない。
- 新しい API や権限は要らない。外部依存も増えない。
- 却下: `backend:tools` で `command -v "$bin"` と `mise which "$bin"` の解決先を比較し、食い違えばエラーで失敗させる案。隠蔽を自己修復しないため、`~/go/bin` に同名バイナリを置いた開発者の `mise run check` が失敗するようになる (このマシンでは `~/go/bin/goimports` (go1.21.4 ビルド) の隠蔽により今すぐ失敗する)。
- 却下: mise の `go.set_gobin` を有効にして PATH から `GOPATH/bin` を外す案。`mise.toml` の `[settings]` は全開発者に効き、`go install` の出力先が変わる。
- 却下: `[env]` の `_.path` で mise 管理ツールの bin ディレクトリを前置する案。ツールのインストール先パスを `mise.toml` に書き込むことになり、`[tools]` の宣言と二重管理になる。

## 完了条件

- 再現手順 2 の状態で `mise run backend:lint` を実行したとき、`backend:tools` が隠蔽を検知して原因 (実行されるパスと mise 管理のパスの食い違い) を英語のエラーメッセージで示して失敗するか、または `backend:lint` が mise 管理の staticcheck を実行して通過する。どちらを採るかは実装時の Step 4 で方針として確定する。
- 隠蔽が無い状態 (再現手順 7) では `backend:tools` の挙動が現状と変わらない (docs/issues/closed/0146 の完了条件を満たしたまま)。
- `mise run check` が通過する。

## 見送りの記録

- 日付: 2026-09-25
- 見送った Step: implement-issues の Step 4 (並列実装の実装エージェントの起動)
- 理由: 環境要因。opencode のランナー (`issue-implementer`、モデル `cf-fireworks/deepseek-v4p1-flash`) が起動直後に `APIError: Unauthorized: invalid_token` (ゲートウェイの Cloudflare Access が `Missing or invalid access token` を返す HTTP 401) で失敗し、同じプロンプトでの 1 回の再実行も同じ理由で失敗した (`status=error`、`tool_calls=0`)。implement-issues は実装エージェントの失敗を親や Agent ツールで代行しないと定めているため、実装に入らなかった。方針は「## 修正方針」に確定済みで、修正前の再現 (再現手順 2 の状態で `mise run backend:lint` が `module requires at least go1.23, but Staticcheck was built with go1.22.4` で exit 1、`backend:tools` は `skip staticcheck: built with go1.26.6 (matches active toolchain)` を出力) は 2026-09-25 に実測した。実装とテストの変更は無く、破棄したものは無い。
- 再開に必要な条件: opencode が使う AI ゲートウェイのアクセストークンを更新し、ランナーが `status=ok` で応答を返せること。再開時は「## 修正方針」のとおり実装する (方針の再確認は不要)。

- 日付: 2026-09-25
- 見送った Step: implement-issues の Step 4 (並列実装の実装エージェントの起動)
- 理由: 環境要因。opencode のランナー (`issue-implementer`、モデル `cf-fireworks/deepseek-v4p1-flash`) が起動直後に `APIError: Unauthorized: invalid_token` で失敗し、同じプロンプトでの 1 回の再実行も同じ理由で失敗した (`status=error`、`steps=0`、`tool_calls=0`)。同じランナーは同日 22:46 から 23:05 にかけてレビューエージェントを `status=ok` で実行できていたため、その後にゲートウェイのアクセストークンが失効したものと判断した。implement-issues は実装エージェントの失敗を親や Agent ツールで代行しないと定めているため、実装に入らなかった。worktree に変更は無く、破棄したものは無い。
- 再開に必要な条件: opencode が使う AI ゲートウェイのアクセストークンを更新し、ランナーが `status=ok` で応答を返せること。再開時は「## 修正方針」のとおり実装する (方針の再確認は不要)。

## 解決方法

### 変更したファイルとシンボル

- `mise.toml`: go: 管理ツールを実行する 3 タスクを、PATH 解決から `mise which <bin>` で解決した実体の実行に変えた。
  - `[tasks."backend:lint"]`: `staticcheck ./...` を `"$(mise which staticcheck)" ./...` に、`govulncheck ./...` を `"$(mise which govulncheck)" ./...` にした。
  - `[tasks."backend:fmt"]`: `goimports -w .` を `"$(mise which goimports)" -w .` にした。`gofmt` は Go 本体に付属し `~/go/bin` の隠蔽対象にならないため変えていない。
  - `[tasks."backend:mocks"]`: `run = "mockery"` を `run = "\"$(mise which mockery)\""` にした。
- `[tasks."backend:tools"]` は「## 修正方針」のとおり変更していない。同タスクが `mise which` で検査する実体と、上記 3 タスクが実行する実体が一致し、「## 原因」の「検査対象と実行対象の食い違い」が消える。

### 完了条件の検証

このリポジトリには `mise.toml` のタスク定義を検証する自動テストが無いため、以下はすべて 2026-09-26 に手動で実測した。実行したコマンドをそのまま記す。

- 再現手順 2 の状態で `backend:lint` が mise 管理の staticcheck を実行して通過すること: 退避してあった旧 staticcheck (`~/go/bin/staticcheck.stale-2024.1.1`、`go version -m` で `go1.22.4`、2024-08-26 に `go install`) を `cp ~/go/bin/staticcheck.stale-2024.1.1 ~/go/bin/staticcheck` で複製して隠蔽を再現し、次を実測した。
  - `mise exec -- sh -c 'command -v staticcheck'` (リポジトリルート) が `~/go/bin/staticcheck` を返し、`go version -m` がそのバイナリを `go1.22.4` と報告した。隠蔽が成立している。
  - 修正前の形: `backend/` で `mise exec -- sh -c 'staticcheck ./...'` は終了コード 1。出力は 11 行で、内訳は `-: internal error in importing "<pkg>" (unsupported version: 2); please report an issue (compile)` が 6 件 (`<pkg>` は `cmp`、`internal/byteorder`、`internal/cpu`、`internal/goarch`、`math/bits`、`unicode/utf8`)、`-: module requires at least <ver>, but Staticcheck was built with go1.22.4 (compile)` が 5 件 (`<ver>` は `go1.23`、`go1.24`、`go1.25`、`go1.25.0`、`go1.26.0`) である。
  - 報告された 5 通りのうち 4 通りは `backend/go.mod` の `go 1.26.0` と異なる値である。メッセージが `module requires at least <ver>` の形であるとおり、staticcheck は解析時に読み込むモジュールの `go` ディレクティブを個別に見ており、報告値は解析対象モジュールの `go.mod` の値 1 つには揃わない。`go list -deps -f '{{if .Module}}{{.Module.GoVersion}}{{end}}' ./...` で数えると、読み込み対象のモジュールには `go1.22.4` より新しい `go` ディレクティブが 10 通り (`1.23`、`1.23.0`、`1.24`、`1.24.0`、`1.24.1`、`1.24.2`、`1.24.6`、`1.25`、`1.25.0`、`1.26.0`) あり、そのうち 5 通りが報告された。報告される値がこの 5 通りに絞られる機構までは特定していない。この issue の「## 症状」が引用する `go1.26.0` の行と「## 見送りの記録」が引用する `go1.23` の行は、いずれもこの 11 行のうちの 1 行であり、どちらの引用も誤りではない。
  - 修正後の形: `backend/` で `mise exec -- sh -c '"$(mise which staticcheck)" ./...'` は出力なし、終了コード 0。
  - 修正後の `mise.toml` で `mise run backend:lint` (リポジトリルート、タスクは `dir = "backend"`) を実行し、隠蔽があるまま終了コード 0 で通過した。
  - 確認後、複製した `~/go/bin/staticcheck` を `rm` で削除し、退避の状態 (`staticcheck.stale-2024.1.1` だけが残る) に戻した。退避ファイル自体は削除していない。
- staticcheck の隠蔽のみを解いた状態で `backend:tools` の挙動が現状と変わらないこと (docs/issues/closed/0146 の完了条件): 上記の複製を削除したうえで `mise run backend:tools` を実行し、`skip mockery` / `skip goimports` / `skip govulncheck` / `skip staticcheck` の 4 行がいずれも `built with go1.26.6 (matches active toolchain)` で出て終了コード 0 になった。判定ロジックには手を入れていない。`~/go/bin/goimports` による隠蔽はこの時点でも残っている (「### 補足」を参照)。
- `backend:mocks` の実行: 変更した 3 タスクのうち `backend:mocks` だけは `mise run check` の経路に入らないため、リポジトリルートで `mise run backend:mocks` を単独実行した。修正後の形は `mise which mockery` が返す `~/.local/share/mise/installs/go-github-com-vektra-mockery-v2/latest/bin/mockery` を実行する。mockery は `~/go/bin` に同名バイナリが無く、修正前の PATH 解決 (`mise exec -- sh -c 'command -v mockery'`) も同じ実体を返すため、本修正でこのタスクが実行するバイナリは変わらない。このタスクは修正の前後どちらの形でも `couldn't read any config file` の後に `Use --name to specify the name of the interface or --all for all interfaces found` を出して終了コード 1 で失敗する。本修正とは独立した既存の不具合であり、docs/issues/0199 として登録した (「### 補足」を参照)。
- `mise which` が名前を解決できない場合の挙動: `[tools]` に宣言の無い名前で `mise which no-such-tool-xyz` を実行すると、標準出力は空、標準エラー出力に `no-such-tool-xyz is not a mise bin. Perhaps you need to install it first.` を出して終了コード 1 になる。タスクと同じ構文 `sh -c '"$(mise which no-such-tool-xyz)" ./...'` は空文字列を実行することになり、`sh: : command not found` で終了コード 127 になる。ここで実測したのは単一コマンドの挙動であり、タスクのスクリプト全体がこの失敗で中断することまでは実測していない (この issue の「## 症状」で `staticcheck` の失敗が `[backend:lint] ERROR task failed` になっていることから、同じく非ゼロで終わると判断した)。なお `[tools]` に宣言済みのツールは mise がタスク実行前に自動導入するため、この経路には実運用では到達しない。
- `mise run check` が通過すること: 統合後の作業ツリーで実行して終了コード 0。frontend は 113 ファイル / 1234 テスト通過、eslint は 0 errors / 9 warnings (warnings は本 issue の変更前から存在する)、backend は 18 パッケージすべて ok (`-race`)、`go vet` / `staticcheck` / `govulncheck` すべて通過 (0 vulnerabilities)。ベースラインからの新たな失敗は無い。

### 方針からの乖離

方針の方式を変える乖離は無い。「## 修正方針」が挙げた 3 タスク 4 箇所をそのまま置き換え、`backend:tools` は変更していない。

- `backend:mocks` だけ `run = "\"$(mise which mockery)\""` と引用符を二重にしているのは、この行が他の 2 タスクと違い複数行文字列ではなく 1 行の `run = "..."` だからである。TOML の文字列内で `"$(...)"` を保つためにエスケープが要る。
- このリポジトリには `mise.toml` のタスク定義を検証する自動テストが無いため、完了条件の検証は上記の手動実行で行った。テストは追加していない。

### 補足 (この issue の範囲外)

- 実測した時点で実際に隠蔽されていたのは staticcheck と goimports の 2 つで、govulncheck と mockery は `~/go/bin` に同名バイナリが無い。後の 2 つについて本修正が変えるのは、隠蔽が起きうる状態を将来にわたって塞ぐことであり、現時点で実行されるバイナリは変わらない。
- `~/go/bin/goimports` (2023-12-02、`go version -m` で `go1.21.4`) は現在も残っており、修正前は `mise exec -- which goimports` がこれを返していた。本修正により `backend:fmt` は mise 管理の goimports を実行するため、このバイナリの隠蔽は `mise run` の経路では影響しなくなった。`~/go/bin` 配下のファイルはこの issue の対象外なので削除していない。
- `mise run backend:mocks` が設定ファイル不在で必ず失敗することを本 issue の検証中に確認し、docs/issues/0199 として登録した。リポジトリに mockery の設定ファイルも生成済みのモックも無く、`backend/` の Go ファイルと `go.mod` にも mockery / `go.uber.org/mock` への参照が無い。本修正の前後で結果が変わらないため、この issue の完了条件には含めない。
