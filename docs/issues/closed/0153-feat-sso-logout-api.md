# SSO トークンキャッシュを start URL 単位で削除するログアウト API を追加する

Created: 2026-08-26
Model: Claude Fable 5
Completed: 2026-08-27

## 背景

docs/issues/TODO.md の次の項目に由来する。

> Frontend に SSO のログアウトボタンがほしい
>     - issue 0149 の手動確認で `thief sso logout` コマンドを使ってログアウト状態を作った際に、Frontend からログアウトできると便利だと気付いた

この要望の実現は 2 つの issue に分割した。本 issue はその第 1 段階で、backend にログアウトのエンドポイントを追加する。frontend のアクティブセッションカードにボタンを置いてこのエンドポイントを呼ぶのは第 2 段階 (docs/issues/0154-feat-sso-logout-button-frontend.md) で扱う。分割の形は docs/issues/closed/0148 (backend API) と docs/issues/closed/0149 (frontend) の先例に揃えた。

現状、SSO のログアウト手段は CLI の `thief sso logout` (backend/internal/cli/sso.go の `ssoLogout`) だけである。
`ssoLogout` は `ssoauth.CacheDir()` (backend/internal/ssoauth/ssoauth.go、`~/.aws/sso/cache`) 配下の全ファイルを `filepath.Walk` で無条件に `os.Remove` する。
対象を start URL や profile で絞る機構は無い。

backend の SSO 関連エンドポイントは backend/internal/api/routes.go の 3 つである。

- `GET /api/aws/profiles/{profile}/sso` (`handleSSO`)
- `POST /api/aws/profiles/{profile}/sso/login/start` (`handleSSOLoginStart`)
- `POST /api/aws/profiles/{profile}/sso/login/complete` (`handleSSOLoginComplete`)

`handleSSO` は backend/internal/api/handlers_aws.go、`handleSSOLoginStart` / `handleSSOLoginComplete` は backend/internal/api/handlers_sso.go にあり、ログアウト用のエンドポイントは無い。
`ssoauth` パッケージにもログアウト用の公開関数は無く、公開 API は `Start` / `Wait` / `CacheDir` / `DefaultDeps` と型 `TokenCache` / `Session` / `Deps` である。

profile 一覧の SSO 状態 (`valid` / `expired` / `not_logged_in`) は backend/internal/aws/profiles.go の `applySSOStatus` が決める。
backend/internal/aws/sso_cache.go の `readSSOCacheStatuses` で読んだキャッシュファイルの `startUrl` と profile の start URL を `normalizeStartURL` で突き合わせる。
キャッシュファイル名は AWS CLI の形式 (legacy は SHA-1(startUrl)、sso-session は SHA-1(セッション名)) と書き込み元で SHA-1 の入力が異なるため、ファイル名から対象を特定できず、中身の `startUrl` で突き合わせる必要がある (同ファイルのコメントに明記)。

Frontend からログアウトするには、まず backend に start URL 単位でトークンキャッシュを削除するエンドポイントが要る。
それが存在しないことが、要望が満たされていない理由である。

## 目的

frontend が `POST /api/aws/profiles/{profile}/sso/logout` を呼ぶだけで、その profile が属する start URL のトークンキャッシュを削除できる。
削除後は profile 一覧の SSO 状態が未ログインに変わる (`handleListProfiles` はリソースキャッシュを経由しない)。AWS リソースの取得が `SSO_TOKEN_EXPIRED` の経路に入るのは backend のリソースキャッシュ (`serveCached`、TTL 1 時間) にヒットしない場合で、キャッシュの破棄は 0154 が `POST /api/cache/invalidate?view=aws` で行う。本 API はリソースキャッシュに触れない。

## 設計判断

### ログアウトの単位は profile が属する start URL とする

