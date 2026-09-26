# SSO デバイス認可でサーバが返す verification_uri を捨て、自前構築した URL を利用者に表示している

Created: 2026-08-08
Model: Claude Opus 5
Completed: 2026-08-09

## 概要

`thief sso login` は、デバイス認可のレスポンスに含まれる `verification_uri` を保持せず、start URL に `#/device` を連結した文字列を利用者への表示に使っている。RFC 8628 §3.2 は `verification_uri` を REQUIRED と定め、§3.3 はこれを利用者へ提示することを求めている。サーバが指示した URL を捨てて自前で組み立てているため、AWS が返す値と食い違ったときにブラウザが開けなかった利用者の退路が塞がる。

## 根拠 (RFC 8628 原文)

§3.2 (Device Authorization Response) のパラメータ定義。

> verification_uri
>    REQUIRED. The end-user verification URI on the authorization server.

> verification_uri_complete
>    OPTIONAL. A verification URI that includes the "user_code" ...

§3.3 (User Interaction)。

> the client displays or otherwise communicates the "user_code" and the "verification_uri" to the end user.

§3.3.1 (Non-textual Verification URI)。

> clients MAY present this URI in a non-textual manner

> it is RECOMMENDED for clients to still display the textual verification URI.

> Clients MUST still display the "user_code", as the authorization server will require the user to confirm it.

つまり `verification_uri_complete` をブラウザで開く運用 (§3.3.1 の MAY) を採る場合でも、テキストの `verification_uri` を併せて表示するのが RECOMMENDED であり、その表示にはサーバが返した値を使うべきである。

## 該当箇所

`backend/internal/aws/sso_oidc.go` の `SSODeviceAuthorization` はサーバの `verification_uri` を保持していない。

```go
type SSODeviceAuthorization struct {
	DeviceCode              string
	UserCode                string
	VerificationURIComplete string
	Interval                int32
	ExpiresIn               int32
}
```

`startSSODeviceAuthorization` も SDK の `StartDeviceAuthorizationOutput.VerificationUri` を写していない。SDK 側にはフィールドが存在する (`ssooidc@v1.37.1/api_op_StartDeviceAuthorization.go:73` の `VerificationUri *string`)。

`backend/internal/cli/sso.go:411` の `ssoLoginDisplay` は start URL から URL を自前で組み立てている。

```go
func ssoLoginDisplay(startUrl, userCode string) {
	url := fmt.Sprintf("%s#/device", startUrl)
	...
	fmt.Println("If the browser does not open or you wish to use a different device to authorize this request, open the following URL:")
```

`user_code` の表示 (§3.3.1 の MUST) は満たしている。問題は URL の出所だけである。

## なぜ対応が必要か

`#/device` の連結は AWS Identity Center の現在のホスト名の形に対する経験的な当て推量であり、仕様上の保証がない。ブラウザの自動起動が失敗した場合 (`openBrowser` のエラーは警告として流している) や、利用者が別の端末で承認したい場合に頼るのがこのテキスト URL であり、ここが誤っていると承認そのものが行えない。サーバが REQUIRED として返している値があるのに、それを捨てて推測する理由がない。

なお start URL から組み立てた URL が実際の `verification_uri` と一致するかは未検証である。一致していたとしても、AWS が形式を変えた時点で黙って壊れる作りであることは変わらない。

## 再現手順

1. `thief sso login` を実行する。
2. 表示される「open the following URL」の URL を記録する。
3. `StartDeviceAuthorization` のレスポンスの `verification_uri` を実際に確認し (SDK のログを有効にするか、`aws sso-oidc start-device-authorization` を直接叩く)、2 の値と比較する。
4. 実装が参照しているのはレスポンスの値ではなく `startURL + "#/device"` であることが `internal/cli/sso.go:411` から確認できる。

## 修正方針

1. `SSODeviceAuthorization` に `VerificationURI string` を追加し、`startSSODeviceAuthorization` で `o.VerificationUri` を写す。
2. `ssoTokenDeps.display` のシグネチャを `func(verificationURI, userCode string)` に変え、`getSSOTokenWith` が `deviceAuth.VerificationURI` を渡す。`ssoLoginDisplay` の `fmt.Sprintf("%s#/device", startUrl)` を削除する。
3. サーバが `verification_uri` を返さなかった場合 (仕様違反の応答) のフォールバックをどうするか決める。現在の自前構築を退避先として残すか、エラーにするか。RFC 上は REQUIRED なので欠落は異常だが、表示だけの話でポーリング自体は続行できるため、警告つきで従来の構築に倒すのが穏当と考える。

## 完了条件

- `SSODeviceAuthorization` がサーバの `verification_uri` を保持し、`StartDeviceAuthorizationOutput.VerificationUri` から写されることをテストで検証する。
- 利用者へ表示される URL がサーバの `verification_uri` であることをテストで検証する (`startURL + "#/device"` を期待値にすると落ちること)。
- `verification_uri` が空の応答に対する挙動が決められ、テストで検証されている。
- `user_code` の表示 (§3.3.1 の MUST) が維持されている。

