# EC2 セッション実行の失敗が errors.Is で到達できず、成功時は同じ文言が 2 回表示される

Created: 2026-08-08
Completed: 2026-08-08
Model: Claude Opus 5

## 背景

issue 0126 (`internal/cli` の `%v` ラップの棚卸し) の作業中に、`backend/internal/cli/ec2.go` の `startEC2Session` (117 行) で 2 つの独立した問題を見つけた。

該当箇所は 158-164 行である。

```go
execErr := util.ExecCommand(plug, string(sessJSON), cfg.Region, "StartSession", cfg.Profile, string(paramsJSON), ssmEndpoint)

if execErr != nil {
	cmd.PrintErrf("execute command: %v\n", execErr)
	if termErr := awsinternal.TerminateSSMSession(ctx, cfg.Profile, cfg.Region, session.SessionID); termErr != nil {
		return fmt.Errorf("terminate session after exec error: %w (original exec error: %v)", termErr, execErr)
	}
	return fmt.Errorf("execute command: %w", execErr)
}
```

`ExecCommand` は `internal/util/executer.go:14` で `exec.Command` を実行する。失敗時は `*exec.ExitError` など `os/exec` の型付きエラーを返す。

### 問題 1: セッション終了も失敗した場合に execErr が errors.Is / errors.As で到達できない

161 行は `termErr` を `%w` で包む一方、`execErr` は `%v` で文字列として埋め込んでいる。`errors.Is` と `errors.As` は `Unwrap()` を辿るため、この経路を通ると `execErr` の型と値に到達できない。

`session-manager-plugin` の終了コードで分岐したい場合、`*exec.ExitError` を `errors.As` で取り出す必要があるが、セッション終了も失敗した場合だけ取り出せなくなる。同じ関数の同じ失敗が、無関係な後処理の成否によって判別可能かどうか変わるのは一貫性がない。

Go 1.20 以降 `fmt.Errorf` は `%w` を複数書ける。`go.mod` の `go` ディレクティブは 1.26 系であり、この機能は使える。

AGENTS.md の「backend (Go)」節は「ラップは `fmt.Errorf` の `%w` 動詞を使う」「比較は `errors.Is`、型取り出しは `errors.As` を用いる」と定めており、`execErr` を `%v` に落としている点はこれに反する。

なお issue 0126 の起票時の集計では、この行は `%w` を含むため「`%v` 4 箇所」ではなく「`%w` 側」に数えられており、0126 のスコープからは外れていた。

### 問題 2: セッション終了が成功した場合に同じ文言が 2 回表示される

159 行は `execute command: <err>` を標準エラー出力へ書く。その後 `TerminateSSMSession` が成功すると 163 行が `fmt.Errorf("execute command: %w", execErr)` を返す。

`SilenceErrors` はリポジトリのどこにも設定されていないため、Cobra が返り値のエラーを `Error: ` を前置して標準エラー出力へ表示する。結果として同じ内容が 2 回並ぶ。

```
execute command: exit status 1
Error: execute command: exit status 1
```

159 行の `PrintErrf` は、後続の `TerminateSSMSession` に時間がかかる場合でも実行失敗を先に知らせる意図と読めるが、成功時は重複した表示になる。

同ファイル 96 行の `PrintErrf` は、リージョンごとの取得失敗を報告して `continue` する縮退処理であり、エラーを返さないため重複しない。こちらは問題ない。

## 再現手順

### 問題 1

1. `startEC2Session` を通すコマンドを実行する (`thief ec2 session` 相当)。
2. `session-manager-plugin` が非ゼロ終了する条件を作る (プラグインを実行不可にする、権限不足のインスタンスを指定する等)。
3. あわせて `TerminateSSMSession` が失敗する条件を作る (`ssm:TerminateSession` 権限を外す)。
4. 返り値のエラーに対し `errors.As(err, new(*exec.ExitError))` が false になる。

### 問題 2

