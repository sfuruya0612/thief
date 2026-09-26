# 0022 Google Cloud Logging のログ閲覧サービスを追加する

Created: 2026-07-17
Completed: 2026-07-18
Model: Claude Fable 5 claude-fable-5

## 背景 / 根拠

thief の GCP ビューは Cloud Run、GCS、IAM、Service Accounts、BigQuery を扱うが、ログを閲覧する手段がない。
リソースの状態確認とログ調査は一連の作業なのに、ログだけ Google Cloud コンソールへ移動する必要があり、アプリ内で調査が完結しない。

Cloud Logging のログ閲覧をサービスとして追加し、期間指定での取得、Logging query language によるフィルター、Live Tail によるリアルタイム可視化をアプリ内でできるようにする。

## 対応内容

- GCP ビューに Cloud Logging サービス (サービスキー `cloudlogging`) を追加する
- 期間指定 (プリセット: 直近 15 分 / 1 時間 / 6 時間 / 24 時間 / 7 日、およびカスタム範囲) でログエントリを取得できるようにする
- Logging query language のフィルター文字列を入力してログを絞り込めるようにする (severity 等の頻出条件は補助 UI から挿入できると望ましい)
- Live Tail: フィルター適用状態のまま、新着ログを WebSocket 経由でリアルタイム表示できるようにする

## 実装方針

### 依存の追加

`cloud.google.com/go/logging` を直接依存に追加する。
gRPC ベースの Cloud Logging API (特に Live Tail の双方向ストリーミング) は標準ライブラリでは代替できず、AGENTS.md の依存方針 4 (クラウドプロバイダ公式 SDK は許容) に該当する。
`v1.18.0` が推移的依存として `go.sum` に既に存在するため、import して `mise run backend:tidy` を実行すれば直接依存へ昇格する。

用途でパッケージを使い分ける。

- 期間指定の取得: `cloud.google.com/go/logging/logadmin` (`Client.Entries` にフィルター文字列とソート順を渡すイテレータ形式)
- Live Tail: `cloud.google.com/go/logging/apiv2` の `LoggingServiceV2Client.TailLogEntries` (双方向 gRPC ストリーム。logadmin には tail 相当がない)

### backend: service 層

`backend/internal/gcp/logging.go` を新規作成する。

- 既存 GCP サービスの規約に従い、ADC 認証 + `option.WithQuotaProject(projectID)` を必ず渡す (`backend/internal/gcp/cloudrun.go:32` のコメント参照)
- `ListLogEntries(ctx, projectID, filter, start, end, pageToken string, pageSize int)` を実装する。期間は利用者フィルターに `timestamp >= "..." AND timestamp <= "..."` (RFC3339) を AND 結合して表現し、`logadmin.NewestFirst()` で新しい順に返す
- 戻り値はページ型 (エントリ配列 + `next_page_token`) とし、`LogEntryInfo` 構造体 (JSON タグ snake_case) に `timestamp` / `severity` / `log_name` / `resource_type` / `resource_labels` / `labels` / `payload` / `insert_id` / `trace` を持たせる
- payload は TextPayload / JSON payload / ProtoPayload の 3 形態があるため、表示用に文字列へ正規化するヘルパー (`payloadToString`) を分離する。JSON payload は `encoding/json` で整形せずそのまま直列化する (整形は frontend の責務)

`backend/internal/gcp/logging_tail.go` を新規作成する。

- `TailLogEntries(ctx, projectID, filter string, send func(LogEntryInfo) error)` の形で、apiv2 のストリームを開き `TailLogEntriesRequest{ResourceNames: ["projects/" + projectID], Filter: filter}` を送信、`Recv` ループで受信エントリをコールバックへ渡す
- Cloud Logging 側の制約 (レスポンスは数秒単位でバッファリングされる、tail セッションは約 1 時間で切断される、entries.tail には同時セッション数の quota がある) を godoc コメントに明記し、切断時はエラーで返して呼び出し側にセッション終了を伝える

### backend: handler 層

`backend/internal/api/handlers_gcp.go` に 2 ハンドラを追加し、`backend/internal/api/routes.go` の GCP セクションにルートを登録する。

