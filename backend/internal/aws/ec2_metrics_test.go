package aws

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/google/go-cmp/cmp"
)

// fakeAutoScalingClient は DescribeAutoScalingGroups の応答を固定で返すモック。
// NextToken によるページ送りを模すため、pages を順に返す。
type fakeAutoScalingClient struct {
	pages []*autoscaling.DescribeAutoScalingGroupsOutput
	calls []*autoscaling.DescribeAutoScalingGroupsInput
	err   error
}

func (f *fakeAutoScalingClient) DescribeAutoScalingGroups(_ context.Context, in *autoscaling.DescribeAutoScalingGroupsInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingGroupsOutput, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.pages) == 0 {
		return &autoscaling.DescribeAutoScalingGroupsOutput{}, nil
	}
	out := f.pages[0]
	f.pages = f.pages[1:]
	return out, nil
}

func TestListAutoScalingGroupNamesWith(t *testing.T) {
	client := &fakeAutoScalingClient{pages: []*autoscaling.DescribeAutoScalingGroupsOutput{
		{
			AutoScalingGroups: []autoscalingtypes.AutoScalingGroup{
				{AutoScalingGroupName: aws.String("web-asg")},
				{AutoScalingGroupName: nil},
				{AutoScalingGroupName: aws.String("")},
			},
			NextToken: aws.String("next"),
		},
		{
			AutoScalingGroups: []autoscalingtypes.AutoScalingGroup{
				{AutoScalingGroupName: aws.String("api-asg")},
			},
		},
	}}

	names, err := listAutoScalingGroupNamesWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listAutoScalingGroupNamesWith: %v", err)
	}
	// 空名と nil は落とし、ページを跨いで順に並べる。
	if diff := cmp.Diff([]string{"web-asg", "api-asg"}, names); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if len(client.calls) != 2 {
		t.Errorf("calls = %d, want 2", len(client.calls))
	}
}

func TestListAutoScalingGroupNamesWithError(t *testing.T) {
	sentinel := errors.New("denied")
	client := &fakeAutoScalingClient{err: sentinel}
	if _, err := listAutoScalingGroupNamesWith(context.Background(), client); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// TestEC2InstanceCountSeriesQueries は GetMetricData に載せるクエリの形を検証する。
// グループ 1 つにつき 1 クエリで、AWS/AutoScaling の GroupInServiceInstances を
// MetricStat で直接引く (SEARCH 式は使わない)。
func TestEC2InstanceCountSeriesQueries(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 42, 30, 0, time.UTC)
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{}}}

	window := NewTimeseriesWindow(Range7Days.Window(now))
	if _, err := ec2InstanceCountSeries(context.Background(), client, []string{"beta", "alpha"}, Range7Days, window); err != nil {
		t.Fatalf("ec2InstanceCountSeries: %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(client.calls))
	}
	queries := client.calls[0].MetricDataQueries
	if len(queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(queries))
	}
	// グループ名の昇順に並べ、Id はバッチ内の添字から決める。
	wantIDs := []string{"q0", "q1"}
	wantGroups := []string{"alpha", "beta"}
	for i, q := range queries {
		if ptrStr(q.Id) != wantIDs[i] {
			t.Errorf("queries[%d].Id = %q, want %q", i, ptrStr(q.Id), wantIDs[i])
		}
		if ptrStr(q.Label) != wantGroups[i] {
			t.Errorf("queries[%d].Label = %q, want %q", i, ptrStr(q.Label), wantGroups[i])
		}
		ms := q.MetricStat
		if ms == nil {
			t.Fatalf("queries[%d].MetricStat is nil", i)
		}
		if ptrStr(ms.Metric.Namespace) != autoscalingMetricNamespace {
			t.Errorf("queries[%d] namespace = %q, want %q", i, ptrStr(ms.Metric.Namespace), autoscalingMetricNamespace)
		}
		if ptrStr(ms.Metric.MetricName) != autoscalingInServiceInstancesMetric {
			t.Errorf("queries[%d] metric = %q, want %q", i, ptrStr(ms.Metric.MetricName), autoscalingInServiceInstancesMetric)
		}
		dims := ms.Metric.Dimensions
		if len(dims) != 1 || ptrStr(dims[0].Name) != "AutoScalingGroupName" || ptrStr(dims[0].Value) != wantGroups[i] {
			t.Errorf("queries[%d] dimensions = %+v, want AutoScalingGroupName=%q", i, dims, wantGroups[i])
		}
		if ms.Period == nil || *ms.Period != 300 {
			t.Errorf("queries[%d].Period = %v, want 300", i, ms.Period)
		}
		if ptrStr(ms.Stat) != ec2InstanceCountStatistic {
			t.Errorf("queries[%d].Stat = %q, want %q", i, ptrStr(ms.Stat), ec2InstanceCountStatistic)
		}
	}
}

