# CLI に CloudWatch Logs のイベント取得を追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-25

## 背景

Web の CloudWatch Logs ビュー (`frontend/src/views/CloudWatchLogsView.tsx`) はロググループの複数選択、フィルタ式、期間プリセット、静的取得と Live Tail を備える。backend には `backend/internal/aws/cloudwatchlogs.go` のイベント取得と Tail があり、API ハンドラ (`handlers_cwlogs.go`) とルート (`/logs/groups` / `/logs/events` / `/logs/tail`) が揃っている。

一方 CLI は `thief logs ls` (ロググループ一覧のみ、`backend/internal/cli/cloudwatchlogs.go`) しかなく、ログイベントを取得できない。

## 目的

CLI から期間とフィルタ式を指定して CloudWatch Logs のイベントを取得できるようにする。

## 設計判断

- `thief logs events <log-group> [--filter] [--since] [--limit]` を追加する。`gcp logging ls` (`backend/internal/cli/gcp.go`) と同じ引数の形に揃える。
- Live Tail (follow) は WebSocket 前提のためスコープ外とする (`gcp logging ls` と同じ判断)。1 回取得のみを提供する。
- 追加の API・権限は不要。

## 完了条件

- `thief logs events <log-group>` が既定期間 (`--since` 既定 1 時間) のイベントを Timestamp / Severity 相当の列で tab / CSV 出力する。
- `--filter` と `--limit` が効く。
- `mise run check` が通過する。

## 解決方法

implement-issues (2026-09-25) で実装した。実装は `issue-implementer` エージェントが worktree で行い、親が作業ツリーへ統合した。

### 変更したファイルとシンボル

- `backend/internal/cli/cloudwatchlogs.go`: `logs` コマンドに `events <log-group>` サブコマンドを追加した。フラグは `--filter` (FilterLogEvents のフィルタパターン)、`--since` (既定 1 時間)、`--limit` (既定 100) の 3 つで、名前と型は `gcp logging ls` と同じにした (`--filter` と `--since` は既定値も同じ。`--limit` の既定値だけは下記の乖離のとおり異なる)。列定義 `cwLogsEventColumns` (Timestamp / Severity / LogStream / Message) と既定件数 `cwLogsDefaultEventLimit` を追加し、取得は既存の `runList` 経由で、フラグと位置引数を `cwLogsEventsRequestFromFlags` で取得条件 `cwLogsEventsRequest` に写し (0 以下の `--since` と 1 から 10000 の範囲外の `--limit` はここでエラーにする)、`cwLogsFetchEvents` が差し替え可能な `cwLogsFilterEventsFunc` (本番は `awsinternal.FilterLogEvents`) をロググループ 1 つ、開始時刻 `cwLogsEventsStart` (現在時刻から `--since` を引いた RFC 3339)、終了時刻とページトークン空で呼ぶ。`ls` の上にあった「イベント検索はスコープ外」のコメントを `events` の説明に置き換えた (横断とページングは `gcp logging ls` と同じ判断、Live Tail は WebSocket 前提、と理由を分けて書いた)。
- `backend/internal/aws/cloudwatchlogs.go`: `LogEventInfo` に `util.Row` を満たす `ToRow()` を追加した。Severity 列は `cwLogsSeverityFromMessage` でメッセージ本文から推定する (先頭 `cwLogsSeverityHeadUnits` (200) UTF-16 コード単位を `cwLogsSeverityHead` で切り出して大文字化し、単語境界付きのレベル語で ERROR / WARN / INFO の 3 段階へ丸める。Web の `frontend/src/lib/logSeverity.ts` の `cwSeverityFromMessage` と同じ語彙で、判定範囲も同じ `slice(0, 200)` の単位に揃えた)。`FilterLogEvents` の godoc を、識別子が名前でも ARN でもよいことが分かるように直した (実装の変更は無い。FilterLogEventsInput の `LogGroupIdentifier` はもとから両方を受け付ける)。`StartFromHead` のコメントを、期間指定が UI のプリセットだけでなく CLI の `--since` からも来ることが分かるように直した。
- `backend/internal/aws/cloudwatchlogs_test.go`: `TestLogEventInfoToRow`、`TestCWLogsSeverityFromMessage` (ASCII のレベル語、単語境界、先頭 200 単位の境界、multibyte とサロゲートペアを含む前置き) と、API の失敗が対象グループを含めてラップされ最初の失敗で打ち切ることを検証する `TestFilterLogEventsWrapsClientError` (`mockCWLogsFilterEventsClient` にエラー注入用の `err` を追加)、0 以下の `perGroupLimit` が既定値 100 に置き換わることを検証する `TestFilterLogEventsDefaultsPerGroupLimit` を追加し、既存の `TestFilterLogEventsSendsRequestParams` に `perGroupLimit` が API の `Limit` にそのまま載ることの検査を加えた。
- `backend/internal/cli/cloudwatchlogs_test.go` (新規): `TestLogsEventsCmdArgs` で `events` の位置引数の個数検査 (0 個と 2 個はエラー、1 個は通る) と 3 フラグの登録と既定値を固定した。`TestCWLogsEventsRequestFromFlags` で実コマンドツリーのフラグから取得条件への写し (未指定、全指定、上限 10000、0 以下と上限超過の拒否、int が 64 bit の環境では int32 で折り返す値の拒否も) を、`TestCWLogsFetchEventsWiring` で取得条件が `FilterLogEvents` の各引数 (profile / region / グループ / パターン / 開始時刻 / 空の終了時刻とトークン / 上限) に届きイベントがそのまま返ることを、`TestCWLogsFetchEventsPropagatesError` で失敗の伝播を固定した。
- `backend/internal/cli/columns_torow_test.go`: `TestColumnsToRowOrder` に `cwLogsEventColumns` / `LogEventInfo` の組を追加し、列ヘッダと `ToRow()` の値の対応を固定した。

