# 0009. API のエラーを HTTP ステータスとコードで分類し、frontend はコードで分岐する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

frontend は、エラーの種類ごとに違う対応を取る。
SSO の期限切れなら再ログインを案内し、アクセス拒否なら権限の不足を示す。
以前は AWS の AccessDenied を SSO の期限切れと取り違え、不要な再ログインを促していた (issue 0073)。
Google Cloud の REST と gRPC で、同じ種類のエラーが別の形で返っていた (issue 0037、0046)。

## 決定

- エラーの応答を `{"error", "code", "details"}` の形に揃える (`backend/internal/api/models.go` `ErrorResponse`)。
  frontend は `error` を本文として読み、`message` は互換のために受けるだけである (`frontend/src/api/client.ts`)。
  `http.ServeMux` の既定の 404 と 405、および `websocket.Accept` の失敗は、この形を通らず text/plain で返る。
- 種類ごとに HTTP ステータスとコードを決める。
  例: 401 `SSO_TOKEN_EXPIRED`、403 `ACCESS_DENIED`、403 `GCP_API_DISABLED`、503 `GCP_NOT_CONFIGURED`、401 `DATADOG_NO_CREDENTIALS`、413 `PREVIEW_TOO_LARGE`。
- 外部サービスのエラーから応答への変換は、系統ごとの関数に集める (`backend/internal/api/errors.go` の `writeAWSError`、`writeGCPError`、`writeDatadogError`、`writePricingError` と、`backend/internal/api/handlers_bigquery.go` の `writeBQError`)。
- `writeAWSError` は、判定の正確な `IsAccessDenied` を、判定の緩い `IsSSOTokenExpired` より先に調べる。
- frontend はメッセージの文言ではなくコードで分岐する (`frontend/src/api/client.ts` `ApiError`、`frontend/src/lib/ssoError.ts` `isSSOExpiredError`、`frontend/src/lib/datadogAuthError.ts` `isDatadogAuthError`)。

## 検討した代替案

issue 0166 は次の案を採らなかった。

- 500 のまま返し、frontend で `err.Error()` の部分一致で判定する。
  文言は安定した契約ではないため。
- Datadog のスコープ不足の 403 を `DATADOG_NO_CREDENTIALS` に含める。
  再ログインで解決しない失敗を再ログインに誘導するため。

## 結果

- 新しい種類のエラーを足すときは、backend のコードと frontend の分岐を両方変える。
- AWS が返した英語のエラーメッセージは、翻訳せずに表示する (ADR 0020)。

## 根拠資料

- `docs/issues/closed/0037`、`0046`、`0073`、`0105`、`0166`
- `backend/internal/api/errors.go`
- `AGENTS.md` の frontend 「API クライアントとエラーハンドリング」
