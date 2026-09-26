# CLI に Athena のクエリ実行コマンドを追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-25

## 背景

Web には Athena の専用ビュー (`frontend/src/views/AthenaView.tsx`) があり、catalog / database / workgroup の選択、スキーマツリー、クエリの実行・ドライラン相当の確認・キャンセル・履歴・スニペット保存ができる。backend の取得層にも `backend/internal/aws/athena.go` があり、API ハンドラ (`backend/internal/api/handlers_athena.go`) とルート (`backend/internal/api/routes.go` の `/athena/*`) が揃っている。

一方 CLI には `athena` コマンドが無く、`backend/internal/cli/root.go` にも登録されていない。シェルやスクリプトから Athena のクエリを実行できない。

## 目的

CLI から Athena のクエリを実行し、結果を tab / CSV で取得できるようにする。

## 設計判断

- 一覧系 (`catalogs` / `databases` / `workgroups` / `tables`) と `query` 実行をサブコマンドとして提供する。
- クエリ実行は既存の `StartQueryExecution` / ポーリング / `GetQueryResults` の経路を CLI から呼ぶ。ポーリングは context のキャンセルで中断できるようにする。
- 結果セットは固定列ではなく動的な列になるため、`[][]string` を直接組み立てて `printRowsOrGroupBy` に渡す。
- 追加の API・権限は不要。

## 完了条件

- `thief athena catalogs` / `databases` / `workgroups` / `tables` が一覧を tab 出力し、`-o csv` が使える。
- `thief athena query "<sql>"` がクエリを実行し、完了後の結果を tab / CSV で出力する。
- `mise run check` が通過する。

## 解決方法

implement-issues (2026-09-25) で実装した。実装は `issue-implementer` エージェントが worktree で行い、親が作業ツリーへ統合した。

### 変更したファイルとシンボル

- `backend/internal/aws/athena.go`
  - `WaitAthenaQuery(ctx, profile, region, id)` を追加した。内部の `waitAthenaQuery(ctx, client, id, interval)` が既存の `getAthenaQuery` (GetQueryExecution) を `athenaQueryPollInterval` (2 秒) 間隔でポーリングし、`SUCCEEDED` で実行情報を返す。`FAILED` / `CANCELLED` はエラーにし、`StateReason` があればメッセージに含める。`QUEUED` / `RUNNING` は間隔を空けて再確認し、それ以外の状態 (Status 欠落による空文字、SDK が知らない状態) は待ち続けずエラーにする。`ctx` のキャンセルは `select` で `ctx.Err()` を返して待機を打ち切る。最初の確認は間隔を待たずに行う。
  - `GetAthenaQueryResultsAll(ctx, profile, region, id, maxResults)` を追加した。内部の `getAllAthenaQueryResults` が既存の `getAthenaQueryResults` (GetQueryResults) を 1 つのクライアントで `NextToken` が空になるまで呼び、`Columns` と `Rows` を結合する。ヘッダ行の除去は既存の `athenaResultPageFrom` が最初のページにだけ適用する。
