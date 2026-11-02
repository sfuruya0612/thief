# Web に Access Logs ビューを追加する

Created: 2026-11-02
Model: DeepSeek V4.1 Flash 1017

## 背景

ADR 0037 は、AWS ビューの Security グループに Access Logs を追加し、ALB、CloudFront、WAF のアクセスログをアクセス元の IP とリクエスト URI で集計することを決めた。
バックエンドは issue 0238 までで、ソースの検出、テーブルの検証、Athena と Logs Insights の集計、ジョブの管理を提供する。
フロントエンドには、ソースを選び、テーブルを対応付け、期間とフィルタを指定して集計を実行し、結果を読む画面が無い。

## 目的

`Access Logs` を AWS ビューのサービスとして追加し、ソースのセットアップ、集計の実行、結果の表示を行う。

## 設計判断

- `frontend/src/lib/serviceMeta.ts` の `SERVICES` に `accesslogs` を追加する。
  - `name`: `Access Logs`
  - `sub`: `Threat check`
  - `group`: `security`
  - `color`: `var(--svc-accesslogs)`
  - `SERVICE_TO_PATH` には追加しない。専用ビューとして `AccountView` の `AwsServicePane` から描画する。
  - アイコンは AWS 公式アイコンではなく、`components/icons/Icons.tsx` の汎用のシールドを使う。
  - サービス数が 23 から 24 になるため、`serviceMeta.test.ts` と `Sidebar.test.tsx` を更新する。
- `AccessLogsView` を `frontend/src/views/AccessLogsView.tsx` に追加する。
  - `key={`${profile}:${region}`}` でリージョン変更時に再マウントする。
  - 左ペインにソースの一覧、右ペインに期間、フィルタ、サマリ、結果のタブを置く。
  - タブは `IP x URI`、`Source IP`、`Setup` とする。
  - `IP x URI` は `(client_ip, uri_path)` の行を表示する。
  - `Source IP` は IP 単独の集計を表示する。
  - `Setup` はソースごとの出力先、テーブルの選択、DDL のコピー、検証の結果、必要権限を表示する。
- ソースの状態は、`logging_enabled`、`destination_type`、`destination_readable`、`table_configured`、`table_valid`、`data_present`、`last_delivery_at`、`issues` を個別のバッジで表示する。
  - ログが無効、未対応の形式、権限不足、配信の遅延を 1 つの「利用不可」にまとめない。
  - ログが無効なソースは一覧に残し、「Logging disabled」と有効化の手順を表示する。
    集計の対象には選べない。
  - すべてのソースが無効な場合は、結果の領域にセットアップの案内を表示し、実行ボタンを無効にする。
  - ログは有効だがデータがまだ無い場合は、「No data yet」と配信の遅延の目安を表示する。
  - CloudFront と CLOUDFRONT スコープの WAF は Global と表示する。
- 同じ出力先を共有するソースは、ソースの一覧で「Shared」の印と、共有するリソースの一覧を表示する。
  共有するソースを選ぶと、同じテーブルまたはロググループを 1 回だけ選べるようにする。
  `Attribution` が false のソースは、共有するほかのソースと同時に選べないようにし、理由を表示する。
- テーブルの対応付けは、検出した出力先の ID ごとに localStorage に保存する。
  キーは `cloudlens:accesslogs:v1:<profile>:<region>` とする。
  同じ出力先を共有する複数のソースは、1 つのテーブルの対応付けを共有する。
  既存の Athena のカタログ、データベース、テーブルのフック (`useAthenaCatalogs`、`useAthenaDatabases`、`useAthenaTables`) を再利用する。
- DDL はバックエンドが返す文字列をモーダルに表示し、クリップボードにコピーできるようにする。
  DDL にはパーティション投影を含める。
  DDL を実行するのは利用者であり、画面から Athena のクエリを実行しない。
  CloudFront の標準ログ v2 の Parquet は、Glue Crawler の手順を表示し、できたテーブルを選ぶ。
- 期間は 1h、3h、24h、7d、カスタムとする。
  既定は 24h とする。
  7d は確認のチェックを入れたときだけ実行できる。
  WAF の CloudWatch Logs のソースが含まれる場合は 24h を上限とする。
