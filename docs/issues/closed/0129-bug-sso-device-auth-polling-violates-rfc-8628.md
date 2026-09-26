# SSO デバイス認可のポーリングが RFC 8628 に違反し、ブラウザ承認に 1 分以上かかると必ず失敗する

Created: 2026-08-08
Completed: 2026-08-08
Model: Claude Opus 5

## 背景

issue 0127 (`internal/aws/sso_oidc.go` にテストの差し込み口を作る) のレビュー中に、`WaitForSSOToken` のポーリング方針が RFC 8628 (OAuth 2.0 Device Authorization Grant) に 4 点で違反していることが判明した。

該当箇所は `backend/internal/aws/sso_oidc.go` の以下である。

```go
const (
	ssoTokenPollMaxAttempts = 60
	ssoTokenPollInterval    = 1 * time.Second
)
```

```go
	interval := policy.interval
	for i := 0; i < policy.maxAttempts; i++ {
		o, err := client.CreateToken(ctx, input)
		...
		case errors.As(err, &slowDown):
			interval *= 2
		...
	}
	return nil, fmt.Errorf("timeout waiting for authentication")
```

issue 0127 は挙動を変えない変更であり、この 4 点はスコープ外として残した。ただし 0127 でこの値はテストによって仕様として固定されたため、変更する際は 0127 のテストの書き換えも伴う。

グローバル `~/.codex/AGENTS.md` の原則は「RFC / 仕様書 準拠を最優先すること」と定めている。

## 問題

### 問題 1: StartDeviceAuthorization が返す interval を無視している

`ssooidc.StartDeviceAuthorizationOutput` は `Interval int32` を持ち、SDK の doc コメントは "Indicates the number of seconds the client must wait between attempts when polling for a session" と述べている。

`SSODeviceAuthorization` (sso_oidc.go:30) はこのフィールドを写しておらず、`DeviceCode` / `UserCode` / `VerificationURIComplete` の 3 つだけを保持する。結果として初期間隔はサーバの指示を無視した 1 秒固定になる。

RFC 8628 §3.2 は device authorization response の `interval` を「クライアントが polling requests の間に待つべき最小秒数」と定め、値が無い場合の既定を 5 秒とする。§3.4 は "the client MUST wait at least the number of seconds specified by the interval" と定める。1 秒固定はサーバが 5 秒を指示していても 1 秒で叩くことになり、MUST 違反である。

### 問題 2: SlowDown で間隔を 2 倍にしている (RFC は +5 秒)

RFC 8628 §3.5 の `slow_down` の定義は次のとおりである。

> A variant of "authorization_pending", the authorization request is still pending and polling should continue, but the interval MUST be increased by 5 seconds for this and all subsequent requests.

現在の実装は `interval *= 2` で倍加している。増分が固定 5 秒ではないため MUST 違反である。

### 問題 3: 倍加に上限が無く、待機が指数的に伸びたうえで最終的に int64 をオーバーフローする

`interval *= 2` に上限が無い。SlowDown が連続すると 1s → 2s → 4s → ... と伸びる。`ssoTokenPollMaxAttempts = 60` は試行回数を縛るだけで実時間を縛らないため、待機時間に上限が存在しない。

`ssoTokenPollInterval = 1 * time.Second` を起点にした倍加の到達点は以下である。`time.Duration` は int64 のナノ秒であり、上限は約 9.22e18 ns = 約 9.22e9 秒である。

| 倍加回数 | 間隔 |
| --- | --- |
| 10 | 約 17 分 |
| 20 | 約 12 日 |
| 33 | 2^33 秒 = 約 272 年 |
| 34 | 2^34 秒 = 約 1.72e19 ns で int64 を超え、負の duration になる |

負の duration を `time.After` に渡すとタイマーは即座に発火する。つまり待機がゼロになり、SlowDown への対処であるバックオフが逆転して `CreateToken` の連打になる。オーバーフロー後の値は倍加を繰り返すうちに 0 に収束するため、以降の試行はすべて無待機になる。

ただし実用上まず問題になるのは倍加 10 回前後の時点である。デバイスコードの有効期限 (`ExpiresIn`、通常 600 秒) を超えて待ち続けることになり、既に無効なコードのために CLI が十数分から数日黙る。倍加 34 回に到達するには先に 272 年待つ必要があるため、オーバーフローは実際には到達しない退化した終端であり、実害の本体は上限の不在そのものである。

