# CLI の SSO デバイス認可フローを API サーバと共有できるパッケージへ抽出する

Created: 2026-08-24
Model: Claude Fable 5
Completed: 2026-08-24

## 背景

docs/issues/TODO.md の次の項目に由来する。

> Frontend で SSO Login をして別タブで開いたリダイレクト先の AWS のページでの認証が通ったあと Frontend のタブに切り替わるようにしたい

この要望の実現は 3 つの issue に分割した。本 issue はその第 1 段階で、CLI に実装済みの SSO デバイス認可フローを API サーバからも使える共有パッケージへ抽出する (CLI の外部挙動は変えない)。第 2 段階 (docs/issues/0148-feat-sso-device-auth-api-endpoints.md) が backend への開始 / 完了エンドポイントの追加、第 3 段階 (docs/issues/0149-feat-sso-login-refocus-frontend-tab.md) が frontend の切り替えとタブ制御である。分割の理由は、各段階が現行のログイン経路 (`aws sso login` の exec 方式) を壊さずに独立して実装、検証、close できることである。

抽出対象は `backend/internal/cli/sso.go` にある。

- `getSSOTokenWith` (sso.go:446-496) は RFC 8628 のデバイス認可フロー (RegisterClient → StartDeviceAuthorization → CreateToken ポーリング) を実装している。依存の型は `ssoTokenDeps` (sso.go:408) で定義され、`registerClient` / `startDeviceAuth` / `waitForToken` / `openBrowser` を注入できる (docs/issues/closed/0129, 0132, 0133 で RFC 準拠と堅牢化済み)。
- `saveSSOCacheFile` (sso.go:568) はトークンを `~/.aws/sso/cache/<sha1>.json` に AWS CLI 互換の JSON (`SSOTokenCache`) で保存する。この形式は backend のリソースハンドラが使う SDK のトークン解決と互換である。

これらは `internal/cli` パッケージの非公開シンボルであるため、`internal/api` からは使えない。また `getSSOTokenWith` は RegisterClient → StartDeviceAuthorization → 表示 → openBrowser → CreateToken ポーリング → キャッシュ生成を 1 関数で行う CLI 向けの合成であり、API 側 (0148) は「認可 URL を返す」段階と「トークンを待つ」段階を別のリクエストとして呼ぶ必要がある。単純な公開や移動では済まず、関数の分割を伴う抽出が要る。これが本 issue の対象である。

併せて、系列全体が依存する前提を本 issue で最初に検証する。第 3 段階のタブ制御は「スクリプトが `window.open` で開いたタブを `close()` すると、フォーカスが opener のタブへ戻る」というブラウザ挙動に依存するが、この挙動は主要ブラウザで一般的なものの Web 標準で保証されてはいない。前提が成立しない場合は 0148 / 0149 の設計自体を見直す必要があるため、大きな実装に入る前の本 issue で PoC として確認する。

## 目的

デバイス認可フローとトークンキャッシュ保存が、`internal/cli` と `internal/api` の両方から使える共有パッケージの公開関数になる。CLI (`thief sso login`) の外部挙動 (出力とキャッシュ) は変わらない。併せて、系列の前提であるタブクローズ時のフォーカス復帰を PoC で検証し、結果を記録する。

## 設計判断

- 共有パッケージ (例: `internal/ssoauth`) に 2 つの公開関数として分割して移す: 「デバイス認可の開始」(RegisterClient + StartDeviceAuthorization を行い、認可 URL とデバイスコードを含む中間状態を返す) と「トークン待機」(CreateToken ポーリングから `SSOTokenCache` 生成・保存まで)。この分割は 0148 の開始 / 完了エンドポイントの単位に対応する。
- `ssoTokenDeps` 相当の依存注入は分割後も維持する (0129, 0132, 0133 で確立したテスト構造を保つ)。
- `openBrowser` と進行表示は CLI 固有の関心事なので共有パッケージへ持ち込まない。CLI の `ssoLoginWith` (sso.go:142) を、共有パッケージの 2 関数と `openBrowser` / 表示の合成に書き換える。
- パッケージの置き場所は `internal/` 配下とする (外部公開しないため。AGENTS.md のプロジェクト構造の方針に従う)。

