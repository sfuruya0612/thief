# 0027 AWS / Google Cloud のサイドバーカテゴリを公式プロダクトカテゴリに揃える

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Fable 5 claude-fable-5

## 解決方法

`types/common.ts` の `ServiceGroup` union 型を廃止して `ServiceMeta.group: string` に緩め、新設した `ServiceGroupMeta` 型と `serviceMeta.ts` の `AWS_SERVICE_GROUPS` / `GCP_SERVICE_GROUPS` (key / label、配列順 = 表示順) で公式カテゴリを定義した。
`SERVICES` / `GCP_SERVICES` 各エントリの `group` を issue の表どおりに更新し (cfn → management、ecr/ecs → containers、ssm → management、athena/kinesis → analytics、sqs → integration など)、`Sidebar.tsx` / `GcpSidebar.tsx` の `SECTIONS` 定数はカテゴリ定義 + 各サービスの `group` から導出する形に書き換えた (空カテゴリは非表示)。
`serviceMeta.test.ts` を新設し、全サービスがいずれかの定義済みカテゴリに属すること、導出結果が表どおりのセクション構成になることを固定した。
GCP の Observability セクションは issue 0022 (Cloud Logging) 実装時に追加する。
`mise run frontend:lint` (0 errors) と `mise run frontend:test` (278 件全通過、新規 4 件) で確認済み。

## 背景 / 根拠

サイドバーのサービス分類が、クラウドプロバイダの公式なプロダクトカテゴリと食い違っている。
実例が CloudFormation で、issue 0023 の実装時に Compute セクションへ配置された (`frontend/src/components/Sidebar.tsx:21`) が、AWS の公式カテゴリでは Management & Governance である。
公式カテゴリと違う場所にあるサービスは、AWS / Google Cloud コンソールの分類に慣れた利用者が探すときに見つけられない。

CloudFormation 以外にも食い違いがある (Systems Manager Parameter Store が Security、Athena / Kinesis / S3 が独自分類の Data / Messaging、など)。
本 issue では「適切なカテゴリ」の基準を **AWS / Google Cloud の公式プロダクトカテゴリ** と定義し、AWS / Google Cloud 両方のサイドバーを揃える。

## 現状

カテゴリ定義は 2 箇所で二重管理されている。

- セクション表示: `frontend/src/components/Sidebar.tsx:20` の `SECTIONS` (AWS) と `frontend/src/views/GcpSidebar.tsx:21` の `SECTIONS` (GCP)。`{ label, services[] }` の配列で、表示順もこの配列が決める
- サービスメタ: `frontend/src/lib/serviceMeta.ts` の `SERVICES` / `GCP_SERVICES` 各エントリの `group` フィールド

現在の AWS セクションは Compute / Data / Network / Messaging / Security / Cost の 6 つ、GCP は Compute / Data / Security の 3 つ。

## 対応内容

### AWS のカテゴリマッピング (公式カテゴリ準拠)

| セクション (公式カテゴリ名) | サービス | 現状からの移動 |
| --- | --- | --- |
| Compute | ec2, lambda | 変更なし |
| Containers | ecr, ecs | Compute から移動 |
| Storage | s3 | Data から移動 |
| Database | rds, dynamo, cache | Data から移動 |
| Networking & Content Delivery | elb, cloudfront, apigw, natgw | ラベル変更 (Network) |
| Analytics | athena, kinesis | Data / Messaging から移動 |
| Application Integration | sqs | Messaging から移動 (Messaging セクションは消滅) |
| Security, Identity, & Compliance | iam, waf, secrets | ラベル変更 (Security)。ssm を外す |
| Management & Governance | cfn, ssm | cfn は Compute から、ssm は Security から移動 |
| Cloud Financial Management | costexplorer | ラベル変更 (Cost) |

セクション数は 6 から 10 に増え、1 サービスだけのセクション (Storage / Application Integration / Cloud Financial Management) ができる。
これは公式準拠を正とした帰結として受け入れる (独自の統合分類は今回の食い違いの原因そのものであるため)。
ラベルが長い場合の折り返しは実装時に確認する。

### Google Cloud のカテゴリマッピング (公式カテゴリ準拠)

| セクション | サービス | 現状からの移動 |
| --- | --- | --- |
| Compute | cloudrun | 変更なし (公式大分類は Compute のため維持) |
| Data Analytics | bigquery | Data から移動 |
| Storage | gcs | Data から移動 (Data セクションは消滅) |
| Security & Identity | gcpiam, gcpserviceaccounts | ラベル変更 (Security) |

issue 0022 (Cloud Logging) が実装される際は Observability セクション (Google Cloud の公式カテゴリ名は Observability and monitoring) を追加してそこへ配置する (0022 側の GcpSidebar 変更はこの分類に合流させる)。

### カテゴリ定義の一元化

`SECTIONS` (表示) と `group` (メタ) の二重管理は、今回のような分類変更で片方だけ直る事故のもとになるため、この機会に一元化する。

- `frontend/src/lib/serviceMeta.ts` にカテゴリ定義 (`{ key, label }` の配列。配列順 = 表示順) を新設し、各サービスの `group` にはカテゴリ key を持たせる
- `Sidebar.tsx` / `GcpSidebar.tsx` の `SECTIONS` 定数を廃止し、serviceMeta のカテゴリ定義 + 各サービスの `group` から導出する
- `group` フィールドの既存参照箇所を洗い出し (`serviceMeta.ts` 外での利用の有無)、新しいカテゴリ key に追随させる

## 実装方針

1. `serviceMeta.ts` に `AWS_SERVICE_GROUPS` / `GCP_SERVICE_GROUPS` (key / label / 表示順) を定義し、`SERVICES` / `GCP_SERVICES` の `group` 値を上記マッピングどおりに更新する
2. `Sidebar.tsx` / `GcpSidebar.tsx` を、グループ定義とサービスの `group` からセクションを導出する形に書き換える (空になったセクションは表示しない)
3. サービスとカテゴリの対応は上記の表を正とし、テスト (vitest) で「全サービスがいずれかの定義済みカテゴリに属すること」「導出結果が期待のセクション構成と一致すること」を固定する
4. `group` の他の参照箇所 (facet やフィルタ等に使われていれば) を確認して追随させる

## スコープ外

- サービスの追加や削除 (分類の変更のみ)
- サイドバーの折りたたみやカテゴリ別の開閉 UI
- Datadog / TiDB ビューのナビゲーション (サービスカテゴリの概念がない)

## 検証

- `mise run check` を通す
- AWS / GCP ビューでサイドバーの全サービスが表の分類どおりのセクションに表示され、選択と件数バッジが従来どおり機能することを確認する
