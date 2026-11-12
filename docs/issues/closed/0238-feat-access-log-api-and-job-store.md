# アクセスログ集計の API とジョブ管理を追加する

Created: 2026-11-02
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-11-02

## 背景

Access Logs ビューは、ログソースの検出、テーブルの検証、Athena と Logs Insights の集計の開始、状態の取得、結果の取得、取り消しを API サーバ経由で行う。
Athena と Logs Insights のクエリは非同期で、複数のソースを並行して実行するため、実行 ID と状態をまとめて管理する必要がある。
フロントエンドに実行 ID を持たせて毎回送り返す方式は、タブを閉じた後にクエリが走り続け、取り消せなくなる。
ADR 0037 は、API サーバのメモリでジョブを管理し、TTL と取り消しを持つことを決めた。

## 目的

`backend/internal/api` に Access Logs のエンドポイントとジョブの管理を追加し、`internal/aws` と `internal/accesslog` を配線する。

- `GET /api/aws/profiles/{profile}/access-logs/sources`
- `POST /api/aws/profiles/{profile}/access-logs/table/validate`
- `POST /api/aws/profiles/{profile}/access-logs/query`
- `GET /api/aws/profiles/{profile}/access-logs/query/{job}`
- `GET /api/aws/profiles/{profile}/access-logs/query/{job}/results`
- `POST /api/aws/profiles/{profile}/access-logs/query/{job}/cancel`

## 設計判断

- ジョブの管理は `backend/internal/api/access_log_jobs.go` に `accessLogJobStore` を置く。
  - ジョブは ID、プロファイル、リージョン、ビュー (`pairs` か `ips`)、ソースの一覧、実行 ID、状態、作成時刻、更新時刻を持つ。
  - ID は `crypto/rand` の 16 バイトを base64 URL で表す。
  - TTL は 10 分とする。期限切れのジョブは janitor が削除し、実行中のクエリは `StopQueryExecution` または `StopQuery` で止める。
  - 取得と更新は `sync.Mutex` で保護する。ロックの取得順序は 1 つだけにし、ロックの中で AWS を呼ばない。
  - 既存の `ssoLoginSessionStore` と `datadogLoginSessionStore` の形に合わせる。
- ソースの検出は `serveCached` を使い、キーを `access-logs-sources:<profile>:<region>`、TTL を 1 時間とする。
  `X-Cache-*` のヘッダは既存の `serveCached` が付ける。
- テーブルの検証はキャッシュしない。
  検証の結果は `TableValidation` として返し、必須列、パーティション、Location、SerDe、推奨するスキーマプロファイルを含める。
- 集計の開始は、リクエストのソースを `accesslog.Source` に解決し、次の制限を掛ける。
  - ソースは最大 5 つ
  - 期間は 24 時間以内、7 日は Athena のソースでパーティションがあり、`confirm_long_range` が `true` のときだけ
  - Logs Insights のソースは 24 時間以内
  - 結果は最大 200 行
  - 認証済みのプロファイルとリージョンが、ソースのプロファイルとリージョンに一致する
  - `destination_type=none` または `logging_enabled=false` のソースは拒否する
  - `Attribution` が false のソースを、同じ出力先を共有する別のソースと同時に選んだ場合は拒否する
  - ログ出力が無効なソースしか選ばれていない場合は、400 `ACCESS_LOGS_SOURCE_NOT_READY` を返す
- ソースの指定は、検出の応答に含まれる `id` を使う。
  `id` は種類、リージョン、リソースの ARN、出力先、`source_key` を含む不透明な文字列とし、API 層で検出結果と照合する。
  クライアントが任意の ARN やテーブル名を直接指定することはできない。
  テーブルの指定は `table` オブジェクトで受け取り、データベース名とテーブル名を `^[A-Za-z0-9_]+$` で検証する。
  同じ出力先を共有するソースは、同じ `table` を指定できる。
  テーブルの対応付けはソースの ID ではなく出力先の ID をキーにして保存し、共有するソースで 1 つのテーブルを再利用できるようにする。
- ジョブの状態は、Athena は `GetQueryExecution`、Logs Insights は `GetQueryResults` で取得する。
  API は `GET /query/{job}` でソースごとの状態、スキャンしたバイト数、レコード数、エラーを返す。
  結果は `GET /query/{job}/results` で、全ソースが終端状態になったときに返す。
  1 つのソースが失敗しても、ほかのソースの結果は返す。
