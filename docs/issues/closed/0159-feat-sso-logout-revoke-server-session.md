# SSO ログアウトで削除前に sso:Logout を呼んで AWS 側のサインインセッションを失効させ CLI の logout を ssoauth に統合する

Created: 2026-08-27
Model: Claude Fable 5
Completed: 2026-08-28

## 背景

docs/issues/TODO.md の次の項目に由来する。

> thief sso logout と Frontend の SSO ログアウトで、削除前に sso:Logout を呼んで AWS 側のサインインセッションも失効させたい
>     - issue 0153 の設計判断で、ローカルのキャッシュ削除だけでは AWS 側のセッションが有効なまま残ると気付いた。CLI の ssoLogout を ssoauth.Logout に寄せる統合も同じ issue で扱う

docs/issues/closed/0153-feat-sso-logout-api.md の設計判断「ローカルのキャッシュ削除のみを行い、AWS 側のセッション失効は行わない」で、SSO Portal API の `Logout` の呼び出しは要望の範囲外として見送り、「## 関連」で別 issue 候補とした。本 issue はその候補を起票したものである。

### 現状の実装

thief には SSO のログアウト経路が 2 つあり、どちらもローカルのトークンキャッシュ (`~/.aws/sso/cache`) を削除するだけで AWS 側のサインインセッションを失効させない。

1. CLI `thief sso logout` (backend/internal/cli/sso.go の `ssoLogout`)。`ssoauth.CacheDir()` 配下の全ファイルを `filepath.Walk` と `os.Remove` で無条件に削除する。start URL や profile で対象を絞る機構は無く、`logoutCmd` にフラグは無い。`cacheDir` が存在しない場合は `directory does not exist` のエラーを返す。docs/issues/closed/0153 で追加された `ssoauth.Logout` は使っておらず、走査と削除のロジックを独自に持つ。
2. backend API `POST /api/aws/profiles/{profile}/sso/logout` (backend/internal/api/handlers_sso.go の `handleSSOLogout`)。`ResolveSSOConfig` で profile の `SSOConfig{Region, StartURL}` を解決し、`ssoLoginDeps.logout` (既定は `ssoauth.Logout`) に `cfg.StartURL` を渡す。`ssoauth.Logout(startURL string) error` (backend/internal/ssoauth/ssoauth.go) は `aws.RemoveSSOTokenCache(cacheDir, startURL)` (backend/internal/aws/sso_cache.go) を呼ぶ薄い関数で、`startUrl` が一致するキャッシュファイルを削除する。frontend は `AwsActiveSessionCard` (frontend/src/components/session/AwsActiveSessionCard.tsx) のボタンから `useSSOLogout` (frontend/src/api/queries.ts) 経由でこのエンドポイントを呼ぶ。

`ssoauth.Logout` のコメントは「削除はローカルのキャッシュに限り、AWS 側のサインインセッションは失効させない」、`handleSSOLogout` のコメントは「AWS 側のセッション失効 (sso:Logout) は呼ばない」と書いている。frontend の `postSSOLogout` (frontend/src/api/endpoints.ts) のコメントも「ローカルのみ、AWS 側のセッションは失効させない」と書いている。

### 要望が満たされていない理由

SSO のアクセストークンは IAM Identity Center 側のサインインセッションに紐づく。ローカルのキャッシュファイルを削除しても、そのトークンは AWS 側で有効期限 (`expiresAt`) まで有効なまま残る。削除前にファイルの内容を退避していれば、そのトークンで `sso:ListAccounts` や `sso:GetRoleCredentials` を続けて呼べる。

SSO Portal API には `Logout` がある (`github.com/aws/aws-sdk-go-v2/service/sso` の `Client.Logout`、入力は `LogoutInput{AccessToken *string}` のみ)。SDK の doc コメントは「ローカルのトークンキャッシュを削除し、IAM Identity Center に API 呼び出しを送って対応するサーバ側のサインインセッションを無効化する」と説明する。この説明は AWS CLI の `aws sso logout` の挙動を指しており、Go SDK の `Client.Logout` 自体は HTTP 呼び出しだけを行いローカルファイルには触れない。thief はこの API を呼んでいない。`service/sso` は backend/go.mod に既にあり (v1.32.0)、backend/internal/aws/sso.go の `newSSOClient` と `ListSSOAccounts` で使っている。

アクセストークンの読み出しについては、`sso_cache.go` の `ssoCacheEntry` は `startUrl` と `expiresAt` しか持たず、コメントで「accessToken / clientSecret 等の秘密情報はフィールドに持たない」と明記している。`accessToken` を読む既存の関数は backend/internal/aws/sso_token.go の `loadSSOAccessToken(profile string)` だけで、これは `startUrl` を見ずにディレクトリ内で最初に見つかった `accessToken` 付きのファイルを評価し、有効ならそのトークンを返し、期限切れなら他のファイルを試さずに `ErrSSOTokenExpired` を返す。start URL に対応するトークンを読み出す関数は無い。

## 目的

`thief sso logout` と `POST /api/aws/profiles/{profile}/sso/logout` のどちらでログアウトしても、ローカルのトークンキャッシュを削除する前に `sso:Logout` が呼ばれ、AWS 側のサインインセッションが失効する。
CLI の `ssoLogout` は独自の走査と削除を持たず、`ssoauth` パッケージのログアウト関数を呼ぶ。

## 設計判断

### ログアウトの共通処理を `ssoauth` に置き、CLI と API の両方から呼ぶ

- `ssoauth.Logout` のシグネチャを `Logout(ctx context.Context, startURL, fallbackRegion string, deps Deps) (LogoutResult, error)` に変える (`LogoutResult` は後述)。処理列は次のとおり。
  1. `deps.ListTokens(cacheDir, startURL)` (既定は `aws.ListSSOTokenCache`、新設、後述) で `startUrl` が一致するトークンキャッシュを列挙する。
  2. 列挙したトークンを `accessToken` で重複排除し、ユニークな `accessToken` ごとに 1 回だけ `deps.RevokeToken(ctx, region, accessToken)` を呼ぶ。legacy 形式と sso-session 形式の併存で同じトークンが 2 ファイルに書かれている場合 (AWS CLI の挙動でありリポジトリ内では確認できないが、`readSSOCacheStatuses` も同一 `startUrl` の複数ファイルを前提にしている)、2 回目の `sso:Logout` は失効済みで失敗し `RevokeFailed` に偽の失敗が計上されるため、これを避ける。グループ化の前にトークンをファイル名の辞書順に並べ、同じ `accessToken` のグループでは辞書順で最初のファイルの `region` と `expiresAt` を代表値として使う (期限切れ判定と region 決定はグループ単位で 1 回だけ行う)。同じ `accessToken` で `region` や `expiresAt` が食い違うのは破損か書き込み途中の不整合であり、どちらを採っても正しさを保証できないため、決定的で説明しやすい規則を選ぶ。
  3. `deps.RemoveTokenCache(cacheDir, startURL)` (既定は既存の `aws.RemoveSSOTokenCache`) で一致するファイルを削除する。
