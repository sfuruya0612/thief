package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	smithy "github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// doTimeseries は登録済みルート経由で時系列エンドポイントを叩く。
func doTimeseries(t *testing.T, s *Server, target string) *httptest.ResponseRecorder {
	t.Helper()
	s.mux = http.NewServeMux()
	s.registerRoutes()
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

// decodeTimeseries は応答本文を TimeseriesResponse として読む。
func decodeTimeseries(t *testing.T, w *httptest.ResponseRecorder) awsinternal.TimeseriesResponse {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var got awsinternal.TimeseriesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return got
}

// TestTimeseriesRangeParam は range パラメータの検証を両エンドポイントで確認する。
// 期間が決まらなければ粒度もキャッシュキーも決まらないため、既定値には倒さない。
func TestTimeseriesRangeParam(t *testing.T) {
	paths := []string{
		"/api/aws/profiles/prod/ec2/timeseries",
		"/api/aws/profiles/prod/ecs/timeseries",
	}
	tests := []struct {
		name  string
		query string
	}{
		{name: "missing", query: ""},
		{name: "empty", query: "?range="},
		{name: "unknown", query: "?range=90d"},
	}
	for _, path := range paths {
		for _, tt := range tests {
			t.Run(path+" "+tt.name, func(t *testing.T) {
				s := newTestServer(t)
				w := doTimeseries(t, s, path+tt.query)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
				}
				var body ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("unmarshal body: %v", err)
				}
				if body.Code != "BAD_REQUEST" {
					t.Errorf("code = %q, want BAD_REQUEST", body.Code)
				}
			})
		}
	}
}

// TestHandleEC2Timeseries は Auto Scaling グループごとの系列をそのまま返し、同じ期間の
// 2 回目がキャッシュで返ることを検証する。
func TestHandleEC2Timeseries(t *testing.T) {
	s := newTestServer(t)
	value := 2.0
	var gotRanges []awsinternal.TimeseriesRange
	s.ec2InstanceCountSeries = func(_ context.Context, profile, region string, r awsinternal.TimeseriesRange, _ awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error) {
		if profile != "prod" || region != "ap-northeast-1" {
			t.Errorf("profile/region = %q/%q, want prod/ap-northeast-1", profile, region)
		}
		gotRanges = append(gotRanges, r)
		return []awsinternal.TimeseriesSeries{{
			Name:   "web-asg",
			Points: []awsinternal.MetricPoint{{T: 1000, V: &value}, {T: 2000}},
		}}, nil
	}

	target := "/api/aws/profiles/prod/ec2/timeseries?range=30d&region=ap-northeast-1"
	w := doTimeseries(t, s, target)
	body := w.Body.String()
	got := decodeTimeseries(t, w)
	if got.Range != "30d" || got.PeriodSeconds != 3600 {
		t.Errorf("range = %q period = %d, want 30d / 3600", got.Range, got.PeriodSeconds)
	}
	want := []awsinternal.TimeseriesSeries{{
		Name:   "web-asg",
		Points: []awsinternal.MetricPoint{{T: 1000, V: &value}, {T: 2000, V: nil}},
	}}
	if diff := cmp.Diff(want, got.Series); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	// 欠測は JSON でも null のまま残す。
	if !strings.Contains(body, `"v":null`) {
		t.Errorf("body must keep a missing point as null: %s", body)
	}

	// 同じ期間の 2 回目はキャッシュから返るため CloudWatch を呼び直さない。
	doTimeseries(t, s, target)
	// 別の期間は別のキーなので取得し直す。
	doTimeseries(t, s, "/api/aws/profiles/prod/ec2/timeseries?range=1d&region=ap-northeast-1")
	if diff := cmp.Diff([]awsinternal.TimeseriesRange{awsinternal.Range30Days, awsinternal.Range1Day}, gotRanges); diff != "" {
		t.Errorf("fetched ranges mismatch (-want +got):\n%s", diff)
	}
}