- 取り消しは `POST /query/{job}/cancel` で行い、実行中のクエリをすべて止める。
  すでに終端状態のソースは無視する。
- エラーコードは既存の `errors.go` に追加する。
  - `ACCESS_LOGS_INVALID_SOURCE` (400)
  - `ACCESS_LOGS_SOURCE_NOT_READY` (400)
  - `ACCESS_LOGS_TABLE_INVALID` (400)
  - `ACCESS_LOGS_QUERY_FAILED` (502)
  - `ACCESS_LOGS_JOB_NOT_FOUND` (404)
  - `ACCESS_LOGS_JOB_EXPIRED` (410)
  SSO の期限切れは `SSO_TOKEN_EXPIRED`、権限不足は `ACCESS_DENIED` を既存の `writeAWSError` の方針で返す。
- `backend/internal/contract/contract.go` の `Registry` に Access Logs の応答型を追加し、ゴールデン JSON を再生成する。
- ログにはソースの種類と ID だけを出し、IP、URI、User-Agent は出さない。
- この issue ではフロントエンドを含めない。

## 完了条件

- 6 つのエンドポイントが `routes.go` に登録され、`handlers_access_logs.go` にハンドラがある。
- `accessLogJobStore` がジョブの作成、取得、状態の更新、取り消し、TTL の清理を行う。
- TTL の期限切れと取り消しで、実行中の Athena と Logs Insights のクエリが停止される。
- 検出の応答が 1 時間キャッシュされ、`X-Cache-Status` などのヘッダが付く。
- ソースの ID、プロファイル、リージョン、テーブル名、期間が検証され、不正な場合は 400 になる。
- ソース数、期間、結果行数の上限が API 層で強制される。
- ログが無効なソースだけが選ばれた場合に 400 `ACCESS_LOGS_SOURCE_NOT_READY` になる。
- `Attribution` が false のソースを、同じ出力先を共有する別のソースと同時に選ぶと 400 になる。
- 同じ出力先を共有する複数のソースに同じテーブルを対応付けられ、ジョブが `source_key` ごとの結果を返す。
- 1 つのソースが失敗しても、ほかのソースの結果が 200 で返り、`sources[].error` にコードとメッセージが入る。
- SSO の期限切れが 401 `SSO_TOKEN_EXPIRED`、権限不足が 403 `ACCESS_DENIED` になる。
- ジョブが見つからない場合が 404、期限切れが 410 になる。
- ハンドラのテストが、正常系、部分失敗、SSO の期限切れ、権限不足、不正なソース、期間の上限、取り消し、TTL の清理を確認する。
- contract のゴールデン JSON が再生成され、`frontend/src/types/contract.check.ts` の型検査が通る。
- `mise run backend:test` が通る。
- `mise run check` が通る。

## 関連

- ADR 0037
- issue 0235、0236、0237、0239、0240
- `backend/internal/api/server.go`、`routes.go`、`errors.go`、`handlers_cache.go`、`sso_login_sessions.go`
- `backend/internal/contract/contract.go`

## 解決方法

- `backend/internal/api` に Access Logs の 6 つのエンドポイントを追加した。
- `accessLogJobStore` を追加し、ジョブの作成、取得、状態の更新、取り消し、TTL の期限切れによる清理を実装した。期限切れと取り消しでは実行中の Athena と Logs Insights のクエリを止める。
- ソースの検出を 1 時間キャッシュし、TopBar の Refresh で破棄される `access-logs-sources` のキーを追加した。
- ソースの ID、プロファイル、リージョン、テーブル名、期間、ソース数、結果行数を検証し、ログが無効なソースと `Attribution` が false のソースの同時選択を拒否するようにした。
- 1 つのソースが失敗しても、ほかのソースの結果を 200 で返し、ソース別のエラーを含めるようにした。
- SSO の期限切れ、権限不足、不正なソース、テーブル、期間、ジョブの不在と期限切れのエラーコードを追加した。
- `accessLogJob` にミューテックスを追加し、状態の取得と取り消しの競合を防ぐようにした。
- ハンドラのテストで、正常系、部分失敗、SSO の期限切れ、不正なソース、期間の上限、ジョブの不在を確認した。
- contract に `AccessLogQueryStartResponse` と `AccessLogSourceError` を追加し、ゴールデン JSON を再生成した。
- `mise run check` が通る。