- `POST /api/aws/profiles/{profile}/sso/logout` を追加し、`ResolveSSOConfig(profile)` (backend/internal/aws/sso_config.go) で得た start URL に対応するトークンキャッシュファイルだけを削除する。対象ファイルは `readSSOCacheStatuses` と同じく中身の `startUrl` を `normalizeStartURL` で比較して特定する。ファイル名 (SHA-1) からの特定は入力が一定でなく取りこぼすため採らない。
- 採らなかった案: CLI と同じく `~/.aws/sso/cache` の全ファイルを削除する。同じ端末で別の start URL (別の IAM Identity Center 組織) にログインしている profile まで巻き込んでログアウトさせるため却下する。
- 採らなかった案: backend を変えず、カードに `thief sso logout` コマンドのコピー導線を足す。TODO の要望は Frontend からログアウトできることであり、コマンドのコピーでは満たさないため却下する。
- 同じ start URL を共有する複数 profile は 1 つのキャッシュファイルを共有するため、1 つの profile でログアウトすると同じ start URL の他の profile も未ログインになる。AWS CLI の `aws sso logout` と同じ振る舞いである。第 2 段階の frontend はボタンの title 属性でその旨を示す。

### ローカルのキャッシュ削除のみを行い、AWS 側のセッション失効は行わない

- ログアウトはローカルのトークンキャッシュの削除に限定し、CLI の `thief sso logout` と同じ範囲にする。TODO の要望は `thief sso logout` で作れる未ログイン状態を Frontend から作ることであり、AWS 側のサインインセッションの失効は要望に含まれない。
- 採らなかった案: 削除前に SSO Portal API の `Logout` (`github.com/aws/aws-sdk-go-v2/service/sso` の `Client.Logout`) を呼んで AWS 側のセッションも失効させる。トークンが AWS 側で有効なまま残ることを防げるが、呼び出しに使う region の決定 (キャッシュファイルの `region` と profile の `sso_region` のどちらを優先するか)、タイムアウト、失敗時の扱いが増え、CLI の `ssoLogout` にも無い分岐を本 issue だけが持つことになる。要望の範囲を超えるため本 issue では採らず、CLI と API の両方に対する失効の追加は「## 関連」の別 issue 候補とする。

### backend の実装位置

- キャッシュディレクトリの走査と `startUrl` の突き合わせは、`readSSOCacheStatuses` と `normalizeStartURL` が非公開関数として置かれている backend/internal/aws/sso_cache.go に `RemoveSSOTokenCache(cacheDir, startURL string) error` (公開関数) として追加し、`readSSOCacheStatuses` と同じ規則 (`.json` のみ、`startUrl` を持たない client registration ファイルは対象外、`normalizeStartURL` で比較) を同じパッケージ内の関数呼び出しで共有する。`ssoauth` パッケージには `Logout(startURL string) error` を置き、`CacheDir()` で得たディレクトリを渡して `aws.RemoveSSOTokenCache` を呼ぶだけの薄い関数にする。`ssoauth` は既に `internal/aws` を import しており (backend/internal/ssoauth/ssoauth.go)、逆方向の import は無いため依存方向は変わらない。
- 走査中に読めないファイル (`ssoCacheMaxFileSize` の 1MB 超過、`os.ReadFile` の失敗、`json.Unmarshal` の失敗) は `readSSOCacheStatuses` と同じく `slog.Warn` を出して対象外にし、削除しない。削除対象かどうかを確定できないファイルを消すと別の start URL のトークンを巻き込む恐れがあり、逆に 500 にすると無関係な破損ファイル 1 つで正常なログアウトが失敗する。読めないファイルは `readSSOCacheStatuses` でも `valid` の判定材料にならないため、残しても profile が `valid` 表示に戻ることは無い。読めないファイルを除いた一致ファイルの削除が全て成功すれば (一致が 0 件でも) 204 を返す。
- 採らなかった案: `ssoauth` パッケージに走査と比較のロジックを複製する。`normalizeStartURL` が将来変わったときに複製が追従せず、`readSSOCacheStatuses` が `valid` と判定するファイルと削除対象がずれるため却下する。
- 採らなかった案: `normalizeStartURL` だけを公開して `ssoauth` 側で走査を書く。`.json` 判定と client registration の除外も `readSSOCacheStatuses` と揃える必要があり、公開する関数が増えるだけで複製の問題が残るため却下する。
- ハンドラ `handleSSOLogout` は backend/internal/api/handlers_sso.go に置き、`handleSSOLoginStart` と同じく `ValidateProfileName` と `resolveConfig` を通す。エラーは既存の `writeSSOLoginStartError` と同じコード (`PROFILE_NOT_FOUND` 404、`SSO_NOT_CONFIGURED` 400) を使い、削除失敗は `SSO_LOGOUT_FAILED` 500 とする。`ssoauth.Logout` は `ssoLoginDeps` と同じく関数値として `Server` に注入し、`httptest` で差し替える。
- 対象の start URL に一致するキャッシュファイルが 1 つも無い場合 (既に未ログイン) は 204 を返す。ログアウトは冪等な操作であり、二重押しやタブ間の競合でエラーを出す理由が無い。
- `RemoveSSOTokenCache` は、`os.Remove` が `fs.ErrNotExist` を返した場合 (走査と削除の間に別のリクエストや CLI が同じファイルを消した場合) を成功として扱う。ログアウトの目的はファイルが無い状態にすることであり、二重押しやタブ間の同時ログアウトで片方が 500 になる理由が無い。キャッシュディレクトリ自体が無い場合 (`os.ReadDir` が `fs.ErrNotExist`) も `readSSOCacheStatuses` が未ログインの正常系として扱うのと同じく一致無しとして nil を返し、ハンドラは 204 を返す。`os.ReadDir` が `fs.ErrNotExist` 以外のエラー (権限エラー等) を返した場合は、`readSSOCacheStatuses` が判定不能 (第 2 返り値 `false`) として扱うのと同じく成功と見なさず、エラーをそのまま返してハンドラは 500 `SSO_LOGOUT_FAILED` にする。対象ファイルを読めていないのに 204 を返すと、削除できていないトークンが残ったまま「ログアウト済み」と表示されるためである。`CacheDir()` の失敗 (`os.UserHomeDir` のエラー) は `ssoauth.Logout` がそのまま返し、ハンドラは 500 `SSO_LOGOUT_FAILED` にする。
- 一致するファイルが複数ある場合 (AWS CLI と thief が別のファイル名で同じ start URL を書いた場合。backend/internal/aws/sso_cache.go のコメントが想定する状況) は全件を対象にし、1 件の削除失敗で止めずに残りも削除する。失敗を `errors.Join` でまとめ、1 件でも失敗があれば `SSO_LOGOUT_FAILED` 500 を返す。途中で止めると一部のファイルが残って `valid` 表示のままになるためである。
- 進行中の SSO ログイン (`ssoLoginSessions` に積まれた `complete` 待ち) はログアウトで取り消さない。ログアウト後に `complete` が成功すればトークンが再保存されてログイン済みに戻るが、それはユーザが認可を完了した結果であり、両方の操作を尊重する。セッションストアをロックして排他する案は、ブラウザタブ間の操作順を backend が裁定することになり、実装の複雑さに見合わないため採らない。
- CLI の `ssoLogout` の振る舞い (全削除) は本 issue では変更しない。