// TestHandleEC2TimeseriesWindow は応答の窓が ec2InstanceCountSeries へ渡した窓
// (グリッドを組むのに使う窓) と一致することを検証する。窓を 2 か所で別々に計算すると、
// グリッドの範囲と frontend が描く X 軸の範囲がずれる。
func TestHandleEC2TimeseriesWindow(t *testing.T) {
	tests := []struct {
		name string
		rng  awsinternal.TimeseriesRange
	}{
		{name: "1d", rng: awsinternal.Range1Day},
		{name: "7d", rng: awsinternal.Range7Days},
		{name: "30d", rng: awsinternal.Range30Days},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			var gotWindow awsinternal.TimeseriesWindow
			value := 1.0
			s.ec2InstanceCountSeries = func(_ context.Context, _, _ string, r awsinternal.TimeseriesRange, w awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error) {
				gotWindow = w
				// グリッドの両端に点を置く。実装と同じく終端は含まない。
				step := int64(r.PeriodSeconds()) * 1000
				return []awsinternal.TimeseriesSeries{{
					Name: "web-asg",
					Points: []awsinternal.MetricPoint{
						{T: w.Start, V: &value},
						{T: w.End - step, V: &value},
					},
				}}, nil
			}

			got := decodeTimeseries(t, doTimeseries(t, s,
				"/api/aws/profiles/prod/ec2/timeseries?range="+string(tt.rng)+"&region=ap-northeast-1"))

			if got.Start != gotWindow.Start || got.End != gotWindow.End {
				t.Errorf("response window = %d..%d, want %d..%d (the window used for the grid)",
					got.Start, got.End, gotWindow.Start, gotWindow.End)
			}
			if want := tt.rng.Duration().Milliseconds(); got.End-got.Start != want {
				t.Errorf("window width = %d ms, want %d ms", got.End-got.Start, want)
			}
			// EC2 の窓も粒度で切り下げた終端を使う (キャッシュの間は同じ窓になる)。
			if step := int64(tt.rng.PeriodSeconds()) * 1000; got.End%step != 0 {
				t.Errorf("end = %d, want a value truncated to the period (%d ms)", got.End, step)
			}
			for i, p := range got.Series[0].Points {
				if p.T < got.Start || p.T >= got.End {
					t.Errorf("points[%d].T = %d, outside the window %d..%d", i, p.T, got.Start, got.End)
				}
			}
		})
	}
}

