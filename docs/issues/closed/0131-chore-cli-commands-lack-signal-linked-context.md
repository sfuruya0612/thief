# CLI のコマンドが signal 連動の context を作らず、中断可能な処理が中断できない

Created: 2026-08-08
Completed: 2026-08-09
Model: Claude Opus 5

## 概要

`internal/cli` のコマンドは `server` を除きすべて `context.Background()` をそのまま使っており、`signal.NotifyContext` によるシグナル連動キャンセルを設定していない。そのため長時間かかるコマンドを Ctrl-C で中断しても、`ctx.Done()` を見て後片付けするコードには制御が渡らず、プロセスが OS 既定の SIGINT 処理で即死する。

AGENTS.md の「backend (Go) / context.Context」節は次のように定めている。

> 既存の `ctx` がない最上位(`main` 関数や CLI コマンド)では `context.Background()` を起点に、`signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` を用いてシグナル連動キャンセルを設定する。

`server` コマンドだけがこの規約を満たしている (`internal/cli/server.go:21`)。

## なぜ対応が必要か

規約違反であること自体に加えて、実装済みのキャンセル処理が本番で死んでいる。

`internal/aws/sso_oidc.go` の `waitForSSOToken` は待機中の `ctx.Done()` を見て `ctx.Err()` を返す分岐を持つ。この分岐は device code の有効期限 (AWS の既定で 600 秒) いっぱいポーリングし続ける可能性のあるループの唯一の中断口である。しかし呼び出し元の `internal/cli/sso.go:130` が `context.Background()` を渡すため、この分岐は `thief sso login` の実行中に一度も発火しない。中断可能に設計したループが実際には中断できていない。

同じ構図は他のコマンドにもある。`internal/cli/ecs.go` の ECS Exec、`internal/cli/ec2.go` の SSM セッション、`internal/aws/cloudwatchlogs.go` の Live Tail のように、外部プロセスや長命なストリームを扱う経路では、キャンセルが届かないことが後片付けの取りこぼしに直結する。

## 該当箇所

`signal.NotifyContext` を使っていない `context.Background()` の使用箇所 (2026-08-08 時点)。

- `internal/cli/sso.go:130`, `internal/cli/sso.go:193`
- `internal/cli/bq.go:112`, `:144`, `:187`, `:227`
- `internal/cli/ec2.go:83`, `:157`
- `internal/cli/ecs.go:129`, `:158`, `:209`
- `internal/cli/gcp.go:174`, `:193`, `:224`, `:252`, `:279`, `:305`, `:332`, `:359`
- `internal/cli/cfn.go:100`, `:154`
- `internal/cli/ssm.go:69`, `:95`
- `internal/cli/secretsmanager.go:57`
- `internal/cli/datadog.go:88`
- `internal/cli/cost.go:264`, `:279`, `:294`, `:309`
- `internal/cli/helper.go:100`

`internal/cli/server.go:46` の `context.WithTimeout(context.Background(), 5*time.Second)` はシャットダウン用の独立した context であり、対象外。

## 再現手順

1. SSO セッションが切れた状態で `thief sso login` を実行する。
2. ブラウザでの承認を行わずに、ポーリング中のターミナルで Ctrl-C を押す。
3. `waitForSSOToken` の `ctx.Done()` 分岐が発火せず、`context canceled` のエラー経路を通らないままプロセスが即死する。

`waitForSSOToken` に一時的なログを入れると、`ctx.Done()` の case が選ばれていないことを確認できる。

## 修正方針 (案)

各コマンドで個別に `signal.NotifyContext` を書くと同じ 5 行が 30 箇所に散る。`internal/cli` に共通のヘルパーを 1 つ置き、コマンドの `RunE` がそれを経由して `ctx` を得る形に揃えるのが素直である。候補は 2 つある。

1. `RunE` の中で `ctx, stop := cli.signalContext()` を呼ぶ形にする。既存の各コマンドの `context.Background()` を機械的に置き換えられる。
2. ルートコマンドの `PersistentPreRunE` で 1 度だけ作り、`cmd.SetContext` で載せて各コマンドは `cmd.Context()` から取る形にする。Cobra の標準的な流儀に近く、`stop` の呼び出し漏れも 1 箇所で防げる。