### 権限

- AWS API を呼ばないため IAM 権限の追加は不要である。新規の依存追加は無い。

## 完了条件

- `POST /api/aws/profiles/{profile}/sso/logout` が routes.go に登録され、対象 profile の start URL に一致する `~/.aws/sso/cache` のファイルだけを削除し、他の start URL のファイルと `startUrl` を持たないファイルを残す。
- 一致するキャッシュファイルが無い場合に 204 を返す。
- 存在しない profile で 404 `PROFILE_NOT_FOUND`、SSO 設定の無い profile で 400 `SSO_NOT_CONFIGURED`、ファイル削除の失敗で 500 `SSO_LOGOUT_FAILED` を返す。
- 一致するファイルが 2 件あり両方の削除に成功した場合に 204 を返す。
- 一致するファイルが 2 件あり 1 件目の削除が失敗しても 2 件目の削除を試み、500 `SSO_LOGOUT_FAILED` を返す。
- `RemoveSSOTokenCache` が backend/internal/aws/sso_cache.go に公開関数として追加され、`readSSOCacheStatuses` と同一の `normalizeStartURL` 関数を呼び出して比較し、start URL の正規化や `.json` 判定を別に実装していない (diff で `RemoveSSOTokenCache` の本体に正規化処理が無いことを確認する)、`ssoauth.Logout` が `CacheDir()` の結果を渡して `RemoveSSOTokenCache` を呼ぶ。
- backend/internal/aws/sso_cache_test.go に `RemoveSSOTokenCache` のテスト (一致ファイルのみ削除、一致無しで nil を返す、`startUrl` を持たないファイルの除外、`.json` 以外のファイルの除外、末尾スラッシュ違いの start URL の一致、削除時に `fs.ErrNotExist` が返っても成功扱いになること、一致ファイルの隣に JSON として壊れたファイルと 1MB 超のファイルがあるとき両方を残して一致ファイルだけを削除し nil を返すこと、キャッシュディレクトリが無いとき nil を返すこと、キャッシュディレクトリの `os.ReadDir` が権限エラーのときエラーを返すこと、2 件中 1 件失敗時の続行と `errors.Join` による両方の報告) が追加されている。
- backend/internal/api/handlers_sso_test.go に `handleSSOLogout` の `httptest` テスト (上記の 204 / 404 / 400 / 500 の各ステータスコードとエラーコード、注入した関数値に profile の start URL が渡ること、注入した関数値が nil を返したとき (一致無し、キャッシュディレクトリ不在) 204 になること、注入した関数値がエラーを返したとき 500 `SSO_LOGOUT_FAILED` になること) が追加されている。`CacheDir()` の失敗はこの注入した関数値のエラーとして同じ経路を通るため、個別のテストは書かない。
- AWS API (`sso:Logout` を含む) を呼ばない。
- frontend の変更 (ボタン、`postSSOLogout`、`useSSOLogout`、i18n) は行わない。
- CLI の `thief sso logout` の振る舞いは変更しない。
- 「## 未確定論点」の実ファイル確認を行い、確認したファイルの生成方法 (`aws sso login --sso-session <name>` または `--profile <name>`) と `startUrl` の有無を PR 説明に書く。`startUrl` が無い形式が見つかった場合は、その形式のファイルを対象外にすることをテストに追加する。実 SSO 組織にログインできる環境が無い場合は、`readSSOCacheStatuses` が同じ前提で動いている実績に依拠することを PR 説明に書き、この確認は省いてよい。
- `mise run check` が通過する。