1. `startEC2Session` を通すコマンドを実行する。
2. `session-manager-plugin` が非ゼロ終了する条件を作る。
3. `TerminateSSMSession` は成功する状態にしておく (通常の権限構成)。
4. 標準エラー出力に `execute command: <err>` と `Error: execute command: <err>` の 2 行が並ぶ。

## 影響

- セッション実行の失敗を、後処理の成否に依存せず `errors.Is` / `errors.As` で判別できない。プラグインの終了コードに応じた処理を CLI 側で足せない。
- ユーザーが目にする出力に同じ内容が 2 回並び、2 つの異なる失敗が起きたように読める。
- 読み取り専用の接続フローであり、データの破壊や外部への影響はない。

## 修正方針

- 161 行の `execErr` を `%v` から `%w` に変更し、`termErr` と `execErr` の両方に到達できるようにする。文言は「動詞 + 対象」の形に揃え、2 つの失敗が起きたことが読み取れる形にする。
- 159 行の `PrintErrf` と 163 行の返り値の重複を解消する。次のいずれかを実装時に比較して決める。
  - `PrintErrf` を削り、失敗の報告を返り値だけに任せる (Cobra が表示する)。161 行の経路でも `execErr` の情報は `%w` で残るため、情報は失われない)。
  - `PrintErrf` を残し、163 行の返り値をラップしない裸の `execErr` に変える。ただし Cobra が表示する以上、重複自体は解消しない。
- 変更後の文言を文字列比較しているコードやテストがないことを、backend と frontend の両方で確認してから変更する。

## 完了条件

- `internal/cli/ec2.go:161` が `termErr` と `execErr` の両方を `%w` でラップしている。
- `startEC2Session` の失敗経路で、`errors.As` により `execErr` の型に到達できることを検証するテストがある。セッション終了が成功する経路と失敗する経路の両方を検証する。
- `session-manager-plugin` の実行と `TerminateSSMSession` の呼び出しを、テストから差し替えられる形になっている。
- 同じ文言が標準エラー出力と Cobra の表示で 2 回並ばない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0126: 起票元。`internal/cli` の `%v` ラップを棚卸しした issue。この行はスコープ外として残した。
- docs/issues/0127: `internal/aws/sso_oidc.go` にテストの差し込み口がない issue。本 issue も `util.ExecCommand` と `TerminateSSMSession` の差し込み口が必要になる点で同種である。

## 解決方法

### issue の前提が不完全だった

起票時の修正方針は `internal/cli/ec2.go:161` の `%v` を `%w` に変えることを完了条件に置いていたが、実装に入って `internal/util/executer.go` を読んだところ `ExecCommand` の末尾が次の形だった。

```go
if err := call.Run(); err != nil {
	return fmt.Errorf("%v", err)
}
return nil
```

チェーンはここで既に切れており、`ec2.go` 側をいくら `%w` にしても `errors.As` は `*exec.ExitError` へ到達できない。完了条件「`errors.As` により `execErr` の型に到達できることを検証するテストがある」は `ec2.go` の 1 行を直すだけでは満たせなかった。

`ExecCommand` は `return call.Run()` としてラップをやめた。ラップを足す選択肢もあったが、呼び出し元 2 箇所 (`ec2.go` と `ecs.go:232`) がいずれも自前の接頭辞を持っているため、ここで足すと接頭辞が重なる。

`fmt.Errorf("%v", err)` を外してもユーザーが見る文字列は変わらない。`os/exec` のエラー型 (`*exec.Error` / `*exec.ExitError`) はいずれも `fmt.Formatter` を実装していないため、`fmt` は `%v` に対して `Error()` の戻り値をそのまま使う。既存の `internal/util/executer_test.go` の 3 テストもメッセージを固定していない。変わるのは `errors.Is` / `errors.As` の到達可能性だけである。

### 実装した形

