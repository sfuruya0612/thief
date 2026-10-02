# EC2 Session と ECS Exec の GET ルートが Origin の検査より前に AWS のセッションを開始し、他サイトからコマンドを実行できる

Created: 2026-10-02
Model: Claude Opus 5.5
Completed: 2026-10-02

## 症状

`GET /api/aws/profiles/{profile}/ec2/{instance}/session` と `GET /api/aws/profiles/{profile}/ecs/{cluster}/tasks/{task}/exec` は、WebSocket のアップグレードと Origin の検査 (`websocket.Accept`) より前に、AWS のセッションを開始してデータチャネルへ接続する。
利用者が backend を起動したままブラウザで悪意のあるサイトを開くと、そのサイトは `<img src="http://127.0.0.1:8089/api/aws/profiles/<profile>/ecs/<cluster>/tasks/<task>/exec?container=<container>&command=<command>">` のような GET を送れる。
このリクエストで、利用者の AWS 認証情報により ECS の ExecuteCommand が呼ばれ、`command` クエリパラメータで指定したコマンドがコンテナで実行される。
EC2 のルートでは、指定したインスタンスへの Session Manager のセッションが開始される。

issue 0200 の修正 (docs/issues/closed/0200、ADR 0032) は、Origin を持たない GET を通す。
Fetch Standard の「append a request `Origin` header」は、CORS モードでない GET (`<img>`、`<script>`、トップレベルのナビゲーション) に `Origin` を付けない。
そのため、このリクエストは `corsMiddleware` と `hostCheckMiddleware` を通り、ハンドラに届く。
この不具合は issue 0200 の修正前からあり、0200 の変更による退行ではない。

## 再現手順

実 AWS アカウントが要る (example/ の floci では ECS Exec と Session Manager を確認できない)。
プロファイル名、クラスタ名、タスク ID、コンテナ名は仮の名前 (`example-common` など) に置き換えて読む。

1. ECS Exec を有効にしたタスクを用意し、`mise run backend:run` で backend を起動する (`127.0.0.1:8089`)。
2. WebSocket のアップグレードのヘッダと `Origin` を付けずに、次のリクエストを送る。

   ```bash
   curl -s -D - 'http://127.0.0.1:8089/api/aws/profiles/example-common/ecs/example-cluster/tasks/<task-id>/exec?container=app&command=touch%20/tmp/thief-csrf'
   ```

3. 期待する結果: AWS の API を呼ぶ前にリクエストが拒否され、ECS の ExecuteCommand は呼ばれない。
4. 実際の結果: 応答は `websocket.Accept` の失敗 (WebSocket のアップグレードのヘッダが無い) によるエラーになるが、その前に `awsinternal.ExecuteECSCommand` と `session.OpenDataChannel` が実行されている。CloudTrail に `ExecuteCommand` のイベントが残り、コンテナ内で `touch /tmp/thief-csrf` が実行される (`aws ecs execute-command ... --command 'ls /tmp'` で確認できる)。
5. EC2 のルート (`GET /api/aws/profiles/example-common/ec2/<instance-id>/session`) でも、同じリクエストで CloudTrail に `StartSession` のイベントが残る。

## 原因

`backend/internal/api/handlers_session.go` の処理順による。

- `handleEC2Session` は `awsinternal.StartSSMSession` を、`handleECSExec` は `awsinternal.ExecuteECSCommand` を、`runSessionBridge` の呼び出しより前に呼ぶ。
- `runSessionBridge` は `session.OpenDataChannel` でデータチャネルに接続してから `websocket.Accept` を呼ぶ。Origin の検査 (`OriginPatterns: s.cfg.WebOrigins`) と、WebSocket のアップグレードのリクエストであることの検査は、この `websocket.Accept` の中で初めて行われる。
- `websocket.Accept` が失敗すると `terminateBestEffort` でセッションを終了するが、コンテナでのコマンドの実行はデータチャネルへの接続の時点で始まっている。
- `handleECSExec` は実行するコマンドを `command` クエリパラメータから受け取る (未指定時は `ecsExecDefaultCommand` の `/bin/sh`)。
- 2 つのルートは `backend/internal/api/routes.go` で GET として登録されている。issue 0200 の修正方針は、状態を変えるメソッドを GET、HEAD、OPTIONS 以外とみなして GET を通すが、この 2 つのルートは GET で状態を変える。
- `handlers_session.go` に対応するテストは無い。`StartSSMSession` と `ExecuteECSCommand` はパッケージ関数を直接呼んでおり、テストで差し替える口が無い。

