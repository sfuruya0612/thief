# Datadog 認証が無効なときに frontend からブラウザで再ログインできるようにする

Created: 2026-09-11
Model: Claude Sonnet 5
Completed: 2026-09-11

## 背景

現状の実装はファイルとシンボルで次のとおり確認できる。

- backend: `backend/internal/api/handlers_datadog.go` の `handleDatadogHistorical`/`handleDatadogEstimated` は `serveCached` のエラー writer に `writeInternalFromError` (`backend/internal/api/errors.go:56-58`) を渡しており、これはどんな error でも無条件に 500 `INTERNAL_ERROR` にマップする。
- `backend/internal/api/datadog_auth_context.go` の `datadogStaticKeyContext` (105-111 行) が返す `ErrDatadogNoCredentials` (17 行) は、OAuth トークンが無い / 壊れている / リフレッシュに失敗した、のいずれかの場合で、かつ静的キー (`DATADOG_API_KEY`/`DATADOG_APP_KEY`) も未設定 (値が空) のときに返る唯一の choke point である。`datadogStaticKeyContext` は静的キーの値の有無だけを見ており、その値が Datadog に対して実際に有効かどうかは検証しない。
- 実機で `GET /api/datadog/cost/historical` を呼ぶと次のエラーが返ることを確認済みである。

  ```
  500 INTERNAL_ERROR no usable Datadog credentials: no Datadog OAuth token is stored; run 'thief datadog auth login', and DATADOG_API_KEY / DATADOG_APP_KEY are not both set
  ```

  この文言は `backend/internal/api/datadog_auth_e2e_test.go:229-233` の既存 e2e テストが検証している内容と一致する。

- frontend の `DatadogView.tsx` (`frontend/src/views/nonaws/DatadogView.tsx` 81 行) は `error` を汎用の `ErrorBanner` にしか渡しておらず、Datadog 専用の判定は存在しない (`frontend/src/lib/` 配下に Datadog 認証エラー判定のファイルが無いことを確認済み)。
- 対して AWS 側は `frontend/src/lib/ssoError.ts` の `isSSOExpiredError` が 401 + `code === 'SSO_TOKEN_EXPIRED'` を判定し、`frontend/src/components/SSOExpiredBanner.tsx` が `useSSOLogin` (`frontend/src/api/queries.ts:624-669`) でブラウザの再ログインを行う。backend 側は `writeUnauthorized` (`errors.go:44-46`) がこのコードを返す専用 helper であり、`writeAWSError`/`writePricingError` (67-102 行) が判定関数によるエラーの種別分けを行っている。`useSSOLogin` は `postSSOLoginStart` → `postSSOLoginComplete` という 1 回のブロッキング POST で完了を待つ実装であり、ポーリングは行わない。
- issue 0165 (`docs/issues/closed/0165-feat-datadog-oauth-api-server-fallback.md`) は backend の OAuth 対応 (ログイン系 4 エンドポイント: `POST /api/datadog/auth/login/start`、`GET /api/datadog/auth/callback`、`GET /api/datadog/auth/login/status`、`POST /api/datadog/auth/logout`。実装は `backend/internal/api/handlers_datadog_auth.go`) を実装済みだが、frontend の UI 変更を明示的にスコープ外としている (同 issue 110 行)。
- `login/start` は成功時 `{state, authorization_url}` を返す一方、クライアント登録に今回の redirect_uri が無い場合は 409 `DATADOG_REDIRECT_URI_NOT_REGISTERED` を返す (`handlers_datadog_auth.go:99-109`)。`OAuthRedirectBase` の変更後にクライアントを再登録すると、以前発行されたトークンがすべて無効になるため、再ログインしても解決しない構成変更が起きたことを示す (同ファイル 82-86 行のコメント)。
- `login/status` はセッションが見つからない場合 404 `DATADOG_LOGIN_SESSION_NOT_FOUND` を返す (`handlers_datadog_auth.go:184-188`)。ログインセッションはメモリのみで保持され TTL 10 分であり、永続化されない (`backend/internal/api/datadog_auth_sessions.go:12-13,38-40`)。
- Datadog は `s.cfg.Datadog.Site` による単一サイト運用であり、AWS のような複数 profile の概念を持たない (`backend/internal/config/config.go` の `DatadogConfig`/`Datadog` フィールド定義)。
- このリポジトリの frontend は、状態が変わるまでポーリングする問題を `useQuery` + `refetchInterval` (状態に応じて interval か `false` を返す関数) で解決する規約を既に複数箇所で確立している (`frontend/src/api/queries.ts` の `useProfiles`/`useHealthCheck` がそれぞれ個別のインライン関数でこの形を使い、`useBQQueryJob` 系は共通関数 `pollWhileActive` を使う)。

