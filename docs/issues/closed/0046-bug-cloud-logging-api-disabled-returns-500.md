# 0046 Cloud Logging API 未有効化時のエラーが 403 ではなく 500 になる

Created: 2026-07-18
Completed: 2026-07-19
Model: Claude Sonnet 5 claude-sonnet-5

## 症状

Cloud Logging API が無効な GCP プロジェクトに対して `/api/gcp/logging/entries` を呼ぶと、
他の GCP サービス (BigQuery 等の REST 系クライアント) と同様に 403 `GCP_API_DISABLED` を
返すべきところ、500 `INTERNAL_ERROR` が返る。エラーメッセージ自体には「Enable it by visiting
...」という有効化用 URL 付きの分かりやすい文言が含まれているにもかかわらず、frontend 側は
`GCP_API_DISABLED` 用の 403 分岐ではなく汎用の 500 エラー表示に落ちる。

## 再現手順

1. `mise run backend:run` でバックエンドを起動する
2. Cloud Logging API (`logging.googleapis.com`) を有効化していない GCP プロジェクト (今回は
   `example-project-c` で確認) を対象に、以下を実行する

   ```
   curl "http://127.0.0.1:8089/api/gcp/logging/entries?project_id=<対象プロジェクト>&start=<RFC3339>&end=<RFC3339>&filter="
   ```

3. レスポンスを確認する

期待する結果は、他の GCP サービス (BigQuery 等) と同じ 403 `GCP_API_DISABLED` + 有効化 URL
付きメッセージである。
実際には次の 500 `INTERNAL_ERROR` が返る (実際に取得した実プロジェクトでの応答をそのまま
記載する)。

```json
{"error":"list gcp log entries: rpc error: code = PermissionDenied desc = Cloud Logging API has not been used in project example-project-c before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/logging.googleapis.com/overview?project=example-project-c then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry.\nerror details: name = ErrorInfo reason = SERVICE_DISABLED domain = googleapis.com metadata = map[activationUrl:https://console.developers.google.com/apis/api/logging.googleapis.com/overview?project=example-project-c consumer:projects/example-project-c containerInfo:example-project-c service:logging.googleapis.com serviceTitle:Cloud Logging API]","code":"INTERNAL_ERROR"}
```

## 原因

`backend/internal/api/errors.go` の `writeGCPError` (L85-102) は `errors.As(err, &gerr)` で
`*google.golang.org/api/googleapi.Error` (REST/JSON API クライアントのエラー型) にのみマッチ
する分岐を持ち、マッチしなければ無条件に 500 `INTERNAL_ERROR` へフォールバックする (L101)。
`gcpAPIDisabled` (L104-123) が見ている `SERVICE_DISABLED`/`accessNotConfigured` の判定も、
この `*googleapi.Error` の `Errors`/`Details` フィールドが前提になっている。

一方 `backend/internal/gcp/logging.go` の `ListLogEntries` (L41-71) は
`cloud.google.com/go/logging/logadmin` を使っており、これは gRPC トランスポート経由の
クライアントである。API 未有効化時に返るのは `*googleapi.Error` ではなく
`google.golang.org/grpc/status.Status` 実装のエラー (`rpc error: code = PermissionDenied
desc = ...`。詳細は `google.rpc.ErrorInfo` 等の gRPC エラー detail として埋め込まれる) で
あり、`errors.As(err, &gerr)` は false になる。結果として `writeGCPError` は必ず L101 の
汎用 500 分岐に落ちる。

`backend/internal/gcp/` 配下は `cloudrun.go`/`gcs.go`/`logging_tail.go` も
`cloud.google.com/go/...` (gRPC 系) と `google.golang.org/api/...` (REST 系) の両方を
import しており、呼び出し箇所によっては同じ分類漏れが起きている可能性がある。ただし今回
実際に確認できたのは Cloud Logging の `entries.list` (`ListLogEntries`) のみであり、他の
箇所が実際に gRPC エラーを返すかどうかは未確認。

## 根拠

403 `GCP_API_DISABLED` は frontend に「API を有効化してください」という専用の案内 (有効化
URL 付き) を表示させるために存在する分類であり、BigQuery 等では実際にこの経路で分かりやすい
案内が出ている。Cloud Logging だけこの経路から漏れて汎用 500 表示になると、原因は同じ
(API 未有効化) なのにサービスによって案内の質が変わってしまい、ユーザーが「有効化すれば
直る」ことに気付きにくくなる。

## 対応方針

以下のいずれか、または組み合わせで対応する。実装時に最も堅牢な方式を選定する。

- `writeGCPError` に、`errors.As` で `*googleapi.Error` を見る既存分岐に加えて、
  `google.golang.org/grpc/status.FromError(err)` で gRPC ステータスを取り出し、
  `codes.PermissionDenied`/`codes.NotFound` 等と `google.rpc.ErrorInfo` detail
  (`reason == "SERVICE_DISABLED"`) を検査する分岐を追加する。