- `cacheDir` は処理列の前に `Logout` / `LogoutAll` が現行と同じく `CacheDir()` で解決する。`CacheDir()` の失敗と `deps.ListTokens` の失敗は失効も削除も始まっていないため、`LogoutResult{}` (ゼロ値) とエラーを返す。後述の「`error` が非 nil でも `LogoutResult` は有効」という契約は削除の失敗にだけ適用する。
- 採らなかった案: 同じ `accessToken` のグループで `expiresAt` が最も先のファイルを代表にする (`readSSOCacheStatuses` の規則)。`expiresAt` が解釈できないファイルとの比較規則を別に決める必要があり、決定性のためだけに規則が増える。
- `ssoauth.LogoutAll(ctx context.Context, deps Deps) (LogoutResult, error)` を新設する。`deps.ListTokens(cacheDir, "")` (空の startURL は全トークンを対象にする) で全トークンを列挙し、`Logout` と同じ規則で重複排除して各トークンに `deps.RevokeToken` を呼び、`deps.RemoveAllCache(cacheDir)` (既定は `aws.RemoveAllSSOCache`、新設) で `cacheDir` 直下の全ファイル (client registration を含む) を削除する。`RemoveAllSSOCache` は backend/internal/aws/sso_cache.go に置き、`os.ReadDir` でディレクトリ直下のエントリを列挙し、`DirEntry.Type().IsRegular()` が真のものだけを `ssoCacheRemove` で削除し、サブディレクトリとシンボリックリンク等の通常ファイル以外のエントリは削除せず `slog.Warn("sso cache entry skipped", "name", ..., "type", ...)` を出して残し、`cacheDir` 不在と削除時の `fs.ErrNotExist` は成功、それ以外の失敗は `errors.Join` でまとめて返す (`RemoveSSOTokenCache` と同じ規則)。拡張子は問わず、`.json` 以外も削除する。現状の `ssoLogout` (`filepath.Walk` + `os.Remove`) が拡張子を問わず消しており、その範囲を保つ。
- 採らなかった案: `RemoveAllSSOCache` を `.json` に限定する。`walkSSOCacheTokens` の規則と揃うが、現状の CLI が消している `.json` 以外のファイルが残るようになり、`aws sso logout` 相当の「キャッシュを空にする」結果が得られない。
- 採らなかった案: `ListSSOTokenCache(cacheDir, "")` で列挙した start URL ごとに `RemoveSSOTokenCache` を回し、client registration だけ別に消す。`ListSSOTokenCache` と `RemoveSSOTokenCache` は `startUrl` を持たないファイルを対象外にするため client registration と読めないファイルが残り、結局それらを消す別の走査が要る。現状の CLI `ssoLogout` の `filepath.Walk` はサブディレクトリを再帰するが、thief の `saveCacheFile` は `cacheDir` 直下にしか書かず、他ツールがサブディレクトリを作るかはコード上で確認できない。`RemoveAllSSOCache` は直下の通常ファイルだけを削除し、サブディレクトリが存在した場合は削除せず `slog.Warn` を出す。
- `ssoauth.Deps` に `RevokeToken func(ctx context.Context, region, accessToken string) error`、`ListTokens func(cacheDir, startURL string) ([]awsinternal.SSOCachedToken, error)`、`RemoveTokenCache func(cacheDir, startURL string) error`、`RemoveAllCache func(cacheDir string) error` の 4 フィールドを追加し、`DefaultDeps()` で `awsinternal.RevokeSSOToken` / `ListSSOTokenCache` / `RemoveSSOTokenCache` / `RemoveAllSSOCache` を配線する。既存の `SaveCache` がファイル書き込みを `Deps` に置いているのと同じ形で、列挙と削除も `Deps` に置く。`ssoauth_test.go` の `TestDefaultDepsIsFullyWired` は手書きのフィールド別 nil チェックなので、4 フィールドの検証を追加する。
- 理由: `Logout` / `LogoutAll` のテストで削除失敗を決定的に再現するため。`aws` パッケージは削除失敗の再現に非公開変数 `ssoCacheRemove` の差し替えを使い、読み取り失敗の再現に「`cacheDir` に通常ファイルを指定する」方法を使っているが、どちらも `ssoauth` のテストからは使えない。
- 採らなかった案: `ssoauth.Logout` / `LogoutAll` から `aws.ListSSOTokenCache` 等を直接呼び、削除失敗のテストは `cacheDir` のパーミッションを `0o500` にして再現する。root で実行される環境 (Docker 内の CI 等) ではパーミッションが効かず削除が成功してテストが崩れる。`aws` パッケージが chmod に依存しない再現方法を選んでいる方針とも矛盾する。API の `defaultSSOLoginDeps` は `logout` を `ssoauth.Logout(ctx, startURL, fallbackRegion, ssoauth.DefaultDeps())` を包むクロージャに、CLI の `ssoLogoutDeps.logoutAll` の既定値は `ssoauth.LogoutAll(ctx, ssoauth.DefaultDeps())` を包むクロージャにする (CLI の `defaultSSOTokenDeps(ssoauth.DefaultDeps())` と同じ形)。`Start` と `Wait` は `RevokeToken` を使わない。既存の `Start` / `Wait` のテスト (`okDeps()` はフィールド名指定のリテラル) はフィールド追加の影響を受けないが、`Logout` / `LogoutAll` のテストでは `RevokeToken` / `ListTokens` / `RemoveTokenCache` / `RemoveAllCache` を明示的に設定する (nil のままだと呼び出しで panic する)。`okDeps()` に 4 フィールドの成功ダミーを追加する。
- `awsinternal.RevokeSSOToken(ctx, region, accessToken string) error` を backend/internal/aws/sso.go に新設する。`newSSOClient(ctx, "", region)` で生成したクライアントの `Logout` を `LogoutInput{AccessToken: aws.String(accessToken)}` で呼び、失敗を `sso logout: %w` でラップして返す。`sso:Logout` は `ListAccounts` と同じくアクセストークンで認可される SSO Portal API であり、AWS 認証情報も IAM 権限も要らない。profile を空にして `newSSOClient(ctx, "", region)` を呼ぶ先例は backend/internal/aws/sso_oidc.go の `ListSSOAccountInfos` と `ListSSOAccountRoleNames` (どちらもアクセストークンを引数で受ける) にあり、これに揃える。この 2 関数は `thief sso generate-config` (backend/internal/cli/sso.go の `ssoGenerateConfig`) から実運用で呼ばれており、同じ経路の実績がある。
- `aws.ListSSOTokenCache(cacheDir, startURL string) ([]SSOCachedToken, error)` を backend/internal/aws/sso_cache.go に新設する。`SSOCachedToken` は `FileName` / `StartURL` / `Region` / `AccessToken` / `ExpiresAt` を持つ公開構造体とする。`ExpiresAt` は `ssoCacheEntry.ExpiresAt` と同じ `string` (RFC 3339 の生値) とし、解釈は `ssoauth.Logout` / `LogoutAll` が行う。走査の規則 (`.json` のみ、1MB 超と読めないファイルと壊れた JSON は `slog.Warn` を出して対象外、`startUrl` を持たないファイルは対象外) は `walkSSOCacheTokens` を共有する。`startURL` の一致判定は `RemoveSSOTokenCache` と同じく両辺を `normalizeStartURL` で正規化して比較し、末尾スラッシュの差で失効対象と削除対象が食い違わないようにする。`walkSSOCacheTokens` の `visit` を `func(fileName string, ce ssoCacheEntry, data []byte)` に変え、`ListSSOTokenCache` だけがその場で `data` を `accessToken` と `region` を含む非公開構造体へデコードする。既存の呼び出し元 `readSSOCacheStatuses` と `RemoveSSOTokenCache` は `data` を使わず、コールバックの引数追加だけの機械的な変更にとどめる。`ssoCacheEntry` には `accessToken` を追加せず、`readSSOCacheStatuses` の経路で秘密情報を構造体に展開しない現状の方針を保つ (生バイト列はどの経路でも `os.ReadFile` の時点でメモリにあり、`visit` に渡しても展開の範囲は変わらない)。`startURL` が空のときは全トークンを返す。`cacheDir` が無い (`os.ReadDir` が `fs.ErrNotExist`) 場合は空スライスと nil を返し (`readSSOCacheStatuses` が不在を一度もログインしていない正常系として扱うのと同じ方針。戻り値の型は `readSSOCacheStatuses` の `(map[string]ssoCacheStatus, bool)` とは異なる)、`Logout` / `LogoutAll` が列挙の段でエラーにならず削除まで進めるようにする。
- 採らなかった案: `loadSSOAccessToken` を start URL 対応に拡張して使う。`loadSSOAccessToken` は最初に見つかった有効なトークンを 1 つ返す設計で、同じ start URL のトークンファイルが複数ある場合 (legacy 形式と sso-session 形式の併存) に全件を失効させられない。また `sso_cache.go` のコメントが `loadSSOAccessToken` と `readSSOCacheStatuses` を意図的に統合していないと明記しており、その判断を覆す理由が本 issue には無い。
- 採らなかった案: CLI の `ssoLogout` に `sso:Logout` の呼び出しを直接足し、`ssoauth.Logout` には別に足す。走査、失効、削除の順序と失敗時の扱いが 2 箇所に分かれ、片方だけ直す事故が起きる。TODO も統合を同じ issue で扱うと定めている。