## 未確定論点

- AWS CLI v2 が sso-session 形式で書いたキャッシュファイルに `startUrl` が入っていることは、backend/internal/aws/sso_cache.go のコメントと `readSSOCacheStatuses` がその前提で動いている実績に依拠している。実装時に `aws sso login --sso-session <name>` で生成した実ファイルを 1 件確認し、`startUrl` が無い形式が見つかった場合は `readSSOCacheStatuses` と同じ扱い (対象外) にする。
  - 調査結果 (2026-08-27、実装時): 実装環境の `~/.aws/sso/cache` には JSON が 3 件あり、キー名だけを確認した。トークンキャッシュ 2 件はいずれも `startUrl` を持ち (キー: accessToken / clientId / clientSecret / expiresAt / region / registrationExpiresAt / startUrl)、ファイル名は SHA-1(startUrl) に一致する legacy 形式だった。残る 1 件は `startUrl` を持たない client registration (キー: clientId / clientSecret / expiresAt)。`~/.aws/config` の `[sso-session <name>]` の名前の SHA-1 に一致するファイルは無く、sso-session 形式で生成された実ファイルは確認できなかった。完了条件の定めに従い、この形式については `readSSOCacheStatuses` が同じ前提で動いている実績に依拠し、テストの追加は行わない。解消済み (実ファイルで確認できる範囲では `startUrl` が無い形式は見つからなかった)。
  - 代替規定の適用根拠 (2026-08-27、レビュー観点 1 の指摘により追記): `aws sso login --sso-session <name>` はブラウザでの対話的な認可を要し、自律実行中のこの作業では完了させられないため実行していない。実 SSO 組織にログインしてファイルを新たに生成できる環境は今回の作業には無いものとして扱い、完了条件の「省いてよい」の規定を適用した。確認したファイルの生成方法は「過去の `aws sso login` (legacy 形式、ファイル名が SHA-1(startUrl))」で、いずれも `startUrl` を持つ。

## 関連

- docs/issues/0154-feat-sso-logout-button-frontend.md: 本 issue のエンドポイントを呼ぶボタンを `AwsActiveSessionCard` に追加する第 2 段階。本 issue が先行し、0154 は本 issue の close 後に着手する。この前提は手動確認のための順序であり、自動テストの技術的な依存ではない (`SSOAccountResource` 系は backend/internal/contract/contract.go の `Registry` の対象外で、0154 は API クライアントをモックして自動テストを書ける。実際のログアウト動作の確認に本 issue のエンドポイントが要る)。
- docs/issues/closed/0148-feat-sso-device-auth-api-endpoints.md: ハンドラ構成、エラーコード、依存注入のテンプレート。
- docs/issues/closed/0149-feat-sso-login-refocus-frontend-tab.md: TODO 項目の発端となった手動確認。
- 別 issue 候補 (本 issue のスコープ外。docs/issues/TODO.md に独立した項目として記載し、起票は別に行う): CLI の `ssoLogout` と本 API の両方で、削除前に `sso:Logout` を呼んで AWS 側のサインインセッションを失効させる。CLI の `ssoLogout` を `ssoauth.Logout` に寄せる統合も同じ issue で扱う。

