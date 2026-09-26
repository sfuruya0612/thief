# SSO トークンポーリングが接続タイムアウトでポーリング間隔を緩めず即時失敗する

Created: 2026-08-08
Model: Claude Opus 5
Completed: 2026-08-11

## 概要

`waitForSSOToken` は `AuthorizationPendingException` と `SlowDownException` 以外のすべてのエラーを即時失敗として扱う。接続タイムアウト (ネットワーク層の失敗) もこの分岐に入るため、RFC 8628 §3.5 が MUST として定める「接続タイムアウトを受けたらポーリング頻度を自主的に落としてから再試行する」を満たしていない。

## 根拠 (RFC 8628 原文)

§3.5 (Device Access Token Response) の末尾。

> The "authorization_pending" and "slow_down" error codes define particularly unique behavior, as they indicate that the OAuth client should continue to poll the token endpoint by repeating the token request. On encountering a connection timeout, clients MUST unilaterally reduce their polling frequency before retrying.

同じ節は「それ以外のエラーではポーリングを止める」ことも MUST として定めるが、両者は衝突しない。後者が対象にしているのはトークンエンドポイントが返す OAuth のエラーレスポンス (§3.5 の 4 コードと RFC 6749 §5.2 のコード) であり、接続タイムアウトはそもそもレスポンスが返っていない輸送層の失敗である。前者はその輸送層の失敗に対する規定であり、扱いは「頻度を落として再試行」となる。

RFC は具体的な緩め方として指数バックオフを RECOMMENDED としている (該当文は §3.5 の同じ段落。ポーリング間隔を接続タイムアウトごとに倍にする方式を例示している)。

## 該当箇所

`backend/internal/aws/sso_oidc.go` の `waitForSSOToken` の分岐。

```go
var pending *ssooidctypes.AuthorizationPendingException
var slowDown *ssooidctypes.SlowDownException
switch {
case errors.As(err, &slowDown):
	interval += ssoTokenPollSlowDownIncrement
case errors.As(err, &pending):
	// ユーザーのブラウザ承認待ち。間隔は変えずに再試行する。
default:
	// RFC 8628 §3.5 は ... "For any other error, the client MUST stop polling" と定める。
	return nil, fmt.Errorf("create sso oidc token: %w", err)
}
```

接続タイムアウトは `errors.As` のどちらにも該当せず `default` に落ちる。

## なぜ対応が必要か

`thief sso login` は利用者がブラウザで承認を終えるまで最大 600 秒ポーリングする。この間に一時的なネットワーク断が 1 回起きるだけでログインが失敗し、利用者は最初からやり直しになる。MFA を挟む承認は数十秒かかるため、この窓は狭くない。

RFC が MUST としている以上、準拠の観点でも対応が必要である。

## 設計判断が必要な点

そのまま実装すると二重のバックオフになる可能性がある。AWS SDK for Go v2 のクライアントは既定で再試行機構 (`retry.Standard`) を持ち、接続エラーやタイムアウトを指数バックオフで再試行する。したがって次を確認したうえで方針を決める必要がある。

1. 接続タイムアウトが SDK の再試行を尽くしたあとにどの型で `CreateToken` から返るか。`smithy.OperationError` に包まれた `net.Error` か、`*types.InternalServerException` か、あるいは別の形か。実測が必要である。
2. SDK 側の再試行で RFC の MUST が満たされているとみなせるか。SDK の再試行は「同じリクエストの再送」であり、RFC が求めているのは「ポーリング頻度を落とす」ことである。両者は別の概念なので、SDK の再試行だけでは MUST を満たさないと考えるのが妥当だが、実装の重複は避けたい。
3. 判定方法。`errors.As` で `net.Error` を取り出して `Timeout()` を見るか、`errors.Is(err, context.DeadlineExceeded)` や `os.ErrDeadlineExceeded` を見るか。`smithy` の `retry.IsErrorRetryable` を借りるという選択肢もある。トークンエンドポイントの OAuth エラーを誤ってタイムアウトと分類しないことが要件である。
4. 緩め方。RFC は倍加を RECOMMENDED とするが、`slow_down` の固定 5 秒加算と併存させたときに間隔が予期せず伸びないよう、打ち切り期限との関係を整理する必要がある (現在の実装は「次の待機を終える時刻が期限に届くなら打ち切る」判定が間隔の上限を兼ねているため、倍加を入れても待機が期限を超えて伸びることはない)。

これらの確認が済むまでは実装に入らない。

## 調査結果

実装前に、次の実験で point 1 と point 3 を確定させた。

### point 1: 接続タイムアウトが CreateToken からどの型で返るか

実際に接続を受け付けるが応答を返さない TCP リスナーを立て、`http.Client{Timeout: 300 * time.Millisecond}` を設定した `ssooidc.Client` (SDK の再試行は `retry.Standard` の `MaxAttempts: 2`) で `CreateToken` を呼び、SDK の再試行を使い切らせて確認した。エラーチェーンは次のとおり (`%T` で確認)。

