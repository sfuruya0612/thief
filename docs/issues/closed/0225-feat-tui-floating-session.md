# TUI の EC2 / ECS セッションをフロートウィンドウで操作する

Created: 2026-10-26
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-10-26

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の EC2 Session Manager と ECS Exec のセッションを、全画面に切り替えず TUI 上のフロートウィンドウで操作したい

現在の実装は次のとおり。

- `backend/internal/tui/model.go` の `Deps.StartSession` と `Deps.StartECSExec` は `*exec.Cmd` と終了処理を返す。`sessionReadyMsg` の処理が `tea.ExecProcess` で子プロセスを実行する。`tea.ExecProcess` は TUI の代替画面を一時的に解除して端末を子プロセスへ渡すため、セッション中は一覧も Drawer も表示されない。
- `backend/internal/cli/tui.go` の `sessionManagerCommand` と `ecsExecCommand` は `session-manager-plugin` の引数を組み立て、AWS の Session Manager / ECS Exec のセッションを開始する。TUI のセッション開始は PATH 上のプラグインに依存している。
- Web UI は `backend/internal/api/handlers_session.go` から `backend/internal/session/bridge.go` を呼び、`backend/internal/session/datachannel.go` の `OpenDataChannel`、`Read`、`SendInput`、`SendSize` で SSM のデータチャネルを直接扱う。`backend/internal/aws/ssm_session.go` の `StartSSMSession` と `backend/internal/aws/ecs_exec.go` の `ExecuteECSCommand` が返す `StartSessionResult` を使う。
- `backend/internal/tui/program.go` の `Run` は `tea.NewProgram(...).Run()` の戻り値を捨てており、TUI の終了時にセッションを切断する経路が無い。

TUI も Web UI と同じ `internal/session` の DataChannel を使えば、子プロセスも PTY も挟まずにセッションを TUI の中で扱える。ただし端末出力を解釈して画面に収める仕組みが別に要る。

## 目的

TUI の EC2 Session Manager と ECS Exec のセッションを、TUI を終了せずに本文の中央に開くフロートウィンドウで操作できるようにする。セッションの入力、出力、リサイズ、切断を TUI の中で完結させ、`Ctrl-]` でウィンドウを閉じたときと TUI の終了時に AWS 側のセッションを終了する。

## 設計判断

- セッションは `backend/internal/session` の DataChannel をプロセス内で使う。
  - EC2 は `aws.StartSSMSession`、ECS は `aws.ExecuteECSCommand` で `StartSessionResult` を取り、`session.OpenDataChannel` でデータチャネルを開く。
  - `session-manager-plugin` は TUI では使わない。CLI の `thief ec2 session` と `thief ecs exec` は従来どおりプラグインを使う。
  - 現行の `tea.ExecProcess` で TUI を離れる方式を維持する案は採らない。フロートウィンドウで操作するという要望を満たさない。
  - プラグインを PTY で動かす案は採らない。`creack/pty` の追加、PTY のリサイズ、子プロセスの終了管理に加え、フロート表示のためだけに端末エミュレータも管理することになり、TUI のためだけにプロセスを挟む理由が無い。
  - 端末出力を ANSI のまま `lipgloss` の枠に流す案は採らない。カーソル移動、画面消去、代替画面の切替が TUI の描画を壊す。
- 端末表示には `github.com/charmbracelet/x/vt` v0.1.0 を使う。
  - 標準ライブラリと既存の依存に VT エミュレータが無く、VT の制御シーケンスを自前で実装するのはこの issue の完了条件を満たすには過大である。
  - `x/vt` は Charmbracelet の Bubble Tea と同じ系列で、`Emulator.Write` にデータチャネルの出力を渡し、`Render` で画面を文字列にできる。`Emulator.Resize`、`CursorPosition`、`CursorHidden` も公開されている。
  - `x/vt` は `backend/go.mod` と `backend/go.sum` に無い新しいモジュールである。v0.1.0 を固定し、MIT License、Charmbracelet による維持、Bubble Tea と同じ系列であることを理由に追加する。`go mod tidy` で `github.com/charmbracelet/ultraviolet` などの間接依存も入る。
  - `x/vt` は v0.1.0 であり、`backend/internal/tui/terminal.go` に閉じ込め、Model からは `TerminalScreen` インターフェース越しに使う。差し替えが必要になっても Model とセッション制御を変えずに済ませる。
