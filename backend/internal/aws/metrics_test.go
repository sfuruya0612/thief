package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/google/go-cmp/cmp"
)

// fakeMetricDataClient は GetMetricData の応答を固定で返すモック。
// pages は呼び出し順に 1 つずつ返す (ページネーションの検証に使う)。
type fakeMetricDataClient struct {
	pages []*cloudwatch.GetMetricDataOutput
	err   error
	calls []*cloudwatch.GetMetricDataInput
}

func (f *fakeMetricDataClient) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.calls) > len(f.pages) {
		return nil, errors.New("unexpected extra GetMetricData call")
	}
	return f.pages[len(f.calls)-1], nil
}

func TestParseTimeseriesRange(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    TimeseriesRange
		wantErr error
	}{
		{name: "1d", in: "1d", want: Range1Day},
		{name: "7d", in: "7d", want: Range7Days},
		{name: "30d", in: "30d", want: Range30Days},
		{name: "empty", in: "", wantErr: ErrInvalidTimeseriesRange},
		{name: "unknown", in: "90d", wantErr: ErrInvalidTimeseriesRange},
		{name: "uppercase is not accepted", in: "1D", wantErr: ErrInvalidTimeseriesRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeseriesRange(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("range = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTimeseriesRangeDurationAndPeriod(t *testing.T) {
	tests := []struct {
		name       string
		in         TimeseriesRange
		wantDur    time.Duration
		wantPeriod int32
	}{
		{name: "1d", in: Range1Day, wantDur: 24 * time.Hour, wantPeriod: 60},
		{name: "7d", in: Range7Days, wantDur: 7 * 24 * time.Hour, wantPeriod: 300},
		{name: "30d", in: Range30Days, wantDur: 30 * 24 * time.Hour, wantPeriod: 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Duration(); got != tt.wantDur {
				t.Errorf("duration = %v, want %v", got, tt.wantDur)
			}
			if got := tt.in.PeriodSeconds(); got != tt.wantPeriod {
				t.Errorf("period = %d, want %d", got, tt.wantPeriod)
			}
		})
	}
}

// TestTimeseriesRangeWindow は終端が粒度で切り下げられることを検証する。
// 切り下げないと窓がリクエストごとにずれ、キャッシュキーが毎回変わる。
func TestTimeseriesRangeWindow(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 42, 37, 500_000_000, time.UTC)
	tests := []struct {
		name      string
		in        TimeseriesRange
		wantEnd   time.Time
		wantStart time.Time
	}{
		{
			name:      "1d truncates to the minute",
			in:        Range1Day,
			wantEnd:   time.Date(2026, 9, 17, 10, 42, 0, 0, time.UTC),
			wantStart: time.Date(2026, 9, 16, 10, 42, 0, 0, time.UTC),
		},
		{
			name:      "7d truncates to five minutes",
			in:        Range7Days,
			wantEnd:   time.Date(2026, 9, 17, 10, 40, 0, 0, time.UTC),
			wantStart: time.Date(2026, 9, 10, 10, 40, 0, 0, time.UTC),
		},
		{
			name:      "30d truncates to the hour",
			in:        Range30Days,
			wantEnd:   time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := tt.in.Window(now)
			if !end.Equal(tt.wantEnd) {
				t.Errorf("end = %v, want %v", end, tt.wantEnd)
			}
			if !start.Equal(tt.wantStart) {
				t.Errorf("start = %v, want %v", start, tt.wantStart)
			}
		})
	}
}

// TestTimeseriesRangeRecordedWindow は記録用の窓が終端を切り下げないことを検証する。
// 切り下げると、切り下げた後に記録された直近の点が窓から外れる。
func TestTimeseriesRangeRecordedWindow(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 42, 37, 500_000_000, time.UTC)
	tests := []struct {
		name string
		in   TimeseriesRange
	}{
		{name: "1d", in: Range1Day},
		{name: "7d", in: Range7Days},
		{name: "30d", in: Range30Days},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.RecordedWindow(now)
			want := TimeseriesWindow{
				Start: now.Add(-tt.in.Duration()).UnixMilli(),
				End:   now.UnixMilli(),
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			// 粒度で切り下げる Window とは違い、終端は now のままである。
			if _, end := tt.in.Window(now); got.End == end.UnixMilli() {
				t.Errorf("end = %d, want an end that is not truncated to the period", got.End)
			}
		})
	}
}

