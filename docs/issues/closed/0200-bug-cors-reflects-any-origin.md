# HTTP の API の CORS が任意の Origin を許可し、他サイトからシークレットの値を読める

Created: 2026-09-26
Model: Claude Opus 5.5
Completed: 2026-10-02

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

## 修正方針

2026-10-02 に、利用者の判断で案 A を採用した。
案 B は採らない。

`backend/internal/api` に Origin の検査と Host の検査を置き、`Server.Handler` で `loggingMiddleware` → Host の検査 → `corsMiddleware` → `mux` の順に組む。
Host の検査を CORS の外側に置くのは、DNS リバインディングでは `Origin` のホストと `Host` が一致し、Origin の検査だけでは同一オリジンとして通ってしまうためである。

### Origin の検査 (`corsMiddleware` の変更)

- `Origin` の照合は、WebSocket の `websocket.Accept` (`github.com/coder/websocket` v1.8.15 の `accept.go` `authenticateOrigin`) と同じ規則にする。
  `Origin` を URL として解析し、ホスト部 (`host:port`) が `r.Host` と大文字小文字を無視して一致するか、`cfg.WebOrigins` のいずれかのパターンに `path.Match` (パターンと対象を小文字にして照合) で一致すれば許可する。
  パターンが `://` を含む場合は `scheme://host` と照合する。
  HTTP の API と WebSocket で、許可する Origin の範囲を揃えるためである。
- 許可した Origin にだけ `Access-Control-Allow-Origin`、`Access-Control-Allow-Methods`、`Access-Control-Allow-Headers` を返す。
  `Origin` を持つリクエストの応答には、許可の有無にかかわらず `Vary: Origin` を付ける。
- 許可しない Origin のリクエストのうち、メソッドが GET、HEAD、OPTIONS 以外のものは、ハンドラに渡さず 403 (コード `FORBIDDEN_ORIGIN`) で拒否する。
  GET と HEAD はハンドラに渡し、CORS のヘッダを付けない。
  ブラウザは CORS のヘッダの無い応答を、他サイトのスクリプトに読ませない。
  OPTIONS (プリフライト) は従来どおり 204 を返すが、CORS のヘッダを付けないため、ブラウザは本リクエストを送らない。
- `Origin` ヘッダの無いリクエスト (curl などのブラウザ以外のクライアント、トップレベルのナビゲーションの GET) は従来どおり通す。
  Fetch Standard の「append a request `Origin` header」は、CORS モードのリクエストと、メソッドが GET でも HEAD でもないリクエストに `Origin` を付けると定める (値が `null` になる場合を含む)。
  そのため、ブラウザ上の他サイトからの POST と DELETE は必ず `Origin` を持ち、この検査で止まる。

### Host の検査 (新設)

- `r.Host` が許可するホストの一覧に含まれなければ、ハンドラに渡さず 421 Misdirected Request (コード `MISDIRECTED_REQUEST`) で拒否する。
  RFC 9110 の 15.5.20 は、対象 URI に対して権威のある応答を返せないサーバが 421 を返すと定める。
  DNS リバインディングでは `Host` が攻撃者のドメインになるため、この検査で止まる。
- 許可するホストは、起動時に `cfg` から次のとおり組み立てる。
  比較は大文字小文字を無視する。
  - `cfg.ListenAddr` のポートを `p` として、`localhost:p`、`127.0.0.1:p`、`[::1]:p`。
  - `cfg.ListenAddr` のホスト部が空でも未指定アドレス (`0.0.0.0`、`::`) でもなければ、`cfg.ListenAddr` そのもの。
  - `cfg.Datadog.OAuthRedirectBase` のホスト部。
    `OAuthRedirectBase` は、リバースプロキシ配下などでブラウザが backend に到達する URL を表す既存の設定項目である (`backend/internal/config/config.go` `DatadogConfig`)。
- `cfg.ListenAddr` を未指定アドレスにして、LAN の他の端末から IP アドレスで開く使い方は、`THIEF_DATADOG_OAUTH_REDIRECT_BASE` にその URL を設定しない限り 421 になる。
  thief の API は同じ端末のブラウザで動く frontend だけを利用者として想定する (ADR 0010) ため、この制約を受け入れる。

