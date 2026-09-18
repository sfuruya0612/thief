package api

import (
	"net/http"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// handleEC2Timeseries は Auto Scaling グループごとの InService インスタンス数の
// 推移を CloudWatch から取得して返す。
//
// AWS/EC2 名前空間にはアカウントやリージョン単位の Running 台数の標準メトリクスが
// 無いため、AWS/AutoScaling の GroupInServiceInstances をグループごとに返す。
// したがって Auto Scaling グループに属さないインスタンスは含まれない。
//
// cloudwatch:GetMetricData または autoscaling:DescribeAutoScalingGroups の権限が無い
// 場合は writeAWSError が 403 ACCESS_DENIED を返す。一覧系のエンドポイントとは独立して
// いるため、権限が無くても一覧の表示には影響しない。
func (s *Server) handleEC2Timeseries(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	rng, ok := timeseriesRangeFromQuery(w, r)
	if !ok {
		return
	}
	// 期間ごとに時間窓も粒度も変わるため、キャッシュキーに期間を含める。
	// 時間窓は TimeseriesRange.Window が粒度で切り下げるため、TTL の間は同じ窓になる。
	key := cacheKey("ec2-timeseries", profile, region, string(rng))
	s.serveCached(w, r, key, cacheTTL, writeAWSError, func() (any, error) {
		// 窓の計算はここだけで行い、同じ値をグリッドの生成と応答の両方に渡す。
		window := awsinternal.NewTimeseriesWindow(rng.Window(time.Now()))
		series, err := s.ec2InstanceCountSeries(r.Context(), profile, region, rng, window)
		if err != nil {
			return nil, err
		}
		return awsinternal.TimeseriesResponse{
			Range:         string(rng),
			PeriodSeconds: rng.PeriodSeconds(),
			Start:         window.Start,
			End:           window.End,
			Series:        series,
		}, nil
	})
}

// handleECSTimeseries はクラスタごとのタスク数の推移を CloudWatch から取得して返す。
//
// cloudwatch:GetMetricData の権限が無い場合は writeAWSError が 403 ACCESS_DENIED を返す。
// 一覧系のエンドポイントとは独立しているため、権限が無くても一覧の表示には影響しない。
func (s *Server) handleECSTimeseries(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	rng, ok := timeseriesRangeFromQuery(w, r)
	if !ok {
		return
	}
	// 期間ごとに時間窓も粒度も変わるため、キャッシュキーに期間を含める。
	// 時間窓は TimeseriesRange.Window が粒度で切り下げるため、TTL の間は同じ窓になる。
	key := cacheKey("ecs-timeseries", profile, region, string(rng))
	s.serveCached(w, r, key, cacheTTL, writeAWSError, func() (any, error) {
		// 窓の計算はここだけで行い、同じ値をグリッドの生成と応答の両方に渡す。
		window := awsinternal.NewTimeseriesWindow(rng.Window(time.Now()))
		series, err := s.ecsTaskCountSeries(r.Context(), profile, region, rng, window)
		if err != nil {
			return nil, err
		}
		return awsinternal.TimeseriesResponse{
			Range:         string(rng),
			PeriodSeconds: rng.PeriodSeconds(),
			Start:         window.Start,
			End:           window.End,
			Series:        series,
		}, nil
	})
}

// timeseriesRangeFromQuery は range クエリパラメータを取り出す。
// 既定値を持たせず必須にしているのは、期間によって粒度もキャッシュキーも変わるため、
// どの期間を見ているのかを呼び出し側が明示する必要があるからである。
func timeseriesRangeFromQuery(w http.ResponseWriter, r *http.Request) (awsinternal.TimeseriesRange, bool) {
	raw, ok := requireQueryParam(w, r, "range")
	if !ok {
		return "", false
	}
	rng, err := awsinternal.ParseTimeseriesRange(raw)
	if err != nil {
		writeBadRequest(w, err.Error())
		return "", false
	}
	return rng, true
}
