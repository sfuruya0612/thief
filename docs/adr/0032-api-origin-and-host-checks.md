# 0032. API サーバは Origin と Host を検査し、利用者の認証は持たない

Created: 2026-10-02
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-10-02

## 状況

ADR 0010 は、API サーバをループバックで待ち受け、利用者の認証を持たないと決めた。
同じ ADR は、HTTP の API の CORS がリクエストの `Origin` をそのまま `Access-Control-Allow-Origin` に返す実装を記録した。
この実装では、利用者が backend を起動したままブラウザで開いた任意のサイトが、API の応答 (Secrets Manager と Parameter Store の値を含む) を読み、書き込みの API を呼べる (issue 0200)。
WebSocket の接続だけは `THIEF_WEB_ORIGINS` で `Origin` を検査しており、HTTP の API と方針が一致していなかった。

CORS を直しても防げない経路が 2 つあった。

- `multipart/form-data` の POST はプリフライトの無い単純リクエストであり、CORS はその送信を止めない。
- backend は `Host` を検査していなかった。
  DNS リバインディング (攻撃者のドメインを `127.0.0.1` に解決させる) では、攻撃者のページと API が同じオリジンになり、CORS を経由せずに応答を読める。

## 決定

- API サーバの待ち受けと、利用者の認証を持たないことは ADR 0010 から引き継ぐ。
- HTTP の API は、`Origin` を WebSocket と同じ規則で `THIEF_WEB_ORIGINS` と照合する。
  照合の規則は `github.com/coder/websocket` の `websocket.Accept` に合わせる。
  `Origin` のホスト部が `Host` と大文字小文字を無視して一致するか、`THIEF_WEB_ORIGINS` のパターンに `path.Match` で一致すれば許可する。
  パターンと照合する対象は、パターンが `://` を含めば `scheme://host`、含まなければホスト部 (`host:port`) であり、どちらも小文字にして照合する。
  パターンは先頭から照合し、不正なパターンに当たった時点で許可しない。
  - 許可した `Origin` にだけ CORS のヘッダを返す。
  - 許可しない `Origin` のリクエストは、メソッドが GET、HEAD、OPTIONS 以外なら 403 (`FORBIDDEN_ORIGIN`) で拒否する。
  - `Origin` の無いリクエストは通す。
- API サーバは `Host` を検査し、許可するホストの一覧に無ければ 421 (`MISDIRECTED_REQUEST`) で拒否する。
  許可するホストは、待ち受けのポートの `localhost`、`127.0.0.1`、`[::1]`、待ち受けのアドレスそのもの (未指定アドレスを除く)、`THIEF_DATADOG_OAUTH_REDIRECT_BASE` のホスト部である。
  待ち受けのポートが 80 のときは、`localhost` などのポートを省いた形も許可する。ブラウザは既定ポートを `Host` から省いて送る (RFC 9110 の 4.2.3 と 7.2)。
- Host の検査は CORS より外側で行う。

## 検討した代替案

- 起動時に生成したトークンを全 API で要求する。
  ブラウザ上の他サイトからの読み取りと書き込み、DNS リバインディングは、Origin と Host の検査で止まる。
  同じ端末の他のプロセスからの呼び出しは ADR 0010 のとおり許す前提であり、そうしたプロセスはトークンの置き場も読める。
  frontend へのトークンの渡し方の設計に見合う防御が残らない。
- 許可しない `Origin` の GET も拒否する。
  応答の漏れは CORS のヘッダを返さないことで止まるため、拒否の対象を状態を変えるメソッドに限った。
- `Host` の不一致を 403 で拒否する。
  RFC 9110 の 15.5.20 は、対象 URI に対して権威のある応答を返せないことを表すステータスとして 421 を定めている。
- HTTP の API 用に `THIEF_WEB_ORIGINS` と別の許可リストの設定を設ける。
  HTTP と WebSocket で許可の範囲が分かれることが、issue 0200 の原因の 1 つだった。
- `Host` の許可リストを環境変数で設定できるようにする。
  API の利用者は同じ端末のブラウザで動く frontend だけであり、それ以外の到達経路は既存の `THIEF_DATADOG_OAUTH_REDIRECT_BASE` で表せる。

## 結果

- ブラウザ上の他サイトは、API の応答を読めず、状態を変える API を呼べない。
  DNS リバインディングによる到達も `Host` の検査で止まる。
- frontend の配信元を変えるときは、`THIEF_WEB_ORIGINS` に配信元を加える必要がある。
  変更前は、HTTP の API だけは任意の配信元から使えた。
- 待ち受けを未指定アドレス (`0.0.0.0` など) にして、LAN の他の端末から IP アドレスで開く使い方は、`THIEF_DATADOG_OAUTH_REDIRECT_BASE` にその URL を設定しない限り 421 になる。
- 同じ端末の他のプロセスは、ADR 0010 のとおり API を呼べる。

## 根拠資料

- `docs/issues/closed/0200`
- ADR 0010
- `backend/internal/api/middleware.go` `corsMiddleware`、`backend/internal/api/server.go` `Server.Handler`
- `backend/internal/config/config.go` (`THIEF_LISTEN_ADDR`、`THIEF_WEB_ORIGINS`、`THIEF_DATADOG_OAUTH_REDIRECT_BASE`)
- `github.com/coder/websocket` v1.8.15 の `accept.go` `authenticateOrigin`
- RFC 9110 "HTTP Semantics" 15.5.20 (421 Misdirected Request): https://www.rfc-editor.org/rfc/rfc9110#section-15.5.20
- RFC 9110 "HTTP Semantics" 4.2.3 (http(s) Normalization and Comparison) と 7.2 (Host and :authority): https://www.rfc-editor.org/rfc/rfc9110#section-4.2.3
- Fetch Standard "append a request `Origin` header": https://fetch.spec.whatwg.org/#append-a-request-origin-header