### 「関連する既知の問題」の 2 つの経路

- `multipart/form-data` の POST (プリフライトの無い単純リクエスト) による他サイトからのアップロード: ブラウザは POST に `Origin` を付けるため、Origin の検査の 403 で止まる。
- DNS リバインディング: Host の検査の 421 で止まる。

### frontend と ADR

- frontend は変更しない。
  frontend の配信元 (`http://localhost:8088`、`http://127.0.0.1:8088`) は `THIEF_WEB_ORIGINS` の既定値 (`localhost:8088`、`127.0.0.1:8088`) に一致し、API の宛先の既定値 (`http://127.0.0.1:8089`、`frontend/src/api/client.ts`) は Host の許可に含まれる。
- ADR 0010 を置き換える ADR 0032 (`docs/adr/0032-api-origin-and-host-checks.md`) を書き、ADR 0010 の `Status` を `Superseded by 0032` にし、`docs/adr/README.md` の表に反映する。

### 採らなかった案

- 案 B (起動時に生成したトークンを全 API で要求する): ブラウザ上の他サイトからの 2 つの経路 (Origin の偽装ができない読み取りと書き込み、DNS リバインディング) は、Origin と Host の検査で止まる。
  同じ端末の他のプロセスからの呼び出しは ADR 0010 のとおり許す前提であり、そうしたプロセスはトークンの置き場も読める。
  frontend へのトークンの渡し方の設計に見合う防御が残らない。
- 許可しない Origin の GET も 403 で拒否する: 情報の漏れは CORS のヘッダを付けないことで止まる。
  利用者が承認した案 A は、拒否の対象を状態を変えるメソッドに限っている。
- Host の不一致を 403 で拒否する: RFC 9110 は対象 URI の誤りを表すステータスとして 421 を定めている。
- HTTP の API 用に `THIEF_WEB_ORIGINS` と別の許可リストの設定を設ける: HTTP と WebSocket で方針が分かれることが、本 issue の原因の 1 つである。

### 新しい API と権限

- 新しいエンドポイントと環境変数は追加しない。
- エラーコード `FORBIDDEN_ORIGIN` (403) と `MISDIRECTED_REQUEST` (421) を追加する。
- 外部依存は追加しない (`net`、`net/http`、`net/url`、`path`、`strings` の標準ライブラリで足りる)。
- AWS などの外部サービスの権限は不要である。

## 実装詳細の乖離 (2026-10-02)

方式は修正方針のまま、次の実装詳細を加えた。

- `allowedHostsFromConfig` は、`net.SplitHostPort(cfg.ListenAddr)` が失敗した場合に、ホスト部を空、ポートを `80` として扱う。`cfg.ListenAddr` が空のとき、`net/http` は `:http` (ポート 80) で待ち受けるためである。ポートの無い空でない値 (`127.0.0.1` など) は `net.Listen` が `missing port in address` で拒否してサーバが起動しないため、空と同じ扱いにした。
- 待ち受けのポートが `80` のときは、ループバックと待ち受けのアドレスについて、ポートを省いた形 (`localhost`、`127.0.0.1`、`[::1]` など) も許可する。RFC 9110 の 4.2.3 は既定ポートを省いた形を http の URI の正規形とし、ブラウザは `http://127.0.0.1/` への要求の `Host` を `127.0.0.1` として送る。修正方針の一覧のままでは、ポート 80 で待ち受けたときに frontend の要求が 421 になる (Step 7 のレビューで判明)。
- `originAllowed` は、`cfg.WebOrigins` のパターンを先頭から照合し、不正なパターン (`path.Match` が `path.ErrBadPattern` を返すもの) に当たった時点で、`slog.Warn` (`invalid web origin pattern`) を出して不許可とする。不正なパターンより前のパターンに一致すれば許可する。`websocket.Accept` も同じ順で照合し、不正なパターンに当たるとログを出して 403 を返すため、照合の規則とログの出力を揃えた。
- 許可するホストの一覧は、ループバック、`cfg.ListenAddr`、`cfg.Datadog.OAuthRedirectBase` のホスト部の順に、大文字小文字を無視して重複を除いて組み立てる (既定値では `127.0.0.1:8089` が重複するため)。
- `Access-Control-Allow-Methods` と `Access-Control-Allow-Headers` の値を定数 `corsAllowMethods`、`corsAllowHeaders` にした。
- 許可するホストの一覧は `NewServer` が起動時に 1 回組み立て、`Server.allowedHosts` に持つ。テスト用の `newTestServer` (`backend/internal/api/server_test.go`) も同じく初期化する。