- `backend/internal/cli/athena.go` (新規): `newAthenaCmd` が `thief athena` と `catalogs` / `databases [--catalog]` / `workgroups` / `tables --database <name> [--catalog]` / `query <sql> [--catalog] [--database] [--workgroup] [--output-location]` を定義する。各サブコマンドは `loadConfig` → 既存の `ListAthenaCatalogs` / `ListAthenaDatabases` / `ListAthenaWorkgroups` / `ListAthenaTables` → `[][]string` を組み立てて `printRowsOrGroupBy` へ渡す (設計判断どおり、`runList` は使わず動的な列に対応する)。`query` は `athenaQueryInputFromFlags` が `--catalog` / `--database` / `--workgroup` / `--output-location` と SQL 引数を `StartAthenaQueryInput` に詰め、`athenaRunQuery` が設定を読んだ後、`athenaExecuteQuery(ctx, cmd, cfg, ops, in)` が `ops.start` (`StartAthenaQuery`) → `ops.wait` (`WaitAthenaQuery`) → `ops.results` (`GetAthenaQueryResultsAll`) → 結果メタデータから `athenaResultColumns` で列定義を作って `printRowsOrGroupBy` へ渡す。`ops` は `athenaQueryOps` (3 つの関数値) で、本番は `defaultAthenaQueryOps()` が `internal/aws` の実装を束ね、テストでは偽の操作に差し替えて結線を検証する。0 行の結果は表を出さず `Query returned no results` を出す。`athenaColumnSummary` は `tables` の Columns / PartitionKeys を `name:type` のカンマ区切りにする。
- `backend/internal/cli/root.go`: `newAthenaCmd()` を登録した。
- `backend/internal/aws/athena_test.go`: `TestWaitAthenaQueryPollsUntilTerminal` / `TestWaitAthenaQueryReturnsErrorOnTerminalFailure` / `TestWaitAthenaQueryStopsOnContextCancel` / `TestWaitAthenaQueryPropagatesGetError` / `TestWaitAthenaQueryRejectsUnknownState` / `TestGetAllAthenaQueryResultsMergesPages` / `TestGetAllAthenaQueryResultsFailsOnLaterPage` を追加した。
- `backend/internal/cli/athena_test.go` (新規): `TestNewAthenaCmdTree` / `TestAthenaColumnSummary` / `TestAthenaResultColumns` / `TestNewRootCmdHasAthena` / `TestAthenaTableListOutput` / `TestAthenaQueryInputFromFlags` / `TestAthenaExecuteQueryWiring` / `TestAthenaExecuteQueryNoRows` / `TestAthenaExecuteQueryPropagatesErrors` を追加した。

### 完了条件の検証

- 「`thief athena catalogs` / `databases` / `workgroups` / `tables` が一覧を tab 出力し、`-o csv` が使える」: コマンドツリーは `TestNewAthenaCmdTree` と `TestNewRootCmdHasAthena`、`tables` の tab / csv 出力は `TestAthenaTableListOutput` で検証した。4 コマンドとも既存の `printRowsOrGroupBy` → `util.TableFormatter` の経路で出力し、`-o csv` は root の永続フラグから `cfg.Output` に載る。実装エージェントは SDK の `AWS_ENDPOINT_URL_ATHENA` でローカルの Athena プロトコルスタブへ向けたビルド済み CLI で 4 コマンドの tab 出力と `catalogs` / `tables` の `-o csv` 出力 (セル内カンマの引用を含む) を実測した。実 AWS アカウントでの実行は行っていない。
- 「`thief athena query "<sql>"` がクエリを実行し、完了後の結果を tab / CSV で出力する」: 自動テストでは、フラグから開始パラメータへの詰め替えを `TestAthenaQueryInputFromFlags`、コマンド本体の結線 (開始で得た実行 ID と設定のプロファイル / リージョンが完了待ちと結果取得に渡ること、結果が動的な列で tab / CSV 出力されること) を `TestAthenaExecuteQueryWiring`、0 行のメッセージを `TestAthenaExecuteQueryNoRows`、各段の失敗の伝播と後続の操作を呼ばないことを `TestAthenaExecuteQueryPropagatesErrors`、ポーリングの終端判定を `TestWaitAthenaQueryPollsUntilTerminal` と `TestWaitAthenaQueryReturnsErrorOnTerminalFailure`、未知の状態の打ち切りを `TestWaitAthenaQueryRejectsUnknownState`、context キャンセルによる中断を `TestWaitAthenaQueryStopsOnContextCancel`、ページ結合を `TestGetAllAthenaQueryResultsMergesPages`、途中ページの失敗を部分結果として返さないことを `TestGetAllAthenaQueryResultsFailsOnLaterPage`、結果列の変換を `TestAthenaResultColumns` で検証した。手動検証として、実装エージェントが同じスタブ (QUEUED → RUNNING → SUCCEEDED、2 ページ構成) で tab と `-o csv` の出力、RUNNING 固定のスタブに対する SIGINT で待機が exit 130 で止まること、`DROP TABLE` が既存の `StartAthenaQuery` の読み取り専用検査で拒否されることを実測した。
- 「`mise run check` が通過する」: 統合後の作業ツリーで実行し終了コード 0。ベースライン (失敗テスト無し) からの新たな失敗は無い。

