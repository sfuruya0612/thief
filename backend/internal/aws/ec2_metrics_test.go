package aws

import (
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestCountRunningEC2(t *testing.T) {
	tests := []struct {
		name string
		in   []EC2Resource
		want int
	}{
		{name: "empty", in: nil, want: 0},
		{
			name: "mixed states",
			in: []EC2Resource{
				{State: "running"},
				{State: "stopped"},
				{State: "running"},
				{State: "terminated"},
			},
			want: 2,
		},
		{
			// 表記ゆれは ResourceState (NormalizeState) が吸収する。
			name: "uppercase running is normalized",
			in:   []EC2Resource{{State: "RUNNING"}, {State: "pending"}},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountRunningEC2(tt.in); got != tt.want {
				t.Errorf("count = %d, want %d", got, tt.want)
			}
		})
	}
}

// seriesValues は系列の値だけを取り出す (欠測は NaN ではなく -1 に置き換える)。
func seriesValues(t *testing.T, points []MetricPoint) []float64 {
	t.Helper()
	out := make([]float64, 0, len(points))
	for _, p := range points {
		if p.V == nil {
			out = append(out, -1)
			continue
		}
		out = append(out, *p.V)
	}
	return out
}

func TestEC2CountRecorderSeries(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	rec := NewEC2CountRecorder()

	// 記録は観測した順に追記される (一覧の取得ごとに 1 点)。
	// 最初の 1 点は 1 日の期間からは外れる。
	rec.Record("prod", "ap-northeast-1", 1, now.Add(-30*time.Hour))
	rec.Record("prod", "ap-northeast-1", 5, now.Add(-2*time.Hour))
	rec.Record("prod", "ap-northeast-1", 7, now.Add(-time.Hour))
	// 別の region は混ざらない
	rec.Record("prod", "us-east-1", 99, now.Add(-time.Hour))

	t.Run("1d cuts out older points", func(t *testing.T) {
		got := rec.Series("prod", "ap-northeast-1", Range1Day.RecordedWindow(now))
		if diff := cmp.Diff([]float64{5, 7}, seriesValues(t, got)); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
		if got[0].T != now.Add(-2*time.Hour).UnixMilli() {
			t.Errorf("points[0].T = %d, want %d", got[0].T, now.Add(-2*time.Hour).UnixMilli())
		}
	})

	t.Run("7d includes the older point", func(t *testing.T) {
		got := rec.Series("prod", "ap-northeast-1", Range7Days.RecordedWindow(now))
		if diff := cmp.Diff([]float64{1, 5, 7}, seriesValues(t, got)); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("region is part of the key", func(t *testing.T) {
		got := rec.Series("prod", "us-east-1", Range1Day.RecordedWindow(now))
		if diff := cmp.Diff([]float64{99}, seriesValues(t, got)); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("unknown profile returns an empty series", func(t *testing.T) {
		if got := rec.Series("other", "ap-northeast-1", Range1Day.RecordedWindow(now)); len(got) != 0 {
			t.Errorf("series = %+v, want empty", got)
		}
	})

	t.Run("window includes both ends and drops points after the end", func(t *testing.T) {
		// 窓を確定した後に別リクエストの一覧取得が記録した点は、応答の窓 (X 軸の範囲) の外に
		// なるため返さない。終端ちょうどの点は窓に含める。
		late := NewEC2CountRecorder()
		late.Record("prod", "ap-northeast-1", 2, now.Add(-Range1Day.Duration()))
		late.Record("prod", "ap-northeast-1", 3, now)
		late.Record("prod", "ap-northeast-1", 4, now.Add(time.Millisecond))

		got := late.Series("prod", "ap-northeast-1", Range1Day.RecordedWindow(now))
		if diff := cmp.Diff([]float64{2, 3}, seriesValues(t, got)); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestEC2CountRingSizeCovers30Days はリングバッファが定期サンプリングだけで 30 日分を
// 保持できる大きさであることを検証する。ec2CountRingSize は定数式で求めているため、
// Range30Days.Duration() の値とずれていないことをテストで固定する。
func TestEC2CountRingSizeCovers30Days(t *testing.T) {
	samples := int(Range30Days.Duration() / EC2CountSampleInterval)
	if ec2CountRingSize < 2*samples {
		t.Errorf("ec2CountRingSize = %d, want >= %d (2 * %d samples in 30 days)", ec2CountRingSize, 2*samples, samples)
	}
}

func TestEC2CountRecorderKeys(t *testing.T) {
	rec := NewEC2CountRecorder()
	if got := rec.Keys(); len(got) != 0 {
		t.Fatalf("keys = %+v, want empty", got)
	}

	now := time.Now()
	rec.Record("prod", "ap-northeast-1", 1, now)
	rec.Record("prod", "us-east-1", 2, now)
	// 同じ組への追記で組が増えない。
	rec.Record("prod", "ap-northeast-1", 3, now)

	got := rec.Keys()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	seen := map[EC2CountTarget]bool{}
	for _, target := range got {
		seen[target] = true
	}
	for _, want := range []EC2CountTarget{
		{Profile: "prod", Region: "ap-northeast-1"},
		{Profile: "prod", Region: "us-east-1"},
	} {
		if !seen[want] {
			t.Errorf("keys missing %+v: %+v", want, got)
		}
	}
}

// TestEC2CountRecorderDropsOldest は上限到達時に最古の点が捨てられることを検証する。
func TestEC2CountRecorderDropsOldest(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	rec := NewEC2CountRecorder()

	total := ec2CountRingSize + 3
	for i := 0; i < total; i++ {
		rec.Record("prod", "ap-northeast-1", i, base.Add(time.Duration(i)*time.Millisecond))
	}

	got := rec.Series("prod", "ap-northeast-1", Range30Days.RecordedWindow(base.Add(time.Duration(total)*time.Millisecond)))
	if len(got) != ec2CountRingSize {
		t.Fatalf("len = %d, want %d", len(got), ec2CountRingSize)
	}
	// 残るのは新しい ec2CountRingSize 点で、古い順に並ぶ。
	if got[0].V == nil || *got[0].V != float64(total-ec2CountRingSize) {
		t.Errorf("oldest kept = %v, want %d", got[0].V, total-ec2CountRingSize)
	}
	last := got[len(got)-1]
	if last.V == nil || *last.V != float64(total-1) {
		t.Errorf("newest = %v, want %d", last.V, total-1)
	}
	for i := 1; i < len(got); i++ {
		if got[i].T < got[i-1].T {
			t.Fatalf("points are not in chronological order at %d", i)
		}
	}
}

// TestEC2CountRecorderConcurrent は複数 goroutine からの同時アクセスで競合しないことを検証する
// (go test -race で検出させる)。
func TestEC2CountRecorderConcurrent(t *testing.T) {
	rec := NewEC2CountRecorder()
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				rec.Record("prod", "ap-northeast-1", n, now)
				rec.Record("stg", "us-east-1", n, now)
			}
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				rec.Series("prod", "ap-northeast-1", Range1Day.RecordedWindow(now))
				rec.Series("stg", "us-east-1", Range30Days.RecordedWindow(now))
			}
		}()
	}
	wg.Wait()

	if got := len(rec.Series("prod", "ap-northeast-1", Range1Day.RecordedWindow(now))); got != 8*200 {
		t.Errorf("recorded = %d, want %d", got, 8*200)
	}
}
