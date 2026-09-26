# gcp サブコマンドが config ファイルを二重ロードする無駄を解消する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/cli/gcp.go` の `gcpRequireProjectID` (118-131 行) は内部で `loadConfig(cmd)` を呼ぶ。ところが、これを呼ぶ以下の 5 関数は直後に再度 `loadConfig(cmd)` を呼んでいる。

- `gcpRunCloudRun` (197 行 + 201 行)
- `gcpRunBuckets` (230 行 + 234 行)
- `gcpRunIAMBindings` (262 行 + 266 行)
- `gcpRunServiceAccounts` (293 行 + 297 行)
- `gcpRunObjects` (324 行 + 328 行)

`loadConfig` → `config.Load()` は `config.yaml` の `os.ReadFile` + YAML パースを伴うため、1 コマンド実行あたり設定ファイルの読み込みとパースが 2 回走る。

## 対応方針

`gcpRequireProjectID` のシグネチャを `(cmd *cobra.Command, cfg *config.Config) (string, error)` に変更し、呼び出し側で 1 回だけロードした cfg を渡して再利用する。

## CLI 出力 / API レスポンス / 画面表示への影響

なし。config ロードは決定的処理であり、2 回を 1 回に減らしても解決される値は同一。

## 解決方法

- `backend/internal/cli/gcp.go`: `gcpRequireProjectID` のシグネチャを `(cmd *cobra.Command, cfg *config.Config) (string, error)` に変更し、内部の `loadConfig` 呼び出しを削除した。
- 呼び出し元 5 関数 (gcpRunCloudRun / gcpRunBuckets / gcpRunIAMBindings / gcpRunServiceAccounts / gcpRunObjects) は先に `loadConfig(cmd)` を 1 回だけ実行し、その cfg を `gcpRequireProjectID` に渡すようにした。
- `mise run check` 全通過を確認した。