| 対象 | 変更 |
| --- | --- |
| `internal/util/executer.go` | `fmt.Errorf("%v", err)` を削り `return call.Run()` にする。`fmt` の import を削除 |
| `internal/cli/ec2.go` | `ec2SessionDeps` と `defaultEC2SessionDeps()` を追加し、`startEC2Session` / `startEC2SessionWith` に分割する |
| `internal/cli/ec2.go` | 切断も失敗した経路を `fmt.Errorf("execute command: %w; terminate session: %w", execErr, termErr)` にする |
| `internal/cli/ec2.go` | `cmd.PrintErrf("execute command: %v\n", execErr)` を削除する |

`ec2SessionDeps` は `internal/cli/sso.go` の `ssoTokenDeps` (issue 0126) と同じ形にした。差し替える 5 つ (`selectEC2Instance` / `awsinternal.StartSSMSession` / `lookupSessionManagerPlugin` / `util.ExecCommand` / `awsinternal.TerminateSSMSession`) はいずれも独立した自由関数であり、共通のレシーバを持たない。`internal/aws` のコンシューマ定義インターフェースは SDK クライアントの 1 メソッドを絞り込むための形であって、レシーバの無い関数群には当てはまらない。

### エラー文言を「実行の失敗を先頭」に統一した理由

起票時の修正方針は文言を「動詞 + 対象」にすることまでしか定めておらず、実装では最初 `terminate session after exec error: %w (original exec error: %w)` としていた。しかしこれでは切断が成功したときの `execute command: ...` と先頭の動詞が入れ替わる。

本 issue の問題 1 の主張は「同じ関数の同じ失敗が、無関係な後処理の成否によって判別可能かどうか変わるのは一貫性がない」である。同じ理屈は文言にも当てはまる。実行の失敗の見え方が切断の成否で変わってよい理由は無いため、どちらの経路も `execute command:` で始める形に変えた。

```
execute command: exit status 3
execute command: exit status 3; terminate session: terminate boom
```

`errors.Join` は使わなかった。`Error()` が改行区切りになり、Cobra が `Error: ` を前置して表示する CLI の 1 行出力に合わない。リポジトリ内に `errors.Join` の使用例も無い。

### テストの配置

`ExecCommand` がチェーンを保つこと自体は `internal/util/executer_test.go` に `TestExecCommand_PreservesExitErrorChain` として置いた。既存 3 テスト (`TestExecCommand_Simple` / `_Error` / `_WithArgs`) のアンダースコア命名に揃えてある。実装当初は `internal/cli/ec2_test.go` に `TestExecCommandKeepsExitErrorChain` として置いていたが、対象パッケージが `internal/util` である以上そちらに置くのが正しく、`go test -run TestExecCommand ./...` が 2 パッケージに当たる紛らわしさもある。

`internal/cli` 側の end-to-end 版は削除した。残る検出対象は「`execPlugin` に別の関数が代入される」ミューテーションのみで、同じシグネチャ `func(string, ...string) error` を持つ関数が他に無いため実効的な検出力がほぼ無い。

### 測定した検出力

最終形で 14 のミューテーションを入れて測定し、13 が検出された。

| ミューテーション | 結果 |
| --- | --- |
| `execute command: %w` の `execErr` を `%v` に戻す | 検出 |
| `terminate session: %w` の `termErr` を `%v` にする | 検出 |
| 2 つの `%w` の引数順を入れ替える | 検出 |
| `cmd.PrintErrf` を復活させる | 検出 |
| `return fmt.Errorf("execute command: %w", execErr)` を裸の `return execErr` にする | 検出 |
| `sessionManagerSessionJSON` のキー名 `SessionId` を `SessionID` にする | 検出 |
| `util.Parser` に渡す `Target` の値を壊す | 検出 |
| exec 失敗経路の `terminateSession` の profile/region を入れ替える | 検出 |
| 成功経路の `terminateSession` の profile/region を入れ替える | 検出 |
| `startSession` の profile/region を入れ替える | 検出 |
| `ExecCommand` を `errors.New(err.Error())` でチェーンを切る | 検出 |
| `ExecCommand` に `fmt.Errorf("exec %s: %w", ...)` の接頭辞を足す | 検出 |
| `ExecCommand` がエラーを握り潰す | 検出 |
| `ExecCommand` の `signal.Stop(sigs)` を削除する | 未検出 |