## 完了条件

- WebSocket のアップグレードのリクエストでないもの、または `Origin` が `THIEF_WEB_ORIGINS` (`websocket.Accept` と同じ規則) で許可されないものは、`StartSSMSession`、`ExecuteECSCommand`、`OpenDataChannel` のいずれも呼ばずに拒否される。
- 上記を、`backend/internal/api` のテストで、AWS の呼び出しを差し替えて検査している (呼び出されないことを確認する)。
- frontend のターミナル (`frontend/src/api/terminal.ts` が開く 2 つの WebSocket) は、許可する Origin (`http://localhost:8088`) から従来どおり接続できる。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0200 (HTTP の API の CORS が任意の Origin を許可する不具合。Origin を持たない GET を通す方針)
- `docs/adr/0032-api-origin-and-host-checks.md`
- `backend/internal/api/handlers_session.go` `handleEC2Session`、`handleECSExec`、`runSessionBridge`
- `backend/internal/api/routes.go` (34-35 行)
- `frontend/src/api/terminal.ts`
- Fetch Standard "append a request `Origin` header": https://fetch.spec.whatwg.org/#append-a-request-origin-header

## 修正方針

2026-10-02 に確定した。

- `backend/internal/api/handlers_session.go` の `handleEC2Session` と `handleECSExec` は、AWS の呼び出しより前に `websocket.Accept` (`OriginPatterns: s.cfg.WebOrigins`) を呼ぶ。
  - Accept が失敗した場合 (WebSocket のアップグレードのリクエストでない、Origin が許可されない) は、Accept が書いた応答のまま終える。`StartSSMSession`、`ExecuteECSCommand`、`OpenDataChannel` はいずれも呼ばない。
  - `handleECSExec` の `container` の欠落 (`requireQueryParam` による 400) は、AWS を呼ばないため、従来どおりアップグレードより前に返す。
- アップグレードの後に AWS のセッションを開始し、データチャネルへ接続する。
  - どちらかが失敗した場合は、ブラウザへ `{"type":"error","message":"..."}` の TEXT メッセージを送ってから WebSocket を閉じる。この形は `frontend/src/components/Terminal/Terminal.tsx` が既に受け取り、メッセージを画面に表示する (`internal/session/bridge.go` の `controlTypeError` と同じ形)。
  - セッションの開始の後に失敗した場合は、従来どおり `terminateBestEffort` でセッションを終了する。
  - 従来はアップグレードの前に HTTP のエラー (401 `SSO_TOKEN_EXPIRED` など) を返していたが、ブラウザの WebSocket API は失敗した handshake の応答を読めず、画面には原因が出なかった。この変更で原因の文言がターミナルに表示される。
- テストで AWS の呼び出しを差し替えられるよう、`Server` に `startSSMSession`、`executeECSCommand`、`openDataChannel` の関数のフィールドを持たせ、`NewServer` で実装の関数を入れる。既存の `ec2Resources` や `s3Objects` と同じ形である。
- `backend/internal/api` のテストで、次を検査する。
  - アップグレードのヘッダの無い GET、許可されない Origin のアップグレード、`container` の無い ECS Exec のリクエストで、差し替えた関数がいずれも呼ばれない。
  - 許可する Origin (`http://localhost:8088`) のアップグレードで、差し替えた関数が呼ばれる。差し替えた関数が失敗を返すと、クライアントが `{"type":"error"}` のメッセージを受け取る。
- Origin を持たないアップグレード (curl などのブラウザ以外のクライアント) は、`websocket.Accept` の既定どおり通す。ブラウザ以外のクライアントを通すのは ADR 0032 と同じ扱いであり、ブラウザの WebSocket は handshake に必ず Origin を付ける (RFC 6455 4.1)。`<img>` や `fetch` は `Upgrade` と `Sec-WebSocket-Key` のヘッダを付けられないため (Fetch Standard の forbidden request-header)、アップグレードを要求すれば他サイトからの GET は通らない。
- ログの tail (`backend/internal/api/logtail.go` の `serveLogTail`) は既に Accept を先に呼んでおり、この変更で 2 つのルートもこれに揃う。

採らなかった案と却下の理由は次のとおりである。