採らなかった案とその却下理由。

- `internal/cli` のシンボルを公開して `internal/api` から import する案。`internal/cli` は cobra のコマンド層であり、ハンドラ層がコマンド層に依存するのは AGENTS.md のレイヤ規約 (上位から下位への単方向依存) に合わない。認可フローはどちらの層でもない共有ロジックなので、独立したパッケージに置く。
- 抽出せず、0148 で API 側にデバイス認可フローを再実装する案。0129, 0132, 0133 で修正した RFC 8628 準拠 (ポーリング間隔、`verification_uri` の扱い、接続タイムアウト時のバックオフ) を二重に保守することになり、片方だけ直す退行の温床になる。
- PoC を 0149 (frontend 実装時) まで遅らせる案。前提が不成立と判明した時点で 0147 / 0148 の実装が無駄になりうるため、系列の最初に検証する。

追加の AWS API 呼び出しや権限は不要である。呼び出す API は既存の CLI ログインと同一で、本 issue はコードの置き場所を変えるだけである。

## 完了条件

- 共有パッケージに「デバイス認可の開始」と「トークン待機 (キャッシュ保存まで)」の 2 つの公開関数が存在し、依存注入 (`ssoTokenDeps` 相当) が維持されている。
- `internal/cli` の `ssoLoginWith` が共有パッケージの 2 関数の合成に書き換わり、`thief sso login` の外部挙動 (出力、生成されるキャッシュファイルの形式とパス) が変わらない。CLI の既存テストが全て通る。
- `internal/api` からの共有パッケージの参照は本 issue では追加しない (追加は 0148 で行う)。
- 既存の `getSSOTokenWith` / `saveSSOCacheFile` のテストが共有パッケージ側のテストとして移設または書き直され、移設前と同じ分岐 (成功、`verification_uri_complete` 欠落、ポーリングタイムアウト、キャッシュ保存失敗) を検証している。
- PoC: ボタンの click ハンドラで同期的に `window.open('', '_blank')` し、`location` を差し替えたのち `close()` する最小の HTML ページを作り、macOS の既定ブラウザで「close 後にフォーカスが opener のタブへ戻るか」を手動確認して、結果 (ブラウザ名、バージョン、挙動) を本 issue に記録する。フォーカスが戻らない場合は 0148 / 0149 に着手せず、タブ制御の代替方針を検討して 0148 / 0149 に記録する。PoC 用ページはリポジトリにコミットしない。
- `mise run check` が通る。

## PoC の結果

完了条件の PoC (タブクローズ時のフォーカス復帰) を 2026-08-24 に実施した。

- 確認環境: macOS (Darwin 25.5.0)、既定ブラウザ Google Chrome 151.0.7922.170 (LaunchServices の https ハンドラが com.google.chrome であることを確認)。
- 手順: ボタンの click ハンドラで同期的に `window.open('', '_blank')` を呼び、返った WindowProxy の `location` を `https://example.com/` (クロスオリジンの認可ページ相当) に差し替え、3 秒後に opener 側から `close()` する最小の HTML ページを作成し、既定ブラウザで手動確認した。
- 挙動: `close()` の後、フォーカス (アクティブなタブ) は opener である PoC ページのタブに戻った。
- 判定: 前提 (スクリプトが開いたタブを `close()` するとフォーカスが opener のタブへ戻る) は成立する。0148 / 0149 に着手できる。
- PoC 用ページはリポジトリにコミットしていない (セッションの作業用一時ディレクトリに置いた)。

## 関連

- docs/issues/0148-feat-sso-device-auth-api-endpoints.md: 同じ TODO 項目の第 2 段階。本 issue が抽出する共有パッケージを backend のエンドポイントから利用する。
- docs/issues/0149-feat-sso-login-refocus-frontend-tab.md: 同じ TODO 項目の第 3 段階。本 issue の PoC が検証する前提 (タブクローズ時のフォーカス復帰) の上に成り立つ。
- docs/issues/closed/0129-bug-sso-device-auth-polling-violates-rfc-8628.md、docs/issues/closed/0132-bug-sso-device-auth-ignores-server-verification-uri.md、docs/issues/closed/0133-bug-sso-token-polling-does-not-back-off-on-connection-timeout.md: 抽出対象の CLI 実装の RFC 8628 準拠と堅牢化。本 issue の抽出はこれらの修正内容を保ったまま行う。

