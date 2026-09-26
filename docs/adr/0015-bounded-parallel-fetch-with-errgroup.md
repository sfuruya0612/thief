# 0015. 項目ごとの詳細の取得を errgroup で上限付きの並列にする

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-18

## 状況

いくつかのサービスの一覧は、ID を集めた後に、項目ごとに詳細の API を呼ぶ構造だった。
この呼び出しが直列だったため、一覧の取得時間が件数に比例して伸びた。
Cloud Run では、43 ロケーションのジョブの取得を直列にしていた。
並列にすると、ジョブの取得は 43.4 秒から 4.8 秒に、一覧全体は 44.9 秒から 6.3 秒に縮んだ (issue 0043、2026-07-18)。
同じ構造が AWS の 5 か所 (Kinesis、DynamoDB、SQS、WAF、IAM) に残っていた (issue 0081)。

## 決定

- 項目ごとの詳細の取得は、`errgroup.WithContext` と `SetLimit` で並列にする。
- 結果は事前に確保したスライスの、自分の添字の位置にだけ書く。
  返す順序は元の順序と同じにする。
- 並列度はサービスごとのファイル内の定数にする。
  既定は 30、WAF と IAM は 10 とする (`backend/internal/aws/waf.go` `wafACLConcurrency`、`backend/internal/aws/iam.go` `iamDetailConcurrency`)。
- キャンセルによる失敗は全体の失敗として返し、欠けた結果をキャッシュに書かない (issue 0081、2026-07-26)。
  対象は issue 0081 の 5 か所である。
  S3 のバケットごとの属性の解決 (`backend/internal/aws/s3.go` の `resolveS3Region`、`resolveS3Encryption`、`resolveS3Public`) は、キャンセルを含むエラーを既定値に置き換えて返し、この扱いに揃っていない。

## 検討した代替案

issue 0081 は次の案を採らなかった。

- 全サービスで共通の並列度の設定。
  サービスごとに API のレート制限が異なり、個別に調整できる方が安全なため。
- 共通のヘルパー関数。
  型とエラーの分類がサービスごとに異なり、読みにくくなるため。

## 結果

- 一覧の取得時間は、件数ではなく、件数を並列度で割った回数に比例する。
- 項目が 0 件のとき、JSON の応答が `null` から `[]` に変わった。
  frontend は `apiGetList` で `?? []` として扱う。
- issue 0081 の 5 か所では、無視するエラーを `slog.Warn` で記録し、観測できるようにした。
  S3 の属性の解決はログを出さない。

## 根拠資料

- `docs/issues/closed/0041`、`0043`、`0067`、`0081`
- `backend/internal/aws/kinesis.go`、`dynamo.go`、`sqs.go`、`waf.go`、`iam.go`、`s3.go`
- `backend/internal/gcp/cloudrun.go` `listJobsConcurrency`
