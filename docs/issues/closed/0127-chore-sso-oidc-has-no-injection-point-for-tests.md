# SSO OIDC の 3 関数にモックの注入点がなく WaitForSSOToken のリトライロジックがテストされていない

Created: 2026-08-08
Model: Claude Opus 5
Completed: 2026-08-08

## 背景

issue 0125 (`sso_oidc.go` のエラーラップ文言の棚卸し) のレビューで見つかった。

`internal/aws` の他ファイルは、issue 0102 から 0121 の系列で「コンシューマ定義インターフェースを受け取るコア関数への分離」が計画的に進められている。

| ファイル | コア関数 | 受け取るインターフェース |
| --- | --- | --- |
| `ssm.go` | `listSSMOnlineInstanceIDs` | `ssm.DescribeInstanceInformationAPIClient` |
| `sqs.go` | `listSQSResourcesWith` | `sqsQueueListClient` |
| `cfn.go` | `listCFNStacks` / `listCfnStackSummaries` | `cfnListStacksClient` |
| `ec2.go` | `listEC2InstancesWith` | `ec2DescribeInstancesClient` |
| `ecr.go` | `listECRImageInfosWith` | `ecrDescribeImagesClient` |

`sso_oidc.go` はこの系列から漏れている。`RegisterSSOClient` (49 行)、`StartSSODeviceAuthorization` (72 行)、`WaitForSSOToken` (97 行) の 3 関数はいずれも先頭で `newSSOOidcClient(ctx, region)` を呼んで実クライアントを生成し、そのまま SDK 呼び出しへ進む。モックを差し込む注入点が存在しない。

結果として `backend/internal/aws/` に `sso_oidc_test.go` は存在せず、この 3 関数はテストされていない。

## 問題

単なるラップの薄い関数なら未テストでも影響は小さいが、`WaitForSSOToken` はそうではない。デバイス認可フローのポーリングという分岐のあるロジックを持つ。

```go
for i := 0; i < ssoTokenPollMaxAttempts; i++ {
	o, err := client.CreateToken(ctx, input)
	if err == nil {
		return &SSOToken{...}, nil
	}

	var pending *ssooidctypes.AuthorizationPendingException
	var slowDown *ssooidctypes.SlowDownException
	switch {
	case errors.As(err, &slowDown):
		interval *= 2
	case errors.As(err, &pending):
		// ユーザーのブラウザ承認待ち。間隔は変えずに再試行する。
	default:
		return nil, fmt.Errorf("create sso oidc token: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(interval):
	}
}

return nil, fmt.Errorf("timeout waiting for authentication")
```

検証されていない分岐は次のとおり。

- `SlowDownException` を受けたときに待ち間隔が倍になること。AWS のレート制限に対する後退がここでしか実装されていない。
- `AuthorizationPendingException` を受けたときに間隔を変えずに再試行すること。
- 上記以外のエラーで即座に失敗し、再試行しないこと。
- `ssoTokenPollMaxAttempts` (60 回) を使い切ったときに `timeout waiting for authentication` を返すこと。
- `ctx` のキャンセルで `ctx.Err()` を返すこと。

いずれも AWS への実接続なしに検証できるが、注入点がないため書けない。

## 影響

- SSO ログインのポーリング挙動を変更したときに、退行を検出する手段がない。とくに間隔の倍加は、壊しても手元では「なんとなくログインできる」ため気づきにくい。
- `ssoTokenPollMaxAttempts` や `ssoTokenPollInterval` の値を変更したときの実際の待ち時間が、コードを読む以外に確認できない。
- issue 0125 で 3 箇所のエラーラップ文言を変更したが、退行を検出するテストを追加できなかった。同種の変更が今後も検証なしで通る。

## 対応方針

`internal/aws` の他ファイルと同じパターンに揃える。