### 失効に使う region はキャッシュファイルの `region` を優先し、無ければ呼び出し側の region で補う

- `sso:Logout` はトークンを発行した IAM Identity Center のインスタンスがある region に送る必要がある。トークンキャッシュの `region` (`ssoauth.TokenCache.Region`。`TokenCache` は AWS CLI 互換のフォーマットを意図しており、AWS CLI が書くファイルにも同じキーがあるかはリポジトリ内では確認できない) はトークンを発行した `ssooidc` クライアントの region そのものなので、これを第一に使う。
- キャッシュファイルに `region` が無い場合は `fallbackRegion` を使う。`Logout` を呼ぶのは API だけで、`handleSSOLogout` が既に持つ `cfg.Region` (profile の `sso_region`) を渡す。他ツールが `region` 無しで書いたキャッシュに対しては `sso_region` と一致する保証が無く、fallback は失敗しうるベストエフォートである。失敗しても削除は続行され、`RevokeFailed` として警告に出る。`LogoutAll` はトークンごとに `region` が異なりうるため `fallbackRegion` を取らない。`region` を決められないトークン (`LogoutAll` で `region` が無い、または `Logout` で `region` も `fallbackRegion` も空) には `RevokeToken` を呼ばず、非公開のセンチネル `errRegionUnknown` (`errors.New("region unknown")`) を `Err` に持つ `RevokeFailure` として `RevokeFailed` に計上する。AWS 側のセッションが残ることを利用者に警告として伝えるためで、`slog.Warn` だけでは CLI の利用者に届かない。`accessToken` が空のトークン (破損や他ツールの不完全な書き込み) も同様に `RevokeToken` を呼ばず、非公開のセンチネル `errAccessTokenMissing` (`errors.New("access token missing")`) を `Err` に持つ `RevokeFailure` として `RevokeFailed` に計上する。AWS 側にセッションが残っているかを判断できない点は region 不明と同じであり、扱いを揃える。
- 採らなかった案: `accessToken` が空のトークンは失効対象が無いものとして `slog.Warn` だけで済ませる。region 不明のケースと扱いが非対称になり、警告を出す基準の説明が複雑になる。
- 採らなかった案: region が決められないトークンを `slog.Warn` だけでスキップする。API の利用者は backend のログを見ないと気付けず、CLI の利用者は何も知らされない。
- 採らなかった案: 常に profile の `sso_region` を使う。キャッシュファイルは他ツール (AWS CLI) が別の region で書いた可能性があり、profile の設定と一致しない場合に失効が `UnauthorizedException` で失敗する。

### 失効の失敗はログアウトを止めず、削除を続行して呼び出し側に警告として伝える

