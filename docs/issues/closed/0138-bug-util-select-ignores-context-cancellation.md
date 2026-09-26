# util.Select が context を見ないため選択画面が SIGTERM で終了しない

Created: 2026-08-09
Model: Claude Opus 5
Completed: 2026-08-11

## 背景

issue 0131 (CLI コマンドがシグナル連動 context を持たない) で、main が `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` で作った context を全コマンドへ渡すようにした。

`signal.NotifyContext` はシグナルを購読するため、購読したシグナルの既定の動作 (プロセスの即時終了) がプロセス全体で止まる。したがって context を見ない待機は、この変更の後に「より止まりにくい」状態になる。

同じ性質の箇所は issue 0131 の中で以下を対応した。

- TiDB Cloud のクライアント: 第一引数に context を追加し、Digest 認証の 2 往復の両方に紐付けた
- `sso generate-config` の選択入力と `ssm param put` / `secretsmanager put` の標準入力: 読み取りを別の goroutine に出し、context のキャンセルで待機を打ち切るようにした (`internal/cli/helper.go` の `readWithContext`)
- `util.ExecCommand`: SIGTERM を子プロセスへ転送するようにした

残っているのが `backend/internal/util/selecter.go` の `Select` である。

```go
// Select は items を対話式リストで表示し、ユーザーが選択した要素を返す。
func Select(items []Item, prompt string) (Item, error) {
	...
	p := tea.NewProgram(initialModel)
	m, err := p.Run()
```

`Select` は context を受け取らず、`tea.NewProgram` に `tea.WithContext(ctx)` を渡していない。

## 問題

選択画面を表示している間、プロセスは SIGTERM で終了しない。`signal.NotifyContext` が既定の動作を止めているため SIGTERM ではプロセスが落ちず、`p.Run()` は利用者が Enter か q を押すまで戻らない。それまで main の ctx がキャンセル済みであることは誰も見ない。

Ctrl-C は影響を受けない。bubbletea が `ctrl+c` をキーイベントとして受け取り (`selecter.go` の `Update` の `case "ctrl+c", "q"`)、`tea.Quit` で画面を閉じるためである。ただしこの経路では選択が `nil` のままとなり、`Select` は `no item selected` を返す。`cli.Run` から見ると中断ではなく実行時の失敗であり、表示は `Error: ...`、終了コードは 1 になる。中断として扱われる他の経路 (`interrupted` の表示と終了コード 130) と揃っていない。

該当するのは以下の 2 コマンドである。

- `backend/internal/cli/ec2.go:265`: `util.Select(items, "Select an EC2 instance:")`
- `backend/internal/cli/ecs.go:166`: `util.Select(ecsSelectItems(arns, 1), "Select an ECS cluster:")`

## 再現手順

1. `thief ec2 session` を実行し、インスタンスの選択画面を表示させる。
2. 別の端末から `kill <thief の pid>` を実行する (SIGTERM)。

期待: プロセスが終了する。

実際: 選択画面が表示されたままとなり、プロセスは終了しない。

手順 2 で `kill -9` を使えば終了するが、これは SIGKILL が捕捉できないためであり、後始末は行われない。

Ctrl-C の場合の挙動も併せて確認する。

3. `thief ec2 session` を実行し、選択画面で Ctrl-C を押す。

実際: `Error: select ec2 instance: no item selected` が表示され、終了コードは 1 になる。他の中断経路は `interrupted` を表示して 130 で終わる。

## なぜ対応が必要か

- SIGTERM で終了しないのは、issue 0131 で `signal.NotifyContext` を導入したことによる退行である。導入前は SIGTERM の既定の動作でプロセスが落ちていた。
- 中断の表示と終了コードが経路によって違う。利用者から見て「Ctrl-C で抜けた」という同じ操作が、選択画面かどうかで `interrupted` / 130 と `Error: ... no item selected` / 1 に分かれる。
- グローバル `~/.codex/AGENTS.md` は、I/O を行う関数は第一引数で context を受け取ると定めている。`Select` は端末に対する I/O を行い、利用者の入力を待つ。