- フロートウィンドウの重ね合わせには `github.com/charmbracelet/x/cellbuf` v0.0.15 を使う。
  - `cellbuf` は既に `lipgloss` の間接依存で、ANSI を含む文字列をセルに分解して矩形へ置き、再度 ANSI 付きの文字列に戻せる。`lipgloss` 単体には ANSI 付き文字列の重ね合わせ API が無い。
  - `cellbuf` を直接依存に加える。新しいモジュールではなく、既に `go.sum` にあるパッケージを直接使う。
- セッションは本文の中央に表示する。ユーザーによるウィンドウの移動とリサイズはできないが、端末のリサイズには追従してサイズを再計算する。ウィンドウの幅は本文の 80% 以下、高さは本文の 70% 以下とし、ウィンドウの最小サイズを 40 桁 x 12 行とする。開始判定は通知行を含めない本文サイズで行い、本文が 50 桁 x 18 行未満のときは開始しない。本文がちょうど 50 桁 x 18 行のときは開始できる。マウスによる移動と複数ウィンドウは扱わない。
- 同時に開けるセッションは 1 つとする。複数のフロートを並べるにはウィンドウの重なり順、フォーカス、入力の振り分けが必要になり、この issue の完了条件を超える。
- 入力は `tea.KeyMsg` から VT のバイト列に変換する。通常文字、Enter、Tab、Backspace、Esc、Ctrl キー、矢印、Home、End、PageUp、PageDown、Insert、Delete、F1 から F12、Alt 付きを対象にする。入力はセッションごとの writer goroutine とキューで直列化し、goroutine の終了経路を context で保証する。
- セッション中は `Ctrl-C` と `q` をリモートへ送る。ウィンドウを閉じるキーは `Ctrl-]` だけにする。`Ctrl-]` は `tea.KeyCtrlCloseBracket` で判別できる。
- リモートから `channel_closed` が届いたら最後の画面を残したまま `Session closed` と表示し、AWS 側のセッションを終了する。ウィンドウは `Ctrl-]` で閉じる。ユーザーが `Ctrl-]` を押したときは、その場でウィンドウを閉じて切断する。通知行の成功時の文言は `session closed`、失敗時は `session closed with error: <理由>` とし、ウィンドウ内の状態表示 `Session closed` とは別の文字列として扱う。
- `InteractiveSession.Close` は冪等にする。`Ctrl-]` で閉じた後に届く古い読み取り結果や、最終 Model の `Close` が二重に呼ばれても、`TerminateSSMSession` は 1 回だけ呼ぶ。`Close` の 2 回目は 1 回目の結果を返すか nil を返す。
- セッションには世代 ID を持たせ、`Ctrl-]` で閉じた後、リモートの `channel_closed` の後、同じ TUI で次のセッションを開始した後に届いた古い出力と終了通知を捨てる。
- `StartSSMSession` または `ExecuteECSCommand` が成功した後に `OpenDataChannel`、初期 `SendSize`、ハンドシェイクが失敗した場合は、開いた DataChannel を閉じ、短命 context で `TerminateSSMSession` を呼んでからエラーを返す。`InteractiveSession.Close` に DataChannel の Close と `TerminateSSMSession` をまとめ、途中失敗の後始末にも使う。`TerminateSSMSession` も失敗した場合は `session closed with error: <理由>` を通知行に表示する。
- 開始前の失敗と開始後の後始末の失敗を区別するため、`backend/internal/cli/tui.go` のアダプタは「AWS 側のセッションを開始済みか」を持つエラー型を返す。Model は開始前なら `session: <理由>`、開始済みなら `session closed with error: <理由>` を通知行に表示する。
- フロートウィンドウのサイズは、通知行の出現と消滅では変えない。`tea.WindowSizeMsg` を受けたときだけ本文のサイズから再計算し、`TerminalScreen.Resize` と `SendSize` を呼ぶ。`Session closed` の表示で最後の画面がリフローしないようにするためである。
- 端末のリサイズ時は `Emulator.Resize` と `DataChannel.SendSize` の両方を呼ぶ。`SendSize` はハンドシェイク完了までブロックするため、Update から直接呼ばず writer goroutine に渡す。
- `program.go` の `Run` は `tea.NewProgram(...).Run()` が返す最終 Model を受け取り、終了時に有効なセッションがあれば `Close` を呼ぶ。外部シグナルで TUI を終了した場合も AWS 側のセッションを残さない。セッション中の `q` と `Ctrl-C` はリモートへ送るため、これらで TUI は終了しない。
- 追加する API 呼び出しは `StartSession`、`TerminateSession`、`ExecuteCommand` で、CLI と Web UI が既に使う権限の範囲に収まる。新しい IAM 権限は要らない。
- `StartSessionResult.TokenValue` とセッション出力はログに出さない。`internal/aws` の既存のコメントと同じ扱いにする。