- `ssooidc.Client` が満たす狭いインターフェース (例: `ssoOidcClient`) を `internal/aws` 側で定義する。必要なメソッドは `RegisterClient` / `StartDeviceAuthorization` / `CreateToken` の 3 つ。関数ごとに分けるか 1 つにまとめるかは実装時に決める。
- 3 関数それぞれを「クライアント生成」と「インターフェースを受け取るコア関数」に分離する。公開関数のシグネチャは変えない。
- 手書きモックを用意し、`WaitForSSOToken` の 5 分岐を検証するテストを追加する。
- 待ち時間の実測でテストが遅くならないよう、`ssoTokenPollInterval` の扱いを検討する。コア関数の引数で受け取る、パッケージ変数にして差し替える、などの案を比較して決める。テストのためだけに本番コードへ複雑さを持ち込まないこと。
- `RegisterSSOClient` / `StartSSODeviceAuthorization` については、SDK へ送る入力 (`ClientName` / `ClientType` / `ClientId` / `ClientSecret` / `StartUrl`) が引数どおりに構築されることを検証する。他ファイルのテストと同じ粒度に揃える。

## 完了条件

- `backend/internal/aws/sso_oidc_test.go` が存在する。
- `WaitForSSOToken` について、`SlowDownException` での間隔倍加、`AuthorizationPendingException` での再試行継続、それ以外のエラーでの即時失敗、最大試行回数超過、`ctx` キャンセルの 5 つが検証されている。
- `RegisterSSOClient` / `StartSSODeviceAuthorization` について、SDK へ送る入力が引数どおりに構築されることが検証されている。
- 3 つの公開関数のシグネチャが変わっていない。
- テストの実行時間が実際のポーリング間隔に引きずられていない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0125: 起票元。`sso_oidc.go` のエラーラップ文言を棚卸ししたが、注入点がないためテストを追加できなかった。
- docs/issues/0126: 同じ SSO ログイン経路の `internal/cli/sso.go` 側の問題。`errors.Is` / `errors.As` の到達性を検証するテストを要求しており、本 issue のコア分離とは独立に実施できる。
- docs/issues/closed/0121: `ssm.go` に同じパターンのコア分離とテストを入れた issue。実装の参考にする。

## 解決方法

### 実装した形

`internal/aws` の既存パターンに揃え、3 関数を「クライアントを生成する公開関数」と「コンシューマ定義インターフェースを受け取るコア関数」に分離した。

| 公開関数 | コア関数 | インターフェース |
| --- | --- | --- |
| `RegisterSSOClient` | `registerSSOClient` | `ssoOidcRegisterClientAPI` |
| `StartSSODeviceAuthorization` | `startSSODeviceAuthorization` | `ssoOidcStartDeviceAuthorizationAPI` |
| `WaitForSSOToken` | `waitForSSOToken` | `ssoOidcCreateTokenAPI` |

インターフェースは 3 つに分けた。1 つにまとめると、register だけを見るテストのモックが使わないメソッドを 2 つ実装する必要があり、「受け取り側はインターフェース」で最小のメソッド集合を要求する原則に反する。同一ファイル内で用途別に分ける前例は `dynamodb.go` / `iam.go` / `ecs.go` / `sqs.go` に揃っている。

命名は `<svc>API` 系にした。`<svc><Op>Client` 系を機械適用すると `ssoOidcRegisterClientClient` になって成立しない。

待ち時間は `ssoTokenPollPolicy` (`interval` / `maxAttempts` / `after`) にまとめ、本番の値は `productionSSOTokenPollPolicy()` が組む形にした。

### 依存の差し替えに構造体ではなく引数を増やさなかった理由

`after func(d time.Duration) <-chan time.Time` を差し替え可能にしたのは 2 つの理由からである。実際に 1 秒ずつ待つとテストが最大試行回数に比例して遅くなる。加えて間隔の倍加は経過時間の実測では計測誤差に左右されるため、要求された間隔そのものを記録して検証する必要がある。

3 つを個別引数にすると `waitForSSOToken` の引数が 8 個になり、呼び出し側で順序を間違えても型が通る組み合わせが生まれる。1 つの構造体にまとめた。

### ゼロ値を本番設定として解釈する設計を一度入れて撤回した

当初は `ssoTokenPollPolicy` のゼロ値を本番の設定として解釈させ (`interval <= 0` なら `ssoTokenPollInterval`、`maxAttempts <= 0` なら `ssoTokenPollMaxAttempts`、`after == nil` なら `time.After`)、公開関数が `ssoTokenPollPolicy{}` を渡す形にした。狙いは「公開関数が `after` を渡し忘れて nil 参照で panic する」変異を設計で消すことだった。