- `RevokeToken` の失敗 (ネットワーク到達不能、`UnauthorizedException` 等) は `slog.Warn` に記録し、ローカルのキャッシュ削除を続行する。`Logout` と `LogoutAll` の返り値のエラーは削除の失敗だけを表す。
- 失効の結果は呼び出し側に返す。`Logout` と `LogoutAll` は `(LogoutResult, error)` を返し、`LogoutResult` に `Revoked int` と `RevokeFailed []RevokeFailure{FileNames []string, Err error}` を持たせる。どちらも重複排除後のユニークな `accessToken` (AWS 側のサインインセッション) 単位で数え、`FileNames` にはそのトークンを含む全ファイル名を入れる。警告文言の `N SSO session(s)` はこの単位で数えるため、ファイル数とずれない。`FileNames` は `ssoauth` が失効失敗ごとに出す `slog.Warn("sso token revoke failed", "files", failure.FileNames, "err", failure.Err)` の属性として使い、`Revoked` は `Logout` / `LogoutAll` の完了時に (対象トークンが 0 件でも) `slog.Info("sso logout completed", "revoked", result.Revoked, "revoke_failed", len(result.RevokeFailed))` として記録する。API と CLI は `RevokeFailed` の件数だけを見る。CLI は `RevokeFailed` が 1 件以上あるとき `cmd.ErrOrStderr()` へ `Warning: failed to revoke N SSO session(s) on AWS; the sign-in session may remain valid until it expires` の形式で 1 行出し、終了コードは 0 にする。API は `RevokeFailed` を `slog.Warn` に出すだけで、レスポンスは現状どおり 204 とする。
- 理由: ログアウトの主目的は thief と AWS CLI から見て未ログイン状態を作ることで、これはローカルの削除で達成される。失効を必須にすると、オフラインでログアウトできず、`expiresAt` を過ぎたトークンで常に失敗する。一方で失効の失敗を完全に黙殺すると、利用者は AWS 側のセッションが残っていることを知る手段が無い。
- `expiresAt` を RFC 3339 として解釈でき、現在時刻より過去のトークンは `RevokeToken` を呼ばずにスキップし、`Revoked` にも `RevokeFailed` にも数えない。期限切れトークンへの `sso:Logout` は失敗するだけで意味が無い。`expiresAt` が無い、または解釈できないトークンは失効を試みる。
- 採らなかった案: 失効に失敗したら 500 (API) / 非ゼロ終了 (CLI) にしてファイルを残す。上記のオフラインと期限切れの問題に加え、frontend の `useSSOLogout` が失敗として扱い、ローカルは未ログインなのに `logoutFailed` を表示する矛盾が起きる。
- 採らなかった案: API のレスポンスを 200 と JSON ボディに変え、frontend で失効の失敗を表示する。frontend の変更範囲が広がり、`postSSOLogout` の返り値の型も変わる。まず backend のログで観測できる状態にし、表示の要否は運用で判断するため、本 issue のスコープ外とする。

### 削除の前に失効する

- 失効にはアクセストークンが要るため、ファイルの削除より前に呼ぶ。失効が成功して削除が失敗した場合、ファイルは残るが中のトークンは無効になっており、`readSSOCacheStatuses` は `expiresAt` で `valid` と判定し続ける。この場合は削除の失敗をエラーとして返し (現状の `RemoveSSOTokenCache` の扱いと同じ)、利用者に再実行させる。
- 削除が失敗した場合も `LogoutResult` は返す (`error` が非 nil でも `LogoutResult` は有効)。Go の一般的な契約 (error 非 nil のとき他の戻り値はゼロ値) からの逸脱であり、`Logout` / `LogoutAll` の godoc に明記する。失効は削除より前に終わっており、削除が失敗しても失効の警告を出す必要があるため、1 回の呼び出しで両方を返す。
- 採らなかった案: 失効と削除を別の関数 (`Revoke` と `Remove`) に分けて呼び出し側で順に呼ぶ。列挙 → 失効 → 削除の順序と重複排除を呼び出し側 (CLI と API の 2 箇所) が担うことになり、統合の目的に反する。CLI は `RevokeFailed` の警告を先に標準エラー出力へ出し、その後に削除のエラーを返して非ゼロで終了する。削除のエラーは `RemoveSSOTokenCache` が `errors.Join` でまとめた複数行になりうるが、backend/internal/cli/root.go が `SilenceErrors` を立てているため cobra は表示せず、`cli.Run` (backend/internal/cli/run.go) が `Error:` を前置して標準エラー出力に出す。`ssoLogoutWith` では整形しない。API は `RevokeFailed` を `slog.Warn` に出した上で削除のエラーを 500 `SSO_LOGOUT_FAILED` として返す。
- 列挙と削除は別々に走査するため、失効の待ち時間 (件数 × 最大 30 秒) の間に同じ start URL で再ログインして書かれた新しいトークンファイルも `RemoveTokenCache` / `RemoveAllCache` が削除する。ログアウトの途中に同じ利用者が再ログインする操作は競合する操作であり、削除される結果 (再ログインが要る) は現状の `RemoveSSOTokenCache` と CLI `ssoLogout` の走査と削除の間にも同じ幅で存在する。本 issue ではこの競合を許容する。
- 採らなかった案: `ListTokens` が返した `FileName` の集合を `RemoveTokenCache` に渡し、削除対象を列挙時のスナップショットに固定する。`LogoutAll` は列挙に載らない client registration も削除する必要があり、スナップショット方式では別に走査が要る。また列挙後に書かれたトークンだけが残ると、`sso:Logout` で失効させたセッションと同じ start URL の有効なトークンが混在し、`readSSOCacheStatuses` の表示 (期限が最も先のものを採用) と実際の状態の対応が読みづらくなる。
- 再実行時は既に失効済みのトークンに再度 `sso:Logout` を呼ぶことになる。この呼び出しが失敗すれば `RevokeFailed` に計上され警告が出るが、警告の文言は「有効なまま残るかもしれない」であり、失効済みの場合にも偽ではない。失効済みかどうかを区別する手段が無いため、区別しない。
- 採らなかった案: 削除の後に失効する。削除の前にトークンを退避することになり、順序を変えるだけで退避が要らなくなる。

### タイムアウト

