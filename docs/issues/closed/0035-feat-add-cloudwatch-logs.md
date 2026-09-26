# AWS CloudWatch Logs サービスを追加する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8

## 背景

thief は GCP Cloud Logging のログ検索と Live Tail を既に提供している (docs/issues/closed/0022)。
一方 AWS 側にはログ閲覧手段が無く、CloudWatch Logs のイベントを thief 上で確認できない。
デザイン検討 (Query Editor.dc.html, Turn 8a) では CloudWatch Logs を Cloud Logging と同型のログビューアとして追加する構成が示された。

## 目的

CloudWatch Logs のロググループ横断検索と Live Tail を提供する。
UI はデザイン Turn 8a に準拠した専用ログビューアページとする。

## 要件

### backend

- `backend/internal/aws/cloudwatchlogs.go` を新規作成する。
  - `ListLogGroups(ctx, profile, region)` で `DescribeLogGroups` をページネートしてロググループ一覧を返す。
  - `FilterLogEvents(ctx, profile, region, groups, pattern, start, end, nextToken, limit)` で複数ロググループ横断のイベント検索を返す。
  - `StartLiveTail(ctx, profile, region, groups, pattern, send)` で StartLiveTail のイベントストリームを push する。
- SDK は `github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs` を追加する。
- HTTP ハンドラを `handlers` に追加し routes に登録する。
  - `GET /api/aws/profiles/{profile}/logs/groups` (キャッシュあり)
  - `GET /api/aws/profiles/{profile}/logs/events` (検索、キャッシュなし)
  - `GET /api/aws/profiles/{profile}/logs/tail` (WebSocket Live Tail)
- Live Tail は GCP の `handleGCPLoggingTail` と同じ WebSocket 規約 (coder/websocket, errgroup + cancel, TEXT JSON push, 終了時に `{"type":"end"}`, OriginPatterns による Origin 検証) を踏襲する。
- SSO トークン期限切れは `writeAWSError` 経由で 401 SSO_TOKEN_EXPIRED にマップする。
- CLI に `logs ls` (ロググループ一覧) を追加する。Live Tail は WebSocket 前提のため CLI スコープ外とする (GCP logging と同方針)。

### frontend

- `types/aws.ts` に Raw/Row 型 (`CWLogGroupRaw/Row`, `CWLogEventRaw/Row`, `CWLogEventPageRaw`) を追加する。
- `lib/normalize.ts` に変換関数を追加する。
- ログビューアの共通表示コンポーネント (レイアウト, ヒストグラム, ログ一覧, ツリー) を新設し AWS/GCP で共有する。
- ヒストグラムは取得済みイベントのクライアント集計で描画する (件数は表示分)。バックエンドの集計 API は追加しない。
- エクスポートは表示中の行を CSV/JSON でクリップボードにコピーする。
- サイドバーのカテゴリは Management & Governance とする。
- `AccountView` に `activeService === 'cloudwatchlogs'` の分岐を追加する (Athena と同型の専用ページ埋め込み)。
- サービスアイコンは AWS 公式 Architecture Icons の CloudWatch Logs を使う。

## 検証

- mise run backend:build / backend:test
- mise run frontend:lint / frontend:test
- floci + ブラウザで表示確認

## 解決方法

### backend

- `backend/internal/aws/cloudwatchlogs.go` を新規作成した。
  - `ListLogGroups` は `DescribeLogGroups` をページネートして名前昇順で返す。
  - `FilterLogEvents` は選択ロググループごとに `FilterLogEvents` をファンアウトし、時刻降順でマージして返す。複数グループのページングは各グループの nextToken を JSON + base64 でまとめた複合トークンで継続する。
  - `StartLiveTail` は `LogGroupIdentifiers` (ARN, 最大 10) で Live Tail セッションを開き、`SessionUpdate` のイベントを send へ push する。
- Live Tail の WebSocket 中継処理を `backend/internal/api/logtail.go` の `serveLogTail` に共通化し、既存の GCP Cloud Logging (`handleGCPLoggingTail`) も同関数を使うよう変更した。
- `handlers_cwlogs.go` にロググループ一覧 (キャッシュあり) とイベント検索 (キャッシュなし)、Live Tail (WebSocket) のハンドラを追加し、routes に登録した。
- CLI に `logs ls` (ロググループ一覧) を追加した。
- SSO トークン期限切れは `writeAWSError` 経由で 401 SSO_TOKEN_EXPIRED にマップされる。
- SDK は `github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs` を追加した。

### frontend

- 共通ログビューアコンポーネント (`components/logviewer/`) を新設した。`LogViewerShell` (レイアウト)、`LogTree` (チェックボックスツリー)、`LogHistogram` (単色 / severity 積み上げ)、`LogList` (JSON 展開行)、`LogToolbarActions`、`useLiveTail`、`useCopy`。
- 純関数ライブラリ (`lib/logSeverity.ts` / `logHistogram.ts` / `logFormat.ts` / `logGroupTree.ts`) を追加した。ヒストグラムは取得済みイベントのクライアント集計とし、バックエンドの集計 API は追加しない。
- `views/CloudWatchLogsView.tsx` を新設し、`AccountView` に `activeService === 'cloudwatchlogs'` の分岐を追加した。サイドバーカテゴリは Management & Governance とした。
- 型 (`types/aws.ts`) と変換 (`lib/normalize.ts`)、API (`endpoints.ts` / `queries.ts` / `terminal.ts`) を追加した。

### 検証

- backend build / vet / test (-race)、frontend lint / test 全通過。
- floci (AWS エミュレータ) にロググループとイベントをシードし、ブラウザで複数ロググループ横断検索、severity 色分け、ヒストグラム、行の JSON 展開をライト / ダーク両テーマで確認した。
