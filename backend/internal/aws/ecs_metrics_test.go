package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/google/go-cmp/cmp"
)

func TestEcsTaskCountExpression(t *testing.T) {
	got := ecsTaskCountExpression("prod", 300)
	want := `SUM(SEARCH('{AWS/ECS,ClusterName,ServiceName} MetricName="LiveTaskCount" ClusterName="prod"', 'Average', 300))`
	if got != want {
		t.Errorf("expression = %q, want %q", got, want)
	}
}

// TestEcsTaskCountSeriesQueries は GetMetricData に載せるクエリの形を検証する。
// クラスタ 1 つにつき 1 クエリで、粒度は期間から決まる。
func TestEcsTaskCountSeriesQueries(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 42, 30, 0, time.UTC)
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{}}}

	window := NewTimeseriesWindow(Range7Days.Window(now))
	if _, err := ecsTaskCountSeries(context.Background(), client, []string{"beta", "alpha"}, Range7Days, window); err != nil {
		t.Fatalf("ecsTaskCountSeries: %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(client.calls))
	}
	queries := client.calls[0].MetricDataQueries
	if len(queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(queries))
	}
	// クラスタ名の昇順に並べ、Id はバッチ内の添字から決める。
	wantIDs := []string{"q0", "q1"}
	wantClusters := []string{"alpha", "beta"}
	for i, q := range queries {
		if ptrStr(q.Id) != wantIDs[i] {
			t.Errorf("queries[%d].Id = %q, want %q", i, ptrStr(q.Id), wantIDs[i])
		}
		if ptrStr(q.Label) != wantClusters[i] {
			t.Errorf("queries[%d].Label = %q, want %q", i, ptrStr(q.Label), wantClusters[i])
		}
		if q.Period == nil || *q.Period != 300 {
			t.Errorf("queries[%d].Period = %v, want 300", i, q.Period)
		}
		if want := ecsTaskCountExpression(wantClusters[i], 300); ptrStr(q.Expression) != want {
			t.Errorf("queries[%d].Expression = %q, want %q", i, ptrStr(q.Expression), want)
		}
	}
}

// TestEcsTaskCountSeriesPoints は欠測が null のまま残ること、点がグリッド上に並ぶことを検証する。
// CloudWatch はデータ点の無い時刻を応答に含めないため、グリッドは応答とは独立に組む。
func TestEcsTaskCountSeriesPoints(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 3, 0, 0, time.UTC)
	start, end := Range1Day.Window(now)
	window := NewTimeseriesWindow(start, end)
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{
		MetricDataResults: []cwtypes.MetricDataResult{{
			Id:         aws.String("q0"),
			Timestamps: []time.Time{end.Add(-2 * time.Minute), end.Add(-time.Minute)},
			Values:     []float64{3, 5},
		}},
	}}}

	series, err := ecsTaskCountSeries(context.Background(), client, []string{"prod"}, Range1Day, window)
	if err != nil {
		t.Fatalf("ecsTaskCountSeries: %v", err)
	}
	if len(series) != 1 || series[0].Name != "prod" {
		t.Fatalf("series = %+v, want 1 series named prod", series)
	}

	points := series[0].Points
	want := int(end.Sub(start).Seconds()) / 60
	if len(points) != want {
		t.Fatalf("points = %d, want %d", len(points), want)
	}
	if points[0].T != start.UnixMilli() {
		t.Errorf("points[0].T = %d, want %d", points[0].T, start.UnixMilli())
	}
	if points[0].V != nil {
		t.Errorf("points[0].V = %v, want nil", *points[0].V)
	}
	last := points[len(points)-1]
	// グリッドは渡した窓をそのまま覆う (終端は含まない)。
	if want := end.Add(-time.Minute).UnixMilli(); last.T != want {
		t.Errorf("last point T = %d, want %d", last.T, want)
	}
	if last.V == nil || *last.V != 5 {
		t.Errorf("last point = %v, want 5", last.V)
	}
	prev := points[len(points)-2]
	if prev.V == nil || *prev.V != 3 {
		t.Errorf("second to last point = %v, want 3", prev.V)
	}
}

// TestEcsTaskCountSeriesNoClusters はクラスタが無ければ AWS を呼ばず空の系列を返すことを検証する。
func TestEcsTaskCountSeriesNoClusters(t *testing.T) {
	client := &fakeMetricDataClient{}
	series, err := ecsTaskCountSeries(context.Background(), client, nil, Range1Day,
		NewTimeseriesWindow(Range1Day.Window(time.Now())))
	if err != nil {
		t.Fatalf("ecsTaskCountSeries: %v", err)
	}
	if len(series) != 0 {
		t.Errorf("series = %+v, want empty", series)
	}
	if len(client.calls) != 0 {
		t.Errorf("calls = %d, want 0", len(client.calls))
	}
}

// TestEcsTaskCountSeriesSkipsUnsafeNames は SEARCH 式の構文を壊しうる名前を対象外にすることを検証する。
func TestEcsTaskCountSeriesSkipsUnsafeNames(t *testing.T) {
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{}}}
	series, err := ecsTaskCountSeries(context.Background(), client,
		[]string{`bad"name`, "ok-cluster", ""}, Range1Day, NewTimeseriesWindow(Range1Day.Window(time.Now())))
	if err != nil {
		t.Fatalf("ecsTaskCountSeries: %v", err)
	}
	names := make([]string, 0, len(series))
	for _, s := range series {
		names = append(names, s.Name)
	}
	if diff := cmp.Diff([]string{"ok-cluster"}, names); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	for _, q := range client.calls[0].MetricDataQueries {
		if strings.Contains(ptrStr(q.Expression), `bad"name`) {
			t.Errorf("expression leaked an unsafe cluster name: %q", ptrStr(q.Expression))
		}
	}
}

// TestEcsTaskCountSeriesBatches はクエリ数の上限でリクエストを分割することを検証する。
func TestEcsTaskCountSeriesBatches(t *testing.T) {
	clusters := make([]string, ecsMetricDataBatchSize+2)
	for i := range clusters {
		// 桁を揃えて文字列の昇順と添字の昇順を一致させる。
		clusters[i] = fmt.Sprintf("c%04d", i)
	}
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{}, {}}}

	series, err := ecsTaskCountSeries(context.Background(), client, clusters, Range30Days,
		NewTimeseriesWindow(Range30Days.Window(time.Now())))
	if err != nil {
		t.Fatalf("ecsTaskCountSeries: %v", err)
	}
	if len(series) != len(clusters) {
		t.Errorf("series = %d, want %d", len(series), len(clusters))
	}
	if len(client.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(client.calls))
	}
	if got := len(client.calls[0].MetricDataQueries); got != ecsMetricDataBatchSize {
		t.Errorf("first batch = %d, want %d", got, ecsMetricDataBatchSize)
	}
	if got := len(client.calls[1].MetricDataQueries); got != 2 {
		t.Errorf("second batch = %d, want 2", got)
	}
}

// TestEcsTaskCountSeriesError は GetMetricData のエラーが伝播することを検証する。
func TestEcsTaskCountSeriesError(t *testing.T) {
	sentinel := errors.New("denied")
	client := &fakeMetricDataClient{err: sentinel}
	window := NewTimeseriesWindow(Range1Day.Window(time.Now()))
	if _, err := ecsTaskCountSeries(context.Background(), client, []string{"prod"}, Range1Day, window); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}