- `sso:Logout` は 1 往復のネットワーク呼び出しなので、トークン 1 件ごとに `context.WithTimeout` で上限を置く。値は backend/internal/api/handlers_sso.go の `ssoLoginStartTimeout` (30 秒) と同じ根拠 (ブラウザの操作を待たない純粋なネットワーク往復) で 30 秒とし、`ssoauth` パッケージの定数 `revokeTimeout` として置く。
- API では `r.Context()` をそのまま親にし、`handleSSOLoginStart` のような追加の `context.WithTimeout` は掛けない。上限はトークン単位の `revokeTimeout` で既に掛かる。クライアントが切断して `r.Context()` が Done になった場合は次項の扱いで削除まで進み、204 の書き込みは届かないが処理は完了する。CLI では `commandContext(cmd)` を親にする。
- 失効はトークンを 1 件ずつ逐次に呼ぶ。1 つの cacheDir に存在するトークンは通常 1 桁件で、最悪でも件数 × 30 秒で終わる。件数に上限は設けないため総時間にも上限は無く、到達不能な region のトークンが大量にある場合は CLI が件数 × 30 秒まで待つ。Ctrl-C で親の ctx が Done になれば次項の規則で残りをスキップして削除に進むため、利用者は途中で打ち切れる。件数の上限や件数に応じた警告は、実際に大量のトークンが観測されてから別 issue で扱う。
- 採らなかった案: `errgroup` で並列に失効する。件数が少なく待ち時間の短縮効果が無い一方、region ごとの SDK クライアントを同時に生成し、`slog.Warn` と `RevokeFailed` の順序が非決定になるため却下する。
- 採らなかった案: 全体に 1 つのタイムアウトを掛ける。1 件目で 30 秒使い切ると残りが全て失敗し、失敗の原因が呼び出しごとの遅延か上限の使い切りか区別できなくなるため却下する。
- 親の ctx が失効の途中で Done になった場合 (Ctrl-C、クライアントの切断)、残りのトークンへの `RevokeToken` は呼ばず、`ctx.Err()` を `Err` に持つ `RevokeFailure` として `RevokeFailed` に計上する。ローカルの削除は ctx を取らないため、そのまま実行する。途中で止めてファイルを残すと、失効済みのトークンを `readSSOCacheStatuses` が `valid` と判定し続けるためである。

### CLI `thief sso logout` の引数

- `ssoLogout` は `ssoauth.LogoutAll(ctx, deps)` を呼び、現状と同じく全トークンを対象にして全ファイルを削除する。TODO は「`ssoauth.Logout` に寄せる」と書いているが、`ssoauth.Logout` は start URL 単位であり、CLI は start URL を受け取らず全件を対象にするため、同じ列挙 → 失効 → 削除の本体を共有する全件版 `LogoutAll` を `ssoauth` に置いて CLI をそこへ寄せる。TODO が求める統合 (CLI 固有の削除ロジックを無くし `ssoauth` に集約する) はこれで満たす。フラグと引数は追加しない。`Short` と `Long` の文言は失効を含む内容に更新する。
- `cacheDir` が存在しない場合は現状の `directory does not exist` エラーをやめ、削除対象が無い正常系として成功にする。docs/issues/closed/0153 で追加された現行の `ssoauth.Logout` (API 経由) が `cacheDir` の不在を成功として扱うのと揃える。CLI の挙動変更なので CHANGES.md では `[CHANGE]` として記載する。
- `ssoLogout` は `ssoLoginWith` と同じ形で `ssoLogoutWith(cmd, deps ssoLogoutDeps)` に本体を分け、`ssoLogoutDeps` に `logoutAll` の関数値を持たせてテストで差し替える。
- 採らなかった案: `login` と同じ `--url` フラグを追加し、指定時はその start URL のトークンだけを失効して削除する。TODO の要望は失効の追加と `ssoauth` への統合であり、start URL 単位の部分ログアウトは含まれない。統合は `LogoutAll` を呼ぶだけで達成できるため、本 issue では追加しない。必要になれば別 issue で扱う。

### 追加の API 呼び出しと権限

- 追加する AWS API 呼び出しは `sso:Logout` のみ。アクセストークンで認可され、IAM 権限と AWS 認証情報は不要。
- `sso:Logout` はサインインセッションを失効させるが、SDK の doc コメントにあるとおり、その permission set から既に発行済みの IAM ロールの一時認証情報は permission set の duration が切れるまで有効に残る。AWS の仕様であり、本 issue では扱わない。同様に、`LogoutAll` がローカルの client registration (`clientId` / `clientSecret`) を削除しても AWS 側の OIDC クライアント登録は `registrationExpiresAt` まで残る。登録はサインインセッションではなく、失効させる API も呼ばないため、本 issue では扱わない。

### frontend

- `AwsActiveSessionCard` のボタンの挙動と `useSSOLogout` は変えない。翻訳キー `awsActiveSessionCard.logoutTitle` (frontend/src/i18n/locales/ja/session.json と en/session.json) の文言だけを、AWS 側のサインインセッションも失効させる旨を含む内容に更新する。

### 実装の順序

- 依存の方向に沿って backend/internal/aws (`ListSSOTokenCache`、`RemoveAllSSOCache`、`RevokeSSOToken`) → backend/internal/ssoauth (`Deps` の 4 フィールド、`Logout`、`LogoutAll`) → backend/internal/api (`handleSSOLogout`) → backend/internal/cli (`ssoLogoutWith`) → frontend (翻訳文言とテスト) の順に進める。各層を書き終えるごとに `mise run backend:build` と `mise run backend:test` を通してから次の層に進み、差分が大きくても層単位で自己検証する。層ごとに別コミットにはせず 1 issue 1 コミットとする。差分は 5 層にまたがり大きくなるが、下記の理由で分割しない。
- 採らなかった案: backend/internal/aws と ssoauth と api を先行 issue、CLI 統合を後続 issue に分ける。TODO が CLI の統合を同じ issue で扱うと明記しているため分割しない。補足として、先行 issue だけを close すると CLI は旧 `ssoLogout` の独自削除ロジックのまま残り、`LogoutAll` を後続で足すまで CLI では「削除前に失効する」が満たされない (docs/issues/closed/0153 と 0154 のように段階分割は可能だが、TODO の指示に反する)。frontend の変更は翻訳文言だけであり単独の issue にする大きさではない。

## 完了条件

