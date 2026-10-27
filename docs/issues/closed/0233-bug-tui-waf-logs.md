# TUI で WAF を開くと取得層のログが画面に出る

Created: 2026-10-27
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-10-27

## 背景

TUI (`thief tui`) で WAF のサービスを開くと、`backend/internal/aws/waf.go` の計測用 `slog.Info` が stderr に出力され、代替画面の描画に混ざってログが表示される。WAF の一覧取得は `ListWAFResources` と `listWAFACLs` で 4 つの `slog.Info` を出しており、TUI の起動時に既定のロガーを画面向けに制御していないためである。

同じ計測用の `slog.Info` は SQS、IAM、Kinesis、DynamoDB の一覧取得にもあり、これらのサービスを開いたときにも同じ問題が起きる。

## 再現手順

1. `mise run backend:install` で `thief` を導入する
2. 実 AWS のプロファイルで `thief tui -p <profile>` を起動する
3. `S` でサービスの選択肢を開き、`WAF` を選んで Enter を押す
4. WAF の一覧を取得する間に `waf clients created`、`waf list web acls done`、`waf get web acl details done`、`waf list all done` のようなログが画面に表示される

## 原因

- `backend/internal/aws/waf.go` などの計測用ログが `slog.Info` で出ており、既定のロガーは stderr に書く。
- `backend/internal/tui/program.go` の `Run` が、TUI の実行中に既定のロガーを画面へ出さないよう制御していない。

## 解決方法

- `backend/internal/tui/program.go` の `Run` で、TUI の実行中だけ既定のロガーを `slog.DiscardHandler` に置き換え、終了時に元へ戻すようにした。TUI の代替画面に取得層や SDK のログが混ざらない。
- `backend/internal/aws/waf.go`、`sqs.go`、`iam.go`、`kinesis.go`、`dynamo.go` の計測用 `slog.Info` を `slog.Debug` に下げた。計測ログは既定の Info レベルでは出ず、CLI やサーバで必要なときだけ Debug を有効にして確認できる。
- `backend/internal/tui/program_test.go` で、`discardLogs` の実行中はログが出ず、戻した後に出ることを確認した。
- `mise run check` が通る。