## 修正方針

`Select` の第一引数に `ctx context.Context` を追加し、`tea.NewProgram(initialModel, tea.WithContext(ctx))` を渡す。呼び出し側 2 箇所は `commandContext(cmd)` を渡す。

bubbletea v1.3.10 の挙動は以下である (`tea.go` を確認済み)。

- `tea.WithContext(ctx)` で渡した context がキャンセルされると `Program.Run` が戻る。
- そのとき返るエラーは `fmt.Errorf("%w: %w", ErrProgramKilled, p.externalCtx.Err())` である (`tea.go:727`)。したがって `errors.Is(err, context.Canceled)` が真になる。
- `Select` は現在も `%w` でラップしているため、この連鎖は呼び出し側まで保たれる。

`cli.Run` は ctx のキャンセル自体を見て中断と判定するため、これだけで SIGTERM は `interrupted` と終了コード 130 になる。

Ctrl-C で `no item selected` になる件も併せて決める。候補は以下である。

- 案 A: `Update` の `ctrl+c` を `tea.Quit` から `tea.Interrupt` に変える。`Program.Run` は `ErrInterrupted` を返す (`tea.go:406`)。`Select` はこれを `errors.Is` で判別し、中断であることが分かる形のエラーを返す。`cli.Run` の中断判定に載せるには、`context.Canceled` を包むか、`internal/cli` 側で `ErrInterrupted` を判別する必要がある。どちらにするかは判断が必要。
- 案 B: `ctrl+c` の扱いは変えず、`q` と同じ「選択せずに終了」のままとする。この場合 Ctrl-C の見え方は揃わない。

いずれの案でも、選択せずに終了した場合と context のキャンセルで終了した場合を `Select` の戻り値で区別できるようにすること。

## 依存

先に issue 0134 (`internal/util/selecter.go` にテストの差し込み口が無い) を対応すること。

現在の `Select` は `tea.NewProgram` に入出力の指定を渡していないため、標準入力と標準出力に直結する。テストから選択操作を与える手段が無く、context のキャンセルで抜けることも検証できない。issue 0134 で `tea.WithInput` / `tea.WithOutput` を渡せる形にした後であれば、キャンセルで抜けることをテストで固定できる。

テストで固定できない状態で挙動を変えることはしない。

## 完了条件

- `Select` が第一引数で context を受け取り、`tea.WithContext` に渡している。
- 呼び出し側 2 箇所が `commandContext(cmd)` を渡している。
- 選択画面の表示中に context がキャンセルされると `Select` が戻り、返るエラーから `errors.Is(err, context.Canceled)` が真になる。
- 上記をテストで検証している。
- Ctrl-C の扱い (案 A / 案 B) を決め、決めた側の挙動をテストで固定している。
- `mise run check` が通る。

## 関連

- docs/issues/0131: 起票元。CLI コマンドへのシグナル連動 context の導入。
- docs/issues/0134: 本 issue の前提となる差し込み口の追加。
- `backend/internal/util/executer.go`: 同じ性質 (context を見ない待機) を SIGTERM の転送で解決した箇所。

## 解決方法

Ctrl-C の扱いは案 A (中断として統一する) を採用した。ユーザーに確認して決定した。

### 実装

`backend/internal/util/selecter.go` の `Select` の第一引数に `ctx context.Context` を追加し、`selectRunner` (`func(context.Context, tea.Model) (tea.Model, error)`) と本番実装 `runTeaProgram` を新設した。`runTeaProgram` は `tea.NewProgram(m, tea.WithContext(ctx)).Run()` を呼び、`Select` の実体である `selectWith` はこの `run` を経由して呼び出す。issue 0134 で導入した DI の差し込み口をそのまま拡張する形になる。

呼び出し側 2 箇所を更新した。