レビュー (Rev0127A 指摘 5) の指摘を受けて撤回した。理由は 3 つある。

- 狙った変異は閉じなかった。公開関数が `ssoTokenPollPolicy{maxAttempts: 1}` のような「妥当だが誤った」値を渡す変異は依然として検出できない。
- 既定値を解決する `if` が 3 つ、12 行増えた。
- テストから `maxAttempts: 0` や `interval: 0` を意図的に渡せなくなった。境界値を表現できないのは損失である。

いまは `productionSSOTokenPollPolicy()` が明示的に値を組み、コアは `policy.interval` / `policy.maxAttempts` / `policy.after` をそのまま読む。

### 測定した検出力

変異を実際に当てて `go test` の生死を測った。計 28 個のうち 26 個が検出された。

| 変異 | 結果 |
| --- | --- |
| SlowDown と AuthorizationPending の条件を入れ替える | 検出 |
| `interval *= 2` を `*= 3` にする / 削除する | 検出 |
| `errors.As` を型アサーションに書き換える (SlowDown / Pending の 2 通り) | 検出 |
| `policy.interval` / `policy.maxAttempts` の代わりに定数を直読みする | 検出 |
| 試行回数の境界を `<=` にする | 検出 |
| `create sso oidc token:` のラップを `%v` にする / 削除する | 検出 |
| `select` から `case <-ctx.Done()` を削る | 検出 |
| `ctx` の判定を待機の前に移す | 検出 |
| 待機自体を削る | 検出 |
| 待機に倍加前の `policy.interval` を渡す | 検出 |
| `timeout waiting for authentication` の文言を変える | 検出 |
| `AccessToken` / `ExpiresIn` を落とす | 検出 |
| `RegisterClientInput` の name/type を入れ替える | 検出 |
| `StartDeviceAuthorizationInput` の id/secret を入れ替える | 検出 |
| `register sso oidc client` / `start sso oidc device authorization` の文言を変える | 検出 |
| `productionSSOTokenPollPolicy` の interval / maxAttempts / after を壊す | 検出 |
| `ssoTokenPollInterval` / `ssoTokenPollMaxAttempts` の定数を変える | 検出 |
| 本番の `after` を duration を無視する実装にする | 検出 |
| `WaitForSSOToken` が誤った policy を渡す | **未検出** |
| `RegisterSSOClient` が引数を入れ替えて渡す | **未検出** |

### 採用したレビュー指摘

- モックが返すエラーを `smithy.OperationError` で 1 段包むようにした (Rev0127A 高 1)。`*ssooidc.Client` は middleware を抜けたエラーを必ずこれで包む (`api_client.go` の `invokeOperation`)。裸の typed exception を返すモックでは、`errors.As` を型アサーションに書き換える変異が通ってしまい、本番では SlowDown と AuthorizationPending の両方が即時失敗の分岐に落ちる。包む深さは 1 段で足りる (`errors.As` は任意段数を辿る)。
- `create sso oidc token: ` のラップ文言を検証するアサーションを追加した (Rev0127A 中 3)。register と start device authorization の 2 つは検証していたが、3 つ目だけ抜けていた。
- 打ち切りまでの総待ち時間を `60 * time.Second` のリテラルで固定した (Rev0127A 中 4)。`ssoTokenPollMaxAttempts * ssoTokenPollInterval` と書くと定数を変えたときに期待値も一緒に動き、何も固定できない。
- ctx キャンセルのテストを「待機に入る瞬間に `after` の中で `cancel()` を呼ぶ」形に変えた (Rev0127B 高 2)。呼び出し前にキャンセルする形では、`select` を「待機に入る前に `ctx.Err()` を見るだけ」に書き換えた実装でもテストが通ることが実測で確認された。それは待機中の Ctrl-C を最大 1 回分の間隔だけ無視する壊れた実装である。`cancel()` は Done チャネルを閉じてから返るため、この形でも非決定性は入らない。`-race` で 30 回連続緑を確認した。
- 本番の `after` が要求された間隔を実際に使っていることを、経過時間の下限で固定した (Rev0127B 中 3)。これが無いと `after` を `time.After(0)` を返す実装に書き換えても通り、本番が 60 回無待機で連打する形に化けても検出できない。上限は実行環境の負荷でフレークするので見ない。
- モック名をインターフェース名に揃えた (Rev0127C 中 2)。既存 17 件がすべて `mock` + インターフェース名であることを確認した。
- `ssoOidcDeviceAuthorizationAPI` を `ssoOidcStartDeviceAuthorizationAPI` に改名した (Rev0127C 低 5)。他 2 つは操作名と完全一致しており、これだけ `Start` が落ちていた。
- `maxAttempts: 5` を `testSSOPollMaxAttempts` として定数化した (Rev0127C 低 7)。
- 本番の doc コメントに書いていた差し替えの詳細な理由をテスト側へ寄せた (Rev0127C 低 8)。3 箇所に同じ説明があり、1 つだけ古くなる形の drift が起きる。
- テーブルから全ケース同値の `interval` / `maxAttempts` 列を削った (Rev0127C 低 9)。
- コミットメッセージの「コア分離し」を「抽出して」に変えた (Rev0127C 低 11)。前者は履歴に前例のない造語で、同種の変更は 56c727c / d71b458 が「抽出して」を使っている。

