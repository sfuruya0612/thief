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
	// 記録は任意の時刻に行われるため、終端を粒度で切り下げない RecordedWindow を使う。
	// 切り下げると、切り下げた後に記録された直近の点が窓の外に出る。
	// 同じ窓を絞り込みと応答の両方に渡し、軸の範囲と点の絞り込みの境界を一致させる。
	window := rng.RecordedWindow(time.Now())
	writeJSON(w, awsinternal.TimeseriesResponse{
		Range:         string(rng),
		PeriodSeconds: rng.PeriodSeconds(),
		Start:         window.Start,
		End:           window.End,
		Series: []awsinternal.TimeseriesSeries{{
			Name:   ec2RunningSeriesName,
			Points: s.ec2Counts.Series(profile, region, window),
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
