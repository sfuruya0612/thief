# HTTP の API の CORS が任意の Origin を許可し、他サイトからシークレットの値を読める

Created: 2026-09-26
Model: Claude Opus 5.5

## 概要

`backend/internal/api/middleware.go` の `corsMiddleware` は、リクエストの `Origin` ヘッダの値を検査せずに `Access-Control-Allow-Origin` に返す。
API には利用者の認証が無い (`docs/adr/0010-local-api-without-authentication.md`)。
そのため、利用者が thief の backend を起動したままブラウザで悪意のあるサイトを開くと、そのサイトの JavaScript が `http://127.0.0.1:8089` の API を呼び、応答を読める。
読めるものには、Secrets Manager のシークレットの値 (`GET /api/aws/profiles/{profile}/secretsmanager/value`) と Parameter Store の値 (`GET /api/aws/profiles/{profile}/ssm/parameters/value`) が含まれる。
書き込みの API (`POST /api/aws/profiles/{profile}/secretsmanager`、`POST .../ssm/parameters`、`POST .../s3/{bucket}/objects/upload` など) も、プリフライトを通って呼べる。

## なぜ対応が必要か

- 関数のコメントは「adds CORS headers for localhost web clients」であり、許可の対象を localhost の Web クライアントに限る意図がある。実装はこの意図と異なる。
- WebSocket の接続 (`backend/internal/api/handlers_session.go` `runSessionBridge`) は `THIEF_WEB_ORIGINS` で Origin を検査している。HTTP の API だけが検査していない。同じ backend の中で方針が一致していない。
- 漏れる情報は、利用者の AWS アカウントのシークレットである。利用者は thief の backend を起動したまま、同じブラウザで他のサイトを開く。攻撃の前提は、利用者が細工したページを開くことだけである。

## 再現手順

1. `mise run backend:run` で backend を起動する (`127.0.0.1:8089`)。
2. 次のコマンドで、thief と無関係な Origin からのリクエストを送る。

   ```bash
   curl -s -D - -o /dev/null -H 'Origin: https://evil.example' http://127.0.0.1:8089/api/health
   ```

3. 応答ヘッダに `Access-Control-Allow-Origin: https://evil.example` が含まれる。
4. プリフライトも同じく許可される。

   ```bash
   curl -s -D - -o /dev/null -X OPTIONS \
     -H 'Origin: https://evil.example' \
     -H 'Access-Control-Request-Method: POST' \
     -H 'Access-Control-Request-Headers: Content-Type' \
     http://127.0.0.1:8089/api/aws/profiles/default/secretsmanager
   ```

   応答は 204 で、`Access-Control-Allow-Origin: https://evil.example` と `Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS` を含む。
5. ブラウザでは、`https://evil.example` のページの `fetch('http://127.0.0.1:8089/api/aws/profiles/<profile>/secretsmanager/value?name=<name>')` が成功し、本文を読める。

期待する挙動: `THIEF_WEB_ORIGINS` に含まれない Origin には `Access-Control-Allow-Origin` を返さず、ブラウザが応答の読み取りを拒否する。

## 関連する既知の問題

CORS を直すだけでは防げない経路がある。修正の方針を決めるときに併せて検討する。

- CORS はブラウザが応答を読むことを止めるだけで、リクエストの送信は止めない。`multipart/form-data` の POST はプリフライトの無い単純リクエストなので、CORS を直しても、他サイトからのオブジェクトのアップロード (`POST .../s3/{bucket}/objects/upload`) は送られる。
- backend は `Host` ヘッダを検査していない。DNS リバインディング (攻撃者のドメインを `127.0.0.1` に解決させる) では、攻撃者のページと API が同じオリジンになり、CORS を経由せずに応答を読める。

## 修正方針の案

方針は設計判断を要する。案を挙げる。

- 案 A: `corsMiddleware` が、`Origin` が `THIEF_WEB_ORIGINS` に含まれるときだけ `Access-Control-Allow-Origin` を返す。状態を変えるメソッド (POST、DELETE) は、`Origin` が許可リストに無ければ 403 で拒否する。`Host` ヘッダが待ち受けのアドレス (`127.0.0.1:8089`、`localhost:8089`) でなければ 421 か 403 で拒否する。
- 案 B: 案 A に加え、backend が起動時に生成したトークンを frontend に渡し、全 API でトークンを要求する。frontend へのトークンの渡し方 (起動時の URL、設定ファイル) の設計が要る。

## 完了条件

- `THIEF_WEB_ORIGINS` に含まれない Origin のリクエストの応答に `Access-Control-Allow-Origin` が含まれないことを、`backend/internal/api/middleware_test.go` のテストで検査している。
- 許可リストに含まれる Origin (`http://localhost:8088`) では、従来どおり frontend から全機能が使える。
- 「関連する既知の問題」の 2 つの経路について、対応したか、対応しない理由を本 issue に記録している。
- 方針を決めたら、`docs/adr/0010-local-api-without-authentication.md` を置き換える ADR を書いている。

## 関連

- `docs/adr/0010-local-api-without-authentication.md`
- `docs/prd/thief.md` の「非機能要求」の「セキュリティ」と「未確定論点」
- `backend/internal/api/middleware.go` `corsMiddleware`
- `backend/internal/config/config.go` (`THIEF_WEB_ORIGINS`)
- コミット ed4726d (2026-07-26、CORS の許可メソッドに DELETE を追加)
