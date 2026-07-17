package bigquery

import (
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
)

func TestQueryJobStatusFrom(t *testing.T) {
	start := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	end := start.Add(2300 * time.Millisecond)

	tests := []struct {
		name   string
		status *bigquery.JobStatus
		want   *QueryJobStatus
	}{
		{
			name:   "nil status falls back to pending",
			status: nil,
			want:   &QueryJobStatus{JobID: "j1", Location: "US", State: "PENDING"},
		},
		{
			name: "done with statistics",
			status: &bigquery.JobStatus{
				State: bigquery.Done,
				Statistics: &bigquery.JobStatistics{
					StartTime:           start,
					EndTime:             end,
					TotalBytesProcessed: 1_240_000_000,
					Details:             &bigquery.QueryStatistics{CacheHit: true},
				},
			},
			want: &QueryJobStatus{
				JobID:               "j1",
				Location:            "US",
				State:               "DONE",
				StartTime:           "2026-07-16T12:00:00Z",
				EndTime:             "2026-07-16T12:00:02Z",
				ElapsedMs:           2300,
				TotalBytesProcessed: 1_240_000_000,
				CacheHit:            true,
			},
		},
		{
			name: "failed job surfaces first error",
			status: &bigquery.JobStatus{
				State:  bigquery.Done,
				Errors: []*bigquery.Error{{Reason: "invalidQuery", Message: "syntax error"}},
			},
			want: &QueryJobStatus{
				JobID:        "j1",
				Location:     "US",
				State:        "DONE",
				ErrorMessage: (&bigquery.Error{Reason: "invalidQuery", Message: "syntax error"}).Error(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryJobStatusFrom("j1", "US", tt.status)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("queryJobStatusFrom mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestQueryJobStatusFromRunningElapsed(t *testing.T) {
	// 実行中 (EndTime 未確定) は現在時刻までの経過時間を返すため、正の値のみ検証する。
	status := &bigquery.JobStatus{
		State: bigquery.Running,
		Statistics: &bigquery.JobStatistics{
			StartTime: time.Now().Add(-3 * time.Second),
		},
	}
	got := queryJobStatusFrom("j1", "US", status)
	if got.State != "RUNNING" {
		t.Errorf("State = %q, want RUNNING", got.State)
	}
	if got.ElapsedMs < 3000 {
		t.Errorf("ElapsedMs = %d, want >= 3000", got.ElapsedMs)
	}
	if got.EndTime != "" {
		t.Errorf("EndTime = %q, want empty while running", got.EndTime)
	}
}

func TestJobStateString(t *testing.T) {
	tests := []struct {
		name   string
		status *bigquery.JobStatus
		want   string
	}{
		{name: "nil", status: nil, want: "PENDING"},
		{name: "pending", status: &bigquery.JobStatus{State: bigquery.Pending}, want: "PENDING"},
		{name: "running", status: &bigquery.JobStatus{State: bigquery.Running}, want: "RUNNING"},
		{name: "done", status: &bigquery.JobStatus{State: bigquery.Done}, want: "DONE"},
		{name: "unspecified", status: &bigquery.JobStatus{State: bigquery.StateUnspecified}, want: "PENDING"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobStateString(tt.status); got != tt.want {
				t.Errorf("jobStateString = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValueString(t *testing.T) {
	tests := []struct {
		name string
		in   bigquery.Value
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "string", in: "abc", want: "abc"},
		{name: "int", in: int64(42), want: "42"},
		{name: "float", in: 1.5, want: "1.5"},
		{name: "bool", in: true, want: "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := valueString(tt.in); got != tt.want {
				t.Errorf("valueString(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
