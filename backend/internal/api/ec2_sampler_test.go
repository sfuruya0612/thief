package api

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// waitForSampler は条件が満たされるまで短い間隔で待つ。満たされなければテストを落とす。
func waitForSampler(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

// TestRunEC2CountSamplerRecordsOnEachTick はサンプラーが複数周期で記録し、ctx の
// キャンセルで goroutine が抜けることを検証する。
func TestRunEC2CountSamplerRecordsOnEachTick(t *testing.T) {
	s := newTestServer(t)
	s.ec2Counts.Record("prod", "ap-northeast-1", 0, time.Now())

	var mu sync.Mutex
	calls := 0
	s.ec2Resources = func(context.Context, string, string) ([]awsinternal.EC2Resource, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return []awsinternal.EC2Resource{{State: "running"}, {State: "running"}}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runEC2CountSampler(ctx, 10*time.Millisecond)
	}()

	waitForSampler(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return calls >= 3
	}, 2*time.Second)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sampler did not return after context cancellation")
	}

	points := s.ec2Counts.Series("prod", "ap-northeast-1", awsinternal.Range30Days.RecordedWindow(time.Now()))
	if len(points) < 4 { // 初回記録 1 点 + 3 周期分以上
		t.Fatalf("points = %d, want >= 4", len(points))
	}
	newest := points[len(points)-1]
	if newest.V == nil || *newest.V != 2 {
		t.Errorf("newest = %v, want 2", newest.V)
	}
}

// TestRunEC2CountSamplerReturnsWhenContextAlreadyCancelled はキャンセル済みの context を
// 渡すとサンプラーが即座に抜けることを検証する。
func TestRunEC2CountSamplerReturnsWhenContextAlreadyCancelled(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runEC2CountSampler(ctx, time.Hour)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sampler did not return for an already cancelled context")
	}
}

// TestSampleEC2CountsRecordsSuccessfulTargetsDespiteFailures は、失敗した組を記録せず、
// 成功した組の処理と次の周期を続けることを検証する。
func TestSampleEC2CountsRecordsSuccessfulTargetsDespiteFailures(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	s.ec2Counts.Record("good", "ap-northeast-1", 0, now)
	s.ec2Counts.Record("bad", "ap-northeast-1", 0, now)
	s.ec2Resources = func(_ context.Context, profile, _ string) ([]awsinternal.EC2Resource, error) {
		if profile == "bad" {
			return nil, errors.New("describe instances: access denied")
		}
		return []awsinternal.EC2Resource{{State: "running"}}, nil
	}

	// 2 周期分を回し、失敗した組があっても次の周期が続くことを見る。
	s.sampleEC2Counts(context.Background())
	s.sampleEC2Counts(context.Background())

	window := awsinternal.Range30Days.RecordedWindow(time.Now())
	good := s.ec2Counts.Series("good", "ap-northeast-1", window)
	if len(good) != 3 { // 初回記録 1 点 + 2 周期分
		t.Errorf("good points = %d, want 3", len(good))
	}
	if len(good) > 0 {
		newest := good[len(good)-1]
		if newest.V == nil || *newest.V != 1 {
			t.Errorf("good newest = %v, want 1", newest.V)
		}
	}
	bad := s.ec2Counts.Series("bad", "ap-northeast-1", window)
	if len(bad) != 1 { // 失敗した組は初回記録の 1 点だけ
		t.Errorf("bad points = %d, want 1 (failure must not record)", len(bad))
	}
}
