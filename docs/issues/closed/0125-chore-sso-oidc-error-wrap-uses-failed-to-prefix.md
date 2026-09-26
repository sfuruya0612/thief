# internal/aws/sso_oidc.go のエラーラップ文言から failed to を除く

Created: 2026-08-08
Completed: 2026-08-08
Model: Claude Opus 5

## 背景

issue 0123 (SSM のエラーラップ文言に対象リソース名を含める) のレビューで見つかった。

0123 で `backend/internal/aws/ssm.go` のエラーラップ文言を棚卸しした結果、`internal/aws` で "failed to" 形式の冗長な接頭辞が残っているのは `sso_oidc.go` の 3 箇所だけになった。

```
sso_oidc.go:60   fmt.Errorf("failed to register client: %w", err)
sso_oidc.go:84   fmt.Errorf("failed to start device authorization: %w", err)
sso_oidc.go:128  fmt.Errorf("token creation failed: %w", err)
```

`internal/aws` の他ファイルは「動詞 + サービス名 + リソース名」の形式で統一されている。

- `ec2.go`: `describe ec2 instances: %w`
- `cfn.go`: `list cfn stacks: %w`
- `rds.go`: `describe rds instances: %w`
- `iam.go`: `list iam users: %w`
- `sqs.go`: `list sqs queues: %w`
- `ssm.go`: `describe ssm instance information: %w` (0123 で変更)

Go では、エラー文言はラップのたびに `: ` で連結されて 1 本の文字列になる。"failed to" のような接頭辞は連結後に「failed to A: failed to B: 元のエラー」という形になり、情報を増やさずに長さだけを増やす。標準ライブラリおよび Go のエラー文言の慣習でも、接頭辞は何をしようとしたかを示す動詞句のみとするのが一般的である。

issue 0123 は対応方針で対象を `ssm.go` に限定していたため、`sso_oidc.go` はスコープ外として手を付けなかった。0123 のレビューでは 3 名のレビュアーが独立にこの残存を指摘し、いずれも「0123 のスコープ外であり別途の棚卸しが妥当」と判断している。

## 対応方針

- 3 箇所のラップ文言を `internal/aws` の他ファイルと同じ形式に揃える。`sso-oidc` は AWS SSO OIDC の API を叩いているため、対応する API 名を動詞に反映する。
  - `sso_oidc.go:60` は `RegisterClient` API に対応する。`register sso oidc client: %w` を候補とする。
  - `sso_oidc.go:84` は `StartDeviceAuthorization` API に対応する。`start sso oidc device authorization: %w` を候補とする。
  - `sso_oidc.go:128` は `CreateToken` API に対応する。`create sso oidc token: %w` を候補とする。
- `%w` によるラップは維持する。`errors.Is` / `errors.As` の到達性を変えない。
- `sso_oidc.go:138` の `timeout waiting for authentication` はラップを伴わない自前のエラーであり、"failed to" 形式でもないため対象外とする。
- `backend/internal/aws/errors.go:40` の `"failed to refresh cached credentials"` は AWS SDK が返すエラー本文に対する部分一致判定用の文字列リテラルであり、自前のラップ文言ではない。変更すると判定が壊れるため対象外とする。
- 文言を文字列比較している箇所がないことを、backend と frontend の両方で確認してから変更する。

## 完了条件

- `internal/aws` に `fmt.Errorf("failed to` 形式のラップが残っていない。
- `sso_oidc.go` の 3 箇所が、対応する API を示す動詞句を含む文言になっている。
- `%w` によるラップが維持されている。
- 変更した文言を文字列比較しているコードやテストが存在しない (存在する場合は併せて更新されている)。
- `mise run check` が通る。

## 解決方法

`backend/internal/aws/sso_oidc.go` の 3 行を、対応方針が挙げた候補文言のとおりに変更した。他ファイルへの変更はない。

| 行 | 変更前 | 変更後 | 対応する API |
| --- | --- | --- | --- |
| 60 | `failed to register client: %w` | `register sso oidc client: %w` | `RegisterClient` |
| 84 | `failed to start device authorization: %w` | `start sso oidc device authorization: %w` | `StartDeviceAuthorization` |
| 128 | `token creation failed: %w` | `create sso oidc token: %w` | `CreateToken` |