```
*smithy.OperationError
  *retry.MaxAttemptsError                                  (github.com/aws/aws-sdk-go-v2/aws/retry)
    *http.ResponseError                                    (github.com/aws/smithy-go/transport/http)
      *http.RequestSendError                               (github.com/aws/smithy-go/transport/http)
        *url.Error
          *http.timeoutError                               (net/http の非公開型)
```

`*http.timeoutError` は `net.Error` を実装し `Timeout() bool` が `true` を返す。`errors.As(err, &v)` (`v interface{ Timeout() bool }`) は `true`、`v.Timeout()` も `true`。

OAuth のエラーレスポンス型 (`AccessDeniedException`、`ExpiredTokenException`、`InternalServerException`、`InvalidGrantException`、`UnauthorizedClientException`。いずれも `aws-sdk-go-v2/service/ssooidc/types/errors.go`) はソースを確認した限りいずれも `Timeout() bool` を実装していない。したがって `interface{ Timeout() bool }` による判定が OAuth のエラーレスポンスを誤って接続タイムアウトの分岐へ吸い込むことはない。

### point 3: 判定方法、および実験で判明した注意点

`errors.As(err, &v)` (`v interface{ Timeout() bool }`) で判定する。これは aws-sdk-go-v2 が内部で使う `retry.TimeouterError.IsErrorTimeout` (`aws/retry/timeout_error.go`) と同じ判定式であり、SDK 自身の分類方法をそのまま再利用する形になる。

実験で追加の事実が判明した。`errors.Is(err, context.DeadlineExceeded)` も `true` を返した。`net/http` の `Client.Timeout` は内部で `context.deadlineExceededError` (これも `Timeout() bool` を実装する) を使って実現されているため、「呼び出し元が渡した `ctx` 自体が期限切れ/キャンセルされた場合」と「SDK 内部の HTTP タイムアウトによる接続タイムアウト」を、返ってきた `err` の型やメッセージだけから区別することはできない。

これは無視できない問題である。`waitForSSOToken` は `main` が `signal.NotifyContext` で作った、シグナル連動の `ctx` を受け取る (直近のコミットで CLI 全体に配線済み)。`err` の中身だけで接続タイムアウトを判定すると、利用者が Ctrl-C で中断したときも `ctx` が終了した結果として `CreateToken` が同じ形のエラーを返しうるため、中断がポーリング頻度を落とす分岐に飲み込まれ、`sso login` の Ctrl-C が効かなくなる回帰を生む。したがって判定は `err` の中身より前に、呼び出し元の `ctx.Err() != nil` を直接見て、`ctx` 自体の終了を優先させる。

### point 2 と point 4 について

point 2 (SDK 再試行だけでは MUST を満たさない) と point 4 (倍加しても有効期限を超えて伸びない) は、本 issue の「設計判断が必要な点」に書かれている分析のとおりで、追加の調査を要しない。point 2 は、SDK の再試行 (`retry.Standard`) が「同一リクエストの再送」であるのに対し、`waitForSSOToken` のポーリングループが使う `interval` (試行間の待機) は別の値であり、SDK の再試行を尽くした後に `waitForSSOToken` 側へ返ってくるため、ここで `interval` を緩めない限り次のポーリングが直前と同じ間隔で再試行され、RFC の MUST を満たさないことによる。point 4 は、既存の打ち切り判定 (`policy.now().Add(interval).Before(deadline)` が偽になったら打ち切る) が間隔の実質的な上限を兼ねる設計になっているため、倍加を追加しても待機が有効期限を超えて伸びないことによる (`slow_down` の固定加算でも同じ役割を果たしている)。

## 修正方針

1. `waitForSSOToken` のループで `CreateToken` がエラーを返した直後、`AuthorizationPendingException` と `SlowDownException` の分岐の後、接続タイムアウトの分類 (point 2 で追加する分岐) と既存の `default` 分岐 (OAuth のエラーレスポンスによる即時失敗) を含むそれ以外の分類より前に `ctx.Err() != nil` を確認し、真であれば即座に `ctx.Err()` を返す。理由は上記調査結果のとおり、SDK 内部の HTTP タイムアウトと呼び出し元のキャンセル/期限切れを `err` の型だけでは区別できないため。ctx が終了した後に届いた err はもはや利用者にとって意味を持たないため、接続タイムアウトかどうかによらず ctx.Err() を優先してよい。
2. 分岐に `case errors.As(err, &timeoutErr) && timeoutErr.Timeout():` を追加する (`timeoutErr` は `interface{ Timeout() bool }`)。該当時は `interval *= 2` する。RFC 8628 §3.5 が MUST とする「接続タイムアウトを受けたらポーリング頻度を自主的に落としてから再試行する」に対応し、倍加は同節が緩め方として RECOMMENDED とする方式である。
3. 既存の `AuthorizationPendingException` / `SlowDownException` の分岐、打ち切り判定 (`policy.now().Add(interval).Before(deadline)`)、待機処理 (`select` ブロック) は変更しない。