- backend/internal/aws/sso.go に `RevokeSSOToken(ctx context.Context, region, accessToken string) error` があり、`sso.Client.Logout` を `LogoutInput{AccessToken}` で呼ぶ。
- backend/internal/aws/sso_cache.go に `ListSSOTokenCache(cacheDir, startURL string) ([]SSOCachedToken, error)` があり、`startURL` が一致する (空なら全ての) トークンの `FileName` / `StartURL` / `Region` / `AccessToken` / `ExpiresAt` を返す。1MB 超のファイル、読めないファイル、壊れた JSON、`startUrl` を持たないファイルは `slog.Warn` を出して対象外にする。走査は `walkSSOCacheTokens` を共有し、`ssoCacheEntry` に `accessToken` フィールドを追加していない。
- backend/internal/aws/sso_cache.go に `RemoveAllSSOCache(cacheDir string) error` があり、`cacheDir` 直下の通常ファイルを拡張子を問わず削除する。サブディレクトリとシンボリックリンク等の通常ファイル以外のエントリは削除せず `slog.Warn` を出す。`cacheDir` 不在は nil を返し、`cacheDir` の読み取りが `fs.ErrNotExist` 以外で失敗した場合はエラーを返し、複数の削除失敗は `errors.Join` で返す。
- `ssoauth.Deps` に `RevokeToken` / `ListTokens` / `RemoveTokenCache` / `RemoveAllCache` があり、`DefaultDeps()` が `awsinternal.RevokeSSOToken` / `ListSSOTokenCache` / `RemoveSSOTokenCache` / `RemoveAllSSOCache` を配線し、`TestDefaultDepsIsFullyWired` が 4 フィールドの非 nil を検証している。
- `Logout` / `LogoutAll` は列挙と削除を `deps.ListTokens` / `deps.RemoveTokenCache` / `deps.RemoveAllCache` 経由で行い、`awsinternal` の関数を直接呼ばない。
- 同じ `accessToken` の複数ファイルでは、ファイル名の辞書順で最初のファイルの `region` と `expiresAt` を代表値に使う。
- `ssoauth.Logout(ctx, startURL, fallbackRegion, deps)` と `ssoauth.LogoutAll(ctx, deps)` が「列挙 → 失効 → 削除」の順で動き、`LogoutResult{Revoked, RevokeFailed}` を返す。失効の失敗は `slog.Warn` に出して削除を続行し、返り値のエラーにしない。
- `expiresAt` が RFC 3339 として解釈でき現在時刻より過去のトークンには `RevokeToken` を呼ばず、`Revoked` にも `RevokeFailed` にも数えない。
- `expiresAt` が無い、または RFC 3339 として解釈できないトークンには `RevokeToken` を呼ぶ。
- `region` があるトークンはキャッシュの `region` を使い、`fallbackRegion` で上書きしない。
- `region` が無いトークンは `Logout` では `fallbackRegion` を使う。
- `LogoutAll` は `fallbackRegion` を持たないため `region` が無いトークンは常に `RevokeFailed` になる。
- region を決められないトークンには `RevokeToken` を呼ばず `RevokeFailed` に計上する。
- `accessToken` が空のトークンには `RevokeToken` を呼ばず `RevokeFailed` に計上する。
- `Logout` / `LogoutAll` は複数トークンの `RevokeToken` を goroutine を使わず順番に呼ぶ。
- `Logout` / `LogoutAll` は `error` が非 nil でも有効な `LogoutResult` を返し、godoc にその旨が書かれている。
- `Logout` / `LogoutAll` は完了時に (対象 0 件でも) `slog.Info("sso logout completed", "revoked", <Revoked>, "revoke_failed", <len(RevokeFailed)>)` を出す。
- 同じ `accessToken` を持つ複数ファイルに対して `RevokeToken` は 1 回だけ呼ばれ、`RevokeFailure.FileNames` に該当する全ファイル名が入る。
- `RevokeToken` の呼び出しごとに 30 秒の `context.WithTimeout` が掛かる。
- 親の ctx が Done になった後は残りのトークンに `RevokeToken` を呼ばず、`ctx.Err()` を持つ `RevokeFailure` として `RevokeFailed` に計上し、削除は実行する。
- `ListSSOTokenCache` は `cacheDir` が無い場合に空スライスと nil を返す。
- `ListSSOTokenCache` は `startURL` を `normalizeStartURL` で正規化して比較する。
- `Logout` / `LogoutAll` は `CacheDir()` または `deps.ListTokens` が失敗したとき `LogoutResult{}` とエラーを返す。
- `ssoLoginDeps.logout` が `func(ctx context.Context, startURL, fallbackRegion string) (ssoauth.LogoutResult, error)` になり、`handleSSOLogout` が `s.ssoLogin.logout(r.Context(), cfg.StartURL, cfg.Region)` を呼ぶ。返り値のエラーが nil なら `RevokeFailed` の有無に関わらず 204 を返し、`RevokeFailed` が 1 件以上あるときは `slog.Warn` に出す。エラーが非 nil のときは現状どおり 500 `SSO_LOGOUT_FAILED` を返す。
- `thief sso logout` が全トークンを失効して `cacheDir` 直下の通常ファイルを削除する (サブディレクトリと通常ファイル以外のエントリは残す)。`logoutCmd` にフラグと引数は追加されていない。`cacheDir` が存在しない場合はエラーにならず終了コード 0 で終わる。
- CLI は削除が失敗した場合、`RevokeFailed` が 1 件以上あるときだけ後述の `Warning: failed to revoke ...` の警告を先に出し (0 件なら出さない)、その後に削除のエラーをそのまま return して終了コードは非ゼロ。`ssoLogoutWith` はエラーメッセージに独自の前置きを付けない (表示は `cli.Run` に委ねる)。
- CLI は削除が成功し `RevokeFailed` が 1 件以上あるとき、標準エラー出力に `Warning: failed to revoke N SSO session(s) on AWS; the sign-in session may remain valid until it expires` (N は失効に失敗した件数) の形式で 1 行出し、終了コードは 0。
- `ssoLogout` の本体は `ssoLogoutWith(cmd, deps ssoLogoutDeps)` に分離され、`ssoLogoutDeps` は `logoutAll` を関数値として持ち、テストから差し替えられる。`ssoLogout` と `ssoLogoutWith` の中に `filepath.Walk` と `os.Remove` の直接呼び出しが無い。
- `logoutCmd` の `Short` と `Long` の文言に `revoke` の語が含まれ、AWS 側のサインインセッションを失効させることを述べている。
- `ssoauth.Logout`、`handleSSOLogout`、frontend の `postSSOLogout` のコメントから「AWS 側のセッション失効は行わない」旨の記述が消え、実際の挙動と一致している。
- 翻訳キー `awsActiveSessionCard.logoutTitle` の ja の文言に「失効」、en の文言に `revoke` が含まれ、AWS 側のサインインセッションを失効させることを述べている。
- テスト (以下のファイルに追加または更新する)。
  - backend/internal/aws/sso_cache_test.go:
    - `ListSSOTokenCache`: startURL が一致するトークンだけを返す。
    - `ListSSOTokenCache`: startURL が一致しないトークンを返さない。
    - `ListSSOTokenCache`: 空 startURL で全トークンを返す。
    - `ListSSOTokenCache`: `startUrl` を持たないファイル (client registration) の除外。
    - `ListSSOTokenCache`: 読めないファイル、1MB 超のファイル、壊れた JSON の除外。
    - `ListSSOTokenCache`: `cacheDir` 不在時に空スライスと nil。
    - `RemoveAllSSOCache`: `.json` と `.json` 以外を含む全ファイルの削除 (client registration を含む)。
    - `RemoveAllSSOCache`: サブディレクトリとシンボリックリンクを残して `slog.Warn` を出す。
    - `RemoveAllSSOCache`: `cacheDir` に通常ファイルを指定して `os.ReadDir` を失敗させたときにエラーを返す (既存の `RemoveSSOTokenCache` のテストと同じ、パーミッションに依存しない方法)。
    - `RemoveAllSSOCache`: `cacheDir` 不在時に nil。
    - `RemoveAllSSOCache`: 一部の削除失敗時に `errors.Join` で全件を返す。
  - backend/internal/aws/sso_test.go (無ければ新設)。SDK クライアントはコンシューマ定義インターフェースかモックで差し替える。
    - `RevokeSSOToken`: モックの `Logout` が受け取った `LogoutInput.AccessToken` が引数の `accessToken` と一致する。
    - `RevokeSSOToken`: SDK クライアントの失敗を `sso logout: %w` でラップして返す (文言の完全一致で検証)。
  - backend/internal/ssoauth/ssoauth_test.go: `ListTokens` / `RemoveTokenCache` / `RemoveAllCache` / `RevokeToken` を記録用のダミーに差し替え (ファイルシステムは使わない)、`Logout` / `LogoutAll` について以下を検証する。
    - `RevokeToken` が `RemoveTokenCache` / `RemoveAllCache` より前に呼ばれる (呼び出し順を記録して検証)。
    - `ListTokens` に `CacheDir()` の値と `startURL` (`LogoutAll` では空文字) が渡る。
    - `ListTokens` がエラーを返したとき、`RevokeToken` と削除を呼ばず `LogoutResult{}` とそのエラーを返す。
    - `HOME` を空にして `CacheDir()` を失敗させたとき、`ListTokens` を呼ばず `LogoutResult{}` とエラーを返す。
    - region 不明と `accessToken` 空の `RevokeFailure.Err` が `errors.Is` で `errRegionUnknown` / `errAccessTokenMissing` に一致する。
    - 同じ `accessToken` の 2 ファイルで `region` / `expiresAt` が異なるとき、ファイル名の辞書順で最初のファイルの値が使われる。
    - `RevokeToken` が失敗しても削除が続行され、`RevokeFailed` に該当トークンの `FileNames` と `Err` が入る。
    - 同じ `accessToken` の 2 ファイルが失効に失敗したとき、1 つの `RevokeFailure.FileNames` に両方のファイル名が入る。
    - 期限切れトークンは `RevokeToken` が呼ばれず `Revoked` にも `RevokeFailed` にも数えられない。
    - `expiresAt` が無いトークンと解釈できないトークンには `RevokeToken` が呼ばれる。
    - `region` があるときはキャッシュの値が `RevokeToken` に渡り、`fallbackRegion` は使われない。
    - `region` が無いとき `Logout` は `fallbackRegion` を渡し、`fallbackRegion` も空なら `RevokeFailed` に計上する。
    - `LogoutAll` で `region` が無いトークンは `RevokeFailed` に計上する。
    - `accessToken` が空のトークンは `RevokeToken` が呼ばれず `RevokeFailed` に計上する。
    - 同じ `accessToken` の 2 ファイルで `RevokeToken` は 1 回だけ呼ばれ、`Revoked` は 1。
    - 失効失敗時の `slog.Warn` に `files` 属性として `FileNames` が出る。
    - 完了時の `slog.Info` に `revoked` と `revoke_failed` が正しい値で出る (対象 0 件のときも出る)。
    - `RevokeToken` に渡る ctx に 30 秒のデッドラインが設定されている。
    - 親の ctx を事前にキャンセルすると残りのトークンに `RevokeToken` が呼ばれず `ctx.Err()` を持つ `RevokeFailure` に計上され、削除は実行される。
    - `RevokeToken` は goroutine を使わず順番に呼ばれる (呼び出し順がファイル名の辞書順になる)。
    - `RemoveTokenCache` / `RemoveAllCache` がエラーを返したとき、そのエラーと共に失効結果を保った `LogoutResult` が返る。
    - `ListTokens` が空スライスを返したとき、`RevokeToken` を呼ばず削除だけ行って成功し、`slog.Info` の `revoked` と `revoke_failed` が 0 で出る。
    - `TestDefaultDepsIsFullyWired` が追加 4 フィールドの非 nil を検証する。
  - backend/internal/api/handlers_sso_test.go: `TestSSOLogoutSucceeds` / `TestSSOLogoutErrors` / `TestSSOLogoutRouteMethod` を新しいシグネチャに合わせ、`RevokeFailed` があっても 204 になるケースを追加する。
  - backend/internal/cli/sso_test.go: `ssoLogoutWith` について以下を検証する。
    - `logoutAll` が呼ばれ、`commandContext(cmd)` の ctx が渡る。
    - `RevokeFailed` が 1 件以上で削除成功のとき、標準エラー出力に所定の警告が 1 行出て、エラーは返らない。
    - `RevokeFailed` が 0 件で削除成功のとき、標準エラー出力に何も出ない。
    - `RevokeFailed` が 1 件以上で `logoutAll` がエラーを返すとき、警告が出た後にそのエラーがそのまま返る。
    - `RevokeFailed` が 0 件で `logoutAll` がエラーを返すとき、警告を出さずそのエラーがそのまま返る。
  - frontend/src/components/session/AwsActiveSessionCard.test.tsx: `logoutTitle` の文言を完全一致で検証しているアサーションを新しい文言に更新する。