exec 失敗経路の `terminateSession` の引数入れ替えは、最初の実装では未検出だった。当該テストが profile/region を記録しないモックで `terminateSession` を上書きしていたためである。記録とアサーションを足して塞いだ。

`signal.Stop` の削除は、シグナル登録の解除をテストから観測する現実的な手段が無いため未検出のまま残した。現状のコードは `defer close(done)` 経由で正しく呼んでおり、バグではない。

### 採用したレビュー指摘

- `ExecCommand` のチェーン検証テストを `internal/util/executer_test.go` へ移す (自力の指摘とレビューが一致)
- `execExitError` のコメントの根拠が誤り。`os.ProcessState.String()` と `ExitCode()` はいずれも `p == nil` をガードしており panic しない。実際に値として組み立てられない理由は `pid` / `status` / `rusage` がすべて非公開フィールドで、任意の終了コードを持つ値を公開 API から作れないことである
- 切断も失敗した経路の文言を `execute command:` 始まりに統一する
- `ExecCommand` の doc コメントの因果関係が読みにくい点を書き直す
- `newEC2SessionCmd` を hermetic にする。`config.Load` が `$XDG_CONFIG_HOME/thief/config.yaml` と `$HOME/.thief/config.yaml` を読むため、実行環境に壊れた YAML があるとテストの意図と無関係に落ちる。両方を空のディレクトリへ向けた
- プラグインに渡す 6 引数を `cmp.Diff` で完全一致に固定する。`session_plugin.go` は「フィールド名はプラグインとの契約であり変更してはならない」と述べているが、値の部分一致しか検査しておらずキー名が固定されていなかった
- `startSession` と `terminateSession` に渡す profile / region を検証対象に加える

### 却下したレビュー指摘

- `startEC2SessionWith` から `cmd *cobra.Command` を外して `*config.Config` と `instanceID string` を受け取る形にする案。`internal/cli` の既存コマンド実装はすべて `cmd` を受け取り内部で `loadConfig(cmd)` を呼ぶ形で統一されており、ここだけ変えると逆に逸脱する
- `startEC2Session` の英語 doc コメントを日本語へ統一する案。この diff で触っていない既存行であり、`sso.go` にも同種の混在が残っている。無関係な書き換えは入れない

### このテストの限界

- 公開ラッパー `startEC2Session` は `defaultEC2SessionDeps()` を渡すだけの 1 行であり、テストで踏んでいない。`internal/aws` の公開関数と同じ既存パターンである
- `defaultEC2SessionDeps()` の各フィールドは nil でないことしか検査していない。関数値は同一性を比較できず、実際に呼べば AWS 接続や外部プロセス起動が起きる。姉妹関数の `TestDefaultSSOTokenDepsIsFullyWired` も同じ範囲に留めている
- `marshal session:` と `marshal start session input:` の 2 経路は到達不能である。`json.Marshal` に渡すのが string だけの構造体であり、チャネル・関数・循環参照・NaN のいずれも含まないためエラーになり得ない。文言を書き換えてもテストは落ちないが、そもそも実行されない経路である
- `ecs.go:232` も同じ `util.ExecCommand` を通るためチェーンが保たれるようになるが、`ecs.go` には差し込み口が無く検証していない。`lookupSessionManagerPlugin` が `errors.New` でチェーンを切っている点も本 issue のスコープ外として残した

### 実際の変更

- `backend/internal/util/executer.go`: `fmt.Errorf("%v", err)` を削除、`fmt` の import を削除、doc コメントを追記
- `backend/internal/util/executer_test.go`: `TestExecCommand_PreservesExitErrorChain` を追加
- `backend/internal/cli/ec2.go`: `ec2SessionDeps` / `defaultEC2SessionDeps()` を追加、`startEC2SessionWith` へ分割、`PrintErrf` を削除、2 つの `%w` へ変更
- `backend/internal/cli/ec2_test.go`: 新規。7 テスト
- `CHANGES.md`: `[FIX]` エントリを追加