## 完了条件

- `backend/internal/tui` に `InteractiveSession` インターフェース（`Read`、`SendInput`、`SendSize`、`Close`）と `TerminalScreen` インターフェース（`Write`、`Resize`、`Render`、`Cursor`）があり、`backend/internal/cli/tui.go` のアダプタが `session.OpenDataChannel` と `aws.StartSSMSession` / `aws.ExecuteECSCommand` を組み合わせている。
- `backend/internal/tui/model.go` の `Deps` から `StartSession` と `StartECSExec` の `*exec.Cmd` が無くなり、TUI のセッション開始が `tea.ExecProcess` を呼ばない。
- EC2 の行で `s` を押すか、ECS のクラスタ行で `s` を押してタスクとコンテナを選ぶと、TUI の本文の中央にフロートウィンドウが開き、その中にセッションの出力が表示される。
- フロートウィンドウのヘッダに接続先（EC2 のインスタンス ID、または ECS のクラスタ / タスク / コンテナ）とプロファイル / リージョン、フッタに `Ctrl-]` で閉じる旨を表示する。
- ウィンドウ内で `Ctrl-C` を押すと `\x03` がリモートへ送られ、TUI は終了しない。
- ウィンドウ内で `q` を押すと `q` がリモートへ送られ、TUI は終了しない。
- ウィンドウ内で `Ctrl-]` を押すとウィンドウが閉じ、`TerminateSSMSession` がセッション ID で呼ばれ、通知行に `session closed` が表示される。
- リモートで `exit` してデータチャネルが閉じると、最後の画面を残したまま `Session closed` と表示され、AWS 側のセッションが終了する。この状態では `Ctrl-]` だけを受け付け、他のキーは無視する。`Ctrl-]` でウィンドウを閉じる。
- ウィンドウはユーザーによる移動とリサイズができず、端末のリサイズに追従する。本文の幅が 80% 以下、高さが 70% 以下、かつ 40 桁 x 12 行以上になる。
- 端末のリサイズ後に `TerminalScreen.Resize` が新しい内側の桁数と行数で呼ばれ、`SendSize` が同じ桁数と行数で呼ばれる。通知行の出現と消滅ではウィンドウのサイズが変わらず、`Resize` と `SendSize` を呼ばない。
- 開始判定は通知行を含めない本文サイズで行う。本文が 50 桁 x 18 行未満のときはセッションを開始せず、通知行に理由を表示する。TUI は終了しない。本文がちょうど 50 桁 x 18 行のときは開始できる。
- セッションの開始前の失敗は通知行に `session:` で始まる理由、開始後の後始末の失敗は `session closed with error:` で始まる理由を表示し、`sessionStarting` が戻って `s` を再び受け付ける。
- ウィンドウを開いたまま `SIGTERM` または `kill -INT` で TUI を終了したとき、最終 Model の `Close` が呼ばれ、AWS 側のセッションが残らない。`Ctrl-]` で先に閉じた場合はセッションが無いため、最終 Model の `Close` は何もしない。
- `InteractiveSession.Close` を 2 回呼んでも `TerminateSSMSession` は 1 回だけ呼ばれる。
- `StartSSMSession` または `ExecuteECSCommand` の成功後に `OpenDataChannel`、初期 `SendSize`、ハンドシェイクが失敗したとき、開いた DataChannel が閉じられ、`TerminateSSMSession` が短命 context で呼ばれる。`TerminateSSMSession` も失敗したときは `session closed with error:` で始まる通知が表示される。
- `backend/internal/tui` の単体テストに、出力描画、入力バイト列（通常文字、Enter、Tab、Backspace、Esc、Ctrl キー、矢印、Home、End、PageUp / PageDown、Insert、Delete、F1 から F12、Alt 付き、日本語）、`Ctrl-C` と `q` のリモート送信、`Ctrl-]` の終了、リモートの `exit` 後の `Session closed` と `Ctrl-]` 以外のキーの無視、`TerminalScreen.Resize` と `SendSize`、通知行の出現と消滅でリサイズしないこと、開始前の失敗と開始後の後始末の失敗の通知文言の区別、開始後のチャネル失敗と DataChannel の Close と `TerminateSSMSession`、終了失敗、二重 `Close`、古い出力と古い終了通知の破棄、同時 1 セッション、終了時の `Close`、50 桁 x 18 行ちょうどの開始、49 桁 x 18 行と 50 桁 x 17 行の開始拒否の各ケースがある。
- `backend/internal/cli/tui_test.go` に、EC2 / ECS のアダプタが `StartSSMSession` / `ExecuteECSCommand` と `OpenDataChannel` を正しい識別子で呼び、`Close` が `TerminateSSMSession` を短命 context で呼ぶことを確かめるテストがある。
- `session-manager-plugin` を PATH から外した状態でも TUI の EC2 / ECS セッションが開始できることを、プラグイン解決を呼ばない実装と単体テストで確認する。CLI のプラグイン経路は変えない。
- TUI で使わなくなる `backend/internal/cli/tui.go` の `sessionManagerCommand`、`sessionManagerCommandWith`、`ecsExecCommand`、`ecsExecCommandWith`、`tuiSessionDeps`、`defaultTUISessionDeps` と、それらに対応する `backend/internal/cli/tui_test.go` のテストを削除する。CLI の `thief ec2 session` と `thief ecs exec` が使う `ec2.go`、`ecs.go` のプラグイン経路は残す。
- `backend/internal/cli/run_test.go` の `TestNoRootContextOutsideDesignatedFunctions` の許可リストを更新する。TUI の `Close` が短命 context を作る関数を追加し、削除した `sessionManagerCommandWith` と `ecsExecCommandWith` を外す。
- `docs/prd/thief.md` の FR-30 がプロセス内 DataChannel とフロートウィンドウの仕様に更新され、プラグインを前提とする記述と受け入れ基準が残っていない。あわせて「外部仕様の前提」にある TUI のセッションが `session-manager-plugin` を起動するという記述を、TUI と Web UI はプロセス内 DataChannel、CLI はプラグインという書き分けに更新する。
- `docs/prd/thief.md` の「関連資料」に ADR 0034 を追加している。
- `docs/adr/0034-tui-floating-terminal-window.md` を追加し、`docs/adr/README.md` の一覧に 0034 の行を追加する。ADR 0034 は ADR 0033 のフレームワークと取得層の決定を引き継ぎ、セッション方式だけを置き換える。ADR 0001 の規則に従い、ADR 0033 の `Status` を `Superseded by 0034` にし、`docs/adr/README.md` の 0033 の行も `Superseded by 0034` に更新する。
- `AGENTS.md` の「プロジェクト構造」の TUI の項目に、フロート端末の実装と `x/vt` / `cellbuf` の依存理由、同時 1 セッション、`Ctrl-]` の操作が追記されている。
- `example/README.md` の「動作確認できない機能」が、TUI と Web UI のセッションは実エージェントとの DataChannel 接続が必要で floci では確認できないこと、CLI の `thief ec2 session` と `thief ecs exec` は引き続き `session-manager-plugin` が必要であることを書き分けている。
- `CHANGES.md` の `## develop` に、TUI のフロートセッションの [ADD] エントリが 1 つある。担当者行は `- @sfuruya0612` とする。
- 実 AWS アカウントで、SSM Agent が動く EC2 インスタンスの行から `s` を押すとフロートウィンドウにシェルが表示され、`echo ok` が `ok` を返す。`Ctrl-]` の後、`aws ssm describe-sessions --state Active` にそのセッションが残らない。
- 実 AWS アカウントで、ECS Exec を有効にしたタスクのコンテナを選ぶとフロートウィンドウにシェルが表示され、`echo ok` が `ok` を返す。`Ctrl-]` の後にセッションが残らない。
- `mise run check` が通る。
- 扱わない範囲: 複数セッションの同時表示、ウィンドウのドラッグやユーザーによるリサイズ、スクロールバックの表示、TUI での `session-manager-plugin` の利用、CLI のセッション方式の変更。