## 解決方法

`backend/internal/api/middleware.go` の `corsMiddleware` が Origin を照合するようにし、Host の検査 `hostCheckMiddleware` を新設した。

- `corsMiddleware(next, originPatterns)`: 引数に `cfg.WebOrigins` を取る形に変えた。`originAllowed` が許可した Origin にだけ `Access-Control-Allow-Origin`、`Access-Control-Allow-Methods` (`corsAllowMethods`)、`Access-Control-Allow-Headers` (`corsAllowHeaders`) を返す。`Origin` を持つ応答には許可の有無にかかわらず `Vary: Origin` を付ける。OPTIONS は従来どおり 204 を返す。許可しない Origin のリクエストのうち GET、HEAD 以外のメソッドは `writeError` で 403 (`FORBIDDEN_ORIGIN`) を返し、ハンドラに渡さない。`Origin` の無いリクエストは通す。
- `originAllowed`: `github.com/coder/websocket` v1.8.15 の `authenticateOrigin` と同じ規則で照合する。`Origin` のホスト部が `r.Host` と大文字小文字を無視して一致するか、パターンに `path.Match` (小文字にして照合、`://` を含むパターンは `scheme://host` と照合) で一致すれば許可する。パターンは先頭から照合し、不正なパターンに当たると `slog.Warn` を出して不許可とする。
- `hostCheckMiddleware(next, allowedHosts)` と `hostAllowed`: `r.Host` が一覧に無ければ 421 (`MISDIRECTED_REQUEST`) を返す。
- `allowedHostsFromConfig`: `cfg.ListenAddr` のポートの `localhost`、`127.0.0.1`、`[::1]`、未指定アドレスでない待ち受けのアドレス、`cfg.Datadog.OAuthRedirectBase` のホスト部から一覧を組み立てる。ポートが `80` のときは、ポートを省いた形も加える (定数 `defaultHTTPPort`)。
- `backend/internal/api/server.go`: `Server.allowedHosts` を追加して `NewServer` で組み立て、`Server.Handler` を `loggingMiddleware` → `hostCheckMiddleware` → `corsMiddleware` → `mux` の順にした。
- `backend/internal/api/server_test.go` `newTestServer`: `allowedHosts` を `NewServer` と同じく初期化する。
- `backend/internal/api/middleware_test.go`: `TestCORSMiddleware` を新しいシグネチャに合わせて書き直し、`TestOriginAllowed`、`TestHostCheckMiddleware`、`TestAllowedHostsFromConfig`、`TestHandlerAppliesOriginAndHostChecks`、`TestNewServerHandlerAllowsLoopbackHost` を追加した。
- ADR 0010 を置き換える `docs/adr/0032-api-origin-and-host-checks.md` を書き、ADR 0010 の `Status` を `Superseded by 0032` にし、`docs/adr/README.md` の表を更新した。
- frontend は変更していない。

方式を保ったままの実装詳細は「## 実装詳細の乖離 (2026-10-02)」に記録した。

完了条件の検証。

