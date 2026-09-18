package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// MetricPoint は時系列の 1 点。
type MetricPoint struct {
	// T はエポックミリ秒。
	T int64 `json:"t"`
	// V は値。欠測は null のまま返す。0 に潰すと「値が 0 だった」と読めてしまう。
	V *float64 `json:"v"`
}

// TimeseriesRange は時系列エンドポイントが受け取る期間。
// 定数以外の値は ParseTimeseriesRange が弾くため、Duration と PeriodSeconds は
// 想定外の値を Range1Day として扱う。
type TimeseriesRange string

const (
	// Range1Day は直近 1 日。粒度は 60 秒 (CloudWatch の 1 分粒度の保持期間は 15 日)。
	Range1Day TimeseriesRange = "1d"
	// Range7Days は直近 7 日。粒度は 300 秒 (5 分粒度の保持期間は 63 日)。
	Range7Days TimeseriesRange = "7d"
	// Range30Days は直近 1 か月。粒度は 3600 秒。
	Range30Days TimeseriesRange = "30d"
)

// ErrInvalidTimeseriesRange は期間の指定が 1d / 7d / 30d のいずれでもないことを表す。
var ErrInvalidTimeseriesRange = errors.New("range must be one of 1d, 7d, 30d")

// ParseTimeseriesRange は文字列を TimeseriesRange に解釈する。
func ParseTimeseriesRange(s string) (TimeseriesRange, error) {
	switch TimeseriesRange(s) {
	case Range1Day:
		return Range1Day, nil
	case Range7Days:
		return Range7Days, nil
	case Range30Days:
		return Range30Days, nil
	default:
		return "", fmt.Errorf("parse timeseries range %q: %w", s, ErrInvalidTimeseriesRange)
	}
}

// Duration は期間の長さを返す。1 か月は 30 日とする。
func (r TimeseriesRange) Duration() time.Duration {
	switch r {
	case Range7Days:
		return 7 * 24 * time.Hour
	case Range30Days:
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// PeriodSeconds は 1 点あたりの粒度を秒で返す。CloudWatch の保持期間 (1 分粒度は 15 日、
// 5 分粒度は 63 日) に収まるよう、期間が長いほど粗い粒度にする。
func (r TimeseriesRange) PeriodSeconds() int32 {
	switch r {
	case Range7Days:
		return 300
	case Range30Days:
		return 3600
	default:
		return 60
	}
}

// Window は now を粒度で切り下げた終端と、そこから Duration だけ遡った開始時刻を返す。
// 切り下げるのは、同じ粒度の間は同じ時間窓になり、キャッシュキーが 1 リクエストごとに
// 変わってキャッシュが一切効かなくなるのを防ぐためである。
func (r TimeseriesRange) Window(now time.Time) (start, end time.Time) {
	period := int64(r.PeriodSeconds())
	end = time.Unix(now.Unix()/period*period, 0).UTC()
	return end.Add(-r.Duration()), end
}

// TimeseriesWindow は時系列が覆う時間窓をエポックミリ秒 (MetricPoint.T と同じ単位) で表す。
// 点の絞り込みやグリッドの境界と、応答に載せて frontend が X 軸の範囲に使う値を同じ値に
// するために、窓を 1 つの値として持ち回る。
type TimeseriesWindow struct {
	// Start は窓の開始。この時刻を含む。
	Start int64
	// End は窓の終端。グリッドはこの時刻を含まない。
	End int64
}

// NewTimeseriesWindow は時刻の組をエポックミリ秒の窓にする。
func NewTimeseriesWindow(start, end time.Time) TimeseriesWindow {
	return TimeseriesWindow{Start: start.UnixMilli(), End: end.UnixMilli()}
}

// timeseriesGrid は窓の開始から終端の手前までを粒度間隔で並べた時刻をエポックミリ秒で返す。
// CloudWatch はデータ点の無い時刻を応答に含めないため、欠測を null として返すには
// 応答とは独立にグリッドを組む必要がある。
func timeseriesGrid(w TimeseriesWindow, periodSeconds int32) []int64 {
	if periodSeconds <= 0 || w.Start >= w.End {
		return nil
	}
	step := int64(periodSeconds) * 1000
	grid := make([]int64, 0, (w.End-w.Start)/step)
	for t := w.Start; t < w.End; t += step {
		grid = append(grid, t)
	}
	return grid
}

// metricPointsOnGrid は grid の各時刻に values の値を並べる。値の無い時刻は V を nil にする。
func metricPointsOnGrid(grid []int64, values map[int64]float64) []MetricPoint {
	points := make([]MetricPoint, 0, len(grid))
	for _, t := range grid {
		point := MetricPoint{T: t}
		if v, ok := values[t]; ok {
			point.V = &v
		}
		points = append(points, point)
	}
	return points
}

// TimeseriesSeries は凡例名を持つ 1 本の時系列。
type TimeseriesSeries struct {
	Name   string        `json:"name"`
	Points []MetricPoint `json:"points"`
}

// TimeseriesResponse は時系列エンドポイントの応答。粒度を添えるのは、点の間隔を
// フロント側が知らないと欠測と「取得していない区間」を区別できないためである。
//
// Start と End は系列が覆う時間窓をエポックミリ秒 (MetricPoint.T と同じ単位) で表す。
// 窓を決めているのは backend なので、frontend が期間から窓を計算し直さずに済むよう
// 応答に載せる。点の範囲から X 軸を決めると、点が少ない系列 (EC2 の台数) では期間を
// 切り替えても軸の範囲が変わらない。
type TimeseriesResponse struct {
	Range         string             `json:"range"`
	PeriodSeconds int32              `json:"period_seconds"`
	Start         int64              `json:"start"`
	End           int64              `json:"end"`
	Series        []TimeseriesSeries `json:"series"`
}

// newCloudWatchClient は CloudWatch (メトリクス) のクライアントを生成する。
// ログ取得の cloudwatchlogs とは別サービスのクライアントである。
func newCloudWatchClient(ctx context.Context, profile, region string) (*cloudwatch.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *cloudwatch.Client {
		return cloudwatch.NewFromConfig(cfg)
	})
}

// getMetricDataValues は GetMetricData を実行し、クエリ Id ごとに
// 「エポックミリ秒 -> 値」のマップを返す。
// CloudWatch はデータ点の無い時刻を応答に含めないため、欠測はマップに現れない。
func getMetricDataValues(
	ctx context.Context,
	client cloudwatch.GetMetricDataAPIClient,
	queries []cwtypes.MetricDataQuery,
	start, end time.Time,
) (map[string]map[int64]float64, error) {
	values := make(map[string]map[int64]float64, len(queries))
	if len(queries) == 0 {
		return values, nil
	}

	paginator := cloudwatch.NewGetMetricDataPaginator(client, &cloudwatch.GetMetricDataInput{
		MetricDataQueries: queries,
		StartTime:         aws.Time(start),
		EndTime:           aws.Time(end),
		ScanBy:            cwtypes.ScanByTimestampAscending,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get metric data: %w", err)
		}
		for _, result := range page.MetricDataResults {
			id := ptrStr(result.Id)
			byTime, ok := values[id]
			if !ok {
				byTime = map[int64]float64{}
				values[id] = byTime
			}
			// Timestamps と Values は同じ添字で対応する。長さが揃わない応答は
			// 対応が取れないため短い方に合わせる。
			n := min(len(result.Timestamps), len(result.Values))
			for i := 0; i < n; i++ {
				byTime[result.Timestamps[i].UnixMilli()] = result.Values[i]
			}
		}
	}
	return values, nil
}