- 本 issue で扱わない範囲: `--url` フラグ等による start URL 単位の部分ログアウト、API レスポンスへの失効結果の追加と frontend での表示、発行済みの IAM ロール一時認証情報の無効化、OIDC クライアント登録 (`clientId` / `clientSecret`) の AWS 側での失効、`loadSSOAccessToken` の start URL 対応。
- CHANGES.md の `## develop` に、`sso:Logout` による AWS 側のサインインセッションの失効を `[ADD]`、`thief sso logout` の `cacheDir` 不在時のエラーを成功に変える挙動変更と、`cacheDir` のサブディレクトリ内のファイルを削除しなくなる (直下の通常ファイルだけ削除する) 挙動変更を、それぞれ `[CHANGE]` として記載している。
- 実 SSO 環境での手動確認。まず手順 1 から 5 を CLI で通して行い、次に手順 1 と 2 をやり直して新しいトークンを控えて有効性を確認し、手順 3 だけを frontend のボタンに差し替えて手順 4 と 5 を行う。確認結果は issue の close 時に「## 解決方法」へ記載する。実 SSO 環境を使えない場合はその旨を「## 解決方法」に明記して省略する。
  1. `thief sso login --url <subdomain>` でログインし (`--url` にはアクセスポータルのサブドメインを渡す。start URL が `https://foo.awsapps.com/start/` なら `foo`)、`~/.aws/sso/cache/` のトークンファイルから `accessToken` と `region` を控える。
  2. `aws sso list-accounts --access-token <控えた accessToken> --region <控えた region>` が成功してアカウント一覧を返すことを確認する。
  3. CLI の確認では `thief sso logout` を実行する。frontend の確認ではアクティブセッションカードのログアウトボタン (`POST /api/aws/profiles/{profile}/sso/logout`) を押す。
  4. 手順 2 と同じコマンドを再実行し、`UnauthorizedException` が返ることを確認する。
  5. CLI の確認では `~/.aws/sso/cache/` が空になっていること、frontend の確認では該当 start URL のトークンファイルが消えていることを確認する。