## 出典

`docs/issues/0129-bug-sso-device-auth-polling-violates-rfc-8628.md` の RFC 8628 準拠レビュー中に判明した。0129 の範囲はポーリング (§3.2 の interval / expires_in、§3.5 のエラー処理) であり、§3.3 の利用者への提示は別の問題として切り出す。RFC の条文は rfc-editor.org の原文で確認済み。

## 解決方法

### 採った方針

修正方針の 3 点をすべて実施した。3 点目 (`verification_uri` が空だった場合) は、issue が挙げた「警告つきで従来の構築に倒す」ではなく「推測値へ倒さず、欠けていることを伝えて URI の行を省く」を採った。

理由は本 issue 自身の主張と揃えるためである。`startURL + "#/device"` に仕様上の裏付けが無いことが問題なのであって、サーバが値を返さなかったときに限って同じ推測値を出すのであれば、誤った URI をサーバの指示として提示する経路が残る。`verification_uri_complete` でブラウザは既に開いているため、URI の行を省いても承認は続行できる。`user_code` の表示は RFC 8628 §3.3.1 の MUST であり、`verification_uri` の有無に関わらず出す。

### 変更点

- `backend/internal/aws/sso_oidc.go`: `SSODeviceAuthorization` に `VerificationURI` を追加し、`startSSODeviceAuthorization` が `StartDeviceAuthorizationOutput.VerificationUri` を写すようにした。既存の `VerificationURIComplete` にも、§3.2 での位置づけ (OPTIONAL) と用途 (§3.3.1 の非テキストでの提示) をコメントで残した。
- `backend/internal/cli/sso.go`: `ssoTokenDeps.display` を `func(verificationURI, userCode string)` に変え、`getSSOTokenWith` が `deviceAuth.VerificationURI` を渡すようにした。`ssoLoginDisplay` は `writeSSOLoginPrompt(w io.Writer, verificationURI, userCode string)` に置き換え、`fmt.Sprintf("%s#/device", startUrl)` を削除した。本番の `display` は `os.Stdout` を渡す薄いクロージャである。

書き出し先を引数で受け取る形にしたのは、表示内容をテストから読めるようにするためである。`cmd.OutOrStdout()` まで届けていないのは、`getSSOToken` が cobra のコマンドを受け取らないためであり、この点は変えていない。

### 完了条件の充足

1. **`SSODeviceAuthorization` がサーバの `verification_uri` を保持し、`StartDeviceAuthorizationOutput.VerificationUri` から写されること**

   `TestStartSSODeviceAuthorizationSendsRegistrationAndStartURL` (`internal/aws/sso_oidc_test.go`) の期待値に `VerificationURI` を加えた。応答の値は `verification_uri_complete` とは別のホスト名を使っており、写す元を取り違えても落ちる。

2. **利用者へ表示される URL がサーバの `verification_uri` であること**

   `TestGetSSOTokenPassesDeviceAuthorizationThrough` (`internal/cli/sso_test.go`) が `display` に渡る値を観測する。期待値との一致に加えて、`startURL + "#/device"` そのものを名指しで否定する検証を置いた。期待値だけを見る形にすると、期待値を書き換えたときに推測値が通ってしまう。

3. **`verification_uri` が空の応答に対する挙動**

   `TestWriteSSOLoginPrompt` (`internal/cli/sso_test.go`) が 2 通りを固定する。値がある場合は URI と user code を出し、`#/device` と `warning:` を含まない。空の場合は URI の行と "open the following URL:" を省き、欠けていることを伝える警告と user code を出す。

4. **`user_code` の表示 (§3.3.1 の MUST) が維持されていること**

   上記の両方の場合について `user_code` が出ることを検証している。

### 検出力の検証

実装を機械的に書き換えて 6 種類を測り、全件検出した。

- `verification_uri` を写さない / `verification_uri_complete` を写してしまう (`internal/aws/sso_oidc.go`)
- `display` に start URL を渡す / `display` に推測値 (`startURL + "#/device"`) を渡す
- `verification_uri` が空のときに推測値へ倒す / `verification_uri` が空のときに `user_code` を出さない

### 起票した issue

- docs/issues/0141: ブラウザを開けないと `sso login` がその場で中断し、テキスト URI による承認経路が使えない。

本 issue の「なぜ対応が必要か」は「`openBrowser` のエラーは警告として流している」と書いているが、実装は `return nil, fmt.Errorf("open browser: %w", err)` で中断していた。この記述は誤りである。ブラウザの起動 (RFC 8628 §3.3.1 の MAY) の失敗でフロー全体が止まるため、本 issue で直した `verification_uri` の表示に到達しない経路が残る。修正には表示の順序の変更と警告の出力先の決定が必要であり、本 issue の完了条件の範囲を超えるため分離した。