TODO 由来であり、`docs/issues/TODO.md:120` の次の項目に対応する。

> Frontend で Datadog の認証 (OAuth トークン/静的キーとも) が無効・期限切れのときに、AWS SSO ログインの SSOExpiredBanner と同じように、ブラウザ上で再ログインできる導線を追加したい

## 目的

Datadog のコスト取得が認証不備 (`ErrDatadogNoCredentials` 相当) で失敗したとき、ユーザーが画面を離れず、ブラウザ上のバナーからワンクリックで Datadog の OAuth ログインを完了し、そのまま同じ画面でコストデータを見られるようにする。

## 設計判断

### 1. backend: `ErrDatadogNoCredentials` 専用の HTTP エラー分類を新設する

- 採用: `errors.go` に `writeDatadogCostError(w http.ResponseWriter, err error)` を新設する。`errors.Is(err, ErrDatadogNoCredentials)` のとき `401` + 新しいコード `DATADOG_NO_CREDENTIALS` を返す (`writeError(w, http.StatusUnauthorized, "DATADOG_NO_CREDENTIALS", err.Error())`)。それ以外は既存どおり `writeInternalError` (500 `INTERNAL_ERROR`) にフォールバックする。`handleDatadogHistorical`/`handleDatadogEstimated` の `serveCached` 呼び出しのエラー writer 引数を `writeInternalFromError` からこれに差し替える。
- 却下: 既存の `writeUnauthorized` (401 + `SSO_TOKEN_EXPIRED` 固定) をそのまま流用する案。`writeUnauthorized` はコード文字列を `SSO_TOKEN_EXPIRED` に固定しており、AWS の SSO 期限切れと意味が異なる Datadog のケースに同じコードを返すと frontend 側の判定 (`isSSOExpiredError` 等) が誤って反応しうるため却下する。
- 却下: 500 のまま `err.Error()` の文字列内容 (`"no usable Datadog credentials"`) を frontend で `includes()` 判定する案。バックエンドのエラーメッセージ文言はログ/デバッグ用であり安定した契約ではなく、文言変更で frontend が壊れるため却下する (このリポジトリは HTTP ステータス + `code` による分類を規約としており、`errors.go` の既存 3 関数もこの方式による)。
- 却下: `datadogCall` (`datadog_auth_context.go:210-225`) の 403/`ddclient.IsForbidden` (OAuth トークンは有効だがスコープ不足で拒否され、静的キーへのフォールバックも失敗した場合) も同じ `DATADOG_NO_CREDENTIALS` に含める案。この場合は有効な OAuth トークンが既に存在しており、再ログインしても同じスコープ不足のトークンが再発行されるだけで解消しない。再ログイン導線の対象外とし、既存どおり 500 `INTERNAL_ERROR` のままとする (この issue のスコープ外として明記する)。
- **対象外の明記**: TODO は「静的キーとも…無効・期限切れ」を含むが、静的キーが設定されているのに Datadog に拒否される (キー失効等) 場合は、`datadogStaticKeyContext` が値の有無しか見ないため `ErrDatadogNoCredentials` にならず (`datadog_auth_context.go:105-111`)、この issue の `writeDatadogCostError` でも 500 `INTERNAL_ERROR` のまま変わらない。静的キーは backend 運用者が環境変数で設定するものであり、ユーザーのブラウザ操作 (再ログイン) では修正できないため、この issue は「OAuth トークン不備、かつ静的キー未設定」の再ログインで解決できるケースに限定する。静的キーが設定されているのに無効な場合の検知・専用エラー分類は対象外とする。