### 完了条件の検証

- 「`thief logs events <log-group>` が既定期間 (`--since` 既定 1 時間) のイベントを Timestamp / Severity 相当の列で tab / CSV 出力する」: `--since` の既定値が `1h0m0s` であることは `TestLogsEventsCmdArgs`、列が Timestamp / Severity / LogStream / Message の順で `ToRow()` と一致することは `TestColumnsToRowOrder` と `TestLogEventInfoToRow`、Severity の推定は `TestCWLogsSeverityFromMessage` で検証した。出力は既存の `runList` → `printRowsOrGroupBy` → `util.TableFormatter` の経路で、`-o csv` は root の永続フラグから `cfg.Output` に載る (他の一覧コマンドと同じ経路。tab / CSV の整形は `backend/internal/util/formatter_test.go` の `TestTableFormatter_PrintHeader_And_PrintRows_Table` / `TestTableFormatter_PrintHeader_CSV` / `TestTableFormatter_PrintRows_CSV`、`printRowsOrGroupBy` の tab / csv の分岐は `backend/internal/cli/athena_test.go` の `TestAthenaTableListOutput` が固定する)。
- 「`--filter` と `--limit` が効く」: フラグの登録と既定値は `TestLogsEventsCmdArgs`。`--filter` は `FilterLogEvents` の `pattern` 引数、`--limit` は `perGroupLimit` 引数に渡され、それぞれ API の `FilterPattern` と `Limit` に載ることは `TestFilterLogEventsSendsRequestParams` で検証した (`Limit` の検査を今回追加)。フラグ値が `FilterLogEvents` の意図した引数に届く結線は `TestCWLogsEventsRequestFromFlags` と `TestCWLogsFetchEventsWiring` で検証した (`--filter` や `--limit` を渡し忘れる変異で落ちる)。RunE が `args[0]` と `cwLogsEventColumns` を渡して `cwLogsEventsRequestFromFlags` と `cwLogsFetchEvents` を呼ぶ結線と、本番の `awsinternal.FilterLogEvents` の選択は、分岐の無い数行の写しになりテストが実装の写しになるため自動テストしない (0193 の RunE の結線と同じ判断。`FilterLogEvents` の選択だけは `cwLogsFilterEventsFunc` との型一致でコンパイル時に守られる)。
- 「`mise run check` が通過する」: 統合後の作業ツリーで実行し終了コード 0。ベースライン (失敗テスト無し) からの新たな失敗は無い。

### 方針からの乖離 (方式は保ったままの実装詳細)

