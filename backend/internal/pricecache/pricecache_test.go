package pricecache

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateService(t *testing.T) {
	tests := []struct {
		name    string
		service string
		wantErr error
	}{
		{name: "ec2", service: "ec2", wantErr: nil},
		{name: "rds", service: "rds", wantErr: nil},
		{name: "elasticache", service: "elasticache", wantErr: nil},
		{name: "ecs", service: "ecs", wantErr: nil},
		{name: "compute-sp", service: "compute-sp", wantErr: nil},
		{name: "ec2-instance-sp", service: "ec2-instance-sp", wantErr: nil},
		{name: "database-sp", service: "database-sp", wantErr: nil},
		// issue 0056: ec2-spot はライブ取得専用でディスクキャッシュ (Load/Save) を
		// 経由しないため、意図的に validServices へ加えない (非対称)。
		{name: "ec2-spot is intentionally not a disk-cached service", service: "ec2-spot", wantErr: ErrInvalidService},
		{name: "unknown", service: "s3", wantErr: ErrInvalidService},
		{name: "empty", service: "", wantErr: ErrInvalidService},
		{name: "path traversal", service: "../etc", wantErr: ErrInvalidService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateService(tt.service)
			if !errors.Is(err, tt.wantErr) && (err == nil) != (tt.wantErr == nil) {
				t.Errorf("ValidateService(%q) = %v, want %v", tt.service, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRegion(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		wantErr error
	}{
		{name: "valid", region: "ap-northeast-1", wantErr: nil},
		{name: "empty", region: "", wantErr: ErrInvalidRegion},
		{name: "uppercase rejected", region: "AP-Northeast-1", wantErr: ErrInvalidRegion},
		{name: "path traversal", region: "../../etc", wantErr: ErrInvalidRegion},
		{name: "path separator", region: "us/east-1", wantErr: ErrInvalidRegion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegion(tt.region)
			if !errors.Is(err, tt.wantErr) && (err == nil) != (tt.wantErr == nil) {
				t.Errorf("ValidateRegion(%q) = %v, want %v", tt.region, err, tt.wantErr)
			}
		})
	}
}

func TestLoadMissingFileIsMiss(t *testing.T) {
	dir := t.TempDir()
	data, fetchedAt, ok, err := Load(dir, "ec2", "ap-northeast-1")
	if err != nil {
		t.Fatalf("Load() err = %v, want nil", err)
	}
	if ok {
		t.Fatalf("Load() ok = true, want false for missing file")
	}
	if data != nil {
		t.Errorf("Load() data = %v, want nil", data)
	}
	if !fetchedAt.IsZero() {
		t.Errorf("Load() fetchedAt = %v, want zero", fetchedAt)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := []byte(`{"service":"ec2","region":"ap-northeast-1","rates":[]}`)
	fetchedAt := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)

	if err := Save(dir, "ec2", "ap-northeast-1", want, fetchedAt); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	got, gotFetchedAt, ok, err := Load(dir, "ec2", "ap-northeast-1")
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}
	if !ok {
		t.Fatalf("Load() ok = false, want true after Save")
	}
	if string(got) != string(want) {
		t.Errorf("Load() data = %s, want %s", got, want)
	}
	if !gotFetchedAt.Equal(fetchedAt) {
		t.Errorf("Load() fetchedAt = %v, want %v", gotFetchedAt, fetchedAt)
	}

	// atomic write: no leftover temp files in the service directory.
	entries, err := os.ReadDir(filepath.Join(dir, "ec2"))
	if err != nil {
		t.Fatalf("ReadDir() err = %v", err)
	}
	for _, e := range entries {
		if e.Name() != "ap-northeast-1.json" {
			t.Errorf("unexpected leftover file %q in cache dir", e.Name())
		}
	}
}

func TestSaveInvalidServiceOrRegion(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "bogus", "ap-northeast-1", []byte("{}"), time.Now()); !errors.Is(err, ErrInvalidService) {
		t.Errorf("Save() with bad service err = %v, want ErrInvalidService", err)
	}
	if err := Save(dir, "ec2", "../etc", []byte("{}"), time.Now()); !errors.Is(err, ErrInvalidRegion) {
		t.Errorf("Save() with bad region err = %v, want ErrInvalidRegion", err)
	}
}

func TestLoadCorruptFileIsMiss(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "rds")
	if err := os.MkdirAll(svcDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() err = %v", err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "us-east-1.json"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("WriteFile() err = %v", err)
	}

	_, _, ok, err := Load(dir, "rds", "us-east-1")
	if err != nil {
		t.Fatalf("Load() err = %v, want nil (corrupt file must degrade to a miss, not an error)", err)
	}
	if ok {
		t.Fatalf("Load() ok = true, want false for corrupt file")
	}
}

func TestLoadIncompleteFileIsMiss(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing fetched_at", content: `{"data":{"service":"ec2"}}`},
		{name: "missing data", content: `{"fetched_at":"2026-07-18T09:00:00Z"}`},
		{name: "empty data object", content: `{"fetched_at":"2026-07-18T09:00:00Z","data":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			svcDir := filepath.Join(dir, "ecs")
			if err := os.MkdirAll(svcDir, 0o700); err != nil {
				t.Fatalf("MkdirAll() err = %v", err)
			}
			if err := os.WriteFile(filepath.Join(svcDir, "ap-northeast-1.json"), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("WriteFile() err = %v", err)
			}
			_, _, ok, err := Load(dir, "ecs", "ap-northeast-1")
			if err != nil {
				t.Fatalf("Load() err = %v, want nil", err)
			}
			if ok {
				t.Fatalf("Load() ok = true, want false for incomplete file %q", tt.content)
			}
		})
	}
}

func TestSaveOverwritesCorruptFile(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "elasticache")
	if err := os.MkdirAll(svcDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() err = %v", err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "ap-northeast-1.json"), []byte("garbage"), 0o600); err != nil {
		t.Fatalf("WriteFile() err = %v", err)
	}

	want := []byte(`{"service":"elasticache"}`)
	if err := Save(dir, "elasticache", "ap-northeast-1", want, time.Now()); err != nil {
		t.Fatalf("Save() err = %v", err)
	}
	got, _, ok, err := Load(dir, "elasticache", "ap-northeast-1")
	if err != nil || !ok {
		t.Fatalf("Load() after overwrite = (%s, %v, %v), want valid data", got, ok, err)
	}
	if string(got) != string(want) {
		t.Errorf("Load() data = %s, want %s", got, want)
	}
}

func TestFetchDedupesConcurrentCalls(t *testing.T) {
	dir := t.TempDir()
	var calls int64
	var wg sync.WaitGroup
	release := make(chan struct{})

	loader := func() ([]byte, error) {
		atomic.AddInt64(&calls, 1)
		<-release
		return []byte("result"), nil
	}

	const n = 5
	results := make([][]byte, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data, err := Fetch(dir, "ec2", "concurrency-test", loader)
			if err != nil {
				t.Errorf("Fetch() err = %v", err)
				return
			}
			results[i] = data
		}(i)
	}

	// give goroutines a chance to all enter singleflight before releasing the loader.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("loader called %d times, want exactly 1 (singleflight should dedupe)", got)
	}
	for i, r := range results {
		if string(r) != "result" {
			t.Errorf("results[%d] = %q, want %q", i, r, "result")
		}
	}
}

func TestFetchPropagatesLoaderError(t *testing.T) {
	dir := t.TempDir()
	wantErr := errors.New("boom")
	_, err := Fetch(dir, "ec2", "error-test", func() ([]byte, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Fetch() err = %v, want %v", err, wantErr)
	}
}