- AWS を呼ぶ前に `originAllowed` (`backend/internal/api/middleware.go`) とアップグレードのヘッダを自前で検査し、処理順は変えない案は採らない。`websocket.Accept` の中の検査と二重になり、両者がずれる余地が残る。また、AWS の呼び出しの後の失敗がブラウザから読めないままになる。
- ルートを POST にする案、または CSRF のトークンを足す案は採らない。ブラウザの WebSocket API は GET の handshake しか送れず、ヘッダも付けられない。
- `corsMiddleware` で、この 2 つのルートの Origin の無い GET を拒否する案は採らない。アップグレードを要求すれば他サイトからの GET は通らず、Origin の検査は Accept が行うため、別の層に規則を足す理由が無い。

新しい API、権限、外部依存は不要である。

## 実装詳細の乖離 (2026-10-02)

方式 (AWS の呼び出しより前に `websocket.Accept` を呼び、アップグレード後の失敗を `{"type":"error"}` の TEXT メッセージで伝える) は変えていない。次の実装の詳細が修正方針の記述と異なる。

- `websocket.Accept` の呼び出しを、各ハンドラに書かず `acceptSessionWebSocket` (`backend/internal/api/handlers_session.go`) にまとめた。2 つのルートで `AcceptOptions` がずれないようにするためである。
- `runSessionBridge` の引数を `(w, r, result, terminate)` から `(ctx, browser, result, terminate)` に変えた。アップグレードが先になったため、ハンドラが受け取った `*websocket.Conn` を渡す。
- アップグレード後の失敗の通知とクローズは `failSessionSetup` にまとめた。クローズのステータスは `websocket.StatusInternalError` (1011。RFC 6455 7.4.1) とした。修正方針はステータスを定めておらず、frontend (`frontend/src/components/Terminal/Terminal.tsx`) はクローズのステータスを読まない。
- エラーメッセージの型 `sessionErrorMessage` を `backend/internal/api` に定義した。`internal/session/bridge.go` の `controlMessage` は非公開のため使えない。JSON の形 (`{"type","message"}`) は同じである。
- データチャネルへの接続に失敗したときは、`terminateBestEffort` でセッションを終了してから、エラーメッセージを送って閉じる。修正方針は「送ってから閉じる」と「セッションの開始の後に失敗したら terminate する」を定めるだけで、両者の順序を定めていない。AWS 側のセッションの後始末を先にした。
- 従来は `writeAWSError` で 401 `SSO_TOKEN_EXPIRED` などに対応付けていたセッションの開始の失敗は、エラーの文言 (`err.Error()`) をそのままメッセージにして送る。アップグレード後は HTTP のステータスを返せない。
- 修正方針は差し替える関数を `startSSMSession`、`executeECSCommand`、`openDataChannel` の 3 つとしていたが、`terminateSSMSession` (`awsinternal.TerminateSSMSession`) も `Server` のフィールドにした。データチャネルへの接続の失敗の経路では `terminateBestEffort` がセッションを終了するため、この関数を差し替えないと、実 AWS へ接続せずにこの経路をテストできない (Step 7 のレビューの指摘による)。

## 解決方法

EC2 Session と ECS Exec の GET ルートが、Origin とアップグレードの検査 (`websocket.Accept`) より前に AWS のセッションを開始していた処理順を、Accept を先に呼ぶ順に直した。アップグレードの要求でないリクエストは 426、`THIEF_WEB_ORIGINS` で許可しない Origin のアップグレードは 403 で、`StartSSMSession`、`ExecuteECSCommand`、`OpenDataChannel` のいずれも呼ばずに終わる (ECS Exec で `container` が無いリクエストは、従来どおりアップグレードより前に 400 で終わる)。

変更したファイルとシンボル。

- `backend/internal/api/server.go`: `Server` に `startSSMSession`、`executeECSCommand`、`openDataChannel`、`terminateSSMSession` の関数のフィールドを加え、`NewServer` で `awsinternal.StartSSMSession`、`awsinternal.ExecuteECSCommand`、`session.OpenDataChannel`、`awsinternal.TerminateSSMSession` を入れた。
- `backend/internal/api/handlers_session.go`:
  - `handleEC2Session` と `handleECSExec` は、`acceptSessionWebSocket` (`websocket.Accept` に `OriginPatterns: s.cfg.WebOrigins` を渡す) を呼んでから、`s.startSSMSession` または `s.executeECSCommand` を呼ぶ。`handleECSExec` の `requireQueryParam` による `container` の検査は、従来どおりアップグレードより前に行う。
  - `runSessionBridge` は、アップグレード済みの `*websocket.Conn` を受け取り、`s.openDataChannel` でデータチャネルへ接続してからブリッジを始める。接続に失敗したら、`terminateBestEffort` で `s.terminateSSMSession` を呼んでセッションを終了する。
  - アップグレード後の失敗 (セッションの開始、データチャネルへの接続) は、`failSessionSetup` が `{"type":"error","message":"..."}` の TEXT メッセージ (`sessionErrorMessage`) を送り、ステータス 1011 で WebSocket を閉じる。`frontend/src/components/Terminal/Terminal.tsx` はこのメッセージを画面に表示する。frontend は変えていない。
