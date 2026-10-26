# 0034. TUI の対話セッションはプロセス内の DataChannel と端末エミュレータでフロート表示する

Created: 2026-10-26
Model: DeepSeek V4.1 Flash 1017
Status: Accepted
Decided: 2026-10-26

## 状況

ADR 0033 は、TUI を bubbletea で作り、取得層をプロセス内で直接呼ぶことを決めた。
その中で EC2 と ECS の対話セッションは、CLI と同じく PATH にある `session-manager-plugin` に端末を渡し、bubbletea の `tea.ExecProcess` で実行する決定だった。
この方式では、セッションの間は TUI の代替画面を解除して子プロセスへ端末を渡すため、TUI の一覧と Drawer は表示されない。

利用者から、ECS Exec と EC2 Session Manager のセッションを、TUI を離れずにフロートウィンドウで操作したいという要望が出た。
Web UI は既に `backend/internal/session` の DataChannel をプロセス内で扱い、SSM のデータチャネルと端末の入出力を中継している。
TUI でも同じ DataChannel を使えば、子プロセスを挟まずにセッションを TUI の中で描画できる。ただし端末の制御シーケンスを解釈して画面に収める仕組みが必要になる。

## 決定

- TUI の EC2 Session Manager と ECS Exec のセッションは、`session-manager-plugin` を使わず、`backend/internal/session` の DataChannel を TUI のプロセス内で使う。
  EC2 は `aws.StartSSMSession`、ECS は `aws.ExecuteECSCommand` で `StartSessionResult` を取り、`session.OpenDataChannel` でデータチャネルを開く。
- 端末の表示には `github.com/charmbracelet/x/vt` v0.1.0 の VT エミュレータを使う。
  エミュレータは `backend/internal/tui/terminal.go` に閉じ込め、Model は `TerminalScreen` インターフェース越しに使う。
  `x/vt` は MIT License で、Bubble Tea と同じ Charmbracelet が維持する。標準ライブラリと既存の依存には VT の制御シーケンスを解釈する実装が無いため、新規のモジュールとして追加する。
- フロートウィンドウの重ね合わせには `github.com/charmbracelet/x/cellbuf` v0.0.15 を使う。
  `cellbuf` は MIT License で、既に `lipgloss` の間接依存である。ANSI 付きの文字列をセルに分解して矩形へ置き、再度 ANSI 付きの文字列に戻せる。`lipgloss` 単体にはこの重ね合わせの API が無いため、直接依存にする。
- セッションは本文の中央に表示する。ウィンドウの幅は本文の 80% 以下、高さは 70% 以下、最小 40 桁 x 12 行とする。
  本文が 50 桁 x 18 行未満のときは開始しない。ユーザーによる移動とリサイズはできず、端末のリサイズには追従する。
- セッション中は `Ctrl-]` だけをウィンドウの操作に使い、それ以外のキーはリモートへ送る。
  `Ctrl-C` と `q` はリモートへ送り、TUI を終了させない。
- 入力はセッションごとの writer goroutine とキューで直列化する。`SendSize` はハンドシェイク完了までブロックするため、Update から直接呼ばない。
- 同時に開けるセッションは 1 つとする。
- `Ctrl-]`、リモートの `channel_closed`、TUI の `SIGTERM` / `kill -INT` のいずれでも、AWS 側のセッションを `TerminateSSMSession` で終える。
  `InteractiveSession.Close` は冪等にし、二重に呼ばれても `TerminateSSMSession` は 1 回だけ呼ぶ。
- ADR 0033 のフレームワーク (bubbletea / bubbles / lipgloss) と取得層をプロセス内で直接呼ぶ決定は引き継ぐ。
  ADR 0033 のうちセッション方式の決定だけをこの ADR で置き換えるため、ADR 0033 の Status を `Superseded by 0034` にする。

## 検討した代替案

- `session-manager-plugin` を PTY で動かし、その出力を端末エミュレータに食わせる。
  `creack/pty` の追加、PTY のリサイズ、子プロセスの終了管理、プラグインの PATH 依存が残る。
  TUI のためだけにプロセスを挟む理由が無く、Web UI と TUI でセッションの実装が二重になるため採らない。
- 端末出力を ANSI のまま `lipgloss` の枠に流す。
  カーソル移動、画面消去、代替画面の切替が TUI の描画を壊すため採らない。
- 現行の `tea.ExecProcess` で TUI を離れる方式を維持する。
  フロートウィンドウで操作するという要望を満たさないため採らない。
- 複数のセッションを同時にフロート表示する。
  ウィンドウの重なり順、フォーカス、入力の振り分けが必要になり、最初の実装の範囲を超えるため、同時 1 セッションに限る。

## 結果

- TUI は `session-manager-plugin` が PATH に無くても EC2 Session Manager と ECS Exec のセッションを開始できる。
  CLI の `thief ec2 session` と `thief ecs exec` は引き続きプラグインを使う。
- `github.com/charmbracelet/x/vt` が直接依存に加わる。
  `github.com/charmbracelet/ultraviolet` などの間接依存も加わる。
  `github.com/charmbracelet/x/cellbuf` は既に間接依存で、直接利用になる。
- 端末エミュレータは v0.1.0 であり、すべての端末制御シーケンスを再現することを保証しない。
  `terminal.go` のインターフェースを境界にして、差し替えや修正を局所化する。
- TUI のセッションはスクロールバックの表示、複数セッション、ウィンドウのドラッグやユーザーによるリサイズを扱わない。
- TUI は引き続き API サーバを経由せず、TCP の待ち受けを持たない。
- セッションのトークンと端末出力はログに出さない。

## 根拠資料

- PRD `docs/prd/thief.md` の FR-30、「外部仕様の前提」、「決定した論点」
- ADR 0012、ADR 0033
- `backend/internal/session/datachannel.go`、`backend/internal/session/bridge.go`
- `backend/internal/aws/ssm_session.go`、`backend/internal/aws/ecs_exec.go`
- `backend/internal/tui/model.go`、`backend/internal/tui/session.go`、`backend/internal/tui/terminal.go`
- `backend/internal/cli/tui.go`
- `github.com/charmbracelet/x/vt` v0.1.0: https://github.com/charmbracelet/x/tree/main/vt
- `github.com/charmbracelet/x/cellbuf` v0.0.15: https://github.com/charmbracelet/x/tree/main/cellbuf