### 2. frontend: Datadog 認証エラー判定の集約

- 採用: `frontend/src/lib/datadogAuthError.ts` を新設し、`ssoError.ts` と同じ形で `DATADOG_NO_CREDENTIALS_CODE = 'DATADOG_NO_CREDENTIALS'` と `isDatadogAuthError(err: unknown): boolean` (`err instanceof ApiError && err.code === DATADOG_NO_CREDENTIALS_CODE`) を定義する。

### 3. frontend: ログイン開始はミューテーション、完了待ちは既存のポーリング規約 (`useQuery` + `refetchInterval`) に従う

- 採用: ログイン開始 (`POST /api/datadog/auth/login/start` の呼び出しと認可タブの遷移) は `useMutation` (`useDatadogLoginStart`) で行う。AWS 同様、click ハンドラの同期処理で `window.open('', '_blank')` してから開始し (transient activation の失効を防ぐ)、応答 `{state, authorization_url}` で認可タブを `location.replace(authorization_url)` へ遷移させる。完了待ちは、このリポジトリが確立済みの規約 (`useProfiles`/`useHealthCheck`/`useBQQueryJob` 系の `pollWhileActive` と同じ、状態に応じて `refetchInterval` が interval か `false` を返す形) に倣い、`useDatadogLoginStatus` という別のフックで `useQuery({queryKey: ['datadog', 'login-status', state], enabled: !!state, refetchInterval: ...})` を実装し、`GET /api/datadog/auth/login/status?state=...` をポーリングする。完了を検知したら認可タブを閉じ、`queryClient.invalidateQueries({queryKey: ['datadog']})` を呼ぶ。`useDatadogLoginStart`/`useDatadogLoginStatus` はいずれも `frontend/src/hooks/useDatadogLogin.ts` に定義し、同ファイルからこの 2 つの名前で export する。
- 却下: ミューテーション本体の中で独自に `setInterval`/`setTimeout` を組んでポーリングする案。このリポジトリは同種の「状態が変わるまでポーリングする」問題を `useQuery` + `refetchInterval` で解決する規約を既に複数箇所 (`useProfiles`、`useHealthCheck`、`useBQQueryJob` 系の `pollWhileActive`) で確立しており、独自ループはこの規約と重複する車輪の再発明になるため却下する。
- 却下: `useSSOLogin` をそのまま呼び出す/汎用化して両方で使う案。AWS の `postSSOLoginComplete` はサーバ側でブロックして完了を待つ 1 回の POST だが、Datadog の完了検知は `login/status` のポーリングによるものであり、待ち方の構造そのものが異なる (ブロッキング呼び出し 1 回 vs 定期ポーリング)。無理に共通化すると、片方に無い分岐 (ポーリングの停止条件、打ち切り) がもう片方の型に漏れ、可読性を損なうため却下する。ポップアップ回避 (`window.open` の同期呼び出し) とタブの close/refocus という UX パターンだけを流用し、待機のロジックは別実装とする。
- `login/status` が 404 `DATADOG_LOGIN_SESSION_NOT_FOUND` を返した場合は、セッションが (TTL 経過またはサーバ再起動で) 失われたことを意味するため、打ち切り時間を待たずポーリングを停止し、ログインフローをエラー終了させる。
- `login/start` が 409 `DATADOG_REDIRECT_URI_NOT_REGISTERED` を返した場合は、認可タブへの遷移を行わず、通常のログイン失敗とは異なる、`OAuthRedirectBase` の変更に伴う再登録が必要である旨の専用メッセージを返す。
- ポーリング間隔・打ち切り時間の具体値は実装時に妥当な値 (例: 2 秒間隔、120 秒程度で打ち切り) を決定する。値そのものは体感速度の調整であり客観的な正誤が無いため、完了条件の対象にしない。