## 関連

- PRD `docs/prd/thief.md` の FR-30
- ADR 0033 (`docs/adr/0033-tui-in-process-bubbletea.md`)
- `docs/issues/closed/0218`、`docs/issues/closed/0224`
- `backend/internal/session/datachannel.go`、`backend/internal/session/bridge.go`
- `backend/internal/aws/ssm_session.go`、`backend/internal/aws/ecs_exec.go`

## 解決方法

- `backend/internal/tui/session.go` を追加し、`InteractiveSession`、`SessionOutput`、`SessionStartError`、`sessionWindow`、`sessionRuntime`、`sessionStartSlot` を実装した。
  `sessionRuntime` は入力とリサイズの書き込みを 1 本の writer goroutine で直列化し、送信の失敗を `lastErr` に保持して読み取り側へ渡す。`InteractiveSession.Close` を冪等にした。
  `sessionStartSlot` は、開始処理が作ったセッションを Update が受け取るまで保持し、TUI が開始処理の途中で終了した場合もそのセッションを閉じる。
- `backend/internal/tui/terminal.go` を追加し、`github.com/charmbracelet/x/vt` の `Emulator` を `TerminalScreen` で包んだ。
  `github.com/charmbracelet/x/cellbuf` でフロートウィンドウを本文中央に重ね、カーソルを反転セルで描いた。