`grep -rn 'fmt.Errorf("failed to' internal/aws/` は 0 件になり、`internal/aws` から "failed to" 形式のラップが無くなった。

### サービス名を sso ではなく sso oidc とした

同じファイル内の既存文言 `list sso accounts` (161 行) と `list sso account roles` (188 行) はサービス名を `sso` としているが、今回の 3 箇所は `sso oidc` とした。前者は `github.com/aws/aws-sdk-go-v2/service/sso`、後者は `github.com/aws/aws-sdk-go-v2/service/ssooidc` と、叩いている SDK パッケージが異なるためである。書き分けが実態と対応している。

### テストを追加しなかった

issue 0123 では同種の文言変更に対して `strings.Contains(err.Error(), "ssm instance information")` の部分一致検証を追加したが、今回は追加していない。

0123 でそれが書けたのは、その前の issue 0121 が `ListSSMOnlineInstanceIDs` を「クライアント生成」と「`ssm.DescribeInstanceInformationAPIClient` を受け取るコア関数」に分離済みだったからである。0123 は既存のテスト関数に 1 行足しただけだった。

`sso_oidc.go` の 3 関数はいずれも先頭で `newSSOOidcClient(ctx, region)` を呼んで実クライアントを生成しており、モックの注入点が無い。文言変更のためだけに 3 関数分のインターフェース新設・コア分離・手書きモックをゼロから作るのは、この chore の範囲を超える。

代わりに issue 0127 を起票し、`sso_oidc.go` のコア分離と `WaitForSSOToken` のリトライロジック (`SlowDownException` での間隔倍加など) の振る舞いテストを独立した作業として扱うことにした。

### 実際にユーザーへ表示される文字列の変化

レビューで、この文言が 3 層に包まれてから表示されることが分かった。`getSSOToken` (`internal/cli/sso.go`) が `%v` で再ラップし、その呼び出し元の `ssoLogin` / `ssoGenerateConfig` が `get token: %w` でさらに包み、Cobra が `Error: ` を前置する (`SilenceErrors` は未設定)。

```
変更前: Error: get token: failed to register client: failed to register client: <sdk error>
変更後: Error: get token: failed to register client: register sso oidc client: <sdk error>
```

一字一句同じ文言が 2 回続く状態は解消された。中間層の重複自体は `internal/cli` 側の問題であり、issue 0126 で扱う。層の数も文字数も変わっていないため、本変更が 0126 の問題を悪化させてはいない。

### CHANGES.md の分類根拠

`### misc` ではなく `[UPDATE]` とした。ただし当初の根拠 (「CLI 出力と API レスポンスの `message` に現れるため」) は誤りで、レビューで訂正された。

`internal/api/handlers_sso.go` の `handleSSOLogin` は外部の `aws sso login` バイナリを `exec.CommandContext` で起動しているだけで、`sso_oidc.go` を一切経由しない。この 3 関数の呼び出し元は `internal/cli/sso.go` の `getSSOToken` のみであり、HTTP API のレスポンスにこの文言は現れない。

正しい根拠は「CLI の標準エラー出力に現れる observable な文字列であるため」である。分類そのものは変わらない。

### 対象外とした 2 箇所

- `sso_oidc.go:138` の `timeout waiting for authentication`: ラップを伴わない自前のエラーで、"failed to" 形式でもない。
- `internal/aws/errors.go:40` の `"failed to refresh cached credentials"`: AWS SDK が返すエラー本文に対する部分一致判定用のリテラル。変更すると `IsSSOTokenExpired` の判定が壊れる。

### スコープ外としたもの

`internal/cli/sso.go` の `getSSOToken` が 4 箇所すべてで `%v` を使いエラーチェーンを切っている問題と、2 箇所で呼び出し先と同じ文言を重ねている問題は issue 0126 として起票済み。本 issue は対応方針で対象を `internal/aws` に限定しているため手を付けていない。

## 関連

- docs/issues/closed/0123: 起票元。`ssm.go` の同種の棚卸しを行った issue。
- docs/issues/0126: `internal/cli/sso.go` 側の `%v` と二重ラップ。本 issue の調査中に発見して起票した。
- docs/issues/0127: `sso_oidc.go` にモックの注入点が無くテストが書けない問題。本 issue のレビューで指摘されて起票した。