増分を RFC 8628 §3.5 どおりの固定 5 秒にすれば、指数的な伸びもオーバーフローも同時に消える。

### 問題 4: ExpiresIn を無視し、実質 60 秒で打ち切っている (ユーザーへの影響が最も大きい)

`ssooidc.StartDeviceAuthorizationOutput.ExpiresIn` は verification code が無効になるまでの秒数を返す (SDK doc: "Indicates the number of seconds in which the verification code will become invalid")。`SSODeviceAuthorization` はこれも写していない。

打ち切りは `ssoTokenPollMaxAttempts = 60` × `ssoTokenPollInterval = 1 * time.Second` で決まり、SlowDown が来なければ実時間で約 60 秒である。

`internal/cli/sso.go:378-390` の `ssoLogin` の流れは次のとおりである。

1. `startDeviceAuth` でデバイス認可を開始する
2. `openBrowser` で検証 URL を開く
3. `display` で start URL とユーザーコードを表示する
4. `waitForToken` でポーリングする

つまり 60 秒はユーザーがブラウザで IdP にログインし、MFA を通し、アカウントとロールを選び、認可を承諾するまでの全体に与えられた時間である。MFA を挟む構成では 60 秒を超えることが普通にある。その場合デバイスコードはまだ有効なのに CLI 側だけが `timeout waiting for authentication` で失敗する。

RFC 8628 §3.5 は `expired_token` を「device_code が期限切れになった」ことを示すエラーとして定義しており、クライアントの打ち切り時点はサーバが示す有効期限に従うべきである。

### 問題 5: 打ち切りを返す直前に 1 回分の待機が無駄に入る

ループは最後の試行のあとにも `select` で待機してから抜け、その後 `timeout waiting for authentication` を返す。ユーザーは打ち切りのメッセージを見る前に余分に 1 回分の間隔 (SlowDown 後なら倍加した分) 待たされる。

```go
	for i := 0; i < policy.maxAttempts; i++ {
		...
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-policy.after(interval):
		}
	}

	return nil, fmt.Errorf("timeout waiting for authentication")
```

問題 4 の修正で打ち切りを期限基準に変える際に一緒に解消する。issue 0127 で追加した `internal/aws/sso_oidc_test.go` はこの挙動を 2 箇所で固定しているため (`TestWaitForSSOTokenTimesOutAfterMaxAttempts` の `len(recorded) != maxAttempts` と `TestProductionSSOTokenPollPolicyPinsWaitTimes` の `total != 60*time.Second`)、書き換えが必要になる。

## 再現手順

### 問題 4 (最も再現しやすい)

1. MFA を要求する IdP に紐づいた SSO プロファイルを用意する。
2. `thief sso login` (`ssoLogin` を通るコマンド) を実行する。
3. ブラウザが開いたあと、意図的に 70 秒以上待ってから承認する。
4. CLI が `Error: get token: timeout waiting for authentication` で失敗する。ブラウザ側の承認は成功しており、デバイスコードは有効期限内である。

### 問題 1

1. `StartSSODeviceAuthorization` の戻り値を確認する (`Interval` は SDK の出力に含まれるが `SSODeviceAuthorization` には無い)。
2. `CreateToken` の呼び出し間隔を計測すると、サーバが返した `Interval` と無関係に 1 秒である。

### 問題 2 / 問題 3

1. SlowDown を返すモックを `waitForSSOToken` に与える (`internal/aws/sso_oidc_test.go` の `ssoCreateTokenSlowDown` を使う)。
2. 記録された待ち時間が 2s → 4s → 8s と倍加し、+5 秒ずつにならないことを確認する。
3. 上限が無いため試行を増やすといくらでも伸びる。

## 影響

- ブラウザ承認に 1 分以上かかる環境で SSO ログインが必ず失敗する。ユーザーは再実行を繰り返すことになり、実用上の障害である。
- サーバが指示した polling interval を無視して 1 秒で叩くため、AWS 側からのスロットリング (SlowDown) を自ら誘発している。
- SlowDown が連続した場合に無応答時間が指数的に伸び、上限が無い。
- 読み取り専用の認証フローであり、データの破壊は生じない。