### 4. frontend: バナーコンポーネント

- 採用: `frontend/src/components/DatadogAuthBanner.tsx` を新設し、`SSOExpiredBanner.tsx` と同じ構造 (ログインボタン、pending/error 表示) を持たせる。`DatadogView.tsx` の `error && <ErrorBanner error={error} />` の分岐を、`isDatadogAuthError(error)` が真のときはこのバナー、それ以外は従来どおり `ErrorBanner` を出す形に分岐する (`SSOExpiredBanner` 採用済みの各ビューと同じ判定順)。
- 却下: `SSOExpiredBanner` を Datadog でも共用する案。`profile` (AWS プロファイル名) を前提にした props と i18n キー (`sso.*`) を持ち、Datadog には profile という概念が無い (`s.cfg.Datadog.Site` が固定される単一サイト運用) ため、共用すると無関係な props が残ってしまい却下する。
- `login/start` が 409 `DATADOG_REDIRECT_URI_NOT_REGISTERED` を返した場合、バナーは通常の pending/error 表示ではなく、`OAuthRedirectBase` の変更に伴う再登録が必要である旨の専用メッセージを表示する (設計判断 3 参照)。

### 5. i18n

- 採用: 既存の `errors` ネームスペース (`frontend/src/i18n/locales/ja/errors.json`) に `datadog.*` キー群を追加する (`sso.*` に倣った命名: `datadog.credentialsMissing`、`datadog.pending`、`datadog.failed`、`datadog.loginButton`、`datadog.redirectNotRegistered` 等)。

## 完了条件

- `backend/internal/api/errors.go` に `writeDatadogCostError` が追加され、`errors.Is(err, ErrDatadogNoCredentials)` の場合に 401 + `DATADOG_NO_CREDENTIALS` を返し、それ以外は 500 `INTERNAL_ERROR` を返すことをユニットテストで確認できる。
- `handleDatadogHistorical`/`handleDatadogEstimated` (`handlers_datadog.go`) の `serveCached` 呼び出しが `writeDatadogCostError` をエラー writer として使うことをコードで確認できる。
- OAuth トークン未ログインかつ静的キー未設定の状態で `GET /api/datadog/cost/historical` または `/estimated` を呼んだとき、レスポンスが 401 + `{"code": "DATADOG_NO_CREDENTIALS"}` であることを httptest で確認できる。
- `datadogCall` の 403 (`IsForbidden`) 経路がこの変更でも 500 `INTERNAL_ERROR` のままであること (再ログイン対象にしない) をユニットテストで確認できる。
- 静的キーが設定されているが Datadog に拒否される (キー失効等) 場合、この issue の対象外 (ブラウザ再ログインでは解決しないため) として専用エラー分類を追加せず、既存どおり 500 `INTERNAL_ERROR` のままであることを確認できる。
- `frontend/src/lib/datadogAuthError.ts` に `isDatadogAuthError` があり、`code === 'DATADOG_NO_CREDENTIALS'` の `ApiError` に対して `true` を、それ以外 (未設定・別コード・`ApiError` でない値) に対して `false` を返すことをユニットテストで確認できる。
- `frontend/src/hooks/useDatadogLogin.ts` の `useDatadogLoginStart`/`useDatadogLoginStatus` の呼び出しで、`POST /api/datadog/auth/login/start` の応答に基づき認可タブが遷移し、`GET /api/datadog/auth/login/status` のポーリング (`useQuery` + `refetchInterval`) が完了を検知した時点で停止し、認可タブが閉じられ、`queryClient` の `['datadog']` クエリが invalidate されることをテストで確認できる。
- ポーリングが打ち切り時間に達しても完了しない場合、ログインフローがエラー終了し、バナーがエラー表示に切り替わることを確認できる。
- `login/status` が 404 `DATADOG_LOGIN_SESSION_NOT_FOUND` を返した場合、打ち切り時間を待たずにポーリングが停止し、ログインフローがエラー終了することを確認できる。
- `login/start` が 409 `DATADOG_REDIRECT_URI_NOT_REGISTERED` を返した場合、バナーが通常のログイン失敗メッセージとは異なる、再登録が必要である旨の専用メッセージを表示することを確認できる (コンポーネントテストまたは手動確認)。
- `frontend/src/components/DatadogAuthBanner.tsx` が `DatadogView.tsx` で `isDatadogAuthError(error)` が真のときに表示され、ログインボタンのクリックでログインフローが開始されることを確認できる (コンポーネントテストまたは手動確認)。
- `DatadogView.tsx` で、`isDatadogAuthError(error)` が偽の Datadog エラー (`DATADOG_NO_CREDENTIALS` 以外) では従来どおり `ErrorBanner` が表示されることを確認できる。
- ポップアップブロック時 (`window.open` が `null` を返す場合) のフォールバックリンクの `href` が `login/start` 応答の `authorization_url` と一致することを確認できる。
- `datadogCall` の 403 経路 (OAuth スコープ不足) はこの issue の対象外とし、再ログインバナーを表示しない。
- `mise run check` が通過する。