検討して採らなかった案。

- `errors.Is(err, context.DeadlineExceeded)` または `errors.Is(err, context.Canceled)` で呼び出し元のキャンセルを判定する案は採らない。上記調査結果のとおり、SDK 内部の HTTP タイムアウトも同じ形で `context.DeadlineExceeded` に一致するため、`err` の中身を見る限り両者を区別できない。`ctx.Err()` は呼び出し元が渡した `ctx` そのものの状態を直接確認できるため、誤判定が起こり得ない。
- 新しい API や外部依存の追加は不要。既存の `aws-sdk-go-v2`/`smithy-go` が提供する型と `errors.As` だけで判定できる。

## 完了条件

- 接続タイムアウトが `CreateToken` からどの型で返るかを実測または SDK のコードで確定させ、issue に記録する。
- 接続タイムアウトを受けたときにポーリング間隔が緩められ、再試行されることをテストで検証する。
- OAuth のエラーレスポンス (`access_denied` / `expired_token` / `invalid_grant` 等) が引き続き即時失敗すること、つまりタイムアウトの分岐に誤って吸われないことをテストで検証する。
- 緩められた間隔が device code の有効期限を超えて伸びないことをテストで検証する。

## 出典

`docs/issues/0129-bug-sso-device-auth-polling-violates-rfc-8628.md` の RFC 8628 準拠レビュー中に判明した。0129 は §3.2 の interval / expires_in と §3.5 のエラーコード 4 種の扱いを対象としており、輸送層の失敗に対する規定は完了条件に含まれていない。RFC の条文は rfc-editor.org の原文で確認済み。

## 解決方法

`backend/internal/aws/sso_oidc.go` の `waitForSSOToken` のエラー分岐に、修正方針の 3 点をそのまま実装した。

- `AuthorizationPendingException` / `SlowDownException` の判定の後、それ以外のエラーを判定する前に `case ctx.Err() != nil:` を追加し、真であれば `ctx.Err()` を返す。
- `case errors.As(err, &timeoutErr) && timeoutErr.Timeout():` (`timeoutErr` は `interface{ Timeout() bool }`) を追加し、該当時は `interval *= 2` する。
- 既存の `AuthorizationPendingException` / `SlowDownException` の分岐、打ち切り判定 (`policy.now().Add(interval).Before(deadline)`)、待機処理 (`select` ブロック) は変更していない。

方針セクションからの乖離は無い。

完了条件の各行への対応は次のとおり。

- 「接続タイムアウトが `CreateToken` からどの型で返るかを実測または SDK のコードで確定させ、issue に記録する」: 「## 調査結果」の point 1 に実測結果 (`*smithy.OperationError` → `*retry.MaxAttemptsError` → `*http.ResponseError` → `*http.RequestSendError` → `*url.Error` → `*http.timeoutError` の連鎖、および `interface{ Timeout() bool }` による判定が成立すること) を記録済み。
- 「接続タイムアウトを受けたときにポーリング間隔が緩められ、再試行されることをテストで検証する」: `sso_oidc_test.go` の `TestWaitForSSOTokenPollIntervals` に追加した `connection timeout doubles the interval` と `interval doubled by connection timeout is kept across pending` の 2 ケースで検証する。前者は `testSSOPollInterval` から 2 回の接続タイムアウトで 2 倍、4 倍と伸びることを、後者は倍加後の間隔がその後の承認待ちでも維持されることを確認する。
- 「OAuth のエラーレスポンス (`access_denied` / `expired_token` / `invalid_grant` 等) が引き続き即時失敗すること、つまりタイムアウトの分岐に誤って吸われないことをテストで検証する」: `TestWaitForSSOTokenFailsImmediatelyOnOtherError` を `AccessDeniedException` / `ExpiredTokenException` / `InvalidGrantException` の 3 ケースを持つテーブル駆動テストに拡張し、いずれも 1 回で即時失敗し待機が発生しないことを確認する。
- 「緩められた間隔が device code の有効期限を超えて伸びないことをテストで検証する」: 新設した `TestWaitForSSOTokenConnectionTimeoutStaysBounded` で、interval 5 秒 / expires_in 600 秒のもとで接続タイムアウトが続いた場合の待機系列が `[10s, 20s, 40s, 80s, 160s]` の 5 回で打ち切りに達すること (6 回目の `CreateToken` で `errSSOTokenTimeout` になること)、および各待機が正かつ device code の猶予以下であることを確認する。

