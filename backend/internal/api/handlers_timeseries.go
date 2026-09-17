package api

import (
	"net/http"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// ec2RunningSeriesName は EC2 の台数の系列名。凡例に出る。
const ec2RunningSeriesName = "Running"

// handleEC2Timeseries は Running な EC2 インスタンス数の推移を返す。
//
// 値は handleEC2 が AWS から一覧を取得したときに記録されたものだけで、AWS への
// 問い合わせは行わない。記録が無ければ空の系列を返す (エラーにはしない。履歴が
// 貯まっていないことは異常ではない)。
func (s *Server) handleEC2Timeseries(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	rng, ok := timeseriesRangeFromQuery(w, r)
	if !ok {
		return
	}
	writeJSON(w, awsinternal.TimeseriesResponse{
		Range:         string(rng),
		PeriodSeconds: rng.PeriodSeconds(),
		Series: []awsinternal.TimeseriesSeries{{
			Name:   ec2RunningSeriesName,
			Points: s.ec2Counts.Series(profile, region, rng, time.Now()),
		}},
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
		series, err := s.ecsTaskCountSeries(r.Context(), profile, region, rng, time.Now())
		if err != nil {
			return nil, err
		}
		return awsinternal.TimeseriesResponse{
			Range:         string(rng),
			PeriodSeconds: rng.PeriodSeconds(),
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
