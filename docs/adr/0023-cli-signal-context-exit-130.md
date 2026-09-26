# 0023. CLI はシグナルで取り消せる context を全コマンドに渡し、中断を終了コード 130 で終える

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-08-09

## 状況

CLI のコマンドが `context.Background()` を 30 か所で使っており、Ctrl-C を押しても外部サービスの呼び出しが止まらなかった (issue 0131)。
シグナルに連動する context を入れた後も、context を見ない待ちは止まらなかった。
標準入力の読み取りは issue 0131 の中で、対話の選択 (`util.Select`) は issue 0138 で対応した。

## 決定

- `backend/cmd/thief/main.go` が `signal.NotifyContext` で SIGINT と SIGTERM に連動する context を作り、cobra の `ExecuteContextC` で全コマンドに渡す。
- 中断したら 1 行だけ表示し、終了コード 130 で終える (`backend/internal/cli/run.go` `interruptExitCode`)。
  usage の全文は表示しない。
- SSM のセッションの終了とサーバのシャットダウンは、中断の後も完了させる必要があるため、専用の短い期限の context を使う。
- `context.Background()` と、context を見ない標準入力の読み取りを使ってよい関数をテストで固定する (`TestNoRootContextOutsideDesignatedFunctions`、`TestNoContextBlindStdinReadOutsideDesignatedFunctions`)。

## 検討した代替案

- issue 0131: `RunE` ごとにシグナルに連動する context を作る (案 1)。
  `RunE` ごとに `stop` を呼ぶ責任が生まれ、30 か所すべてで漏れが無いことを保証できないため採らなかった。
  ルートコマンドで 1 度だけ作ってコマンドに渡す案 2 を採った。

## 結果

- Ctrl-C で、外部サービスの呼び出しを含むすべての処理が止まる。
- 新しいコマンドで `context.Background()` を使うと、テストが失敗する。

## 根拠資料

- `docs/issues/closed/0131`、`0136`、`0138`
- `backend/cmd/thief/main.go`、`backend/internal/cli/run.go` `commandContext`
- `AGENTS.md` の backend 「context.Context」