## 解決方法

backend/internal/aws/sso_cache.go に公開関数 `RemoveSSOTokenCache(cacheDir, startURL string) error` を追加した。`readSSOCacheStatuses` の走査部分 (`.json` のみ、1MB 超と読めないファイルと壊れた JSON は `slog.Warn` を出して対象外、`startUrl` を持たない client registration は対象外) を非公開関数 `walkSSOCacheTokens(cacheDir, visit)` に抽出し、`readSSOCacheStatuses` と `RemoveSSOTokenCache` の両方がこれを呼ぶ形にした。`RemoveSSOTokenCache` の本体は `normalizeStartURL` の呼び出しによる比較と `ssoCacheRemove` (既定は `os.Remove`) による削除だけで、正規化や `.json` 判定を別に実装していない。`readSSOCacheStatuses` の振る舞い (返り値、ログ、not-exist を未ログインの正常系とする扱い) は変えていない。

`RemoveSSOTokenCache` は、一致ファイルが無い場合と `cacheDir` が無い場合 (`os.ReadDir` が `fs.ErrNotExist`) は nil を返し、`os.ReadDir` がそれ以外で失敗した場合は `read sso cache dir: %w` でラップして返す。一致が複数ある場合は 1 件の失敗で止めずに全件の削除を試み、`os.Remove` が `fs.ErrNotExist` を返した削除は成功として扱い、それ以外の失敗を `errors.Join` でまとめて返す。

方針からの乖離 (方式は保ったままの実装詳細): 削除に使う関数をパッケージ変数 `ssoCacheRemove = os.Remove` として置いた。走査と削除の間に別プロセスが同じファイルを消した状況 (`fs.ErrNotExist`) と、2 件中 1 件の削除失敗は、実ファイルシステムでは決定的に再現できないため、テストでこの変数を差し替えて再現する。本番の呼び出し経路は `os.Remove` のままである。

backend/internal/ssoauth/ssoauth.go に `Logout(startURL string) error` を追加した。`CacheDir()` の結果を `aws.RemoveSSOTokenCache` に渡すだけの薄い関数で、`CacheDir()` の失敗は `failed to get cache directory: %w` でラップして返す。

backend/internal/api/handlers_sso.go の `ssoLoginDeps` に関数値 `logout func(startURL string) error` を追加し、`defaultSSOLoginDeps` で `ssoauth.Logout` を配線した。`handleSSOLogout` は `ValidateProfileName` (400 `BAD_REQUEST`)、`resolveConfig` (既存の `writeSSOLoginStartError` で 404 `PROFILE_NOT_FOUND` / 400 `SSO_NOT_CONFIGURED` / 500 `INTERNAL_ERROR`) を通し、`logout` の失敗を 500 `SSO_LOGOUT_FAILED`、成功を 204 で返す。backend/internal/api/routes.go に `POST /api/aws/profiles/{profile}/sso/logout` を登録した。AWS API は呼ばず、リソースキャッシュと `ssoLoginSessions` には触れない。CLI の `ssoLogout` と frontend は変更していない。

テストは次のとおり追加した。

