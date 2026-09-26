# 0030 AWS SSO 再ログイン後もセッション期限が期限切れ表示のまま更新されない

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8 claude-opus-4-8

## 症状

AWS のセッション期限表示が、SSO 再ログイン後も「期限切れ」のまま更新されない。
アクティブセッションカード (`AwsActiveSessionCard`) の有効期限バッジと残り時間、および各ビューの `SSOExpiredBanner` が、ブラウザで `aws sso login` を完了した後も期限切れ状態を表示し続ける。
ページをリロードするか TopBar の手動 Refresh を押すまで、正しい期限に更新されない。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` でアプリを起動する
2. SSO セッションが期限切れの AWS プロファイルを選択する (期限切れバッジと `SSOExpiredBanner` が表示される状態)
3. `SSOExpiredBanner` の「SSO 再ログイン」ボタンを押す
4. ブラウザで開いた AWS の認可画面でログインを完了する
5. アプリの画面に戻る

期待する結果は、期限バッジが「SSO 有効」に変わり、残り時間が新しい期限で表示されることである。
実際には期限切れ表示のまま固まり、リロードするまで更新されない。

## 原因

根本原因は backend の SSO ログイン API がログイン完了を待たずに応答を返し、frontend がその応答を受けた時点で早すぎる再取得を行うことにある。

backend の `handleSSOLogin` (`backend/internal/api/handlers_sso.go` L14-30) は `aws sso login` を `cmd.Start()` で起動し、`cmd.Wait()` を別ゴルーチンに委ねたまま、ログイン完了を待たずに即座に 202 Accepted を返す。

frontend の `useSSOLogin` (`frontend/src/api/queries.ts` L409-417) は、この 202 応答を受けた瞬間に `onSuccess` で `['aws']` を invalidate し、`['aws', 'profiles']` を refetch する。
しかしこの時点ではユーザーはまだブラウザでログインを完了しておらず、`~/.aws/sso/cache` のトークンキャッシュの `expiresAt` は古い値のままである。
したがって refetch しても期限切れの値が返り、表示は期限切れのまま固定される。

ログイン完了後に profiles を再取得するトリガーが存在しないため、以後は更新されない。
`useProfiles` (`frontend/src/api/queries.ts` L109-117) は `staleTime` が 5 分で、成功後のポーリングはなく、グローバル設定 (`frontend/src/main.tsx` L12-19) で `refetchOnWindowFocus: false` になっている。
ログイン成功後はリソース取得も成功して `SSO_TOKEN_EXPIRED` を返さなくなるため、`AccountView` の期限切れ検知 effect (`frontend/src/views/AccountView.tsx` L119-122) による invalidate も発火しない。
結果として、次に期限表示が更新されるのはページのリロードか手動 Refresh のときだけになる。

補助的に、`AwsActiveSessionCard` の 60 秒タイマー (`frontend/src/components/session/AwsActiveSessionCard.tsx` L27-31) は現在時刻だけを更新し、期限値 `ssoExpiresAt` 自体は再取得しない。
このため残り時間はカウントダウンするが、ログインで延びた新しい期限には追随しない。

backend の `handleListProfiles` (`backend/internal/api/handlers_aws.go` L25-49) はキャッシュを通さず毎リクエストで `~/.aws` 配下を読み直すため、トークンキャッシュさえ更新されていれば正しい期限を返す。
したがって backend のキャッシュ滞留は原因ではなく、ログイン完了を待たずに応答する設計が frontend の早すぎる再取得を誘発している点が根本要因である。

## 根拠

SSO セッションの期限表示は、期限切れに気付いて再ログインするための主要な導線である。
再ログインしても表示が更新されないと、ユーザーはログインが成功したかどうかを画面から判断できず、リロードという回避策を毎回強いられる。
`SSOExpiredBanner` (`frontend/src/components/SSOExpiredBanner.tsx` L17-22) が「再取得してください」と案内している通り、現状は自動更新されないことを前提とした UI になっており、期限表示の信頼性を損なっている。

## 対応方針

以下のいずれか、または組み合わせで対応する。
実装時に最も堅牢な方式を選定する。

- backend の `handleSSOLogin` をログイン完了まで待って応答を返す同期方式に変える。既存の `ssoLoginTimeout` (5 分) の範囲で `cmd.Wait()` を待ってから 200 を返し、frontend はその応答を受けてから invalidate する。
- frontend の `useSSOLogin` の `onSuccess` で即時に invalidate せず、profiles を一定間隔でポーリングし、`ssoStatus === 'valid'` になったら停止する。
- `SSOExpiredBanner` にログイン完了後の明示的な再取得トリガー (`['aws', 'profiles']` の invalidate) を設ける。

## 検証

- 期限切れプロファイルで SSO 再ログインを行い、ログイン完了後にリロードせずに期限バッジが「SSO 有効」へ更新され、残り時間が新しい期限で表示されることを確認する。
- ログイン完了までの待機中に期限表示が誤って「SSO 有効」に変わらないことを確認する。
- `mise run check` が全通過することを確認する。

## 解決方法

対応方針の 3 案から「backend を同期方式に変える」を採用した (実装前にユーザーに確認済み。AGENTS.md が「認証フロー (SSO) の挙動変更」「並行処理の同期戦略の変更」を事前確認事項として明示しているため)。

- `backend/internal/api/handlers_sso.go` の `handleSSOLogin` を、`cmd.Start()` + 別ゴルーチンでの `cmd.Wait()` (即時 202 応答) から、`cmd.Run()` によるブロッキング実行に変更した。ブラウザでの認可完了 (または `ssoLoginTimeout` の 5 分経過、または aws CLI のエラー終了) まで応答を保留する。
  - 成功時は 204 No Content を返す。
  - タイムアウト時は 504 + `SSO_LOGIN_TIMEOUT` を返す。
  - その他のエラー時は 500 + `SSO_LOGIN_FAILED` を返す。
  - この判定ロジックを `writeSSOLoginResult` として切り出し、`handlers_sso_test.go` でテーブル駆動テストを追加した (exec.Command 自体はモックせず、判定ロジック単体を検証する)。
- backend の `http.Server` は `WriteTimeout` を意図的に未設定にしている (既存の長時間操作向けの設計方針) ため、変更なしでブロッキング応答に対応できる。
- frontend の `useSSOLogin` (`onSuccess` での `['aws']` invalidate) はコード変更不要になった。応答が実際の完了後に返るようになったことで、invalidate が正しいタイミングで自動的に発火するようになったため。
- `frontend/src/components/SSOExpiredBanner.tsx` の「ログイン成功したがまだ古い期限のはず」という前提の文言 (`login.isSuccess` 時のヒント) を削除し、`login.isPending` (応答待ち = ブラウザでの認可待ち) の間に「ブラウザで認可を完了してください。完了すると自動的に反映されます。」を表示するようにした。
- `frontend/src/components/SSOExpiredBanner.test.tsx` を新規追加し、ログイン中のヒント表示、成功時にヒントが消えること、失敗時のエラーメッセージ表示をテストした。
- `frontend/src/api/client.ts` の `parsePostResponse` に残っていた「202 Accepted (SSO login 起動等)」という古いコメントを実態 (204 No Content) に合わせて修正した (処理自体は変更なし)。
- `mise run check` が全通過することを確認した。