func TestTimeseriesGrid(t *testing.T) {
	start := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		start  time.Time
		end    time.Time
		period int32
		want   []int64
	}{
		{
			name:   "three points at one minute",
			start:  start,
			end:    start.Add(3 * time.Minute),
			period: 60,
			want: []int64{
				start.UnixMilli(),
				start.Add(time.Minute).UnixMilli(),
				start.Add(2 * time.Minute).UnixMilli(),
			},
		},
		{name: "end equals start", start: start, end: start, period: 60, want: nil},
		{name: "end before start", start: start, end: start.Add(-time.Minute), period: 60, want: nil},
		{name: "non positive period", start: start, end: start.Add(time.Hour), period: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grid := timeseriesGrid(NewTimeseriesWindow(tt.start, tt.end), tt.period)
			if diff := cmp.Diff(tt.want, grid); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMetricPointsOnGrid は値の無い時刻が V=nil (JSON の null) のまま残ることを検証する。
// 0 に潰すと「値が 0 だった」と読めてしまう。
func TestMetricPointsOnGrid(t *testing.T) {
	grid := []int64{1000, 2000, 3000}
	got := metricPointsOnGrid(grid, map[int64]float64{1000: 2, 3000: 0})

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].V == nil || *got[0].V != 2 {
		t.Errorf("points[0].V = %v, want 2", got[0].V)
	}
	if got[1].V != nil {
		t.Errorf("points[1].V = %v, want nil", *got[1].V)
	}
	if got[2].V == nil || *got[2].V != 0 {
		t.Errorf("points[2].V = %v, want 0", got[2].V)
	}
	for i, want := range grid {
		if got[i].T != want {
			t.Errorf("points[%d].T = %d, want %d", i, got[i].T, want)
		}
	}
}

// TestGetMetricDataValuesPaginates は NextToken の続きも読み、Id ごとに値を束ねることを検証する。
func TestGetMetricDataValuesPaginates(t *testing.T) {
	start := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{
		{
			NextToken: aws.String("next"),
			MetricDataResults: []cwtypes.MetricDataResult{
				{Id: aws.String("q0"), Timestamps: []time.Time{start}, Values: []float64{1}},
			},
		},
		{
			MetricDataResults: []cwtypes.MetricDataResult{
				{Id: aws.String("q0"), Timestamps: []time.Time{start.Add(time.Minute)}, Values: []float64{2}},
				{Id: aws.String("q1"), Timestamps: []time.Time{start}, Values: []float64{3}},
			},
		},
	}}

	queries := []cwtypes.MetricDataQuery{{Id: aws.String("q0")}, {Id: aws.String("q1")}}
	got, err := getMetricDataValues(context.Background(), client, queries, start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("getMetricDataValues: %v", err)
	}

	want := map[string]map[int64]float64{
		"q0": {start.UnixMilli(): 1, start.Add(time.Minute).UnixMilli(): 2},
		"q1": {start.UnixMilli(): 3},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if len(client.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(client.calls))
	}
	if client.calls[0].ScanBy != cwtypes.ScanByTimestampAscending {
		t.Errorf("ScanBy = %q, want %q", client.calls[0].ScanBy, cwtypes.ScanByTimestampAscending)
	}
	if !client.calls[0].StartTime.Equal(start) || !client.calls[0].EndTime.Equal(start.Add(time.Hour)) {
		t.Errorf("window = %v..%v, want %v..%v",
			client.calls[0].StartTime, client.calls[0].EndTime, start, start.Add(time.Hour))
	}
}

// TestGetMetricDataValuesMismatchedLengths は Timestamps と Values の長さが揃わない応答を
// 短い方に合わせて読むことを検証する (対応の取れない値を取り込まない)。
func TestGetMetricDataValuesMismatchedLengths(t *testing.T) {
	start := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{
		MetricDataResults: []cwtypes.MetricDataResult{{
			Id:         aws.String("q0"),
			Timestamps: []time.Time{start, start.Add(time.Minute)},
			Values:     []float64{7},
		}},
	}}}

	got, err := getMetricDataValues(context.Background(), client,
		[]cwtypes.MetricDataQuery{{Id: aws.String("q0")}}, start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("getMetricDataValues: %v", err)
	}
	want := map[string]map[int64]float64{"q0": {start.UnixMilli(): 7}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// TestGetMetricDataValuesNoQueries は クエリが空なら AWS を呼ばないことを検証する。
func TestGetMetricDataValuesNoQueries(t *testing.T) {
	client := &fakeMetricDataClient{}
	got, err := getMetricDataValues(context.Background(), client, nil, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("getMetricDataValues: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("values = %v, want empty", got)
	}
	if len(client.calls) != 0 {
		t.Errorf("calls = %d, want 0", len(client.calls))
	}
}

// TestGetMetricDataValuesError は API エラーが呼び出し元へ伝播することを検証する
// (権限不足を 403 へ分類する api 層がエラーを受け取れるようにするため)。
func TestGetMetricDataValuesError(t *testing.T) {
	sentinel := errors.New("boom")
	client := &fakeMetricDataClient{err: sentinel}
	_, err := getMetricDataValues(context.Background(), client,
		[]cwtypes.MetricDataQuery{{Id: aws.String("q0")}}, time.Now(), time.Now().Add(time.Hour))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}
