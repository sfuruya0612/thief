package aws

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// autoscalingMetricNamespace は GroupInServiceInstances が発行される CloudWatch の名前空間。
const autoscalingMetricNamespace = "AWS/AutoScaling"

// autoscalingInServiceInstancesMetric は Auto Scaling グループの InService 台数を表す指標名。
const autoscalingInServiceInstancesMetric = "GroupInServiceInstances"

// ec2MetricDataBatchSize は 1 回の GetMetricData に載せるクエリ数の上限。
// CloudWatch の GetMetricData は 1 リクエストあたり 500 件までしか受け付けない。
const ec2MetricDataBatchSize = 500

// ec2InstanceCountStatistic は台数を丸める統計値。台数は瞬間値であり、
// 粒度の間の代表値としては平均が素直である。
const ec2InstanceCountStatistic = "Average"

// ListEC2InstanceCountSeries は Auto Scaling グループごとの InService インスタンス数の
// 時系列を返す。
//
// AWS/EC2 名前空間にはアカウントやリージョン単位の Running インスタンス数を表す標準
// メトリクスが無い (issues/closed/0177)。代わりに AWS/AutoScaling 名前空間の
// GroupInServiceInstances をグループ単位で取得する。したがって Auto Scaling グループに
// 属さないインスタンスは含まれない。
//
// 時間窓 w は呼び出し側が決める。応答に載せる窓とグリッドの窓を同じ値にするためである。
func ListEC2InstanceCountSeries(ctx context.Context, profile, region string, r TimeseriesRange, w TimeseriesWindow) ([]TimeseriesSeries, error) {
	names, err := ListAutoScalingGroupNames(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	cwClient, err := newCloudWatchClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return ec2InstanceCountSeries(ctx, cwClient, names, r, w)
}

// ec2InstanceCountSeries は生成済みクライアントで時系列を組み立てるコア。
// GetMetricData に載せるクエリを単体テストで固定できるよう、クライアントの生成と分離してある。
func ec2InstanceCountSeries(
	ctx context.Context,
	client cloudwatch.GetMetricDataAPIClient,
	groups []string,
	r TimeseriesRange,
	w TimeseriesWindow,
) ([]TimeseriesSeries, error) {
	targets := append([]string(nil), groups...)
	sort.Strings(targets)
	if len(targets) == 0 {
		return []TimeseriesSeries{}, nil
	}

	start, end := time.UnixMilli(w.Start), time.UnixMilli(w.End)
	period := r.PeriodSeconds()
	grid := timeseriesGrid(w, period)

	series := make([]TimeseriesSeries, 0, len(targets))
	for i := 0; i < len(targets); i += ec2MetricDataBatchSize {
		batch := targets[i:min(i+ec2MetricDataBatchSize, len(targets))]
		queries := make([]cwtypes.MetricDataQuery, 0, len(batch))
		for j, name := range batch {
			queries = append(queries, cwtypes.MetricDataQuery{
				Id:    aws.String(ec2InstanceCountQueryID(j)),
				Label: aws.String(name),
				// GroupInServiceInstances はグループ単独のディメンションを持つため、
				// SEARCH 式ではなく MetricStat で直接引く。ディメンションを構造体で渡す
				// ので、名前に式の構文を壊す文字があっても注入にならない。
				MetricStat: &cwtypes.MetricStat{
					Metric: &cwtypes.Metric{
						Namespace:  aws.String(autoscalingMetricNamespace),
						MetricName: aws.String(autoscalingInServiceInstancesMetric),
						Dimensions: []cwtypes.Dimension{
							{Name: aws.String("AutoScalingGroupName"), Value: aws.String(name)},
						},
					},
					Period: aws.Int32(period),
					Stat:   aws.String(ec2InstanceCountStatistic),
				},
			})
		}
		values, err := getMetricDataValues(ctx, client, queries, start, end)
		if err != nil {
			return nil, err
		}
		for j, name := range batch {
			series = append(series, TimeseriesSeries{
				Name:   name,
				Points: metricPointsOnGrid(grid, values[ec2InstanceCountQueryID(j)]),
			})
		}
	}
	return series, nil
}

// ec2InstanceCountQueryID は GetMetricData のクエリ Id を返す。Id は小文字始まりの
// 英数字でなければならないため、バッチ内の添字に接頭辞を付ける。
func ec2InstanceCountQueryID(index int) string {
	return "q" + strconv.Itoa(index)
}