## 関連

- docs/issues/closed/0164、docs/issues/closed/0165: backend の Datadog OAuth 対応 (CLI / API サーバ) の実装元。本 issue はその frontend 側の利用導線を追加する後続である。

## 解決方法

- backend: `backend/internal/api/errors.go` に `writeDatadogCostError` を追加した。`errors.Is(err, ErrDatadogNoCredentials)` のとき 401 + `DATADOG_NO_CREDENTIALS` を返し、それ以外 (403 スコープ不足、静的キー拒否を含む) は既存どおり `writeInternalError` で 500 `INTERNAL_ERROR` を返す。`handlers_datadog.go` の `handleDatadogHistorical`/`handleDatadogEstimated` の `serveCached` 呼び出しのエラー writer 引数を `writeInternalFromError` から `writeDatadogCostError` に差し替えた (他ハンドラの `writeInternalFromError` 利用箇所は変更していない)。`serveCached` の godoc コメントもこの差し替えに合わせて更新した。
- backend: 既存 e2e テスト `datadog_auth_e2e_test.go` の `TestDatadogOAuthEndToEnd`/`TestDatadogCorruptTokenFileFallsBackWithWarning` の資格情報無し時のアサーションを、500 `INTERNAL_ERROR` から 401 `DATADOG_NO_CREDENTIALS` へ更新した。
- backend: `errors_test.go` に `TestWriteDatadogCostError` (素の sentinel / `%w` ラップ / 403 スコープ不足 / 一般エラーの 4 ケース) を、`handlers_datadog_test.go` に `TestDatadogCostWithoutCredentialsReturnsUnauthorized` (historical/estimated 両パス) と `TestDatadogCostRejectedCredentialsStayInternalError` (403 スコープ不足、静的キー拒否の 2 ケース × 両パス) を新設した。
- frontend: `frontend/src/lib/datadogAuthError.ts` を新設し、`ssoError.ts` と同じ形で `DATADOG_NO_CREDENTIALS_CODE` と `isDatadogAuthError` を定義した。`datadogAuthError.test.ts` で `SSO_TOKEN_EXPIRED`/`INTERNAL_ERROR`/素の `Error`/`null` がいずれも偽になることを含む 5 ケースを検証した。
- frontend: `frontend/src/hooks/useDatadogLogin.ts` を新設し、`useDatadogLoginStart` (`POST /api/datadog/auth/login/start` を呼び、click ハンドラが同期的に開いた空タブを認可 URL へ遷移させる `useMutation`) と `useDatadogLoginStatus` (`GET /api/datadog/auth/login/status` を `useQuery` + `refetchInterval` でポーリングし、`succeeded` で認可タブを閉じて `['datadog']` クエリを invalidate する) を実装した。ポーリング間隔 2000ms、打ち切り 120000ms を `DATADOG_LOGIN_POLL_INTERVAL`/`DATADOG_LOGIN_POLL_TIMEOUT` として同ファイルから export した。`useDatadogLogin.test.tsx` で、認可タブの遷移、succeeded 時の停止・タブ close・invalidate、404 `DATADOG_LOGIN_SESSION_NOT_FOUND` での即時停止を検証した。
- frontend: `frontend/src/components/DatadogAuthBanner.tsx` を新設した。`SSOExpiredBanner.tsx` と同じ構造 (ログインボタン、pending/error 表示、ポップアップブロック時のフォールバックリンク) を持ち、`.sso-banner` 系の既存 CSS クラスを再利用する (新規 CSS クラスは追加していない)。`login/start` が 409 `DATADOG_REDIRECT_URI_NOT_REGISTERED` を返した場合は認可タブへ遷移せず、通常の失敗文言 (`datadog.failed`) とは異なる専用メッセージ (`datadog.redirectNotRegistered`) を表示する。`DatadogAuthBanner.test.tsx` で、ログインフロー開始、409 時の専用メッセージ表示 (認可タブへの非遷移を含む)、ポップアップブロック時のフォールバックリンクの `href` が `authorization_url` と一致することを検証した。
- frontend: `frontend/src/views/nonaws/DatadogView.tsx` の `error && <ErrorBanner error={error} />` を `isDatadogAuthError(error)` で分岐させ、真のときは `DatadogAuthBanner`、偽のときは従来どおり `ErrorBanner` を表示するようにした。`DatadogView.test.tsx` で両分岐と、バナーのログインボタンのクリックでログインフローが開始することを検証した。
- frontend: `frontend/src/api/endpoints.ts` に `postDatadogLoginStart`/`getDatadogLoginStatus` を、`frontend/src/types/nonaws.ts` に `DatadogLoginStartRaw`/`DatadogLoginStartRow`/`DatadogLoginStatusRaw`/`DatadogLoginStatusRow`/`DatadogLoginStatus` を、`frontend/src/lib/normalizeNonAws.ts` に `datadogLoginStartFromRaw`/`datadogLoginStatusFromRaw` を追加した (既存の Raw/Row 分離規約に従う)。`frontend/src/lib/normalizeNonAws.test.ts` を新設し、両関数の全分岐 (`datadogLoginStatusFromRaw` の pending/succeeded/failed/error_message 省略/未知の status 値 → failed のフォールバック) を検証した。
- frontend: `frontend/src/i18n/locales/ja/errors.json`/`en/errors.json` に `datadog.*` キー群 (`credentialsMissing`/`pending`/`fallbackLink`/`failed`/`redirectNotRegistered`/`loginButton`/`loginButtonPending`) を `sso.*` に倣って追加した。