- 許可しない Origin の応答に `Access-Control-Allow-Origin` が無いこと: `TestCORSMiddleware` の `disallowed origin GET passes through without CORS headers`、`disallowed origin HEAD passes through without CORS headers`、`disallowed origin preflight gets no CORS headers`、`disallowed origin POST is rejected`、`disallowed origin DELETE is rejected`、`disallowed origin PUT is rejected`、`null origin POST is rejected` が、3 つの CORS のヘッダが無いことを検査する。`TestOriginAllowed` の `suffix-extended host does not match`、`userinfo does not make another host match`、`no patterns allow only the same host` が、許可パターンに似た Origin を許可しないことを検査する。
  - 例外として、`Origin` のホスト部が `Host` と一致する同一オリジンの要求は、`THIEF_WEB_ORIGINS` に無くても許可する (`websocket.Accept` と同じ規則。修正方針に記載)。`TestCORSMiddleware` の `same-host origin passes through with CORS headers` がこの挙動を固定する。同一オリジンの要求はブラウザが CORS を適用しないため、この例外で他サイトに応答が漏れることはない。
- 許可する Origin (`http://localhost:8088`) で従来どおり使えること: `TestCORSMiddleware` の `preflight from web client` が、プリフライトに 204 と CORS のヘッダを返すことを検査する。`cross-origin GET passes through with CORS headers`、`cross-origin POST passes through with CORS headers`、`cross-origin DELETE passes through with CORS headers` が、CORS のヘッダが付いてハンドラに届くことを検査する。`TestHandlerAppliesOriginAndHostChecks` の `allowed origin and host reaches the mux with CORS headers` が、`Server.Handler` を通した `Host: 127.0.0.1:8089`、`Origin: http://localhost:8088` の `GET /api/health` が 200 と `Access-Control-Allow-Origin` を返すことを検査する。`TestNewServerHandlerAllowsLoopbackHost` が、`NewServer` で作ったサーバが既定の待ち受けのループバック宛ての要求を通すことを検査する。frontend の配信元 (`localhost:8088`、`127.0.0.1:8088`) は `THIEF_WEB_ORIGINS` の既定値に含まれ (`TestOriginAllowed` の `default web origins allow the frontend`)、frontend の API の宛先 (`http://127.0.0.1:8089`) は Host の許可に含まれる。`frontend/vite.config.ts` はプロキシを持たず、frontend は API に直接接続するため、Host は `127.0.0.1:8089` のままである。
- 「関連する既知の問題」の 2 つの経路: どちらも対応した。`multipart/form-data` の POST は、ブラウザが POST に `Origin` を付けるため Origin の検査の 403 で止まる (`TestCORSMiddleware` の `disallowed origin POST is rejected`、`TestHandlerAppliesOriginAndHostChecks` の `disallowed origin cannot POST`)。DNS リバインディングは Host の検査の 421 で止まる (`TestHostCheckMiddleware` の `dns rebinding host is rejected`、`TestHandlerAppliesOriginAndHostChecks` の `dns rebinding host is rejected even with a same-host origin`)。
- ADR 0010 を置き換える ADR: `docs/adr/0032-api-origin-and-host-checks.md`。

再現確認。修正前は、ビルドしたサーバへの `curl -H 'Origin: https://evil.example' http://127.0.0.1:8089/api/health` の応答に `Access-Control-Allow-Origin: https://evil.example` が含まれ、プリフライトも 204 で同じヘッダを返した。修正後は、同じ GET が 200 と `Vary: Origin` だけを返し、プリフライトは 204 で CORS のヘッダを返さない。`Host: evil.example` は 421 (`MISDIRECTED_REQUEST`)、許可する Origin (`http://localhost:8088`) は `Access-Control-Allow-Origin` 付きの 200 を返した。POST の 403 は、実行環境の権限で curl の POST が拒否されたため、上記のテストで確認した。

Step 7 のレビューで、副作用を持つ GET のルートが別にあることが分かった。`GET .../ec2/{instance}/session` と `GET .../ecs/{cluster}/tasks/{task}/exec` は、Origin の検査 (`websocket.Accept`) より前に AWS のセッションを開始する。`Origin` を持たない GET (`<img>` など) で、他サイトからコマンドを実行できる。この不具合は本 issue の修正前からあり、本 issue の完了条件の範囲外であるため、docs/issues/0217 として登録した。

`mise run check` の 5 段 (gofmt と goimports、prettier、`mise run lint`、`mise run backend:vuln`、`mise run test`) がすべて終了コード 0 で通った。eslint の警告 9 件と govulncheck の出力はベースラインと同じである。
