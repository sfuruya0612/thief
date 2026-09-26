# Cost Explorer 系ハンドラの SSO トークン期限切れを 401 SSO_TOKEN_EXPIRED にマップする

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「Cost Explorer 系ハンドラの SSO トークン期限切れを 401 SSO_TOKEN_EXPIRED にマップしたい (issue 0094 のスコープ外として送り)」に対応する。

## 症状

SSO トークンが期限切れの状態で Cost Explorer 画面を開くと、他の AWS サービス画面が `SSOExpiredBanner` (再ログインの導線付き) を表示するのに対し、Cost Explorer 画面だけは原因の分からない 500 エラーとして `ErrorBanner` に表示される。

この症状はコードの読解から導いたもので、期限切れトークンでの実機観測はまだ行っていない。
再現手順の実施と観測結果の記録を完了条件に含める。

## 再現手順

1. SSO トークンが期限切れの AWS プロファイルを用意する。時間経過を待てない場合は `~/.aws/sso/cache/*.json` の `expiresAt` を過去の時刻に書き換えて期限切れ状態を作る。
2. `mise run backend:run` と `mise run frontend:run` でサーバを起動する。
3. 期限切れプロファイルで任意の AWS サービス (例: EC2) を開き、`SSOExpiredBanner` が表示されることを確認する (比較対象)。
4. 同じプロファイルで Cost Explorer を開く。
5. 期待: `SSOExpiredBanner` が表示される。観測 (コードからの導出): `/api/cost` が 500 `INTERNAL_ERROR` を返し、`ErrorBanner` が表示される。

## 原因

`backend/internal/api/handlers_cost.go` の 25 行と 51 行は、`serveCached` に渡すエラー writer として `writeInternalFromError` を使っている。
`writeInternalFromError` (`backend/internal/api/errors.go` 56-58 行) はどんなエラーも 500 `INTERNAL_ERROR` にする。
一方、他の AWS リソースハンドラは `writeAWSError` (`errors.go` 67-77 行) を使っており (`handlers_aws.go` に 38 箇所)、こちらは SSO トークン期限切れを 401 `SSO_TOKEN_EXPIRED` に、AccessDenied を 403 にマップする。

frontend は `frontend/src/lib/ssoError.ts` の `isSSOExpiredError` が `code === 'SSO_TOKEN_EXPIRED'` で判定して `SSOExpiredBanner` を出す仕組みを既に持ち、`CostExplorerPanel.tsx` (127 行、152 行) もこの判定を実装済みである。
つまり frontend 側の受け皿は整っており、backend のエラーマップだけが欠けている。

docs/issues/closed/0073 (SSO 期限切れ判定の順序修正) は、同じ `writeInternalFromError` 経路をスコープ外に挙げていたが、それは権限エラーの 403 化の文脈であり、SSO 期限切れの 401 マップの欠落そのものは扱っていない (0073 自身が「401 誤マップは起こさず (500 になる)、症状が異なる」と記録している)。

## 修正方針

- `handlers_cost.go` の 2 箇所のエラー writer を `writeInternalFromError` から `writeAWSError` に差し替える。
- Cost Explorer 専用のエラー writer を新設する案は採らない。`writeAWSError` は AWS SDK のエラー全般を対象にした共通マップであり、Cost Explorer だけ別扱いする理由が無い。
- Cost 系の既存パターンとして `writePricingError` (`errors.go` 90-102 行) があり、こちらは Throttling を 429 にマップする。この形式に揃える案も採らない。本 issue の範囲は SSO 期限切れの 401 マップの欠落であり、Throttling の 429 化は性質の異なる挙動追加になる。したがって差し替え後も Throttling は 500 のままである。429 化が必要になった場合は別 issue として起票する。
- `writeAWSError` は AccessDenied の 403 マップも含むため、差し替えにより Cost Explorer の権限エラーも 403 になる。docs/issues/closed/0073 は `writeInternalFromError` 経路の権限エラーの 403 化を「実環境で問題として観測された事例が無いため、本バッチでは起票しない」として見送っているが、これは 403 化を目的とする変更を単独では起票しないという判断であり、cost 経路の権限エラーを 500 のままにする決定ではない。本 issue は 401 マップの欠落を直す目的で共通 writer に揃え、その結果として権限エラーの扱いも `writeAWSError` を使う他の AWS リソースハンドラ 38 箇所と同じになる。なお Cost Explorer (GetCostAndUsage) の権限エラーが smithy のどのエラーコードで返るかは実環境でしか確認できないため、403 マップの動作確認は完了条件に含めない。
- SSO 期限切れだけを 401 にマップし AccessDenied は 500 のままにする専用 writer を新設する案も採らない。エラー writer の変種が 3 つに増え、Cost Explorer だけ権限エラーの扱いが他のハンドラと恒久的に食い違う上、`writeAWSError` のマップが変わるたびに追随の要否を判断する保守が増える。
- BigQuery / Datadog / TiDB のハンドラは SSO の概念が無いためスコープ外とする。

## 完了条件

