package tui

import "testing"

// TestFormatCount は Web UI の toLocaleString() と同じ "," 区切りになることを確かめる。
func TestFormatCount(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{-1234, "-1,234"},
	}
	for _, tt := range tests {
		if got := formatCount(tt.n); got != tt.want {
			t.Errorf("formatCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// TestFormatBytes は Web UI の formatBytes (columns.tsx) と同じ文字列になることを確かめる。
// 1280 は toFixed(1) が 0.5 を切り上げるので "1.3 KB" になる。
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{-1, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1280, "1.3 KB"},
		{1048576, "1.0 MB"},
		{1099511627776, "1.0 TB"},
		{1099511627776 * 2048, "2048.0 TB"},
	}
	for _, tt := range tests {
		if got := formatBytes(tt.n); got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatInt32(t *testing.T) {
	if got := formatInt32(0); got != "0" {
		t.Errorf("formatInt32(0) = %q, want 0", got)
	}
	if got := formatInt32(3306); got != "3306" {
		t.Errorf("formatInt32(3306) = %q, want 3306 (no separator)", got)
	}
}

// TestFetchFailed は取得失敗の表記が、値が無いとき "fetch failed"、あるとき "<value> (fetch failed)" になることを確かめる。
func TestFetchFailed(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{"", "fetch failed"},
		{"0", "0 (fetch failed)"},
		{"3", "3 (fetch failed)"},
		{"disabled", "disabled (fetch failed)"},
	}
	for _, tt := range tests {
		if got := fetchFailed(tt.value); got != tt.want {
			t.Errorf("fetchFailed(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}