## 修正方針

- `SSODeviceAuthorization` に `Interval` と `ExpiresIn` を追加し、`startSSODeviceAuthorization` で写す。
- `ssoTokenPollPolicy` の組み方を変える。初期間隔はサーバの `Interval` を採用し、`Interval` が 0 のときは RFC 8628 §3.2 の既定である 5 秒を使う。打ち切りは試行回数ではなくサーバの `ExpiresIn` を基準にした期限で行う。
- SlowDown 時の増分を `interval += 5 * time.Second` に変更する。これで問題 2 と問題 3 が同時に解消する (増分が固定なので指数的に伸びない)。
- `internal/cli/sso.go` の `ssoTokenDeps.waitForToken` のシグネチャに `Interval` / `ExpiresIn` を渡す必要がある。公開関数 `WaitForSSOToken` の引数が増えるため、`SSODeviceAuthorization` をそのまま渡す形に変えるのが素直である。
- `ssoTokenPollMaxAttempts` / `ssoTokenPollInterval` の 2 定数は役割が変わるため、残すか置き換えるかを実装時に判断する。
- issue 0127 で追加した `internal/aws/sso_oidc_test.go` の以下を書き換える必要がある。
  - `TestProductionSSOTokenPollPolicyPinsWaitTimes` (`time.Second` / `60` / `60*time.Second` をリテラルで固定している)
  - `TestWaitForSSOTokenPollIntervals` の SlowDown 系 2 ケース (倍加を期待している)
  - `TestWaitForSSOTokenTimesOutAfterMaxAttempts` (試行回数での打ち切りを期待している)

## 完了条件

- `SSODeviceAuthorization` が `Interval` と `ExpiresIn` を保持し、`startSSODeviceAuthorization` がそれを写すことを検証するテストがある。
- 初期間隔がサーバの `Interval` を採用し、`Interval` が 0 のときは 5 秒になることを検証するテストがある。
- SlowDown 時の増分が固定 5 秒であることを検証するテストがある。連続 SlowDown で指数的に伸びないことも検証する。具体的には、最大試行回数ぶん連続で SlowDown を返したときに、記録されたすべての待ち時間が正であり上限以下であることを検証する (現行コードはこのテストで落ちる)。
- 打ち切りを返す直前の余分な 1 回分の待機が無くなっている。
- 打ち切りが `ExpiresIn` を基準にしており、`ExpiresIn` が 600 秒なら 60 秒で打ち切らないことを検証するテストがある。
- テストの実行時間が実際の待ち時間に引きずられていない (issue 0127 で入れた `ssoTokenPollPolicy.after` の差し替えを使う)。
- `internal/cli/sso.go` の `ssoTokenDeps` と `sso_test.go` が新しいシグネチャに追随している。
- `mise run check` が通る。

## 関連

- docs/issues/0127: `internal/aws/sso_oidc.go` にテストの差し込み口を作った issue。本 issue の修正はそこで追加したテストの書き換えを伴う。
- docs/issues/closed/0125: SSO OIDC のエラーラップ文言を整えた issue。
- RFC 8628 §3.2 (device authorization response の `interval`)、§3.4 (polling)、§3.5 (`slow_down` と `expired_token`)

## 解決方法

### ポーリング方針をサーバの指示から組む形に変えた

`backend/internal/aws/sso_oidc.go` の定数 `ssoTokenPollMaxAttempts = 60` と `ssoTokenPollInterval = 1 * time.Second` を廃止し、RFC 8628 に根拠のある 3 定数に置き換えた。

- `ssoTokenPollDefaultInterval = 5 * time.Second` (§3.2 の `interval` の既定値)
- `ssoTokenPollSlowDownIncrement = 5 * time.Second` (§3.5 の `slow_down` の増分)
- `ssoTokenPollDefaultTimeout = 600 * time.Second` (`expires_in` が欠けた非準拠応答への防御的既定値)

`SSODeviceAuthorization` に `Interval int32` と `ExpiresIn int32` を追加し、`startSSODeviceAuthorization` が `StartDeviceAuthorizationOutput` から写すようにした。