- `backend/internal/api/server_test.go`: `newTestServer` に、4 つのフィールドの、呼ばれるとエラーを返す実装を入れた。
- `backend/internal/api/handlers_session_test.go` (新規): 呼び出しの回数と引数を記録する `sessionStub` を 4 つのフィールドに入れ、`httptest.Server` で `s.Handler()` を動かして検査する (テストの一覧は次の「完了条件ごとの検証」)。

実装の詳細の、修正方針との違いは「## 実装詳細の乖離 (2026-10-02)」に書いた。

完了条件ごとの検証。

- アップグレードの要求でないもの、または許可しない Origin のものは、3 つの関数を呼ばずに拒否される:
  - `TestEC2SessionRejectsBeforeAWS`: アップグレードのヘッダの無い GET が 426、`Origin: https://evil.example` と `Origin: null` のアップグレードが 403 になり、`startSSMSession`、`executeECSCommand`、`openDataChannel` の呼び出しがいずれも 0 回である。
  - `TestECSExecRejectsBeforeAWS`: `container` の無い GET が 400、アップグレードのヘッダの無い GET が 426、`Origin: https://evil.example` と `Origin: null` のアップグレードが 403 になり、3 つの関数の呼び出しがいずれも 0 回である。
- AWS の呼び出しを差し替えて検査している: 上記のテストは、`newSessionTestServer` が `Server` のフィールドに入れた `sessionStub` の呼び出しの回数で検査する。テストは実 AWS へ接続しない。
- frontend のターミナルが、許可する Origin から従来どおり接続できる:
  - `TestEC2SessionAcceptsAllowedOrigin`: `Origin: http://localhost:8088`、`Origin: http://127.0.0.1:8088` (`THIEF_WEB_ORIGINS` の既定値の 2 つ)、Origin の無いアップグレードが 101 になり、`startSSMSession` が 1 回呼ばれる。その失敗の文言をクライアントが `{"type":"error"}` のメッセージで受け取り、WebSocket が 1011 で閉じる。
  - `TestECSExecAcceptsAllowedOrigin`: 同じ 2 つの Origin のアップグレードが 101 になり、`executeECSCommand` が 1 回呼ばれる。渡る引数 (プロファイル、リージョン、クラスタ、タスク、`container`、`command`。`command` の未指定時は `/bin/sh`) を確かめ、失敗の文言の受信と 1011 のクローズを確かめる。
  - `TestSessionDataChannelFailureTerminates`: EC2 と ECS で、セッションの開始が成功した後にデータチャネルへの接続が失敗すると、`terminateSSMSession` が開始したセッションの ID で 1 回呼ばれ、クライアントが `open data channel: ...` のメッセージを受け取り、WebSocket が 1011 で閉じる。
  - frontend の `frontend/src/api/terminal.ts` が組み立てる URL と `backend/internal/api/routes.go` のパスは変えていない。ブラウザからの実際の接続は、実 AWS アカウントが要るため確認していない。
- `mise run check` が通る: 5 段 (gofmt と goimports、prettier、`mise run lint`、`mise run backend:vuln`、`mise run test`) がすべて終了コード 0 で、ベースラインからの新たな失敗は無い (eslint の 9 件の警告はベースラインと同じ)。

実 AWS アカウントが無いため、不具合の再現は差し替えた関数で行った。3 つのフィールドだけを入れて処理順を変えない状態でテストを実行すると、`TestECSExecRejectsBeforeAWS` の許可しない Origin のケースは `status = 500, want 403` と `AWS calls = (start 0, execute 1, open data channel 0), want all 0` で失敗し、`TestEC2SessionAcceptsAllowedOrigin` と `TestECSExecAcceptsAllowedOrigin` は `expected handshake response status code 101 but got 500` で失敗した。処理順を直した後は、すべて通過した。`runSessionBridge` から `terminateBestEffort` の呼び出しを消すと `TestSessionDataChannelFailureTerminates` が、`failSessionSetup` のクローズのステータスを 1000 に変えると 1011 を確かめる 3 つのテストが失敗することも確かめた。
