# 0010. API サーバをループバックで待ち受け、利用者の認証を持たない

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

thief の API サーバは、利用者の端末で、利用者の認証情報 (`~/.aws/`、ADC、Datadog のトークン) を使って外部サービスを呼ぶ。
API の利用者は、同じ端末のブラウザで動く thief の frontend だけを想定している。

## 決定

- API サーバは既定で `127.0.0.1:8089` で待ち受ける (`THIEF_LISTEN_ADDR` で変更可)。
- API に利用者の認証 (トークン、セッション) を設けない。
- WebSocket の接続 (ターミナル、Live Tail) は、`THIEF_WEB_ORIGINS` に列挙した Origin からだけ受け付ける (既定 `localhost:8088`、`127.0.0.1:8088`)。
- HTTP の API の CORS は、リクエストの `Origin` をそのまま `Access-Control-Allow-Origin` に返す (`backend/internal/api/middleware.go` `corsMiddleware`)。

## 検討した代替案

記録なし。
認証を設ける案と、CORS を `THIEF_WEB_ORIGINS` に限る案を比べた記録は見つかっていない。

## 結果

- frontend の配信元 (開発サーバ、`vite preview`) が変わっても、HTTP の API はそのまま使える。
- 同じ端末で動く他のプロセスは、API を呼べる。
- HTTP の API は任意の Origin を許すため、利用者のブラウザで開いた任意のサイトが、API の応答 (Secrets Manager と Parameter Store の値を含む) を読み、書き込みの API を呼べる。
  WebSocket の Origin の検査と、HTTP の CORS の方針が一致していない。
- この扱いは `docs/prd/thief.md` の「未確定論点」に残し、不具合として issue 0200 (`docs/issues/0200-bug-cors-reflects-any-origin.md`) に起票した。
  方針を決めたら、この ADR を置き換える ADR を書く。

## 根拠資料

- `backend/internal/api/middleware.go` `corsMiddleware`
- `backend/internal/api/handlers_session.go` `runSessionBridge`
- `backend/internal/config/config.go` (`THIEF_LISTEN_ADDR`、`THIEF_WEB_ORIGINS`)
- コミット 2aac22f (2026-07-08、CORS の実装を含む)