### 却下したレビュー指摘

- Rev0127B 高 1 と Rev0127C 中 3 と Rev0127B 中 5 の RFC 8628 違反は、本 issue のスコープ外として `docs/issues/0129` に切り出した。挙動を変えない変更である本 issue で直すことはできない。ただし「テスト済み」という誤った信用が残らないよう、`TestWaitForSSOTokenPollIntervals` の doc コメントに「固定しているのは連続 2 回までの倍加であり、上限の不在とオーバーフローは意図的に固定していない」と明記し、本番コードの SlowDown 分岐にも RFC 8628 §3.5 からの逸脱であることをコメントで残した。
- Rev0127A 高 1 のうち「インターフェースで受ける形に統一せよ」は、行数を測って却下した。本 issue の対象ではなく issue 0126 側の `ssoTokenDeps` に対する指摘だが、同じ判断が本 issue にも当てはまる。
- Rev0127C 低 5 のうち「doc コメントを `<svc>API` 系の文面に寄せよ」は却下した。`API` 系の定型文は「SDK クライアントのうち本パッケージが利用する操作の集合」であり、1 メソッドのインターフェースには当てはまらない。現行の「〜の呼び出しを抽象化する。テストではモックを差し込み、実行時は `*ssooidc.Client` がこれを満たす」は事実として正確である。
- Rev0127C 低 10 の `t.Helper()` は不要と判断した。追加したヘルパーは `*testing.T` を受け取らない値生成ファクトリであり、失敗を報告しない。

### このテストの限界

- 公開関数 `RegisterSSOClient` / `StartSSODeviceAuthorization` / `WaitForSSOToken` の 3 つは検証していない。`newSSOOidcClient` が資格情報の解決を伴うためテストから実行できない。結果として「公開関数がコア関数へ誤った引数や誤った policy を渡す」変異は検出できない。`internal/aws` の公開ラッパーはどれも同じ構造で未テストであり (`ecr.go` の `ListECRImageInfos` 等)、ここだけ例外にはしていない。
- SlowDown の倍加は連続 2 回までしか固定していない。上限の不在と 34 回でのオーバーフローは `docs/issues/0129` の対象である。
- 打ち切りを返す直前に 1 回分の待機が入る現行の挙動を 2 箇所で固定している。この挙動は望ましくないが、変えると挙動が変わるため `docs/issues/0129` に含めた。
- SDK のエラーの入れ子は本番では `OperationError` の下に `ResponseError` を挟む形になるが、モックは 1 段しか包んでいない。`errors.As` は任意段数を辿るため検出力には影響しない。

### 実際の変更

- `backend/internal/aws/sso_oidc.go`: 3 つのインターフェース定義、3 つのコア関数への分離、`ssoTokenPollPolicy` と `productionSSOTokenPollPolicy` の追加。挙動は変えていない。
- `backend/internal/aws/sso_oidc_test.go`: 新規。9 個のテスト関数。
- `CHANGES.md`: `### misc` に 1 エントリ追加。