- フィルタは、WAF のブロックだけ、4xx と 5xx、IP の完全一致、URI の前方一致とする。
  自由文のフィルタは v1 では扱わない。
- 集計のジョブ ID は sessionStorage に保存し、再読み込みで同じジョブの状態を取得できるようにする。
  ジョブの状態は 1 秒から 2 秒間隔でポーリングし、終端状態になったら結果を取得する。
  結果は TanStack Query の `staleTime: Infinity` で保持し、新しい実行のときに破棄する。
- 結果の列は次のとおりとする。
  - IP x URI: Source IP、URI、Requests、Blocked、Challenged、4xx、5xx、Sources、Last seen、Flags
  - Source IP: Source IP、Requests、Blocked、Challenged、4xx、5xx、Distinct URIs、Sources、Last seen
  `DataTable` と `ColumnDef` を使い、`CellBar` と `StatusBadge` を再利用する。
- サマリはソース別の件数を表示し、ソースをまたいだ単一の合計を出さない。
  複数のソースを選んだ場合は「ログのレコード数であり、重複を含む場合がある」と表示する。
  CloudFront の origin の ALB が選ばれている場合は、ALB が IP の集計から外れることを表示する。
- 空の結果は、ログが無効、配信の遅延、テーブルの不一致、パーティションの欠落、権限不足を切り分けて案内する。
- SSO の期限切れは `SSOExpiredBanner`、部分失敗はソースごとの `ErrorBanner`、全体の失敗は `ErrorBanner` で表示する。
- i18n は `frontend/src/i18n/locales/{ja,en}/accesslogs.json` に追加する。
  ja と en のキー集合を一致させる。
- CSS は `frontend/src/styles/features/accesslogs.css` に追加し、`styles/index.css` に import する。
  色と寸法は `tokens.css` の変数を使う。
- TUI と CLI は対象に含めない。
- この issue では PRD の更新を含めない。

## 完了条件

- サイドバーの Security グループに `Access Logs` が表示され、選ぶと専用ビューが開く。
- プロファイルとリージョンを切り替えると、ビューが再マウントされ、ソースと結果が切り替わる。
- ソースの一覧に ALB、CloudFront、WAF が表示され、出力先、状態、Global の印が表示される。
- ログが無効な ALB、CloudFront、WAF が一覧に残り、「Logging disabled」と有効化の手順が表示される。すべてのソースが無効な場合は、実行ボタンが無効になり、セットアップの案内が表示される。
- 同じ出力先を共有する複数のソースに「Shared」の印と共有するリソースの一覧が表示され、同じ出力先のテーブルまたはロググループを 1 回だけ選べる。
- `Attribution` が false のソースを、共有するほかのソースと同時に選べない。
- `Setup` タブで、既存の Athena のデータベースとテーブルを選び、検証の結果を確認できる。
- DDL のモーダルが開き、パーティション投影を含む DDL をコピーできる。
- 期間、7d の確認、WAF の 24h の上限、フィルタが動作する。
- 集計の実行中、成功、部分失敗、失敗、取り消しの状態が表示される。
- 結果の `IP x URI` と `Source IP` のタブが表示され、列の並びと値が正しい。
- 複数のソースを選んだときに、ソース別の内訳と重複の注意が表示され、単一の合計が出ない。
- CloudFront の origin の ALB が選ばれたときに、ALB が IP の集計から外れることが表示される。
- 空の結果、ログの無効、配信の遅延、テーブルの不一致、権限不足、SSO の期限切れが区別して表示される。
- テーブルの対応付けが出力先の ID ごとに localStorage に保存され、同じ出力先を共有する複数のソースで再利用され、再読み込みで復元される。
- ジョブ ID が sessionStorage に保存され、再読み込みで状態を取得できる。
- ja と en の翻訳のキーが一致する。
- `mise run frontend:lint` と `mise run frontend:test` が通る。
- `mise run check` が通る。

## 関連

- ADR 0037
- issue 0235、0236、0237、0238、0240
- `frontend/src/lib/serviceMeta.ts`、`components/Sidebar.tsx`、`views/AccountView.tsx`
- `frontend/src/views/AthenaView.tsx`、`views/CloudWatchLogsView.tsx`
- `frontend/src/api/queries.ts`、`api/endpoints.ts`
- `frontend/src/i18n/locales/{ja,en}/`