### 完了条件との対応

- `writeDatadogCostError` の追加とマッピング: `TestWriteDatadogCostError` (4 ケース) で確認。
- `serveCached` の呼び出しが `writeDatadogCostError` を使う: コード上の差し替えで確認 (`handlers_datadog.go` 2 箇所)。
- 資格情報無しで 401 `DATADOG_NO_CREDENTIALS`: `TestDatadogCostWithoutCredentialsReturnsUnauthorized` (historical/estimated) で確認。
- 403 スコープ不足は 500 のまま: `TestDatadogCostRejectedCredentialsStayInternalError` で確認。
- 静的キー拒否も対象外のまま 500: 同テストの静的キーケースで確認。
- `isDatadogAuthError`: `datadogAuthError.test.ts` (5 ケース) で確認。
- 認可タブ遷移とポーリングの開始/停止/close/invalidate: `useDatadogLogin.test.tsx` で確認。
- 打ち切りでエラー終了: `useDatadogLogin.test.tsx` の打ち切りケースで確認。
- 404 で即時停止: `useDatadogLogin.test.tsx` のセッション消失ケースで確認。
- 409 で専用メッセージ: `DatadogAuthBanner.test.tsx` で確認。
- バナー表示条件と分岐: `DatadogView.test.tsx` で確認。
- フォールバックリンクの href: `DatadogAuthBanner.test.tsx` で確認。
- 403 経路で再ログインバナーを出さない: `DatadogView.test.tsx`/`TestDatadogCostRejectedCredentialsStayInternalError` で確認 (バナーの対象外は `isDatadogAuthError` が偽になることで担保)。
- `mise run check` 通過: 下記テスト結果のとおり。