## 解決方法

### 共有パッケージ internal/ssoauth の新設

`backend/internal/ssoauth/ssoauth.go` を新設し、`internal/cli/sso.go` のデバイス認可フローを 2 つの公開関数に分割して移した。

- `Start` (デバイス認可の開始): RegisterClient と StartDeviceAuthorization を行い、認可 URL (verification_uri / verification_uri_complete)、ユーザコード、デバイスコードとポーリング指示 (interval / expires_in) を含む中間状態 `Session` を返す。
- `Wait` (トークン待機): CreateToken ポーリングでトークンを取得し、AWS CLI 互換の `TokenCache` を組み立てて `Deps.SaveCache` で保存し、保存済みのキャッシュを返す。Start を経ていない不完全な `Session` (nil、または Registration / DeviceAuth が nil) は panic ではなくエラーで拒否する (公開関数の分割で呼び出し順の契約が破られうるため。レビュー観点 3 の指摘の反映)。
- 依存注入は `Deps` 構造体 (RegisterClient / StartDeviceAuth / WaitForToken / SaveCache) として維持した。本番配線は `DefaultDeps` が返す。awsinternal 由来のエラーは従来どおり包み直さず伝播させ、キャッシュ保存の失敗だけは注入された実装が文脈を述べる保証が無いため `save cache file: %w` で包む。
- 移設したシンボルの対応: `SSOTokenCache` → `ssoauth.TokenCache`、`saveSSOCacheFile` → 非公開 `saveCacheFile` (`DefaultDeps` の SaveCache)、`generateSSOCacheKey` → 非公開 `cacheKey`、`getSSOCacheDir` → 公開 `CacheDir` (sso logout が使うため公開)。定数 ssoClientName / ssoClientType / ssoGrantType も ssoauth へ移した。

### CLI の書き換え

- `getSSOTokenWith` を「`ssoauth.Start` → 表示 (display) → openBrowser → `ssoauth.Wait`」の合成に書き換え、`ssoLoginWith` はこの合成を `ssoTokenDeps` 経由で駆動する形にした。openBrowser と進行表示 (writeSSOLoginPrompt / writeSSOBrowserFailureWarning) は CLI 側に残した。RFC 8628 準拠の分岐 (verification_uri_complete が空ならブラウザを開かない、表示をブラウザ起動より先に行う、ブラウザ起動失敗でフローを止めない) は合成側 (CLI) にそのまま残る。
- `ssoLoginDeps` (getToken / saveCache) は廃止した。保存は `ssoauth.Wait` の中で行われるため、`ssoLoginWith` は合成の結果を待つだけになる。
- `sso generate-config` は従来どおりトークンをキャッシュへ保存しない。`ssoauth.Wait` は保存まで含むため、`ssoTokenDepsWithoutSaving` が SaveCache を何もしない実装に差し替えて従来の挙動を保つ (保存は sso login の責務)。差し替えを関数として切り出しているのは、この不変条件をテストで直接検証するためである (レビュー観点 2 の指摘の反映)。
- `sso logout` は `ssoauth.CacheDir` を参照する。

### 方針からの乖離 (方式を保った実装詳細)

- 設計判断は「`ssoLoginWith` を共有パッケージの 2 関数と openBrowser / 表示の合成に書き換える」としていたが、合成の本体は `getSSOTokenWith` に置き、`ssoLoginWith` はそれを駆動する形にした。`sso generate-config` も同じフロー (開始 → 表示 → ブラウザ → 待機) を必要とし、合成を `ssoLoginWith` 直下へ展開すると RFC 準拠の分岐が 2 箇所へ複製されるため (本 issue が却下した「二重に保守する」状態の再現)、1 箇所の合成を両コマンドで共有した。
- 外部挙動の差分は 1 点のみ: キャッシュ保存失敗時のエラーメッセージが `save cache file: ...` から `get token: save cache file: ...` に変わる (保存が `Wait` の中へ移り、`ssoLoginWith` はトークン取得全体を `get token: %w` で包むため)。成功時出力、キャッシュファイルの形式とパス、他の失敗経路のメッセージは不変。

