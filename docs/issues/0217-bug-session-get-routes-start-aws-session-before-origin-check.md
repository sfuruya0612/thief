# EC2 Session と ECS Exec の GET ルートが Origin の検査より前に AWS のセッションを開始し、他サイトからコマンドを実行できる

Created: 2026-10-02
Model: Claude Opus 5.5

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