- `handlers_cost.go` の 2 箇所が `writeAWSError` を使う。
- SSO 期限切れマップのハンドラテスト (`handlers_cost.go` 経由で HTTP 401 と `code: SSO_TOKEN_EXPIRED` が返るケース) を `net/http/httptest` で追加する。
- 期限切れの SSO トークンで `/api/cost` を呼ぶと HTTP 401 と `code: SSO_TOKEN_EXPIRED` が返り、Cost Explorer 画面に `SSOExpiredBanner` が表示されることを実機で確認し、観測結果を本 issue に記録する。期限切れ状態は再現手順 1 の `expiresAt` の書き換えで作れるため、SSO を設定したプロファイルがあれば実機確認を省略しない。実機確認を省略できるのは、実装環境の `~/.aws/config` に `sso_session` または `sso_start_url` を含むプロファイルが 1 つも無い場合に限る。その場合は上記ハンドラテストでの検証をもって代え、その判定根拠と併せて本 issue に記録する。
- BigQuery / Datadog / TiDB のハンドラには変更を加えない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0094: 本 issue の起票元。Cost Explorer の絞り込み実装時にスコープ外として送られた。
- docs/issues/closed/0073: SSO 期限切れ判定の順序修正。同じ `writeInternalFromError` 経路を権限エラーの 403 化の文脈でスコープ外に挙げていたが、SSO 期限切れの 401 マップは扱っていない。

## 実機確認の観測記録

確認日: 2026-08-07。
実装環境の `~/.aws/sso/cache/` に期限切れ (`expiresAt` が過去) のトークンを持つ SSO プロファイル example-sso が存在したため、完了条件の定めに従い実機確認を省略しなかった。
`expiresAt` の書き換えは不要で、自然に期限切れになった状態をそのまま使った。

### API の観測 (修正前後)

`mise run backend:run` で起動した API サーバに対し、期限切れプロファイル example-sso で観測した。

修正前は `/api/aws/profiles/example-sso/cost` と `/cost/forecast` の両方が HTTP 500 を返した。
レスポンスボディは次のとおりで、`code` は `INTERNAL_ERROR` だった。

```
{"error":"get cost and usage: operation error Cost Explorer: GetCostAndUsage, get identity: get credentials: failed to refresh cached credentials, the SSO session has expired or is invalid","code":"INTERNAL_ERROR"}
```

修正後は同じ 2 エンドポイントが HTTP 401 と `code: SSO_TOKEN_EXPIRED` を返した (症状の解消)。

### 画面の観測 (修正後)

Vite dev server (http://localhost:8088、確認時点で起動済みだったものを使用) を headless Chrome (`--headless=new --screenshot`) で開き、スクリーンショットで表示を確認した。

- 再現手順 3 (比較対象): EC2 画面で `SSOExpiredBanner` (「example-sso の SSO セッションが期限切れです。再ログインしてください。」と SSO 再ログインボタン) が表示された。
- 再現手順 4 と 5: Cost Explorer 画面 (cost & usage) で同じ `SSOExpiredBanner` が表示され、汎用の `ErrorBanner` は表示されなかった。

headless 確認のための一時的な作業として、`localStorage` (`cloudlens:v1`) へ view と profile を注入する一時ページを `frontend/public/` に置いた。
また、初期表示サービスを Cost Explorer にするため `App.tsx` の `useState('ec2')` を一時的に `'costexplorer'` へ変えた。
どちらも確認後に元へ戻し、作業ツリーには残っていない (git status で修正対象の 4 ファイルのみであることを確認済み)。

## 解決方法

- `backend/internal/api/handlers_cost.go` の handleCost と handleCostForecast が `serveCached` に渡すエラー writer を `writeInternalFromError` から `writeAWSError` に差し替えた。これにより SSO トークン期限切れは HTTP 401 と `code: SSO_TOKEN_EXPIRED` に、AccessDenied は 403 にマップされる。
- `backend/internal/api/server.go` の `serveCached` の godoc コメントを、エラー writer の使い分け (AWS リソース系と cost は writeAWSError、gcp は writeGCPError、それ以外は writeInternalFromError) が分かる記述に更新した。
- 回帰テストとして `backend/internal/api/handlers_cost_test.go` に TestHandleCostSSOTokenExpired を追加した。t.Setenv で HOME / AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE を差し替え、期限切れの SSO トークンキャッシュを fixture として注入し、handleCost と handleCostForecast が 401 と SSO_TOKEN_EXPIRED を返すことを httptest で検証する。トークンの期限切れは資格情報の解決段階で検出されるため、AWS への実リクエストは発生しない。
- `frontend/src/views/CostExplorerPanel.test.tsx` に、getCost が 401 SSO_TOKEN_EXPIRED の ApiError で失敗したとき SSOExpiredBanner が表示され、汎用の ErrorBanner が表示されないことを検証するテストを追加した。
- 実機確認の結果は「## 実機確認の観測記録」のとおりで、修正前は 500 INTERNAL_ERROR、修正後は 401 SSO_TOKEN_EXPIRED が返り、Cost Explorer 画面での SSOExpiredBanner の表示を確認した。
- BigQuery / Datadog / TiDB のハンドラには変更を加えていない。
- `mise run check` が通ることを確認した。