どちらを採るかは設計判断であり、実装前に決める必要がある。あわせて、キャンセルされたときのエラーを Cobra がそのまま表示すると `context canceled` という不親切な文言になるため、`errors.Is(err, context.Canceled)` を終了コードとメッセージへ写す扱いも決める。

## 完了条件

- `internal/cli` のコマンドが `signal.NotifyContext` 由来の `ctx` を使うこと (`server.go:46` のシャットダウン用を除く)。
- 派生した context の `stop` / `cancel` が必ず呼ばれること。
- `thief sso login` のポーリング中の Ctrl-C が `waitForSSOToken` の `ctx.Done()` 分岐を通ることをテストで検証すること。
- Ctrl-C による中断がスタックトレースや `context canceled` の生の文言ではなく、意図した終了として扱われること。

## 出典

`docs/issues/0129-bug-sso-device-auth-polling-violates-rfc-8628.md` の実装レビュー中に、ポーリングループの終了性を検査した過程で判明した。0129 の範囲は RFC 8628 準拠のポーリングであり、CLI 全体の context 設計は別の問題として切り出す。

## 解決方法

### 採った方針

修正方針の案 2 (ルートコマンドに 1 度だけ載せ、各コマンドは `cmd.Context()` から取る) を採った。案 1 は `RunE` ごとに `stop` の呼び出し責任が生まれ、30 箇所すべてで漏れがないことを保証できない。

`signal.NotifyContext` を呼ぶ場所は `backend/cmd/thief/main.go` の `interruptContext` の 1 箇所のみとした。`main` は `os.Exit` が defer を実行しないため `run` を分けて `stop` の呼び出しを保証する。`server` コマンドが自前で張っていた `signal.NotifyContext` は削除した (同じシグナルの二重購読になる)。

context は `cli.Run` が呼ぶ cobra の `ExecuteContextC` を通って各コマンドへ渡り、各コマンドは `commandContext(cmd)` で受け取る。`commandContext` は `cmd.Context()` が nil の場合 (テストが `RunE` 相当を直接呼ぶ経路) だけ `context.Background()` に倒す。

### 変更点

- `backend/cmd/thief/main.go`: `signal.NotifyContext` で作った context を `cli.Run` に渡し、その戻り値を終了コードとする。
- `backend/internal/cli/run.go` (新規): `Run` / `markCommandBodyRun` / `commandContext` / `interruptExitCode`。
- `backend/internal/cli/root.go`: `SilenceErrors` / `SilenceUsage` を立て、表示を `Run` に集約する。
- `internal/cli` の 30 箇所の `context.Background()` を `commandContext(cmd)` に置き換えた。
- `internal/cli/server.go`: 自前の `signal.NotifyContext` を削除し、シャットダウンの猶予は `serverShutdownTimeout` として定数化した。

### 完了条件の充足

1. **コマンドが `signal.NotifyContext` 由来の `ctx` を使うこと**

   `TestNoRootContextOutsideDesignatedFunctions` (`internal/cli/run_test.go`) が `internal/cli` の本番コードを AST で走査し、`context.Background()` / `context.TODO()` の呼び出しが `ec2.go:startEC2SessionWith` (SSM セッションの切断)、`run.go:commandContext` (nil の境界)、`server.go:newServerCmd` (シャットダウンの猶予) の 3 箇所だけであることを固定する。置き換え漏れも、新規コマンドでの再発も、この一覧が変わる形で落ちる。

   走査そのものが機能していることは `TestRootContextCallSitesFindsEveryShape` で検証している (サブディレクトリ、レシーバ付きメソッド、パッケージレベルの初期化子、関数リテラルの各形を検出できること)。

2. **派生した context の `stop` / `cancel` が必ず呼ばれること**

   `stop` は `main.go` の `run` の `defer stop()` の 1 箇所のみ。`cancel` は `startEC2SessionWith` と `newServerCmd` の 2 箇所で、いずれも `defer` で呼んでいる。`TestInterruptContextIsCanceledBySignals` (`backend/cmd/thief/main_test.go`) が実際にシグナルを送って、`interruptContext` が返す context が SIGINT と SIGTERM でキャンセルされることを検証する。