- Severity 列はイベントに構造化された値が無いため、Web と同じくメッセージ本文から推定する。判定ロジックは `frontend/src/lib/logSeverity.ts` と Go の 2 か所に重複し、コメントで両方を揃えることを求めている。判定範囲は Web の `slice(0, 200)` と同じ UTF-16 コード単位で切る (実装エージェントはバイト単位で切っており、multibyte の前置きがあるメッセージで Web と結果が食い違うことをレビューで指摘されたため、`unicode/utf16` で数える `cwLogsSeverityHead` に直した)。大文字化は JS の `toUpperCase` (full case mapping) と Go の `strings.ToUpper` (simple case mapping) で写像が異なるため、ß や合字の直後にレベル語が来るメッセージでは結果が異なりうる。語彙と判定範囲を揃えた以上の完全一致は保証しない。
- `--limit` の既定値は 100 で、backend の 1 ロググループ 1 ページあたりの既定上限 `defaultLogEventPerGroupLimit` に揃えた。issue の設計判断は `gcp logging ls` と同じ引数の形に揃えるとしているが、`gcp logging ls` の既定 `gcpLoggingDefaultLimit` は 200 で、`--limit` の既定値だけは一致しない。Web の CloudWatch Logs ビューの 1 ページ (`CW_LOG_EVENT_PAGE_SIZE`、200 件) とも異なり、backend の 1 グループ 1 ページ上限 100 に合わせた。また `--since` は 0 以下を、`--limit` は 1 から 10000 (FilterLogEvents の Limit の有効範囲、`cwLogsMaxEventLimit`) の範囲外を `cwLogsEventsRequestFromFlags` でエラーにする。`gcp logging ls` は値をそのまま取得層へ渡すが、backend の `FilterLogEvents` は 0 以下の上限を黙って既定値 100 に置き換え、int32 変換で折り返す値もあるため、利用者の指定ミスを黙って別の件数にしないことを優先した。
- `<log-group>` はロググループの名前でも ARN でも受け付け、そのまま `FilterLogEvents` の識別子に渡す。Web は一覧が持つ ARN を渡すが、CLI の利用者は名前で指定するのが自然なため。
- `--since` に上限は設けない。`FilterLogEvents` は最新優先 (`StartFromHead=false`) で問い合わせ、この指定は開始時刻が 2024-01-01 以降のときだけ API が許可するため、それより古くなる `--since` を渡すと AWS のエラーがそのまま返る (CLI 側で丸めない)。
- 取得は 1 ページのみで、`NextPageToken` による続きの取得 (ページング) は行わない。`gcp logging ls` と同じ判断で、`--help` の Long と `--limit` の説明に明記した。
- 複数ロググループの横断検索と Live Tail は issue の設計判断どおりスコープ外とし、実装していない。

### 多観点レビューの反映

Step 7 のラウンド 1 (完了条件の充足 / テストと堅牢性 / 規約と整合) で高 1 件、中 4 件 (3 観点で重複する 2 点)、低 6 件が出た。

- 高 (テストと堅牢性): `--filter` / `--limit` が `FilterLogEvents` の引数へ届く結線が自動テストされていない。同じパッケージの 0193 (`athenaQueryInputFromFlags` / `athenaQueryOps`) に前例があるため、取得条件の組み立て (`cwLogsEventsRequestFromFlags`) と取得 (`cwLogsFetchEvents`、`cwLogsFilterEventsFunc` で差し替え) に切り出し、`TestCWLogsEventsRequestFromFlags` / `TestCWLogsFetchEventsWiring` / `TestCWLogsFetchEventsPropagatesError` で固定した。
- 中 (3 観点共通): Severity の判定範囲が Web (UTF-16 コード単位 200) と CLI (200 バイト) で異なり、multibyte の前置きがあるメッセージで結果が食い違うのに、乖離記録は「差が出ない」と書いていた。`cwLogsSeverityHead` で UTF-16 コード単位に切るよう実装を直し、`TestCWLogsSeverityFromMessage` に multibyte とサロゲートペアの境界を加え、記録と Go のコメントを直した。
- 中 (3 観点共通): `--limit` の既定 100 を `gcpLoggingDefaultLimit` (実際は 200) と同じ値と書いていた。乖離として正しく記録し、CHANGES.md エントリにも既定値の違いを補った。
- 低 (完了条件の充足 / テストと堅牢性 / 規約と整合): `LogGroupIdentifiers` (複数形。StartLiveTail のフィールド) を引いていた箇所を `LogGroupIdentifier` に直し、tab / CSV の検証の引用先を `helper_test.go` から `formatter_test.go` と `athena_test.go` の該当テストに直した。
- 低 (規約と整合): `events` のコメントが横断とページングまで WebSocket を理由にしているように読めたため、Live Tail の理由と横断・ページングの理由 (`gcp logging ls` と同じ判断) を分けて書いた。
- 低 (テストと堅牢性): API 失敗のラップが未テストだったため `TestFilterLogEventsWrapsClientError` を追加した。`--limit` 0 以下が黙って既定 100 になる点は CLI 側で拒否するようにし、大きすぎる `--since` (開始時刻が 2024-01-01 より前になる場合) は上記の乖離のとおり API のエラーに委ねる判断を記録した (`StartFromHead` のコメントも CLI を含む表現に直した)。
- 却下: 無し。

