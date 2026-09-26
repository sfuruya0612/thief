# 0011. HTTP サーバのタイムアウトは ReadHeaderTimeout だけを設定する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

`AGENTS.md` は、`http.Server` に `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout` を必ず設定するよう定めている。
一方で thief の API サーバは、EC2 の Session Manager と ECS Exec のターミナル、CloudWatch Logs と Cloud Logging の Live Tail を、1 つのハンドラの中で長時間続く WebSocket の接続として扱う。
`ReadTimeout` と `WriteTimeout` は接続全体に掛かるため、これらの接続を途中で切る。

## 決定

- `ReadHeaderTimeout` (10 秒) だけを設定する。
  Slowloris への対策として必須とする。
- `ReadTimeout`、`WriteTimeout`、`IdleTimeout` は設定しない。
- アイドルな接続の切断は、アプリケーションの層 (ブラウザの切断の検知によるブリッジの終了) に任せる。

## 検討した代替案

記録なし。
WebSocket のルートだけを別の `http.Server` に分ける案や、ハンドラごとに `http.ResponseController` で期限を延ばす案を比べた記録は無い。

## 結果

- `AGENTS.md` の規定と実装が食い違う。
  この差は `docs/prd/thief.md` の「規約と実装の差」に記録した。
- 通常の HTTP の API にも、読み書きの期限が掛からない。
- `AGENTS.md` が標準とするミドルウェアのうち、パニックの回復、リクエスト ID、タイムアウト、リクエストサイズの制限も入っていない (`backend/internal/api/middleware.go` は CORS とアクセスログだけ)。
  この差も同じ節に記録した。

## 根拠資料

- `backend/internal/api/server.go` `readHeaderTimeout`、`HTTPServer` のコメント
- `AGENTS.md` の backend 「HTTP / Web API サーバ」
- コミット 2aac22f (2026-07-08、`ReadHeaderTimeout` だけの構成を含む)
