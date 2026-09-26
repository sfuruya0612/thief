# gcp サブコマンドのテーブル出力を printRowsOrGroupBy に共通化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/cli/gcp.go` の 6 関数 (`printGCPProjects` / `gcpRunCloudRun` / `gcpRunBuckets` / `gcpRunIAMBindings` / `gcpRunServiceAccounts` / `gcpRunObjects`) が、末尾で同一のテーブル整形ブロックを手書きしている。

```go
f := util.NewTableFormatter(cols, cfg.Output)
if !cfg.NoHeader {
    f.PrintHeader()
}
f.PrintRows(rows)
return nil
```

これは `helper.go` の共通関数 `printRowsOrGroupBy` の本体 (GroupBy 分岐を除いた部分) そのものであり、他の全 CLI コマンド (ec2 / rds / ecr / cost / bq / tidb / datadog 等) は `printRowsOrGroupBy` 経由で出力している。gcp だけがこの共通化から外れている。

さらに、root コマンドは `--group-by` / `-g` を永続フラグとして定義しており `loadConfig` は gcp コマンドでも `cfg.GroupBy` を populate するが、gcp の 6 関数は `cfg.GroupBy` を完全に無視している。つまり gcp コマンドでは `-g` が現状サイレントに no-op になっており、共通化はこの潜在バグの修正を兼ねる。

## 対応方針 (対応保留)

各関数の末尾整形ブロックを `return printRowsOrGroupBy(cfg, cols, rows)` に置換する。

## 保留の理由

`printRowsOrGroupBy` への置換により、gcp コマンドで `-g` 指定時の出力が「無視 (通常テーブル)」から「グルーピング集計テーブル」へ変わる。`-g` 未指定時の出力は完全に同一だが、CLI の出力挙動が変わる変更のため、本タスク (出力・表示を変えないリファクタリング) の対象外とし issue 起票のみとする。対応時は `-g` を機能させる仕様で問題ないかを確認すること。

## 解決方法

ユーザーから対応の明示指示を受け、`-g` を機能させる仕様で実装した。

- `gcp.go` の 6 関数 (`printGCPProjects` / `gcpRunCloudRun` / `gcpRunBuckets` / `gcpRunIAMBindings` / `gcpRunServiceAccounts` / `gcpRunObjects`) の末尾テーブル整形ブロックを `return printRowsOrGroupBy(cfg, cols, rows)` に置換した。
- これにより gcp コマンドでも他の全 CLI コマンドと同様に `--group-by` (`-g`) が機能する (サイレント no-op の解消)。`-g` 未指定時の出力は従来と完全に同一。
- `mise run check` 通過を確認した。