3. **`thief sso login` のポーリング中の Ctrl-C が `waitForSSOToken` の `ctx.Done()` 分岐を通ることをテストで検証すること**

   端から端までを 1 つのテストで通すことはしていない。`thief sso login` の実行には SSO のアクセスポータルと AWS の応答が必要であり、テストから再現できない。代わりに、context が届く経路を区間に分けて全区間を固定した。区間が繋がっていれば経路も繋がる。

   - `TestSSOLoginPassesCommandContextToTokenRetrieval` (`internal/cli/sso_test.go`): コマンドに載せた context が `ssoLogin` からトークン取得へ渡ること。
   - `TestSSOLoginFallsBackToBackgroundContext` (同): context が載っていない場合に nil を渡さないこと。
   - `TestGetSSOTokenForwardsContextToEveryStage` (同): `getSSOToken` がクライアント登録、デバイス認可の開始、トークンのポーリングの全段へ同じ context を渡すこと。
   - `TestDefaultSSOTokenDepsIsFullyWired` / `TestDefaultSSOLoginDepsIsFullyWired` (同): 本番用の依存が差し替え用のダミーではなく実装で埋まっていること。差し替えたダミーを通るテストだけでは、本番の配線漏れを検知できないため必要である。
   - `TestWaitForSSOTokenReturnsContextError` / `TestWaitForSSOTokenPrefersContextErrorOverTimeout` (`internal/aws/sso_oidc_test.go`、issue 0129 で追加済み): `waitForSSOToken` が待機中の `ctx.Done()` で `ctx.Err()` を返すこと。

4. **Ctrl-C による中断が意図した終了として扱われること**

   `cli.Run` が中断を `interrupted` の 1 行と終了コード 130 に写す。`errors.Is(err, context.Canceled)` で辿れない場合 (gRPC を使う Google Cloud の呼び出しは context のキャンセルを `codes.Canceled` のステータスへ写すため連鎖が切れる) は `ctx.Err()` を見て中断と判定し、`interrupted: <内容>` を表示する。

   `TestRunMapsCanceledToInterruptExitCode` / `TestRunTreatsFailureAfterCancellationAsInterrupt` / `TestRunReportsRuntimeFailureWithoutUsageHint` / `TestRunOnRealRoot` (`internal/cli/run_test.go`) が表示と終了コードの対応を固定する。

   終了コードについて: Ctrl-C の場合はシグナルの既定の動作でも 130 (128 + SIGINT) であり、値は変わらない。変わるのは、後始末を行ってから終わる点と `interrupted` を表示する点である。SIGTERM は既定の動作による 143 から 130 になる。`signal.NotifyContext` はどのシグナルで解除されたかを伝えないため、区別するには通知チャネルを自前で持つ必要がある。中断はどちらも同じ扱いとし、終了コードを 1 つに固定した (`interruptExitCode`)。

### あわせて閉じた退行

`signal.NotifyContext` は購読したシグナルの既定の動作をプロセス全体で止める。そのため context を見ない待機は、この変更によって「より止まりにくい」状態になる。該当した 3 箇所を同時に修正した。

- `internal/tidb/client.go`: context を受け取る口が無かった。第一引数に追加し、Digest 認証の 2 往復の両方に紐付けた (`internal/tidb/context_test.go`)。
- `internal/cli/helper.go`: `sso generate-config` の選択入力と `ssm param put` / `secretsmanager put` の標準入力の読み取り。読み取りを別の goroutine に出し、context のキャンセルで待機を打ち切る `readWithContext` を入れた。チャネルをバッファ 1 にしているのは、受信側が先に戻っても読み取りの goroutine が送信でブロックせず終了できるようにするためである (`TestReadWithContextLetsTheReaderFinishAfterCancellation` がこの点を goroutine 数で固定する)。
- `internal/util/executer.go`: `ExecCommand` が `call.Wait()` で子プロセス (session-manager-plugin) の終了を待つ間、SIGTERM で終了できなかった。SIGTERM を子プロセスへ転送するようにした。SIGINT は端末がフォアグラウンドのプロセスグループ全体へ配送するため従来どおり親は握りつぶし、子プロセスに委ねる。この 2 つの扱いの違いを `internal/util/executer_signal_test.go` が固定する (テストバイナリをヘルパープロセスとして別のプロセスグループで起動し、シグナルを送って終了コードで観測する。テストプロセス自身にシグナルを送らない形にしている)。