- `GET /api/gcp/logging/entries` (`handleGCPLoggingEntries`): クエリパラメータ `project_id` / `filter` / `start` / `end` / `page_token` / `page_size`。project_id 解決は既存の `gcpProjectIDFromQuery` (`handlers_gcp.go:18`) を使う。ログは実行のたびに結果が変わる読み取りのため、BigQuery のクエリ実行と同じ方針でキャッシュ (`serveCached`) を通さない (`handlers_bigquery.go:54` のコメント参照)
- `GET /api/gcp/logging/tail` (`handleGCPLoggingTail`): WebSocket エンドポイント。`websocket.Accept` (`github.com/coder/websocket`) に既存セッションハンドラと同じ `AcceptOptions{OriginPatterns: s.cfg.WebOrigins}` を渡す (`backend/internal/api/handlers_session.go:84` 参照)。`InsecureSkipVerify` は使わない
  - フレーム規約: ログエントリは TEXT フレームの JSON で 1 件ずつ push する。終了時は `{"type":"end","reason":"..."}` の制御メッセージを送ってからクローズする (ターミナルの exit 通知 `backend/internal/session/bridge.go:174` に倣う)
  - 終了経路の確保: `r.Context()` の cancel (クライアント切断) と tail ストリームのエラーの両方でハンドラが返るよう、`errgroup` + cancel の構図を `bridge.go` から片方向に簡略化して実装する。ブラウザからの受信は切断検知のためだけに読み捨てる
  - HTTP サーバは WebSocket のために `ReadTimeout` / `WriteTimeout` を設定していない (`backend/internal/api/server.go:93` 付近) ため、長時間接続はそのまま成立する

### backend: CLI

`backend/internal/cli/gcp.go` の `newGCPCmd()` に `logging` サブコマンドを追加する。

- `thief gcp logging --filter <expr> --since 1h` 相当の期間指定一覧のみ対応する。project ID 解決は既存の `gcpRequireProjectID` を流用し、出力は `printRowsOrGroupBy` に合わせる
- CLI での Live Tail (follow) はスコープ外とする

### frontend

GCP の 1 サービスとして追加し、トップレベルビュー (`AppView`) は増やさない。

- `frontend/src/lib/serviceMeta.ts`: `GCP_SERVICES` にエントリを追加し、`GCP_SERVICE_TO_PATH` に `cloudlogging: 'logging'` を追加する
- `frontend/src/views/GcpSidebar.tsx`: `SECTIONS` に項目を追加する
- `frontend/src/views/GcpView.tsx`: `activeService === 'cloudlogging'` の分岐で専用ビューを埋め込む。汎用 `GcpServicePanel` ではなく、BigQuery の専用ビュー埋め込み (`GcpView.tsx:275` 付近の `<BigQueryView projectId={activeProject} />`) と同じ形にする。フィルター入力、期間ピッカー、ストリーミング表示という一覧テーブルに収まらない UI を持つため
- 新規 `CloudLoggingView.tsx` (`frontend/src/views/` 配下、BigQueryView と同じ置き方):
  - フィルター入力 (Logging query language のテキストエリア) + 期間プリセット + 「実行」で静的取得、「Live」トグルで tail 接続に切り替える
  - 静的取得は `useInfiniteQuery` によるページング (`page_token` 続き取得)。BigQuery の結果ページング `useBQQueryResults` (`frontend/src/api/queries.ts:473`) が手本
  - Live Tail は `frontend/src/api/terminal.ts` の `buildWsUrl` を流用した `gcpLoggingTailUrl(projectId, filter)` で WebSocket を張り、受信 JSON を行リストへ追記する。生 WebSocket の管理は `frontend/src/components/Terminal/Terminal.tsx` のパターン (effect の依存に URL、cleanup でクローズ) に倣うが、表示は xterm ではなくログ行リストにする
  - 追記時は自動で最下部へスクロールし、利用者が上へスクロールしたら自動スクロールを止める (Live Tail の定石)
  - 行数上限 (例: 5,000 行) を設け、超えたら古い行から捨てる。無制限に DOM へ追記するとタブがメモリと再描画で破綻するため
  - severity 別の行スタイルには `frontend/src/app.css:1709` の `.logbox` 系クラス (`.lvl-info` / `.lvl-warn` / `.lvl-err`。現在未使用) を流用できる