このほか、`ctx.Err()` の優先判定が接続タイムアウトの分類に紛れないことを確認する `TestWaitForSSOTokenConnectionTimeoutDoesNotOverrideContextError` を追加した (既存の `TestWaitForSSOTokenPrefersContextErrorOverTimeout` は打ち切り判定と ctx のキャンセルが競合する、待機に入る前の別のコードパスを検証するテストであり、今回追加した分岐とは別物である)。

bug の再現確認は、修正前のコードで `TestWaitForSSOTokenConnectionTimeoutStaysBounded` 相当の入力 (接続タイムアウトの連続) を与えると `default` 節に落ちて 1 回目の接続タイムアウトで即時失敗することを確認し (症状の再現)、修正後は上記のとおり間隔を倍にしながら device code の有効期限まで再試行することを確認した (非再現)。

テストコマンドは `mise run check` を実行し、backend / frontend とも全テストが通過することを確認した (Step 1 のベースラインからの新たな失敗は無い)。

多観点レビュー (5 観点 × 1 ラウンド) の結果と反映は次のとおり。

- 観点1 (完了条件充足)：指摘なし。
- 観点2 (テストの品質)：中優先度の指摘 2 件を反映した。
  - `errors.As(err, &timeoutErr) && timeoutErr.Timeout()` の `timeoutErr.Timeout()` 側が独立して検証されていない (既存の `fakeSSOConnectionTimeoutError.Timeout()` は常に true を返すため、`&& timeoutErr.Timeout()` を削っても既存テストは通ってしまう) との指摘を受け、`Timeout()` が false を返す `fakeSSONonTimeoutNetError` と `ssoCreateTokenNonTimeoutNetError()` を追加し、`TestWaitForSSOTokenFailsImmediatelyOnOtherError` に「timeout-shaped error whose Timeout() is false」ケースを追加した。
  - `TestWaitForSSOTokenFailsImmediatelyOnOtherError` をテーブル駆動化した際に `errors.As` で具体的な例外型を取り出す検証が失われていた指摘を受け、テーブルに `checkAs func(t *testing.T, err error)` フィールドを追加し、各ケースで対応する型 (`*AccessDeniedException` 等、上記の新ケースでは `interface{ Timeout() bool }`) への `errors.As` を検証するようにした。テーブルの各ケースは `tt.base` から step をその場で組み立てる構造のため、`ssoCreateTokenNonTimeoutNetError()` ヘルパーは使用箇所が無く、`mise run check` の `staticcheck` (U1000) が未使用として検出したため削除した (`fakeSSONonTimeoutNetError` 型自体は `tt.base` の値として使用しているため残す)。
  - 低優先度の指摘 2 件 (単発の接続タイムアウトの後に成功するケース、`slow_down` と接続タイムアウトが交互に来るケース) は、完了条件のいずれの行にも対応せず、既存の `slow_down` 系テストの被覆水準とも整合する範囲であるため、却下した。
- 観点3 (堅牢性)：指摘なし。
- 観点4 (規約準拠)：中優先度の指摘 1 件を反映した。`CHANGES.md` のエントリが複数文に分かれ末尾が否定形 (「...変更しない」) で終わっており、周囲のエントリが採る「主文 1 つ + 括弧内の補足」という書式から外れているとの指摘を受け、主文を「...不具合を修正する」に統一し、残りを括弧内の補足に収める形へ書き直した。
- 観点5 (回帰と整合)：低優先度の指摘 1 件を反映した。`case ctx.Err() != nil:` の優先順位は接続タイムアウトの分岐だけでなく `default` (OAuth のエラーレスポンス) の分岐にも及ぶが、コードコメントと本 issue の修正方針、`CHANGES.md` の記述がいずれも「接続タイムアウトの分類より優先」とのみ書いており、実際の優先範囲を過小に記述していた。挙動そのものは既存の打ち切り判定における ctx 優先の扱い (`TestWaitForSSOTokenPrefersContextErrorOverTimeout` で検証済み) と同じ考え方であり、コードの分岐条件や優先順位は変更していない。`sso_oidc.go` のコメント、本 issue の「## 修正方針」point 1、`CHANGES.md` の該当箇所を、優先範囲が `default` 分岐を含む旨に書き直した。反映後に `go build ./...` と `go test -race -run TestWaitForSSOToken ./internal/aws/... -v` を再実行し、全テストが通過することを確認した (コメント・文書のみの変更であり、テスト結果への影響は無い)。

いずれの観点にも優先度「高」の指摘は無かった。反映した指摘はいずれもコメント・テスト・`CHANGES.md` の記述の追加または修正にとどまり、`waitForSSOToken` の分岐条件・優先順位・待機処理は当初の実装から変更していないため、変更点に限った追加レビューは行わず、上記の反映内容の記録をもって Step 7 を完了とした。