- 判定ロジックが REST/gRPC で二重化して複雑になる場合、エラーメッセージ文字列に有効化 URL
  パターンが含まれるかを見る、より汎用的な (ただし精度は落ちる) フォールバック判定への統一も
  検討する。
- `backend/internal/gcp/` 配下で gRPC 系クライアントを使っている他の関数 (cloudrun.go/gcs.go/
  logging_tail.go の該当箇所) が実際に同じ形のエラーを返しうるか調査し、必要なら同じ判定
  ロジックを共通ヘルパーとして切り出す。

## 検証

- Cloud Logging API 無効プロジェクトに対して `/api/gcp/logging/entries` を呼び、403
  `GCP_API_DISABLED` + 有効化 URL 付きメッセージが返ることを確認する。
- 既存の BigQuery 等 REST 系での 403 `GCP_API_DISABLED` 判定が壊れていないことを確認する
  (既存テストの回帰確認)。
- `mise run check` が全て通過することを確認する。

## 解決方法

`backend/internal/api/errors.go` の `writeGCPError` に、既存の `*googleapi.Error` (REST 系)
分岐に加えて `google.golang.org/grpc/status.FromError(err)` で gRPC ステータスを取り出す分岐を
追加した。

- `gcpGRPCAPIDisabled` (新規) が `status.Details()` から `*errdetails.ErrorInfo` を探し、
  `Reason == "SERVICE_DISABLED"` であれば 403 `GCP_API_DISABLED` として有効化 URL を含む
  メッセージを返す。REST 系の `gcpAPIDisabled` と対になる判定。
- それ以外の gRPC コードは `grpcCodeToHTTP` (新規) で対応する HTTP ステータスに変換し、4xx
  相当なら REST 系と同じく `GCP_ERROR` として返す。5xx 相当・分類できないコードは 500
  `INTERNAL_ERROR` に倒す (REST 系の `gerr.Code >= 400 && < 500` と同じ方針)。
- `status.FromError` は `err` が `errors.As` 相当で見つかる場合 (`%w` でラップされていても)
  検出できるため、`ListLogEntries` の `fmt.Errorf("list gcp log entries: %w", err)` のような
  ラップも問題なく判定できる。

調査時点で対応方針に挙げていた「他の gRPC 系クライアント (`cloudrun.go` / `logging_tail.go`)
への同じ判定ロジックの適用」は、`writeGCPError` 自体を直したことで、呼び出し元
(`handlers_gcp.go` の各ハンドラ) を一切変更せずに `cloudrun` / `gcs` / `iam` / `logging` /
`logging_tail` を含む全 GCP ハンドラに自動的に適用される。個別の呼び出し箇所を洗い出す必要は
なかった。

### テストで判明した gRPC 特有の仕様

`google.golang.org/grpc/status.FromError` は、渡された `err` が `%w` でラップされている場合
(直接 `GRPCStatus() *Status` を実装していない場合)、返す `*Status` の `Message()` を
**`err.Error()` 全体 (ラップの prefix を含む文字列) へ書き換える**仕様になっている
(grpc-go `status.FromError` の godoc に明記: "For wrapped errors, the message returned
contains the entire err.Error() text and not just the wrapped status.")。

これはバグではなく意図された挙動であり、有効化 URL を含む本質的なメッセージ自体は失われない
(むしろラップの prefix が前に付与されて詳細になる) ため実害はない。ただし
`backend/internal/api/errors_test.go` のラップ済みケースでは、この仕様を踏まえてメッセージの
完全一致ではなく部分一致 (`wantMsgContains`) で検証するようにした。

### 実施した検証

- `backend/internal/api/errors_test.go` の `TestWriteGCPError` に、実際のバグ再現条件
  (gRPC `PermissionDenied` + `ErrorInfo{Reason: "SERVICE_DISABLED"}` detail、かつ `%w` で
  ラップされたケースを含む) を模したテストケースを追加し、403 `GCP_API_DISABLED` が返ることを
  確認した。あわせて gRPC `NotFound` → 404 `GCP_ERROR`、gRPC `Internal` → 500
  `INTERNAL_ERROR` のケースも追加し、既存 REST 系のケース (403 `GCP_API_DISABLED` /
  `GCP_ERROR` / 500 `INTERNAL_ERROR`) が壊れていないことも確認した。
- `mise run check` (backend/frontend の fmt + lint + test) が全て通過することを確認した。
- 実際の Cloud Logging API 無効プロジェクトへの curl による実機確認は、GCP 認証情報と
  対象プロジェクトが必要なため本セッションでは実施していない。ユニットテストで gRPC
  エラーの形状 (`ErrorInfo` detail 付き `PermissionDenied` ステータス) を実際のエラー
  ログの内容に基づいて再現し、判定ロジックを検証した。
