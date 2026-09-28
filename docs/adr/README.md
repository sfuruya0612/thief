# ADR

thief の設計判断の記録。
形式と運用は [0001](0001-record-architecture-decisions.md) に定める。

| 番号 | 判断 | Status | Decided |
| --- | --- | --- | --- |
| [0001](0001-record-architecture-decisions.md) | 設計判断を ADR として後追いで記録する | Accepted | 2026-09-26 |
| [0002](0002-single-binary-server-and-cli.md) | 1 つの Go バイナリで API サーバと CLI を提供する | Accepted | 2026-07-12 |
| [0003](0003-browser-only-spa-monorepo.md) | backend と frontend を 1 つのリポジトリに置き、frontend はブラウザ専用の SPA にする | Accepted | 2026-07-08 |
| [0004](0004-minimal-dependencies.md) | 依存は標準ライブラリを優先して最小に保つ | Accepted | 2026-02-20 |
| [0005](0005-no-database-cache-strategy.md) | データベースを持たず、取得結果をメモリにキャッシュする | Accepted | 2026-07-08 |
| [0006](0006-frontend-state-management.md) | frontend の状態は TanStack Query と React の state で持ち、ルーターを入れない | Accepted | 2026-07-08 |
| [0007](0007-golden-json-type-contract.md) | backend と frontend の型の一致をゴールデン JSON で検査する | Accepted | 2026-08-05 |
| [0008](0008-raw-and-row-types.md) | frontend の型を Raw 型と Row 型の 2 層に分ける | Accepted | 2026-07-08 |
| [0009](0009-error-classification-by-code.md) | API のエラーを HTTP ステータスとコードで分類し、frontend はコードで分岐する | Accepted | 2026-07-08 |
| [0010](0010-local-api-without-authentication.md) | API サーバをループバックで待ち受け、利用者の認証を持たない | Accepted | 2026-07-08 |
| [0011](0011-read-header-timeout-only.md) | HTTP サーバのタイムアウトは ReadHeaderTimeout だけを設定する | Accepted | 2026-07-08 |
| [0012](0012-browser-terminal-over-websocket.md) | ブラウザのターミナルは Go で SSM のデータチャネルを中継し、常駐のドックに置く | Accepted | 2026-07-08 |
| [0013](0013-sso-device-authorization-in-ssoauth.md) | AWS SSO のデバイス認可を internal/ssoauth に集め、ログアウトで AWS 側のセッションも失効させる | Accepted | 2026-08-24 |
| [0014](0014-datadog-oauth-with-static-key-fallback.md) | Datadog は OAuth でログインし、親組織だけ静的キーに切り替える | Accepted | 2026-09-11 |
| [0015](0015-bounded-parallel-fetch-with-errgroup.md) | 項目ごとの詳細の取得を errgroup で上限付きの並列にする | Accepted | 2026-07-18 |
| [0016](0016-list-free-fields-lazy-drawer.md) | 一覧には主な Describe が返す項目だけを載せ、追加の取得は Drawer で行う | Accepted | 2026-07-08 |
| [0017](0017-read-only-sql-guard.md) | Athena と BigQuery のクエリは、書き込みの語を含むものを拒否する | Accepted | 2026-07-08 |
| [0018](0018-duckdb-wasm-object-query.md) | S3 と GCS のオブジェクトの SQL 検索を、ブラウザ内の DuckDB Wasm で行う | Accepted | 2026-09-25 |
| [0019](0019-official-icons-not-committed.md) | AWS と Google Cloud の公式アイコンをリポジトリに含めない | Accepted | 2026-07-08 |
| [0020](0020-i18n-scope.md) | UI の固定の文言を翻訳し、Drawer のタブ名と外部サービスのメッセージは英語のままにする | Accepted | 2026-07-21 |
| [0021](0021-mise-native-startup-without-docker.md) | Docker による起動をやめ、mise のタスクでネイティブに起動し、ポートを 8088 と 8089 にする | Accepted | 2026-07-18 |
| [0022](0022-quality-gates.md) | 品質ゲートを mise run check と pre-commit フックに置く | Accepted | 2026-07-08 |
| [0023](0023-cli-signal-context-exit-130.md) | CLI はシグナルで取り消せる context を全コマンドに渡し、中断を終了コード 130 で終える | Accepted | 2026-08-09 |
| [0024](0024-local-persistence-locations.md) | 端末に残すデータの置き場所をデータの種類ごとに決める | Accepted | 2026-07-08 |
| [0025](0025-two-pane-split-view.md) | 分割表示は 2 ペインまでとし、frontend だけで実装する | Accepted | 2026-09-25 |
| [0026](0026-withdraw-ec2-count-timeseries.md) | EC2 の台数の時系列グラフを撤回する | Accepted | 2026-09-18 |
| [0027](0027-issues-untracked.md) | issue を git で管理しない | Superseded by 0028 | 2026-07-12 |
| [0028](0028-issues-under-docs-tracked.md) | issue を docs/issues/ に移し、git で管理する | Accepted | 2026-09-26 |
| [0029](0029-object-list-delimiter-hierarchy.md) | オブジェクトの階層は API の区切り文字で 1 階層ずつ取得し、フラットモードを残す | Accepted | 2026-09-28 |
| [0030](0030-object-query-multiple-objects.md) | オブジェクトの SQL 検索は、同じ形式の複数のオブジェクトを読み取り関数のリスト引数で 1 つのビューにまとめる | Accepted | 2026-09-28 |