- `backend/internal/cli/ec2.go`: `selectEC2Instance` の呼び出しを `util.Select(ctx, items, "Select an EC2 instance:")` に変更 (`ctx` はこの関数の既存の第一引数で、呼び出し元の `startEC2SessionWith` が `commandContext(cmd)` から渡している)。
- `backend/internal/cli/ecs.go`: `displayECSTasks` の呼び出しを `util.Select(ctx, ecsSelectItems(arns, 1), "Select an ECS cluster:")` に変更 (`ctx := commandContext(cmd)` を直前で取得している)。

Ctrl-C の統一は `model.Update` の `case "ctrl+c"` を `tea.Quit` から `tea.Interrupt` に変えることで実現した。bubbletea v1.3.10 の実装 (`tea.go`) を確認したところ、`tea.Interrupt` を返すと `Program.Run` は最終的に `fmt.Errorf("%w: %w", ErrProgramKilled, ErrInterrupted)` 相当のエラーで戻る。`selectWith` はこれを `errors.Is(err, tea.ErrInterrupted)` で判別し、`fmt.Errorf("select interrupted: %w", context.Canceled)` に変換して返す。これにより `cli.Run` の中断判定 (`errors.Is(err, context.Canceled)`) にそのまま乗り、他の中断経路と同じ `interrupted` 表示・終了コード 130 になる。`q` の挙動は変えていない (`tea.Quit` のまま、`no item selected` を返す)。

ctx のキャンセル (SIGINT / SIGTERM) による中断は、bubbletea 自身が `tea.WithContext` に渡した context のキャンセルを検知して `fmt.Errorf("%w: %w", ErrProgramKilled, ctx.Err())` を返すため、既存の `%w` ラップの連鎖だけで `errors.Is(err, context.Canceled)` まで届く。`selectWith` 側に専用の分岐は不要だった。

### テスト

`backend/internal/util/selecter_test.go` に以下を追加・更新した。

- `TestSelectWith_CtrlCIsTreatedAsInterruption`: `run` が `ErrProgramKilled` と `ErrInterrupted` を包んだエラーを返した場合、`selectWith` が `errors.Is(err, context.Canceled)` を真にすることを検証する。
- `TestSelectWith_NoSelectionReturnsError`: `q` (「選択せずに終了」) の場合に `errors.Is(err, context.Canceled)` が真にならないことを検証するアサーションを追加した (中断と誤判定しないことの確認)。
- `TestSelectWith_ContextCancellationIsDetected`: 既にキャンセル済みの ctx を渡し、`run` が bubbletea の実際の形 (`fmt.Errorf("%w: %w", ErrProgramKilled, ctx.Err())`) のエラーを返した場合に、`selectWith` がそのまま `errors.Is(err, context.Canceled)` まで伝えること、および ctx をそのまま `run` に渡していることを検証する。
- `TestBubbleTeaProgramWithContextReturnsContextCanceled` (新規): 上記のモックが前提にしている「`tea.WithContext` に渡した context がキャンセル済みだと `Program.Run` が `context.Canceled` を辿れるエラーで戻る」という bubbletea 自体の挙動を、モックではなく実際の `tea.NewProgram` / `Run()` で検証する。`tea.WithInput(nil)` で標準入力への依存を切り、`tea.WithoutRenderer()` でレンダラを無効化することで、実端末に依存せず決定的に実行できるようにした。
- `TestModel_Update_CtrlCReturnsInterrupt` / `TestModel_Update_QReturnsQuit`: `Update` が `ctrl+c` で `tea.Interrupt`、`q` で `tea.Quit` を返すことをそれぞれ固定する。