### 方針からの乖離 (方式は保ったままの実装詳細)

- 方針の「既存の `StartQueryExecution` / ポーリング / `GetQueryResults` の経路を CLI から呼ぶ」に対し、ポーリングと全ページ取得を CLI 側に書かず `internal/aws` に `WaitAthenaQuery` / `GetAthenaQueryResultsAll` として置いた。SDK アクセスを `internal/aws` に閉じる既存の構造 (`sso_oidc.go` の `WaitForSSOToken` と同じ形) に合わせるためで、CLI から毎回クライアントを作り直すことも避けられる。
- `query` に `--catalog` / `--database` / `--workgroup` / `--output-location` を設けた。Web の Athena ビューの選択項目に対応し、追加の API や権限は要らない。
- `query` を Ctrl-C で中断しても Athena 上のクエリは停止しない。停止は方針と完了条件に無いため実装せず、`--help` の Long に明記した。
- `GetAthenaQueryResultsAll` は行数上限を持たず全ページを取得する (issue に上限の指定が無い)。行数を絞る手段は `docs/issues/TODO.md` へ追記して別 issue に委ねる。
- 方針に無い防御として、`waitAthenaQuery` は `QUEUED` / `RUNNING` / 終端のどれでもない状態をエラーにする。多観点レビュー (テストと堅牢性) の指摘で、壊れた応答や SDK の状態追加時に Ctrl-C まで待ち続ける失敗を防ぐために加えた。

### 多観点レビューの反映

- `TestAthenaTableListOutput` の tab サブテストが `tables` ではなく `catalogs` の列と行を使っていた点を、`tables` の列定義と行に直した (観点 2 の中、観点 3 の低)。
- `athenaRunQuery` が `internal/aws` のパッケージ関数を直接呼んでいて結線を自動テストできなかった点を、`athenaQueryOps` の注入と `athenaExecuteQuery` への分離で解消し、結線・0 行・エラー伝播のテストを追加した (観点 1 の低、観点 2 の中)。
- 未知の状態で待ち続ける点 (観点 2 の低) と、途中ページの失敗の挙動がテストで固定されていない点 (観点 2 の低) を上記のとおり反映した。
- 一覧 4 コマンドの `RunE` から `ListAthena*` への結線を自動テストで固定していない点 (観点 1 の低) は、結線が既存関数の呼び出しと空メッセージの分岐だけで、0188 から 0192 の一覧コマンドと同じ検証水準であるため、追加テストを書かず却下した。
- 全ページ取得の上限を issue 化する提案 (観点 1 の低、観点 2 の低) は、完了条件が全件取得を求めるため実装では扱わず、`docs/issues/TODO.md` への追記で todo-to-issue に委ねる。
- 追加レビュー 1 で、フラグから `StartAthenaQueryInput` への詰め替えが自動テストを通っていない点 (観点 2 の低) を、`athenaQueryInputFromFlags` の切り出しと `TestAthenaQueryInputFromFlags` で反映した。同じ指摘のうち `defaultAthenaQueryOps` の選定と、`RunE` が `athenaQueryInputFromFlags` → `athenaRunQuery` を呼ぶ結線は実行時テストを追加せず却下した。前者は 3 フィールドが `internal/aws` のシグネチャとコンパイル時に型検査され、後者は 2 つの関数呼び出しだけで分岐が無いため、テストが実装の写しになる。`waitAthenaQuery` の説明が実装の分岐と文字どおりには一致しない点 (観点 3 の低) は、上記の記述を実装に合わせて直した。