### テストの移設と書き直し

- `backend/internal/ssoauth/ssoauth_test.go` (新設): 成功 (Start の中間状態の組み立てと Wait のキャッシュ組み立て・保存)、verification_uri_complete 欠落 (空のまま写し、start URL から補完しない)、ポーリングの失敗 (エラーチェーン維持、失敗時に保存しない)、キャッシュ保存失敗 (文脈付きラップ)、不完全な Session の拒否、開始 2 段のエラー伝播、context の全段転送、DefaultDeps の配線、実ファイル保存 (AWS CLI 互換のパス・JSON キー・パーミッション 0600)、保存先ディレクトリ作成失敗、キャッシュキー生成、CacheDir のパス。
- `backend/internal/cli/sso_test.go` (書き直し): 合成の順序 (display → openBrowser → wait) とブラウザ起動失敗の継続、verification_uri_complete が空のときブラウザを開かず display に attemptingBrowser=false が渡ること、start が返した Session が wait へそのまま渡ること、2 段のエラー伝播、context の転送、sso login の成功時出力とエラーの `get token:` ラップ、defaultSSOTokenDeps の配線、generate-config 用の依存が本物の Start / Wait を通してもキャッシュを保存しないこと。generate-config の既存テストは型を `ssoauth.TokenCache` に追随させたのみで検証内容は不変。

### 多観点レビューの反映

- 観点 2 (高): 新設の SaveCache 無効化が未テストで退行を検出できない → `ssoTokenDepsWithoutSaving` を切り出し、本物の合成を通す TestSSOTokenDepsWithoutSavingDoesNotSaveCache を追加した。
- 観点 2 (中): attemptingBrowser=false 側の display 引数が未検証 → TestGetSSOTokenSkipsBrowserWhenVerificationURICompleteIsEmpty に捕捉と検証を追加した。
- 観点 2 (低) / 観点 4 (参考): エラーチェーンテストの stage 名がケース名と不対応 → ケース名 start / wait と base の stage を揃えた。
- 観点 3 (中): `Wait` が不完全な Session を防御していない → nil ガードと TestWaitRejectsIncompleteSession を追加し、godoc に契約を明記した。
- 観点 4 (中): CHANGES.md エントリはエラーメッセージ変更を含むため misc ではなく [UPDATE] が前例と整合 → [UPDATE] タグ付きで develop の UPDATE グループ末尾に記載した。
- 追加レビュー観点 2 (中): 新テストが本物の openBrowser を差し替えていない → ダミーに差し替え、呼ばれないことの検証を追加した。

### レビューで却下した指摘 (0148 への持ち越しを含む)

- 観点 3 (中): ポーリングの打ち切り期限が Wait 呼び出し時刻起点で、Start と Wait を別リクエストにすると実際より長くポーリングしうる → 本 issue では変更しない。サーバ側の expired エラーで自己修復し、Session の受け渡し設計は 0148 の論点のため、0148 の設計時に考慮する。
- 追加レビュー観点 3 (低): ガードのエラーがセンチネル化されておらず契約違反と他の失敗を errors.Is で区別できない → CLI では包んで表示するだけで実害が無い。0148 で HTTP ステータスへのマップを設計する際に ErrIncompleteSession の導入を検討する。
- 観点 3 (低) 2 件: SaveCache 失敗の二重ラップ構造と RegistrationExpiresAt のゼロ値未考慮 → いずれも移設前から同一の既存問題で、本 issue のスコープ (移設) 外。

### 検証

- `mise run check` 通過 (frontend 73 ファイル 732 テスト、backend 全パッケージ、lint / govulncheck 指摘ゼロ)。ベースラインからの新たな失敗は無い。
- PoC の結果は「## PoC の結果」を参照。前提は成立し、0148 / 0149 に着手できる。