- `backend/internal/tui/keys.go` を追加し、bubbletea のキーを VT のバイト列へ変換した。
  通常文字、Alt、Enter、Tab、Backspace、Esc、Ctrl キー、矢印、Home、End、PageUp、PageDown、Insert、Delete、F1 から F12、Shift 付きの矢印と Home / End を扱った。
- `backend/internal/tui/model.go` の `Deps` を `OpenEC2Session` と `OpenECSExecSession` に変え、`tea.ExecProcess` と `*exec.Cmd` の経路を削除した。
  EC2 と ECS の両方で、開始前に本文が 50 桁 x 18 行以上あるかを確認し、小さい場合は AWS のセッションを開始しない。
  `sessionReadyMsg`、`sessionOutputMsg`、`sessionClosedMsg`、`sessionCleanupMsg` を世代付きにした。
  出力の反映、リモートの終了、`Ctrl-]`、TUI 終了時の後始末を実装した。
  フロートウィンドウのヘッダには接続先、プロファイル、リージョンを表示する。
  `Ctrl-]` 以外のキーはセッションへ送り、リモート終了後のウィンドウでは `Ctrl-]` だけを受け付ける。
  読み取りの失敗は `Close` の結果と結合して通知行に `session closed with error: <理由>` を出し、初期 `SendSize` と `SendInput` の失敗も同じ通知に載せる。
  ユーザーが `Ctrl-]` で閉じたセッションの終了結果だけを通知するため、`closedSessionSeq` で世代を確認する。
- `backend/internal/tui/program.go` の `Run` が最終 Model を受け取り、終了時に `shutdownSession` を呼ぶようにした。
- `backend/internal/cli/tui.go` に `tuiSession` アダプタを追加し、`aws.StartSSMSession` / `aws.ExecuteECSCommand`、`session.OpenDataChannel`、`aws.TerminateSSMSession` を接続した。
  `session-manager-plugin` を使う TUI 用の `sessionManagerCommand`、`ecsExecCommand` と `tuiSessionDeps` を削除し、新しいアダプタの依存は `tuiAdapterDeps` に改名した。
  `tuiSession.Close` は呼び出し元の context から切り離した短命 context で `TerminateSSMSession` を呼ぶ。
  CLI のプラグイン経路は残した。
