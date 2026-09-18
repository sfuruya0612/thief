package aws

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// ecsMetricNamespace は LiveTaskCount が発行される CloudWatch の名前空間。
const ecsMetricNamespace = "AWS/ECS"

// ecsLiveTaskCountMetric は ECS のタスク数を表す指標名。
const ecsLiveTaskCountMetric = "LiveTaskCount"

// ecsMetricDataBatchSize は 1 回の GetMetricData に載せるクエリ数の上限。
// CloudWatch の GetMetricData は 1 リクエストあたり 500 件までしか受け付けない。
const ecsMetricDataBatchSize = 500

// ecsTaskCountStatistic は LiveTaskCount を丸める統計値。台数は瞬間値であり、
// 粒度の間の代表値としては平均が素直である。
const ecsTaskCountStatistic = "Average"

// ecsClusterNamePattern は SEARCH 式へ埋め込んでよいクラスタ名。ECS のクラスタ名は
// 英数字とハイフンとアンダースコアだけで構成されるため、これ以外の文字を含む名前は
// 式の構文を壊しうるので対象から外す。
var ecsClusterNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// ListECSTaskCountSeries はクラスタごとのタスク数の時系列を返す。
//
// AWS/ECS 名前空間の LiveTaskCount は ClusterName と ServiceName の組でしか発行されず、
// クラスタ単独のディメンションを持たない。そのためクラスタ内の全サービスの値を
// メトリクス演算の SUM(SEARCH(...)) で合算し、クラスタあたり 1 本の系列にする。
// ECS/ContainerInsights 名前空間の RunningTaskCount は使わない (クラスタごとの
// Container Insights 有効化と追加課金を利用者に要求することになるため)。
//
// LiveTaskCount は ACTIVATING / RUNNING / DEACTIVATING の合計であり、状態別の内訳は持たない。
//
// 時間窓 w は呼び出し側が決める。応答に載せる窓とグリッドの窓を同じ値にするためである。
func ListECSTaskCountSeries(ctx context.Context, profile, region string, r TimeseriesRange, w TimeseriesWindow) ([]TimeseriesSeries, error) {
	ecsClient, err := newECSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	arns, err := listECSClusterArnsWith(ctx, ecsClient)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(arns))
	for _, arn := range arns {
		names = append(names, arnLastSegment(arn))
	}

	cwClient, err := newCloudWatchClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return ecsTaskCountSeries(ctx, cwClient, names, r, w)
}

// ecsTaskCountSeries は生成済みクライアントで時系列を組み立てるコア。
// GetMetricData に載せるクエリを単体テストで固定できるよう、クライアントの生成と分離してある。
func ecsTaskCountSeries(
	ctx context.Context,
	client cloudwatch.GetMetricDataAPIClient,
	clusters []string,
	r TimeseriesRange,
	w TimeseriesWindow,
) ([]TimeseriesSeries, error) {
	targets := make([]string, 0, len(clusters))
	for _, name := range clusters {
		if ecsClusterNamePattern.MatchString(name) {
			targets = append(targets, name)
		}
	}
	sort.Strings(targets)
	if len(targets) == 0 {
		return []TimeseriesSeries{}, nil
	}

	start, end := time.UnixMilli(w.Start), time.UnixMilli(w.End)
	period := r.PeriodSeconds()
	grid := timeseriesGrid(w, period)

	series := make([]TimeseriesSeries, 0, len(targets))
	for i := 0; i < len(targets); i += ecsMetricDataBatchSize {
		batch := targets[i:min(i+ecsMetricDataBatchSize, len(targets))]
		queries := make([]cwtypes.MetricDataQuery, 0, len(batch))
		for j, name := range batch {
			queries = append(queries, cwtypes.MetricDataQuery{
				Id:         aws.String(ecsTaskCountQueryID(j)),
				Expression: aws.String(ecsTaskCountExpression(name, period)),
				Label:      aws.String(name),
				Period:     aws.Int32(period),
			})
		}
		values, err := getMetricDataValues(ctx, client, queries, start, end)
		if err != nil {
			return nil, err
		}
		for j, name := range batch {
			series = append(series, TimeseriesSeries{
				Name:   name,
				Points: metricPointsOnGrid(grid, values[ecsTaskCountQueryID(j)]),
			})
		}
	}
	return series, nil
}

// ecsTaskCountQueryID は GetMetricData のクエリ Id を返す。Id は小文字始まりの
// 英数字でなければならないため、バッチ内の添字に接頭辞を付ける。
func ecsTaskCountQueryID(index int) string {
	return "q" + strconv.Itoa(index)
}

// ecsTaskCountExpression はクラスタ内の全サービスの LiveTaskCount を合算する
// メトリクス演算の式を返す。SEARCH がクラスタ配下のサービスの系列を集め、SUM が
// それらを 1 本へ畳む。どのサービスにも値が無い時刻は結果に現れず、欠測のまま残る。
func ecsTaskCountExpression(cluster string, periodSeconds int32) string {
	return fmt.Sprintf(
		`SUM(SEARCH('{%s,ClusterName,ServiceName} MetricName="%s" ClusterName="%s"', '%s', %d))`,
		ecsMetricNamespace, ecsLiveTaskCountMetric, cluster, ecsTaskCountStatistic, periodSeconds,
	)
}