- backend/internal/aws/sso_cache_test.go `TestRemoveSSOTokenCache`: 一致ファイルのみ削除 (他の start URL と client registration を残す)、一致無しで nil、`startUrl` を持たないファイルの除外、`.json` 以外の除外、末尾スラッシュ違いの一致、削除時の `fs.ErrNotExist` を成功扱い、壊れた JSON と 1MB 超の隣接ファイルを残して一致ファイルだけ削除、キャッシュディレクトリ不在で nil、`os.ReadDir` の失敗でエラー、1 件目の削除失敗後も 2 件目を削除してエラーを返す、2 件とも失敗したとき `errors.Join` で両方を報告、空の start URL では何も削除しない (レビュー観点 2 の指摘で追加。`startUrl` を持たない client registration を同居させ、`walkSSOCacheTokens` がそのエントリを渡さないことに依存する安全性を回帰テストとして固定する。この分岐を外すとこのサブテストが落ちることを一時変更で確認した)、存在するが空のキャッシュディレクトリで nil (同じく観点 2 で追加)、の 13 サブテスト。`os.ReadDir` の失敗は、完了条件が挙げる権限エラーではなく、通常ファイルを `cacheDir` に指定する方法 (`readSSOCacheStatuses` の既存テストと同じ、root や Windows に依存しない再現方法) で起こしている。`RemoveSSOTokenCache` は `os.ReadDir` のエラーを `fs.ErrNotExist` かどうかだけで分けるため、権限エラーも同じ分岐を通る。
- backend/internal/api/handlers_sso_test.go `TestSSOLogoutSucceeds` (profile の start URL が注入した関数値に渡り、nil で 204 かつ空ボディ)、`TestSSOLogoutErrors` (不正な profile 名 400 `BAD_REQUEST`、404 `PROFILE_NOT_FOUND`、400 `SSO_NOT_CONFIGURED`、設定読み取り失敗 500 `INTERNAL_ERROR`、削除失敗 500 `SSO_LOGOUT_FAILED`。各ケースで `logout` が呼ばれたかも検証)、`TestSSOLogoutRouteMethod` (GET は 405)。`TestDefaultSSOLoginDepsIsFullyWired` に `logout` の非 nil 検査を追加した。注入した関数値が nil を返す経路は、一致無し、キャッシュディレクトリ不在、`CacheDir()` の失敗 (エラー経路) を区別しないため、完了条件のとおり個別のテストは書いていない。

「## 未確定論点」の実ファイル確認の結果は同セクションに追記した。実装環境のトークンキャッシュ 2 件は legacy 形式で `startUrl` を持ち、sso-session 形式の実ファイルは確認できなかったため、完了条件の定めに従い `readSSOCacheStatuses` の実績に依拠した。`aws sso login --sso-session` はブラウザでの対話的な認可を要し自律実行中には完了させられないため実行しておらず、完了条件の「実 SSO 組織にログインできる環境が無い場合は省いてよい」の規定を適用した。本スキルは pull request を作成しないため、完了条件が求める「確認したファイルの生成方法と `startUrl` の有無を PR 説明に書く」は、PR を作る時点で「## 未確定論点」の追記 (生成方法: 過去の `aws sso login` による legacy 形式、`startUrl`: 2 件とも有り) を PR 説明に転記することで満たす。

完了条件の確認結果は次のとおり。

- ルート登録と start URL 一致ファイルだけの削除: routes.go の登録行、`TestRemoveSSOTokenCache` の「removes only files matching the start url」「client registration without startUrl is kept」「non json files are kept」で検証
- 一致無しで 204: `TestSSOLogoutSucceeds` (関数値が nil) と `TestRemoveSSOTokenCache` の「no match returns nil and keeps files」で検証
- 404 / 400 / 500 の各コード: `TestSSOLogoutErrors` で検証
- 2 件一致で両方成功時 204: `TestRemoveSSOTokenCache` の「trailing slash difference still matches」(2 件とも削除して nil) と `TestSSOLogoutSucceeds` の組み合わせで検証
- 1 件目失敗でも 2 件目を試み 500: `TestRemoveSSOTokenCache` の「continues after a failed removal and reports every failure」と `TestSSOLogoutErrors` の「cache removal failure」で検証
- `RemoveSSOTokenCache` が `normalizeStartURL` を共有し正規化と `.json` 判定を別実装していない、`ssoauth.Logout` が `CacheDir()` を渡す: 差分で確認 (本体は `normalizeStartURL` 呼び出しと `ssoCacheRemove` のみ)
- sso_cache_test.go のテスト一覧: 上記 13 サブテストで充足
- handlers_sso_test.go のテスト一覧: 上記 3 テストで充足
- AWS API を呼ばない、frontend と CLI を変更しない: 差分に該当ファイルの変更が無いことで確認
- 未確定論点の実ファイル確認: 同セクションに追記 (代替規定の適用根拠を含む)。PR 説明への転記は PR 作成時に行う
- `mise run check`: 通過 (ベースラインと同じく失敗テストなし)