再発を防ぐため、`TestNoContextBlindStdinReadOutsideDesignatedFunctions` (`internal/cli/run_test.go`) が `fmt.Scan` 系・`fmt.Fscan` 系・`io.ReadAll`・`os.Stdin` の参照を AST で集め、context を見ない標準入力の読み取りが `helper.go` の `promptSelection` と `readUpdateValue` の 2 箇所に限られることを固定する。走査の網羅性は `TestContextBlindReadCallSitesFindsEveryShape` で検証している。

### 検出力の検証

追加したテストがどこまで実装の破壊を捉えるかを、実装を機械的に書き換えて測った。

- `internal/cli` と `internal/tidb` および `cmd/thief` を対象とした 28 種類: 全件検出。当初 2 件が漏れており、1 件は結果チャネルのバッファを外しても goroutine が残るだけでアサーションに現れなかったため、goroutine 数を見るテストを追加して閉じた。
- `internal/util/executer.go` を対象とした 7 種類 (SIGTERM を購読しない、SIGINT を購読しない、受けたシグナルを全て転送する、転送するシグナルを SIGKILL にする、転送しない、goroutine の終了の合図を送らない、`Wait` を待たずに戻る): 全件検出。

### レビューで見つけて閉じたもの

多観点のレビューで挙がった指摘のうち、本 issue の範囲で閉じたものは以下である。

- **cobra が遅延登録するコマンドの本体が包まれていなかった** (`internal/cli/run.go`)。`ExecuteContextC` は `help` (`InitDefaultHelpCmd`)、`completion` とその子コマンド (`InitDefaultCompletionCmd`)、`__complete` (`initCompleteCmd`) を実行の直前に木へ足す。`markCommandBodyRun` を `ExecuteContextC` より前に呼んでいたため、これらは辿った時点で存在せず、本体が包まれなかった。結果、`completion bash` の書き込み失敗のような実行時のエラーが「本体に入っていない」と判定され、使い方の誤りとして help へ誘導していた。`Run` の先頭で `InitDefaultHelpCmd` と `InitDefaultCompletionCmd` を明示的に呼ぶようにした。どちらも冪等であり (前者は同じインスタンスを付け替えるだけ、後者は既にあれば何もしない)、`ExecuteContextC` が後で呼び直しても包んだ本体は残る。`__complete` は非公開のため先に足せないが、`Run` (非 E) で定義されておりエラーを返せないため判定を誤らない。

  `TestRunReportsFailureOfALazilyRegisteredCommandWithoutUsageHint` が `completion bash` の書き込み先を必ず失敗する writer にして、`TestRunReportsFailureOfACustomHelpCommandWithoutUsageHint` が `SetHelpCommand` で `RunE` を持つ help コマンドを差し込んで、それぞれ固定する。後者が必要なのは、cobra の既定の help コマンドが失敗を `CheckErr` でプロセスの終了に直接変えるため、既定のままでは登録の順序を観測できないからである。

- **`markCommandBodyRun` の `Run` (非 E) の分岐が一度も実行されていなかった**。本番コードに `Run` を使うコマンドが無く、記録も本体の呼び出しも消してもテストが通る状態だった。上の修正で cobra の help コマンドがこの分岐を通るようになったが、`Run` のコマンドは本体に入った後に失敗しないため、`cli.Run` の終了コードからは記録の有無を観測できない。`TestMarkCommandBodyRunWrapsBothBodyKinds` が `Run` と `RunE` の両方について、記録が立つことと元の本体が呼ばれることを直接検証する。

- **`internal/tidb` の繰り返しリクエストが途中のキャンセルで止まることが未検証だった**。`ListProjects` と `ListClusters` はページを、`GetCostRange` は月を跨いでリクエストを繰り返す。どのループも `ctx.Err()` を自分では見ず、次のリクエストがキャンセルで失敗することだけを頼りに止まる。`TestClientStopsIteratingAfterCancellation` が 1 周目の応答を返す前にキャンセルし、2 周目のリクエストが飛ばないことと、返るエラーが `context.Canceled` を包むことを検証する。`RoundTripper` の差し替えではなく `httptest.Server` を相手にしているのは、差し替えた `RoundTripper` は自分で context を見ない限りリクエストが成功してしまい、打ち切りを確かめられないためである。