追加レビュー 1 (変更箇所に限定) で高 0 件、中 1 件、低 4 件が出た (テストと堅牢性)。

- 中: `--limit` 既定 100 の理由を「Web の 1 ページと同じ件数」と書いていたが、Web の 1 ページは 200 件 (`CW_LOG_EVENT_PAGE_SIZE`)。乖離記録を「gcp とも Web とも異なり backend の上限 100 に合わせた」に直した。
- 低: `cwLogsSeverityHead` のコメントが「Web と同じく置換文字になる」と書いていたが、JS の `slice` は孤立サロゲートのまま返す。判定結果は変わらないことを含めてコメントを直した。
- 低: RunE の残りの結線 (`args[0]`、列定義) を「コンパイル時の検証」と書いていたが型では守られない。0193 と同じ「分岐の無い写しになるため自動テストしない」理由に書き直した。
- 低: 「既定値フォールバックの検証は別途」に対応するテストが無かったため `TestFilterLogEventsDefaultsPerGroupLimit` を追加した。
- 低: `--limit` に上限が無く、int32 変換で折り返す値が黙って別の件数になるため、`cwLogsMaxEventLimit` (API の有効範囲 10000) を上限として検査し、`TestCWLogsEventsRequestFromFlags` に上限と折り返し値のケースを加えた。

追加レビュー 1 の完了条件の充足と規約と整合は、上の中 1 件 (Web の 1 ページは 200 件) を同じく指摘したほか、完了条件の充足が低 3 件を挙げた。

- 低: 「`--since` の下限を API に委ねる」という反映記録が実装 (0 以下はローカルで拒否) と矛盾していた。「大きすぎる `--since`」に書き直した。
- 低: Severity の大文字化が JS (full case mapping) と Go (simple case mapping) で異なり、ß や合字の直後では結果が異なりうる。Go のコメントと乖離記録に条件を明記し、「Web と一致する」を語彙と判定範囲に限る表現にした。
- 低: 既定値フォールバックのコメントが存在しないテストを指していた点は、テストと堅牢性の低と同じ内容で、`TestFilterLogEventsDefaultsPerGroupLimit` の追加で決着した。
- 却下: 無し。

追加レビュー 2 (上限の 2 回目) は、完了条件の充足と規約と整合が指摘ゼロ、テストと堅牢性が低 2 件を挙げた。いずれもテストファイルのみの変更で反映し、上限に達しているためこれ以上のレビューは行わずに `mise run check` の再実行だけを行った。

- 低: `TestCWLogsSeverityFromMessage` のコメントだけが「Web と同じ判定」の無条件表現のまま残っていた。実装のコメントと同じく、語彙と判定範囲の一致に限り、大文字化の写像差は固定しない旨を明記した。
- 低: `--limit 4294967396` (int32 で 100 に折り返す値) のケースは pflag が int のビット幅で解析するため 32 bit 環境では解析エラーになる。`strconv.IntSize == 64` の環境に限ってケースを加える形にし、上限の境界自体は 10001 のケースで固定した。
- 却下: 無し。