// TestEC2InstanceCountSeriesPoints は欠測が null のまま残ること、点がグリッド上に並ぶことを検証する。
func TestEC2InstanceCountSeriesPoints(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 3, 0, 0, time.UTC)
	start, end := Range1Day.Window(now)
	window := NewTimeseriesWindow(start, end)
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{
		MetricDataResults: []cwtypes.MetricDataResult{{
			Id:         aws.String("q0"),
			Timestamps: []time.Time{end.Add(-2 * time.Minute), end.Add(-time.Minute)},
			Values:     []float64{3, 5},
		}},
	}}}

	series, err := ec2InstanceCountSeries(context.Background(), client, []string{"web-asg"}, Range1Day, window)
	if err != nil {
		t.Fatalf("ec2InstanceCountSeries: %v", err)
	}
	if len(series) != 1 || series[0].Name != "web-asg" {
		t.Fatalf("series = %+v, want 1 series named web-asg", series)
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

// TestEC2InstanceCountSeriesNoGroups はグループが無ければ AWS を呼ばず空の系列を返すことを検証する。
func TestEC2InstanceCountSeriesNoGroups(t *testing.T) {
	client := &fakeMetricDataClient{}
	series, err := ec2InstanceCountSeries(context.Background(), client, nil, Range1Day,
		NewTimeseriesWindow(Range1Day.Window(time.Now())))
	if err != nil {
		t.Fatalf("ec2InstanceCountSeries: %v", err)
	}
	if len(series) != 0 {
		t.Errorf("series = %+v, want empty", series)
	}
	if len(client.calls) != 0 {
		t.Errorf("calls = %d, want 0", len(client.calls))
	}
}

// TestEC2InstanceCountSeriesBatches はクエリ数の上限でリクエストを分割することを検証する。
func TestEC2InstanceCountSeriesBatches(t *testing.T) {
	groups := make([]string, ec2MetricDataBatchSize+2)
	for i := range groups {
		// 桁を揃えて文字列の昇順と添字の昇順を一致させる。
		groups[i] = fmt.Sprintf("g%04d", i)
	}
	client := &fakeMetricDataClient{pages: []*cloudwatch.GetMetricDataOutput{{}, {}}}

	series, err := ec2InstanceCountSeries(context.Background(), client, groups, Range30Days,
		NewTimeseriesWindow(Range30Days.Window(time.Now())))
	if err != nil {
		t.Fatalf("ec2InstanceCountSeries: %v", err)
	}
	if len(series) != len(groups) {
		t.Errorf("series = %d, want %d", len(series), len(groups))
	}
	if len(client.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(client.calls))
	}
	if got := len(client.calls[0].MetricDataQueries); got != ec2MetricDataBatchSize {
		t.Errorf("first batch = %d, want %d", got, ec2MetricDataBatchSize)
	}
	if got := len(client.calls[1].MetricDataQueries); got != 2 {
		t.Errorf("second batch = %d, want 2", got)
	}
}

// TestEC2InstanceCountSeriesError は GetMetricData のエラーが伝播することを検証する。
func TestEC2InstanceCountSeriesError(t *testing.T) {
	sentinel := errors.New("denied")
	client := &fakeMetricDataClient{err: sentinel}
	window := NewTimeseriesWindow(Range1Day.Window(time.Now()))
	if _, err := ec2InstanceCountSeries(context.Background(), client, []string{"web-asg"}, Range1Day, window); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}
