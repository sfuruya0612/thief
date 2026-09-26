# SSM のページネータのエラーラップ文言に対象リソース名を含める

Created: 2026-08-08
Model: Claude Opus 5
Completed: 2026-08-08

## 背景

issue 0121 (SSM のフィルタ検証テスト整備) のレビューで見つかった。

`backend/internal/aws/ssm.go` の `listSSMOnlineInstanceIDs` は、ページ取得の失敗を次のようにラップする。

```go
return nil, fmt.Errorf("failed to get next page: %w", err)
```

`internal/` 全体を検索すると、この `failed to get next page` という文言はこの 1 箇所にしか存在しない。同種のページネータのエラーラップは、いずれも何の取得に失敗したかを文言に含めている。

- `ec2.go`: `describe ec2 instances: %w`
- `cfn.go`: `list cfn stacks: %w`
- `rds.go`: `describe rds instances: %w`
- `iam.go`: `list iam users: %w`
- `sqs.go`: `list sqs queues: %w`

現状の文言では、ログに出たときに SSM のインスタンス情報取得で失敗したことが判別できない。`ListSSMOnlineInstanceIDs` は `internal/cli/ec2.go` の EC2 一覧表示から呼ばれ、同じ処理の中で EC2 と SSM の 2 系統の API を叩くため、どちらの取得で失敗したかの区別が特に必要になる。

issue 0121 の時点でこの行は既存コードのまま無変更であり、0121 は「テストを追加し挙動は変えない」という範囲だったため、そこでは修正していない。

## 対応方針

- `listSSMOnlineInstanceIDs` のラップ文言を、他ファイルと同じ「何を取得しようとしたか」を含む形に変更する。`describe ssm instance information: %w` を候補とする。
- `%w` によるラップは維持する。`errors.Is` / `errors.As` の到達性を変えない。
- 既存の `ssm_test.go` の `TestListSSMOnlineInstanceIDsPropagatesPageError` は `errors.Is` でラップの維持のみを検証し、文言を固定していないため、この変更でテストの書き換えは不要である。
- `ssm.go` の他の関数のエラーラップ文言もこの機会に棚卸しし、対象名を含まないものがあれば併せて揃える。

## 完了条件

- `internal/` に `failed to get next page` という文言が残っていない。
- `ssm.go` のページネータのエラーラップが、対象リソースを含む文言になっている。
- `%w` によるラップが維持されている。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0121: 起票元。この行を無変更のまま残した理由もそこに記載してある。

## 解決方法

`backend/internal/aws/ssm.go` の `fmt.Errorf` 全 7 箇所を棚卸しし、"failed to" 形式で対象を示さなかった 4 行を書き換えた。

| 行 | 変更前 | 変更後 |
| --- | --- | --- |
| 137 | `failed to describe parameters: %w` | `describe ssm parameters: %w` |
| 169 | `failed to get parameter %s: %w` | `get ssm parameter %s: %w` |
| 174 | `failed to get parameter %s: empty response` | `get ssm parameter %s: empty response` |
| 216 | `failed to get next page: %w` | `describe ssm instance information: %w` |

残る 3 箇所 (43 行の `describe ssm parameters`、64 行の `get ssm parameter %s`、256 行の `put ssm parameter %s`) は元から対象名を含んでいたため変更していない。結果として `ssm.go` の 7 箇所すべてが「動詞 + サービス名 + リソース名」の形式になり、`internal/` に `failed to get next page` は残っていない。動詞は対応する AWS API 名 (`DescribeParameters` / `GetParameter` / `DescribeInstanceInformation` / `PutParameter`) から取っている。

`%w` によるラップは全箇所で維持しており、`errors.Is` / `errors.As` の到達性は変わらない。変更した文言を文字列比較しているコードとテストが backend / frontend の双方に存在しないことを確認した。`internal/api/errors.go` の `writeAWSError` は smithy のエラーコードと AWS SDK 側のメッセージを見ており、自前のラップ接頭辞には依存していない。

### 退行を検出するテストを追加した

レビューの指摘を受けて `ssm_test.go` の `TestListSSMOnlineInstanceIDsPropagatesPageError` に文言の部分一致検証を追加した。

issue 0121 はこのテストで `errors.Is` によるラップの維持だけを検証し、文言を意図的に固定しなかった。本 issue が文言を変更する予定だったためである。しかし `errors.Is` は `Unwrap()` チェーンだけを辿るため、`%w` を保っている限り文言をどう書き換えてもこのテストは通り続ける。文言を確定させた本 issue の時点でその判断をそのまま引き継ぐと、対象名がまた落ちても検出できない状態が残る。issue の目的そのものと矛盾するため、対応方針の「テストの書き換えは不要」を「既存アサーションの書き換えは不要、ただし検証は追加する」に改めた。

完全一致では固定せず `strings.Contains(err.Error(), "ssm instance information")` としている。同じパッケージの `cloudwatchlogs_test.go` が `strings.Contains(err.Error(), "live tail stream:")` で同型の検証をしており、`internal/` 全体でもエラー文言の検証は部分一致が主流で、完全一致で固定している例は CLI がユーザーへ直接表示する自前メッセージの 1 件だけである。この形なら動詞や語順を変えても壊れず、対象を示す語が落ちたときだけ落ちる。

### 測定した検出力

| 壊した箇所 | 結果 |
| --- | --- |
| 旧文言 `failed to get next page` に戻す | 失敗する |
| `ssm` の語だけ落とす | 失敗する |
| エラーのラップを `%w` から `%v` に変更 | 失敗する |
| エラー時に取得済みの `ids` を返すよう変更 | 失敗する |

確認後、実装は元に戻し `git diff` で復元を確認した。

### スコープ外としたもの

`internal/aws/sso_oidc.go` に "failed to" 形式のラップが 3 箇所残っている。本 issue の対応方針が `ssm.go` に限定していたためスコープ外とし、issue 0125 として起票した。`internal/aws/errors.go` の `"failed to refresh cached credentials"` は AWS SDK が返すエラー本文への部分一致判定用の文字列であり、自前のラップ文言ではないため対象外とした。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` に `[UPDATE]` として記載済み。