`ssoTokenPollPolicy` を `{interval, maxAttempts, after}` から `{interval, timeout, now, after}` に変え、`newSSOTokenPollPolicy(deviceAuth)` がサーバの指示から組むようにした。打ち切りの判定に現在時刻を使うため `now func() time.Time` を追加している。

`WaitForSSOToken` のシグネチャを `deviceCode string` から `deviceAuth *SSODeviceAuthorization` に変えた。コアの `waitForSSOToken` は `deviceCode string` のまま据え置いた。この関数が応答から読むのは device code だけで、`interval` と `expires_in` は `newSSOTokenPollPolicy` が policy へ畳み込むためである。`internal/cli/sso.go` の `ssoTokenDeps.waitForToken` も新シグネチャに追随させた。

### 問題ごとの対応

- 問題 1 (interval 無視): 初期間隔を `deviceAuth.Interval` から採る。0 以下なら §3.2 の既定 5 秒に倒す。
- 問題 2 (SlowDown で倍加): `interval *= 2` を `interval += ssoTokenPollSlowDownIncrement` に変えた。
- 問題 3 (指数増加とオーバーフロー): 倍加をやめたことで経路そのものが消えた。`ExpiresIn` は int32 なので `time.Duration(ExpiresIn) * time.Second` の最大値は約 2.147e18 ns で、int64 の上限 約 9.223e18 ns に対して 4 倍以上の余裕があり、オーバーフローは構造的に起こらない。
- 問題 4 (ExpiresIn 無視・60 秒打ち切り): 打ち切りを試行回数から `deadline := policy.now().Add(policy.timeout)` の壁時計期限に変えた。期限はループに入る前に 1 回だけ確定させる。
- 問題 5 (打ち切り直前の無駄な待機): 打ち切り判定を `select` の前に置き、`!policy.now().Add(interval).Before(deadline)` で「待ち終える時刻が期限に届くなら待たずに打ち切る」形にした。この判定は間隔の上限も兼ねており、`interval` が `timeout` 以上に伸びた時点で必ず成立するため `slow_down` が続いても待機が猶予を超えない。

`interval` 5 秒 / `expires_in` 600 秒に対する実測は CreateToken 120 回・待機 119 回・合計 595 秒である。修正前は約 60 秒で打ち切っていた。

### 無限ループの穴を塞いだ

期限だけで縛る `for {}` は `interval <= 0` のとき待機で時刻が進まず、判定が永久に成立しないまま `CreateToken` を連打する。`newSSOTokenPollPolicy` で `Interval > 0` / `ExpiresIn > 0` を条件に既定へ倒すことで `interval > 0` を不変条件にし、さらに `waitForSSOToken` の先頭で `policy.interval <= 0 || policy.timeout <= 0` を検証して関数自身の前提として持たせた (`ssoTokenPollPolicy` は構造体リテラルでも組めるため、契約を呼び出し元の善意に依存させない)。

### レビューで判明した点への対応

`WaitForSSOToken` に `deviceAuth == nil` のガードを追加した。この関数は今回の変更で `deviceAuth` の参照外しを 2 箇所持つようになったため、境界で自衛する (AGENTS.md は「リクエスト処理中の panic は禁止」と定める)。検証は AWS クライアントの生成より前に置き、認証情報もネットワークも要らない形にした。

打ち切りのエラーをセンチネル化した (`errSSOTokenTimeout` / `errNilSSODeviceAuthorization` / `errInvalidSSOTokenPollPolicy`)。テストの文字列一致を `errors.Is` に置き換え、利用者に見える文言は別テストでリテラル固定した。

打ち切りの判定は `ctx` を見ないため、直前にキャンセルされていると中断が打ち切りとして返っていた。同時成立時は `ctx.Err()` を優先するようにした (現行の呼び出し元は `context.Background()` を渡すため現時点の挙動は変わらない。この点は issue 0131 に切り出した)。

### RFC の節番号の誤りを直した