### 方針からの乖離 (方式は保ったままの実装詳細)

- `invalidateQueries` に `predicate: (query) => query.queryKey[1] !== 'login-status'` を追加した。方針どおり `queryKey: ['datadog']` で invalidate すると、ポーリングクエリ自身 (`['datadog', 'login-status', state]`) も対象に入り、再取得 → succeeded → invalidate の無限ループになるため、ポーリングクエリ自身を除外する条件を加えた。
- ポーリングの停止条件を「404 のときだけ」ではなく「クエリがエラー状態になったとき (`refetchInterval` が `false` を返す、`retry: false`)」に一般化した。404 (`DATADOG_LOGIN_SESSION_NOT_FOUND`) は queryFn 内で throw してエラー状態にする実装のため、完了条件の「404 で打ち切りを待たず即停止」は満たしたまま、failed 応答や打ち切り (deadline 超過) でも同じ機構で停止する。
- ポーリング間隔 2000ms / 打ち切り 120000ms を採用した (完了条件が対象外とする自由枠)。

### 多観点レビューでの指摘と決着

3 観点 (完了条件の充足、テストと堅牢性、規約と整合) のレビューを行い、次の指摘があった。

- [中、テストと堅牢性] `datadogLoginStatusFromRaw` の未知の status 値 → `failed` フォールバック分岐が無テストで、`normalizeNonAws.test.ts` 自体が存在しなかった。→ 反映: `frontend/src/lib/normalizeNonAws.test.ts` を新設し、全分岐を検証した。
- [低、テストと堅牢性] ログイン中に `DatadogAuthBanner` がアンマウントされる (別ビューへの遷移等) と、開いた認可タブ (`authWindow`) が閉じられないまま残る。→ 却下: `SSOExpiredBanner.tsx` も同じパターン (アンマウント時のクリーンアップなし) であり、本 issue が持ち込んだ回帰ではない既存パターンであることをコードで確認した。完了条件にもアンマウント時の挙動は含まれないため、この issue のスコープ外として扱う。
- [中、規約と整合] `backend/internal/api/server.go` の `serveCached` の godoc コメントが「datadog は `writeInternalFromError`」のまま実装と食い違っていた。→ 反映: `writeDatadogCostError` に触れる記述へ更新した。

上記の反映内容についてラウンド 2 の追加レビュー (3 観点) を行い、`normalizeNonAws.test.ts` の未知 status テストに不要な二重キャスト (`as unknown as 'pending'`) が残っているという低優先度の指摘のみを受けて修正した。以降の指摘はゼロ件で、close 処理に進んだ。

- `mise run check` → exit 0。
  - backend: 全パッケージ ok (go vet / staticcheck / govulncheck 問題なし)
  - frontend: Test Files 80 passed (80) / Tests 824 passed (824)。ベースライン (75 files / 794 tests) から新規追加分のみ純増、新たな失敗なし。
