package cache

import (
	"testing"
	"time"
)

func TestCacheInvalidatePrefix(t *testing.T) {
	tests := []struct {
		name       string
		seed       map[string]string
		prefix     string
		wantRemain []string
	}{
		{
			name: "removes matching prefix, keeps others",
			seed: map[string]string{
				"s3-objects:p:r:bucket:":     "a",
				"s3-objects:p:r:bucket:logs": "b",
				"s3-objects:p:r:other:":      "c",
			},
			prefix:     "s3-objects:p:r:bucket:",
			wantRemain: []string{"s3-objects:p:r:other:"},
		},
		{
			name: "no match leaves everything",
			seed: map[string]string{
				"a": "1",
				"b": "2",
			},
			prefix:     "zzz",
			wantRemain: []string{"a", "b"},
		},
		{
			name:       "empty cache is a no-op",
			seed:       map[string]string{},
			prefix:     "anything",
			wantRemain: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New[string](time.Minute)
			t.Cleanup(c.Close)
			for k, v := range tt.seed {
				c.Set(k, v, time.Minute)
			}

			c.InvalidatePrefix(tt.prefix)

			for _, k := range tt.wantRemain {
				if _, ok := c.Get(k); !ok {
					t.Errorf("expected key %q to remain, but it was removed", k)
				}
			}
			for k := range tt.seed {
				remains := false
				for _, want := range tt.wantRemain {
					if k == want {
						remains = true
						break
					}
				}
				if remains {
					continue
				}
				if _, ok := c.Get(k); ok {
					t.Errorf("expected key %q to be removed, but it remains", k)
				}
			}
		})
	}
}

func TestCacheInvalidateFunc(t *testing.T) {
	tests := []struct {
		name       string
		seed       map[string]string
		pred       func(key string) bool
		wantRemain []string
	}{
		{
			name: "述語に一致するエントリだけ削除される",
			seed: map[string]string{
				"ec2:prof:region":   "a",
				"gcp-gcs:proj":      "b",
				"cost:prof:region":  "c",
				"tidb-clusters:org": "d",
			},
			pred: func(key string) bool {
				return key == "ec2:prof:region" || key == "tidb-clusters:org"
			},
			wantRemain: []string{"gcp-gcs:proj", "cost:prof:region"},
		},
		{
			name:       "常に false の述語は何も削除しない",
			seed:       map[string]string{"ec2:prof:region": "a", "s3:prof:region": "b"},
			pred:       func(string) bool { return false },
			wantRemain: []string{"ec2:prof:region", "s3:prof:region"},
		},
		{
			name:       "常に true の述語は全エントリを削除する",
			seed:       map[string]string{"ec2:prof:region": "a", "s3:prof:region": "b"},
			pred:       func(string) bool { return true },
			wantRemain: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New[string](time.Minute)
			t.Cleanup(c.Close)
			for k, v := range tt.seed {
				c.Set(k, v, time.Minute)
			}

			c.InvalidateFunc(tt.pred)

			remain := make(map[string]bool, len(tt.wantRemain))
			for _, k := range tt.wantRemain {
				remain[k] = true
				if _, ok := c.Get(k); !ok {
					t.Errorf("expected key %q to remain, but it was removed", k)
				}
			}
			for k := range tt.seed {
				if remain[k] {
					continue
				}
				if _, ok := c.Get(k); ok {
					t.Errorf("expected key %q to be removed, but it remains", k)
				}
			}
		})
	}
}