コードとテストのコメントは「the client MUST wait at least the number of seconds specified by the interval parameter」を §3.4 に帰属させていたが、rfc-editor.org の原文で確認したところ、この文は **§3.5 の `authorization_pending` の説明の中**にある。§3.4 はリクエストの形式を定義する節である。該当 4 箇所を §3.5 に修正した。`§3.2` 側の帰属 (interval OPTIONAL・既定 5 の MUST、`expires_in` REQUIRED) は原文どおりで正しかった。本 issue 本文の「関連」節にも同じ誤りが残っているが、issue 本文は追記のみとする方針のためここに訂正を記録する。

あわせて `default` 分岐に §3.5 の「For any other error, the client MUST stop polling」を根拠として明記した。SDK の生成コード (`deserializers.go` の `awsRestjson1_deserializeOpErrorCreateToken`) を確認したところ `CreateToken` が返しうる型は 11 個で、`AuthorizationPendingException` と `SlowDownException` 以外の 9 個 (`AccessDeniedException` と `ExpiredTokenException` を含む) を即時失敗させる現状の実装が準拠である。これらを再試行に回すと逆に仕様違反になる。

### テスト

`backend/internal/aws/sso_oidc_test.go`。

- `recordingAfter` を `fakeSSOClock` に置き換えた。`After(d)` が `d` を記録しつつ時刻を `d` だけ進めるため、期限ベースのループが即座かつ決定的に終わる。
- `TestWaitForSSOTokenTimesOutAfterMaxAttempts` を `TestWaitForSSOTokenPollsUntilDeviceCodeExpires` に置き換えた (120 回 / 119 回 / 595 秒を固定)。
- `TestWaitForSSOTokenSlowDownStaysBounded` を追加。待機の内訳 14 件 (`10s` から `75s` まで 5 秒刻み) を `cmp.Diff` でリテラル固定し、加えて全要素が正かつ猶予以下であることを検証する。
- `TestProductionSSOTokenPollPolicyPinsWaitTimes` を `TestNewSSOTokenPollPolicyFollowsServerInstructions` に置き換えた (5 行のテーブルで指示・0・負値の写り方を固定)。
- `TestNewSSOTokenPollPolicyUsesRealClock` を追加。`now()` が `time.Now()` の範囲に収まることに加え、1 ミリ秒進めた後の 2 回目が進むことも見る (構築時刻を固定して返す時計は本番で打ち切りが恒偽になり永久ループするため、これを捕まえる)。
- `TestWaitForSSOTokenPrefersContextErrorOverTimeout` / `TestWaitForSSOTokenRejectsInvalidPolicy` / `TestWaitForSSOTokenRejectsNilDeviceAuthorization` / `TestSSOTokenPollSentinelMessages` を追加。
- `TestStartSSODeviceAuthorizationSendsRegistrationAndStartURL` の期待値に `Interval: 7` / `ExpiresIn: 900` を追加した (既定値 5 / 600 と別値にすることで「写さず既定に落ちる」変異を捕まえる)。

`backend/internal/cli/sso_test.go` に `TestGetSSOTokenPassesDeviceAuthorizationThrough` を追加した。`deviceAuth` が値として一致することを `cmp.Diff` で見るため、`Interval` / `ExpiresIn` が落ちると検出できる。

### 検証

ミューテーションテストを 26 件実施し、26 件すべて検出された。内訳は倍加への復帰、増分の 4 秒化、既定値の改変、`Interval >= 0` への緩め、写し漏れ、`Interval` と `ExpiresIn` の入れ替え、無駄な最終待機の復活、本番時計の凍結、`after` が引数を無視する変異、期限のループ内再計算、`policy.interval` の無視、単位のミリ秒化、`timeout` が `Interval` を読む変異、CLI 側の指示の取り落ち・URL の誤り・コードの誤表示・`waitForToken` の nil 化、および今回追加したガード類 (nil 応答・方針検証・ctx 優先・センチネル) である。

`mise run check` は backend / frontend ともに通っている。

### 本 issue の範囲外として切り出した issue

- docs/issues/0131: CLI のコマンドが `signal.NotifyContext` を使わず、`waitForSSOToken` の `ctx.Done()` 分岐が本番で発火しない。
- docs/issues/0132: サーバの `verification_uri` (§3.2 で REQUIRED) を保持せず、`startURL + "#/device"` を自前構築して表示している。
- docs/issues/0133: 接続タイムアウトでポーリング頻度を落とさず即時失敗する (§3.5 の MUST)。