- `mise run check` が通過する。

## 関連

- docs/issues/closed/0153-feat-sso-logout-api.md: 本 issue の発端。`ssoauth.Logout` / `RemoveSSOTokenCache` / `handleSSOLogout` を追加し、AWS 側の失効を別 issue 候補とした。
- docs/issues/closed/0154-feat-sso-logout-button-frontend.md: frontend のログアウトボタンと `useSSOLogout`。本 issue では翻訳文言以外を変えない。
- docs/issues/closed/0147-refactor-extract-sso-device-auth-package.md: `ssoauth` パッケージと `Deps` による関数値注入のテンプレート。

## 解決方法

SSO ログアウトの処理列を「列挙 → 失効 → 削除」に組み替え、API と CLI の両方が `internal/ssoauth` の `Logout` / `LogoutAll` を経由するようにした。

- backend/internal/aws/sso.go: `RevokeSSOToken(ctx, region, accessToken)` を追加した。SDK クライアントは `ssoLogoutAPI` インターフェース (コンシューマ定義) で受け、`revokeSSOToken` が `sso.Client.Logout` を `LogoutInput{AccessToken}` で呼び、失敗を `sso logout: %w` でラップする。
- backend/internal/aws/sso_cache.go: `walkSSOCacheTokens` の訪問関数にファイル本文を渡すようにし、`ListSSOTokenCache(cacheDir, startURL)` (本文から `region` / `accessToken` を `ssoCacheTokenSecrets` で読み、`normalizeStartURL` で比較。空 startURL は全件。`cacheDir` 不在は空スライスと nil) と `RemoveAllSSOCache(cacheDir)` (`os.ReadDir` で直下の通常ファイルだけ削除、通常ファイル以外は `slog.Warn` を出して残す、`cacheDir` 不在は nil、複数の失敗は `errors.Join`) を追加した。`ssoCacheEntry` に `accessToken` は追加していない。
- backend/internal/ssoauth/ssoauth.go: `Deps` に `RevokeToken` / `ListTokens` / `RemoveTokenCache` / `RemoveAllCache` を追加し、`DefaultDeps()` で配線した。`Logout(ctx, startURL, fallbackRegion, deps)` と `LogoutAll(ctx, deps)` は `CacheDir()` → `deps.ListTokens` → `revokeTokens` → 削除の順で動き、`LogoutResult{Revoked, RevokeFailed}` を返す。`revokeTokens` はトークンをファイル名の辞書順に並べて `accessToken` でグループ化し (代表値は最初のファイル)、`expiresAt` が RFC 3339 で過去のものを飛ばし、1 件ずつ 30 秒 (`revokeTimeout`) の `context.WithTimeout` で `deps.RevokeToken` を呼ぶ。失敗は `slog.Warn("sso token revoke failed", "files", ...)` に出して `RevokeFailure{FileNames, Err}` に計上し、完了時に `slog.Info("sso logout completed", "revoked", ..., "revoke_failed", ...)` を出す。親 ctx が Done なら残りを `ctx.Err()` で計上して削除に進む。`errRegionUnknown` / `errAccessTokenMissing` はセンチネルにした。
- backend/internal/api/handlers_sso.go: `ssoLoginDeps.logout` を `func(ctx, startURL, fallbackRegion) (ssoauth.LogoutResult, error)` に変え、`handleSSOLogout` が `s.ssoLogin.logout(r.Context(), cfg.StartURL, cfg.Region)` を呼ぶ。`RevokeFailed` があれば `slog.Warn` を出し、返り値のエラーが nil なら 204、非 nil なら 500 `SSO_LOGOUT_FAILED`。
- backend/internal/cli/sso.go: `ssoLogout` を `ssoLogoutWith(cmd, ssoLogoutDeps{logoutAll})` に分離し、`LogoutAll(commandContext(cmd), DefaultDeps())` を呼ぶ。`filepath.Walk` と `os.Remove` は削除した。`RevokeFailed` が 1 件以上なら標準エラー出力に `Warning: failed to revoke N SSO session(s) on AWS; the sign-in session may remain valid until it expires` を 1 行出し、削除エラーはそのまま return する。`Short` / `Long` に `revoke` を含めた。
- frontend: `awsActiveSessionCard.logoutTitle` の ja / en を AWS 側での失効を述べる文言に変え、`AwsActiveSessionCard.test.tsx` のアサーションと `postSSOLogout` のコメントを更新した。

テストは完了条件に列挙したものを backend/internal/aws/sso_cache_test.go、backend/internal/aws/sso_test.go (新設)、backend/internal/ssoauth/ssoauth_test.go、backend/internal/api/handlers_sso_test.go、backend/internal/cli/sso_test.go、frontend/src/components/session/AwsActiveSessionCard.test.tsx に追加または更新した。ssoauth のテストは記録用ダミーでファイルシステムを使わず、`CacheDir()` の失敗は `HOME` を空にして起こす。

実 SSO 環境での手動確認 (完了条件の手順 1 から 5) は、この実装作業の環境にブラウザでの認可を行える SSO 環境が無いため省略した。`sso:Logout` の実呼び出しは自動テストでは SDK クライアントの差し替えで検証している。

`ListSSOTokenCache` と `RemoveAllSSOCache` は issue の完了条件が定めるシグネチャどおり `ctx` を受け取らない。ローカルファイルの列挙と削除だけを行い、既存の `RemoveSSOTokenCache` / `readSSOCacheStatuses` と同じ形に揃えた。AWS への通信を伴う `RevokeSSOToken` と、それを呼ぶ `Logout` / `LogoutAll` は `ctx` を第一引数で受け取る。

`mise run check` は通過した (frontend の lint 警告 10 件は実装前のベースラインと同数)。