- `backend/internal/cli/run_test.go` の `TestNoRootContextOutsideDesignatedFunctions` の許可リストから削除した TUI のプラグイン関数を外し、開始後の後始末で短命 context を作る `tui.go:terminateTUISession` を追加した。`tuiSession.Close` は `context.WithoutCancel` で呼び出し元のキャンセルを切り離すため、`context.Background` を使わず許可リストに含めない。
- `backend/go.mod` と `backend/go.sum` に `github.com/charmbracelet/x/vt` v0.1.0 と `github.com/charmbracelet/x/cellbuf` v0.0.15 を直接依存として追加した。`github.com/charmbracelet/x/ansi` は `ansi.Truncate` を使うため、間接依存から直接依存に移した。
- `docs/prd/thief.md` の FR-30 と「外部仕様の前提」を、TUI はプロセス内 DataChannel、CLI はプラグインという書き分けに更新した。
  同じファイルの「関連資料」に ADR 0034 を追加した。
- `docs/adr/0034-tui-floating-terminal-window.md` を追加し、`docs/adr/README.md` に 0034 を追加した。
  ADR 0033 の Status を `Superseded by 0034` にした。
- `AGENTS.md` の「プロジェクト構造」の TUI の項目に、フロート端末、`x/vt` / `cellbuf`、同時 1 セッション、`Ctrl-]` を追記した。
- `example/README.md` の「動作確認できない機能」を、Web UI と TUI は DataChannel、CLI は `session-manager-plugin` という書き分けに更新した。
- `CHANGES.md` の `## develop` に [ADD] エントリを追加した。

完了条件の検証:

- `backend/internal/tui` の `session_test.go` と `model_test.go` に、出力描画、キー変換、`Ctrl-C` / `q` のリモート送信と TUI を終了しないこと、`Ctrl-]` の終了、リモート終了後のキー無視、リサイズ、通知行で `Resize` と `SendSize` を呼ばないこと、ウィンドウを開いたときの View の高さと下枠の保持のテストを追加した。
  開始前と開始後の失敗の区別、初期 `SendSize` と `SendInput` の失敗、読み取りエラー、古い出力・古い開始結果・古い終了通知の破棄、同時 1 セッション、TUI 終了時の `Close`、`sessionStartSlot` の late set、50 桁 × 18 行の境界もテストした。
- `backend/internal/cli/tui_test.go` に、EC2 / ECS のアダプタが `StartSSMSession` / `ExecuteECSCommand` と `OpenDataChannel` を正しい識別子で呼ぶこと、`Close` が `TerminateSSMSession` を短命 context で呼ぶこと、`Close` が冪等であること、`OpenDataChannel` の失敗で切断することのテストを追加した。
- `backend/` で `go test -race -cover ./internal/tui/ ./internal/cli/` を実行し、`internal/tui` が ok (coverage 94.2%)、`internal/cli` が ok (coverage 51.0%) だった。
- `mise run backend:lint` が通った。
- `mise run check` は `backend:vuln` で、issue 0218 から続く既知の 12 件 (GO-2026-6809、6617、6613、6612、6611、6610、6608、6607、6605、6603、6600、6599) により失敗した。
  今回の変更と `x/vt` / `x/cellbuf` / `x/ansi` の直接依存化に由来する新たな脆弱性は検出されていない。
- `session-manager-plugin` を PATH から外した状態でも TUI がプラグイン解決を呼ばないことを、`TestOpenTUIEC2SessionWith_NoPluginLookup` で確認した。
  `newTUIDeps` が `openTUIEC2Session` / `openTUIECSExecSession` を配線し、アダプタが `session.OpenDataChannel` だけを使う。
- 実 AWS アカウントでの EC2 Session Manager と ECS Exec の接続、`echo ok`、`Ctrl-]` 後の `aws ssm describe-sessions --state Active` は、実アカウントの認証情報がこの実行環境に無いため確認していない。
  `example/` の floci では DataChannel の実エージェント接続を確認できない。
  この 2 点は未確認として残す。

方針からの乖離:

- なし。

## 動作確認

- 実 AWS アカウントでの EC2 Session Manager と ECS Exec の接続、`echo ok`、`Ctrl-]` 後の `aws ssm describe-sessions --state Active` は、実アカウントの認証情報がこの実行環境に無いため未確認である。
- `example/` の floci では DataChannel の実エージェント接続を確認できない。
- 上記の 2 点は、実 AWS のプロファイルと SSM Agent / ECS Exec を有効にしたタスクを持つ環境で開発者が確認する。この issue ではコードと単体テストまでを完了とし、実接続の確認は未完了の残余リスクとして記録する。