// TestHandleEC2TimeseriesAccessDenied は CloudWatch / Auto Scaling の権限が無い場合に
// 403 ACCESS_DENIED を返すことを検証する。SSO の再ログインでは解消しないため 401 にはしない。
func TestHandleEC2TimeseriesAccessDenied(t *testing.T) {
	s := newTestServer(t)
	s.ec2InstanceCountSeries = func(context.Context, string, string, awsinternal.TimeseriesRange, awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error) {
		return nil, fmt.Errorf("describe auto scaling groups: %w", &smithy.GenericAPIError{
			Code:    "AccessDeniedException",
			Message: "User: arn:aws:sts::123456789012:assumed-role/x is not authorized to perform: autoscaling:DescribeAutoScalingGroups",
		})
	}

	w := doTimeseries(t, s, "/api/aws/profiles/prod/ec2/timeseries?range=1d&region=ap-northeast-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
	var body ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Code != "ACCESS_DENIED" {
		t.Errorf("code = %q, want ACCESS_DENIED", body.Code)
	}
}

// TestHandleEC2ListError は一覧の取得失敗が従来どおり 500 になることを検証する。
func TestHandleEC2ListError(t *testing.T) {
	s := newTestServer(t)
	s.ec2Resources = func(context.Context, string, string) ([]awsinternal.EC2Resource, error) {
		return nil, errors.New("describe instances failed")
	}

	w := doTimeseries(t, s, "/api/aws/profiles/prod/ec2?region=ap-northeast-1")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// TestHandleECSTimeseries は期間ごとの粒度と、同じ期間の 2 回目がキャッシュで返ることを検証する。
func TestHandleECSTimeseries(t *testing.T) {
	s := newTestServer(t)
	value := 2.0
	var gotRanges []awsinternal.TimeseriesRange
	s.ecsTaskCountSeries = func(_ context.Context, profile, region string, r awsinternal.TimeseriesRange, _ awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error) {
		if profile != "prod" || region != "ap-northeast-1" {
			t.Errorf("profile/region = %q/%q, want prod/ap-northeast-1", profile, region)
		}
		gotRanges = append(gotRanges, r)
		return []awsinternal.TimeseriesSeries{{
			Name:   "prod-cluster",
			Points: []awsinternal.MetricPoint{{T: 1000, V: &value}, {T: 2000}},
		}}, nil
	}

	target := "/api/aws/profiles/prod/ecs/timeseries?range=30d&region=ap-northeast-1"
	w := doTimeseries(t, s, target)
	body := w.Body.String()
	got := decodeTimeseries(t, w)
	if got.Range != "30d" || got.PeriodSeconds != 3600 {
		t.Errorf("range = %q period = %d, want 30d / 3600", got.Range, got.PeriodSeconds)
	}
	want := []awsinternal.TimeseriesSeries{{
		Name:   "prod-cluster",
		Points: []awsinternal.MetricPoint{{T: 1000, V: &value}, {T: 2000, V: nil}},
	}}
	if diff := cmp.Diff(want, got.Series); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	// 欠測は JSON でも null のまま残す。
	if !strings.Contains(body, `"v":null`) {
		t.Errorf("body must keep a missing point as null: %s", body)
	}

	// 同じ期間の 2 回目はキャッシュから返るため CloudWatch を呼び直さない。
	doTimeseries(t, s, target)
	// 別の期間は別のキーなので取得し直す。
	doTimeseries(t, s, "/api/aws/profiles/prod/ecs/timeseries?range=1d&region=ap-northeast-1")
	if diff := cmp.Diff([]awsinternal.TimeseriesRange{awsinternal.Range30Days, awsinternal.Range1Day}, gotRanges); diff != "" {
		t.Errorf("fetched ranges mismatch (-want +got):\n%s", diff)
	}
}

// TestHandleECSTimeseriesWindow は応答の窓が ecsTaskCountSeries へ渡した窓 (グリッドを
// 組むのに使う窓) と一致することを検証する。窓を 2 か所で別々に計算すると、グリッドの
// 範囲と frontend が描く X 軸の範囲がずれる。
func TestHandleECSTimeseriesWindow(t *testing.T) {
	tests := []struct {
		name string
		rng  awsinternal.TimeseriesRange
	}{
		{name: "1d", rng: awsinternal.Range1Day},
		{name: "7d", rng: awsinternal.Range7Days},
		{name: "30d", rng: awsinternal.Range30Days},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			var gotWindow awsinternal.TimeseriesWindow
			value := 1.0
			s.ecsTaskCountSeries = func(_ context.Context, _, _ string, r awsinternal.TimeseriesRange, w awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error) {
				gotWindow = w
				// グリッドの両端に点を置く。実装と同じく終端は含まない。
				step := int64(r.PeriodSeconds()) * 1000
				return []awsinternal.TimeseriesSeries{{
					Name: "prod-cluster",
					Points: []awsinternal.MetricPoint{
						{T: w.Start, V: &value},
						{T: w.End - step, V: &value},
					},
				}}, nil
			}

			got := decodeTimeseries(t, doTimeseries(t, s,
				"/api/aws/profiles/prod/ecs/timeseries?range="+string(tt.rng)+"&region=ap-northeast-1"))

			if got.Start != gotWindow.Start || got.End != gotWindow.End {
				t.Errorf("response window = %d..%d, want %d..%d (the window used for the grid)",
					got.Start, got.End, gotWindow.Start, gotWindow.End)
			}
			if want := tt.rng.Duration().Milliseconds(); got.End-got.Start != want {
				t.Errorf("window width = %d ms, want %d ms", got.End-got.Start, want)
			}
			// ECS の窓は粒度で切り下げた終端を使う (キャッシュの間は同じ窓になる)。
			if step := int64(tt.rng.PeriodSeconds()) * 1000; got.End%step != 0 {
				t.Errorf("end = %d, want a value truncated to the period (%d ms)", got.End, step)
			}
			for i, p := range got.Series[0].Points {
				if p.T < got.Start || p.T >= got.End {
					t.Errorf("points[%d].T = %d, outside the window %d..%d", i, p.T, got.Start, got.End)
				}
			}
		})
	}
}

// TestHandleECSTimeseriesAccessDenied は cloudwatch:GetMetricData の権限が無い場合に
// 403 ACCESS_DENIED を返すことを検証する。SSO の再ログインでは解消しないため 401 にはしない。
func TestHandleECSTimeseriesAccessDenied(t *testing.T) {
	s := newTestServer(t)
	s.ecsTaskCountSeries = func(context.Context, string, string, awsinternal.TimeseriesRange, awsinternal.TimeseriesWindow) ([]awsinternal.TimeseriesSeries, error) {
		return nil, fmt.Errorf("get metric data: %w", &smithy.GenericAPIError{
			Code:    "AccessDeniedException",
			Message: "User: arn:aws:sts::123456789012:assumed-role/x is not authorized to perform: cloudwatch:GetMetricData",
		})
	}

	w := doTimeseries(t, s, "/api/aws/profiles/prod/ecs/timeseries?range=1d&region=ap-northeast-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
	var body ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Code != "ACCESS_DENIED" {
		t.Errorf("code = %q, want ACCESS_DENIED", body.Code)
	}
}