- 型と正規化: `frontend/src/types/gcp.ts` に `LogEntryRaw` / `LogEntryRow`、`frontend/src/lib/normalizeGcp.ts` に `logEntryFromRaw` を追加する
- API 層: `frontend/src/api/endpoints.ts` に `getGcpLogEntries`、`frontend/src/api/queries.ts` に `useGcpLogEntries` を追加する
- アイコン: `frontend/src/components/icons/GcpIcons.tsx` と `frontend/scripts/fetch-gcp-icons.mjs` の `ICON_FILENAMES` に Cloud Logging の公式アイコンを追加する

### テスト

- backend: フィルターと期間の合成 (`timestamp` 条件の AND 結合)、payload 正規化 (Text / JSON / Proto の 3 形態)、tail の終了経路 (コールバックがエラーを返した場合とコンテキスト cancel) をテーブル駆動でテストする
- frontend: `logEntryFromRaw` のユニットテスト、期間プリセットから start/end への変換のユニットテスト、行数上限による切り捨てのテスト

## スコープ外

- ログベースの指標、ログルーター (シンク)、ログバケット管理
- ログのエクスポート、保存 (ダウンロード)
- CLI での Live Tail (follow)
- 複数プロジェクト横断のログ検索

## 検証

- `mise run check` を通す
- 実プロジェクトで、期間指定 + フィルターの取得、ページング続き取得、Live Tail の受信、Live Tail 中のフィルター変更 (再接続)、クライアント切断で backend の goroutine が終了すること (leak がないこと) を確認する

## 解決方法

方針どおり実装した。

- backend: `internal/gcp/logging.go` (`ListLogEntries`、`composeLogFilter`、`payloadToString` 等) と `internal/gcp/logging_tail.go` (`TailLogEntries`) を新設した。`TailLogEntries` は実際の gRPC 接続処理と `runTailLogEntries` (テスト対象のロジック本体) を分離し、`tailStream` インターフェース経由でモックを注入してコールバックエラー / Recv エラー / context cancel の終了経路をテーブル駆動テストした。`handlers_gcp.go` に `handleGCPLoggingEntries` (serveCached を通さない) と `handleGCPLoggingTail` (WebSocket、errgroup + cancel による片方向 push ループ、終了時に `{"type":"end","reason":"..."}` を送信) を追加し、`routes.go` にルートを登録した。CLI には `gcp logging ls` (一覧のみ、follow はスコープ外) を追加した。`cloud.google.com/go/logging` は `go mod tidy` で直接依存へ昇格させた。
- frontend: GCP サービス一覧に `cloudlogging` を追加し、新カテゴリ `observability` を新設した (issue 0027 の方針を踏襲)。`views/nonaws/CloudLoggingView.tsx` を `GcpView.tsx` から embed する形で追加し、フィルター入力 + 期間プリセット/カスタム範囲 + 「実行」による静的取得 (`useInfiniteQuery` ページング) と「Live」トグルによる Live Tail (生 WebSocket、`gcpLoggingTailUrl`) を実装した。行数上限 (`lib/logLines.ts`、5,000 行) と自動スクロール制御、severity 別スタイル (`.logbox`/`.lvl-*`) も実装した。
- テスト: backend は `logging_test.go` / `logging_tail_test.go` でフィルター合成・payload 正規化 (Text/JSON/Proto/不正値)・tail 終了経路をテーブル駆動でテストした。frontend は `logEntryFromRaw` / `logTimeRange.ts` (プリセット→範囲変換) / `logLines.ts` (行数上限切り捨て) のユニットテストを追加した。`mise run check` (backend fmt/lint/test, frontend fmt/lint/test) が全て通過することを確認した (backend `gcp` パッケージのテストは `go test -race` を含めて通過、frontend 332 件通過)。

### 未検証・残課題

- 「検証」節にある実プロジェクトでの動作確認 (期間指定+フィルター取得、ページング続き取得、Live Tail 受信、フィルター変更時の再接続、クライアント切断時の goroutine leak なし) は、このセッションに GCP 認証情報・実プロジェクトがないため実施できていない。自動テストと静的解析のみで検証済みであり、実環境での確認は利用者側で行うこと。
- Cloud Logging の公式アイコン (`public/assets/gcp-icons/`) は zip 未展開のためこのセッションでは反映できていない。コード上の参照 (`GcpIcons.tsx`、`fetch-gcp-icons.mjs` の `ICON_FILENAMES`) は追加したが、ファイル名が実際の Google Cloud 公式アイコンパッケージと一致するかは未確認。詳細は issue 0029 を参照。アイコン未展開でもプレースホルダ表示に自然にフォールバックするため機能上の支障はない。
