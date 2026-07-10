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