`runTeaProgram` 自体 (`tea.NewProgram(m, tea.WithContext(ctx)).Run()` の 1 行) は直接の単体テストを書いていない。標準入力を明示的に無効化しない限り実端末を要求し (`could not open a new TTY: open /dev/tty: device not configured` で失敗することを実際に確認済み)、本番実装にテストのためだけの入出力差し替えを持たせることは issue 0134 で確定した DI 境界の設計 (`Select` の外側だけを差し替え可能にし、実際の bubbletea 駆動はテストしない実 I/O 境界として扱う) を破る。この境界自体は `TestBubbleTeaProgramWithContextReturnsContextCanceled` が実機構で検証しており、`runTeaProgram` はその機構をそのまま呼ぶだけの薄い関数であるため、これを妥当なトレードオフとして受け入れた。

### 多観点レビューの結果

5 観点のレビューを 1 ラウンド実施し、以下を反映した。

- 観点 1 (完了条件充足)
  - [優先度: 中] `CHANGES.md` の新エントリが `thief ecs execute-command` という誤ったコマンド名を挙げていた (`util.Select` を呼ぶのは `displayECSTasks` / `Use: "tasks"` であり `execute-command` ではない)。`thief ecs tasks` に修正した。
  - [優先度: 低、対応不要] 実際の bubbletea を駆動する統合テストが無いという指摘。観点 2 の高優先度の指摘で `TestBubbleTeaProgramWithContextReturnsContextCanceled` を追加して解消した。
- 観点 2 (テスト品質)
  - [優先度: 高] ミューテーションテストで `runTeaProgram` から `tea.WithContext(ctx)` を取り除いても (issue が指す不具合そのものに戻しても) 既存テストが全て通過することが判明した。既存テストは `selectWith` に手で組み立てたエラー形状を注入するだけで、実際の bubbletea の挙動を検証していなかった。`runTeaProgram` 自体を直接テストする方法を検討したが (キャンセル済み ctx で直接呼ぶ実験用テストを作成して実行)、実端末が無い環境では `/dev/tty` のオープンに失敗し技術的に不可能と確認した。この事実を踏まえ `TestBubbleTeaProgramWithContextReturnsContextCanceled` を追加し、モックが前提とする bubbletea の実際の挙動を実機構で検証することで解消した (このテストからさらに `tea.WithContext(ctx)` を取り除くと `declared and not used: ctx` でコンパイルが失敗することを確認済みで、空振りするテストではない)。
  - [優先度: 中] `TestSelectWith_ContextCancellationIsDetected` のコメントが、実際の bubbletea の挙動まで検証しているように読めた。`selectWith` によるエラーの素通しだけを検証している旨に書き直し、実際の挙動の検証は新設した統合テストが担うことを明記した。
  - [優先度: 低、対応不要] 新規の `Update` テストがテーブル駆動になっていない。レビュー自体もこのファイルの既存の `Update` テストの書き方 (テーブル駆動でない) と一致しているため妥当と判定しており、既存の書式に合わせるという判断を維持した。
  - [優先度: 低] `TestSelectWith_ContextCancellationIsDetected` のクロージャ内で `t.Errorf` を呼んでいた。渡された ctx を変数に受け取り、`selectWith` の呼び出しが終わった後にアサーションする形に直した。
- 観点 3 (堅牢性): 指摘なし。
- 観点 4 (規約準拠)
  - [優先度: 低、対応不要] 新規の `Update` テストがテーブル駆動でない指摘 (観点 2 と同一の指摘)。同じ理由で既存の書式に合わせる判断を維持した。
- 観点 5 (回帰と整合)
  - [優先度: 低、対応不要] `CHANGES.md` の「呼び出し側 2 箇所...は `commandContext(cmd)` を渡す」という記述が、`ec2.go` では実際には `commandContext(cmd)` の呼び出しが 1 段上のコード (`startEC2SessionWith`) にあり、`selectEC2Instance` はそこから渡された `ctx` をそのまま使っている点で厳密さを欠くという指摘。実害は無いとレビューでも判定されており、issue 0137 のレビューでも同種の指摘を過度に厳密として不採用とした前例と一致するため、そのままとした。

反映後、`mise run check` が通過することを確認した (脆弱性 0 件、backend / frontend の全テスト成功)。
