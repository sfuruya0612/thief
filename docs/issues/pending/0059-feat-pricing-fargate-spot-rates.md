# Fargate Spot の単価表示の可否を調査する

Created: 2026-07-21
Model: Claude Opus 4.8

## 背景

Pricing 画面に Spot 単価を追加する要望のうち、EC2 Spot は issue 0056 で独立サービスとして実装する。
一方 Fargate Spot は、単価を返す公開 API が確認できていないため本 issue へ切り出す。
当初の要望は EC2 と Fargate の両方の Spot 単価表示であった。

## pending にした理由

Fargate Spot の単価を返す AWS の公開 API またはデータソースが、起票時点で確認できていない。

- Price List の AmazonECS は On-Demand の Fargate レートのみを返す。
- `ec2:DescribeSpotPriceHistory` は EC2 インスタンスの Spot 価格を返し、Fargate を対象にしない。

正規のデータソースが無ければ実装できない。
割引率の固定値を埋め込む案は、AWS が公開する式に基づかず不正確かつ保守困難になるため採らない。
データソースの有無という外部依存が未確定のため、着手前の調査が必要な pending とする。

## 調査タスク

- Fargate Spot の単価を返す AWS API またはデータソースの有無を、実 AWS で確認する。
- 無い場合、Fargate Spot は正規のデータソースが提供されるまで対応不可とし、UI 上も EC2 Spot のみを扱う。
- 有る場合、本 issue を `issues/` へ戻し、issue 0056 と同じ独立サービス (別キャッシュキー) とライブ取得の方針で Fargate Spot を実装する。

## 調査結果 (2026-08-05)

Price List の公開バルクデータを確認し、Fargate Spot の単価を返すデータソースが現時点でも存在しないことを確認した。

- オファー一覧 (`offers/v1.0/aws/index.json`) に Fargate 単独のオファーは無く、Fargate のレートを含むオファーは AmazonECS のみである。
- AmazonECS の us-east-1 と ap-northeast-1 のオファーファイルには `spot` の語を含む製品も料金区分も無く、料金区分は `OnDemand` のみである。
- 確認は認証情報を使わない公開 HTTP のバルクデータで行った。`pricing:GetProducts` (Query API) はこのバルクデータと同一のオファー内容を返すため、判定はこの確認で足りる。

調査タスクの判定は「無い場合」に該当する。
Fargate Spot は正規のデータソースが提供されるまで対応不可とし、UI 上も EC2 Spot のみを扱う現状を維持して pending に残す。

## 備考

pending の issue は修正せずそのまま残す。
調査で方針が確定した時点で `issues/` へ戻す。

## 再調査結果 (2026-09-25)

2026-08-05 と同じ方法 (認証情報を使わない公開 HTTP のバルクデータ) で再確認し、Fargate Spot の単価を返すデータソースが引き続き存在しないことを確認した。

- オファー一覧 (`offers/v1.0/aws/index.json`、`publicationDate` は 2026-09-25T00:03:13Z) で Fargate または ECS を名前に含むオファーは AmazonECS のみである。
- AmazonECS の us-east-1 と ap-northeast-1 のオファーファイル (`publicationDate` はどちらも 2026-09-11T12:44:25Z) の料金区分は `OnDemand` のみで、`spot` の語を含む製品は 0 件である。Fargate の使用タイプは `Fargate-vCPU-Hours:perCPU`、`Fargate-GB-Hours`、`Fargate-ARM-*`、`Fargate-Windows-*`、`Fargate-EphemeralStorage-GB-Hours` の On-Demand 系だけである。

調査タスクの判定は引き続き「無い場合」に該当するため、pending に残す。