- **`bufio` 経由の読み取りを検査が取りこぼしていた**。`TestNoContextBlindStdinReadOutsideDesignatedFunctions` が使う走査は `fmt.Scan` 系・`fmt.Fscan` 系・`io.ReadAll`・`os.Stdin` だけを見ていた。`bufio.NewReader(cmd.InOrStdin()).ReadString('\n')` は `os.Stdin` を参照しないため挙がらない。`bufio.NewReader` / `NewReaderSize` / `NewScanner` の構築を対象に加えた (読み取り自体は返り値のメソッドで行われ、メソッドだけを見て判別するには型情報が必要になるため、構築の時点で捕まえる)。本番コードに現在 `bufio` の利用は無く、将来の混入に対する網である。

- **goroutine 数を基準にする 2 つのテストが基準値の取り方で脆かった**。直前のテストが残した goroutine が消える前に基準値を取ると基準が 1 つ多く出て、リークした 1 つを見逃す。数が落ち着くまで待って取る `settledGoroutineCount` を `internal/cli/helper_test.go` と `internal/util/executer_test.go` の双方に置いた。後者は加えて、`os/signal` が最初の `signal.Notify` でパッケージ内に起動する常駐 goroutine が基準に混ざらないよう、数える前に 1 度呼んで起動させている (テストを単体で実行した場合に効く)。

- **コメントの記述が実装と合っていなかった** 2 箇所を直した。`run_test.go` の「`FlagErrorFunc` が `usageError` を付ける」は誤りで、cobra v1.10.2 に `usageError` は存在せず、既定の `FlagErrorFunc` は受け取ったエラーをそのまま返す。「`Run` は `CalledAs` が空のままであることで判別する」も誤りで、判別は `markCommandBodyRun` の記録による。

上記の追加分についても実装を機械的に書き換えて検出力を測った。`internal/cli/run.go` を対象とした 5 種類 (`Run` 分岐の記録を消す、`Run` 分岐の本体呼び出しを捨てる、`InitDefaultHelpCmd` だけ消す、`InitDefaultCompletionCmd` だけ消す、`markCommandBodyRun` を `ExecuteContextC` の後に動かす) と、`internal/tidb/resources.go` を対象とした 4 種類 (3 つのループがそれぞれ `context.Background()` を渡す、`GetCostRange` がエラーを無視して次の月へ進む) はいずれも全件検出した。

### 起票した issue

作業中に見つけた別の問題は以下に切り出した。

- docs/issues/0135: `ecs execute-command` が SSM セッションを切断せず AWS 側に残す。
- docs/issues/0136: SSM セッションの切断タイムアウト 5 秒が 3 パッケージに別名で重複している。
- docs/issues/0137: `sso generate-config` の選択入力に空白が混ざると選択全体が黙って破棄される (`fmt.Fscanln` が 1 語しか読まない性質。今回は読み取りの意味を変えないため挙動を維持し、テストで固定した)。
- docs/issues/0138: `util.Select` が context を見ないため選択画面が SIGTERM で終了しない。テストの差し込み口が無く挙動を固定できないため、issue 0134 の後に対応する。
- docs/issues/0139: golangci-lint が `backend:lint` タスクに入っておらず AGENTS.md の記載と食い違う。
- docs/issues/0140: `ssoGenerateConfig` にテストの差し込み口が無く、アカウント選択とロール取得の分岐が検証できない。context と標準入力の配線そのものは上記 2 つの走査で固定済みであり、残るのは分岐の検証である。

### 残した課題

`util.Select` の表示中は SIGTERM で終了しない (docs/issues/0138)。Ctrl-C は bubbletea がキーイベントとして受け取るため画面から抜けられるが、`no item selected` として扱われ、表示と終了コードが他の中断経路と揃わない。挙動を変えるにはテストの差し込み口 (docs/issues/0134) が先に必要であり、テストで固定できない状態で変えないという判断で分離した。
