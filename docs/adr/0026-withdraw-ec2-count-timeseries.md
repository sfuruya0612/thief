# 0026. EC2 の台数の時系列グラフを撤回する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-09-18

## 状況

EC2 と ECS の台数の推移をグラフで見たいという要望があった (issue 0177)。
ECS は CloudWatch の `AWS/ECS` の `LiveTaskCount` で推移を取れる。
EC2 は次の方式を順に試した (issue 0177、0186)。

1. backend のプロセスの中で台数を記録する (リングバッファ)。
   backend が止まっている間の推移が残らない。
2. 定期的に台数を取得して記録する。
   同じ理由で推移が欠ける。
3. Auto Scaling グループのメトリクス (`GroupInServiceInstances`) を使う。

## 決定

EC2 の台数の時系列グラフを撤回し、実装を削除する (issue 0186 の「撤回 (2026-09-18)」)。
ECS の「Tasks per cluster」のグラフは残す。

## 検討した代替案

- 方式 3 は、Auto Scaling グループのメトリクスがグループごとの有効化 (`EnableMetricsCollection`) を要するオプトインであり、実際のアカウントのグループはいずれも無効で、CloudWatch に発行されていなかったため採らなかった。
  インスタンスを持たないグループではそもそも発行されない。
- `AWS/EC2` の名前空間にも、アカウントやリージョンの単位で稼働台数を表す標準のメトリクスが無い。

## 結果

- 削除したもの: `backend/internal/aws/ec2_metrics.go`、`backend/internal/aws/autoscaling.go`、`/ec2/timeseries` のルート、frontend の EC2 のグラフの設定、`github.com/aws/aws-sdk-go-v2/service/autoscaling` の依存。
- 台数の時系列を表示するのは ECS だけである。

## 根拠資料

- `docs/issues/closed/0177`、`0183`、`0184`、`0185`、`0186`
- `backend/internal/aws/ecs_metrics.go` `ListECSTaskCountSeries`
