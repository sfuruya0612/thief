# GetSession の aws.Config キャッシュ化の可否を調査する

Created: 2026-07-25
Model: Claude Fable 5
Completed: 2026-09-18

## 背景

docs/issues/0081 (backend の並列化) の調査で見つかった論点の分離であり、TODO.md 由来ではない。
pending の調査 issue に将来の変更のカテゴリを付ける扱いは docs/issues/pending/0059 (feat) の前例に従い、perf とする。

`backend/internal/aws/session.go` の `GetSession` は呼び出しのたびに `config.LoadDefaultConfig` を実行し、`aws.Config` のキャッシュを持たない。
`NewClient` (`backend/internal/aws/client.go`) が必ずここを経由するため、1 リクエストで複数クライアントを作る関数 (例: `ListWAFResources` の REGIONAL / CLOUDFRONT の 2 スコープ) はその回数だけ `~/.aws/config` のパースと認証情報の解決を行う。
docs/issues/closed/0008 は同種の問題を DynamoDB の 1 箇所だけ解消した。
profile と region をキーにした `aws.Config` のキャッシュを入れれば全箇所で解消するが、効果と安全性のどちらも未確認である。

## 目的

`aws.Config` をキャッシュしてよいか (効果があるか、SSO 再ログインを壊さないか) の判断材料を揃える。

## pending にした理由

`aws.Config` をプロセス内にキャッシュすると、SSO 再ログイン後もキャッシュ済みの認証情報プロバイダを掴み続け、古いトークンでリクエストし続ける恐れがある。
これは docs/issues/closed/0030 (SSO ログイン後に期限切れ表示が残る問題) と同じ領域であり、実 SSO 環境での再ログイン検証なしに設計を確定できない。
効果の計測もまだ無いため、実装 issue ではなく調査 issue として保留する。

## 昇格の決着内容 (2026-09-16)

- 論点 1 (効果の計測): docs/issues/closed/0081 は実環境の計測値を採らずに close した (区間計測ログは dynamo / iam / kinesis / sqs / waf の 5 箇所に追加済み)。
  計測を待たずに昇格し、実環境での計測と判定を本 issue の完了条件に含める。計測には利用者の AWS 認証が要るため、実装時に利用者の協力を得て採取する。
- 論点 2 (SSO 再ログインの安全性): `aws.Config` のキャッシュは、当該 profile のリクエストが SSO 期限切れ (`IsSSOTokenExpired` が真になるエラー) を返した時点で破棄する方式とする。
  SDK のプロバイダ挙動の確認と実 SSO 環境での再ログイン検証は行わず、期限切れ検知による無効化で担保する。
- 置き場は API サーバの `Server` とし、CLI はプロセスが短命なため対象外とする (調査タスクの設計論点の結論)。

## 完了条件

- 実環境で 0081 の区間計測ログを採取し、クライアント生成の区間がキャッシュ MISS 時の一覧 API 応答時間に占める割合をサービスごとに本 issue に記録している。
- 割合が 5% 未満なら効果なしと判定し、キャッシュを実装せずその判定を「## 解決方法」に書いて close する。以下の行はこの場合は対象外とする。
- API サーバに profile と region をキーにした `aws.Config` のキャッシュがあり、同一キーの 2 回目以降のクライアント生成で `config.LoadDefaultConfig` が呼ばれない。CLI の経路ではキャッシュしない。
- キャッシュは複数 goroutine からの同時アクセスで競合しない (`go test -race` で確認)。
- `IsSSOTokenExpired` が真になるエラーを返したリクエストの profile について、キャッシュのエントリが破棄され、次の呼び出しで `config.LoadDefaultConfig` がやり直される。
- SSO ログイン完了 (`handleSSOLoginComplete`) とログアウト (`handleSSOLogout`) でも当該 profile のエントリが破棄される。
- キャッシュのヒット、期限切れ検知による破棄、ログイン / ログアウトによる破棄のそれぞれにテストがある。
- キャッシュ導入後に同じ区間計測を採取し、変更前後の値を本 issue に記録している。
- `mise run check` の通過。

## 備考

- グローバル規約により、pending の issue は修正せずそのまま残す (close しない)。調査タスクの実施と結果の追記は修正に当たらない。

## 関連

- docs/issues/0081: 本 issue の分離元。並列化と区間計測 (クライアント生成を含む) を先に行う。
- docs/issues/closed/0008: `LoadDefaultConfig` の重複呼び出しを 1 箇所だけ解消した前例。
- docs/issues/closed/0030: SSO 再ログイン後の状態更新に関する前例。キャッシュ化のリスクの根拠。

## 解決方法

効果なしと判定し、`aws.Config` のキャッシュは実装しない。完了条件の「割合が 5% 未満なら効果なしと判定し、キャッシュを実装せずその判定を『## 解決方法』に書いて close する」に従う。

実環境 (profile `example-common`、region `ap-northeast-1`、2026-09-18) で、各サービスの一覧 API を `refresh=true` で 1 回ずつ呼び、キャッシュ MISS 時の区間計測ログを採取した。分子は `msg="<svc> client created"` の `duration_ms`、分母は同リクエストの `msg="<svc> list all done"` の `duration_ms` (一覧 API 応答時間) とする。

| サービス | client created | list all done | 割合 |
| --- | ---: | ---: | ---: |
| dynamodb | 2 ms | 1170 ms | 0.17% |
| iam | 2 ms | 7827 ms | 0.026% |
| kinesis | 2 ms | 649 ms | 0.31% |
| sqs | 2 ms | 782 ms | 0.26% |
| waf | 4 ms | 1423 ms | 0.28% |

最大でも 0.31% であり、すべて 5% を大きく下回った。`GetSession` の `config.LoadDefaultConfig` はローカルの `~/.aws/config` パースと認証情報の解決だけで完結し、一覧 API の支配的なコスト (IAM の詳細取得 5671 ms、DynamoDB のテーブル一覧 845 ms 等) に対して無視できる。したがって全箇所で解消するキャッシュを導入する価値はないと判断する。

完了条件のうち、キャッシュの実装、`go test -race` による競合確認、SSO 期限切れ・login/logout での破棄、キャッシュ導入後の再計測は、判定が「実装しない」のため対象外とする。あわせて、SSO 再ログインを壊すリスク (「## pending にした理由」) を負う必要も無くなった。
