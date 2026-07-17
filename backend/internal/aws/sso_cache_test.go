package aws

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestNormalizeStartURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no slash", in: "https://x.awsapps.com/start", want: "https://x.awsapps.com/start"},
		{name: "trailing slash", in: "https://x.awsapps.com/start/", want: "https://x.awsapps.com/start"},
		{name: "double trailing slash", in: "https://x.awsapps.com/start//", want: "https://x.awsapps.com/start"},
		{name: "surrounding spaces", in: "  https://x.awsapps.com/start/ ", want: "https://x.awsapps.com/start"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeStartURL(tt.in); got != tt.want {
				t.Errorf("normalizeStartURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadSSOCacheStatuses(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	future := now.Add(2 * time.Hour)
	past := now.Add(-2 * time.Hour)
	jst := time.FixedZone("JST", 9*60*60)

	writeCache := func(t *testing.T, files map[string]string) string {
		t.Helper()
		dir := t.TempDir()
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		return dir
	}

	t.Run("valid and expired tokens", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"a.json": `{"startUrl": "https://a.awsapps.com/start", "accessToken": "REDACTED", "expiresAt": "` + future.Format(time.RFC3339) + `"}`,
			"b.json": `{"startUrl": "https://b.awsapps.com/start/", "accessToken": "REDACTED", "expiresAt": "` + past.Format(time.RFC3339) + `"}`,
		})
		got, ok := readSSOCacheStatuses(dir, now)
		if !ok {
			t.Fatal("readSSOCacheStatuses() ok = false, want true")
		}
		want := map[string]ssoCacheStatus{
			"https://a.awsapps.com/start": {Status: SSOStatusValid, ExpiresAt: future},
			// trailing slash は正規化されてキーから消える。
			"https://b.awsapps.com/start": {Status: SSOStatusExpired, ExpiresAt: past},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("readSSOCacheStatuses() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("offset timezone is parsed", func(t *testing.T) {
		futureJST := future.In(jst)
		dir := writeCache(t, map[string]string{
			"a.json": `{"startUrl": "https://a.awsapps.com/start", "expiresAt": "` + futureJST.Format(time.RFC3339) + `"}`,
		})
		got, _ := readSSOCacheStatuses(dir, now)
		st := got["https://a.awsapps.com/start"]
		if st.Status != SSOStatusValid {
			t.Errorf("Status = %q, want valid", st.Status)
		}
		if !st.ExpiresAt.Equal(future) {
			t.Errorf("ExpiresAt = %v, want %v", st.ExpiresAt, future)
		}
	})

	t.Run("legacy botocore expiresAt format degrades to expired", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"a.json": `{"startUrl": "https://a.awsapps.com/start", "expiresAt": "2020-06-14T05:26:13UTC"}`,
		})
		got, _ := readSSOCacheStatuses(dir, now)
		st, ok := got["https://a.awsapps.com/start"]
		if !ok {
			t.Fatal("entry not found")
		}
		if st.Status != SSOStatusExpired {
			t.Errorf("Status = %q, want expired", st.Status)
		}
		if !st.ExpiresAt.IsZero() {
			t.Errorf("ExpiresAt = %v, want zero", st.ExpiresAt)
		}
	})

	t.Run("registration and broken files are skipped", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			// client registration: startUrl を持たないが expiresAt は持つ。
			"reg.json":    `{"clientId": "cid", "clientSecret": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`,
			"broken.json": `{not json`,
			"note.txt":    `not a cache file`,
		})
		got, ok := readSSOCacheStatuses(dir, now)
		if !ok {
			t.Fatal("readSSOCacheStatuses() ok = false, want true")
		}
		if len(got) != 0 {
			t.Errorf("readSSOCacheStatuses() = %v, want empty", got)
		}
	})

	t.Run("subdirectory is skipped", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "sub.json"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		got, ok := readSSOCacheStatuses(dir, now)
		if !ok {
			t.Fatal("readSSOCacheStatuses() ok = false, want true")
		}
		if len(got) != 0 {
			t.Errorf("readSSOCacheStatuses() = %v, want empty", got)
		}
	})

	t.Run("oversized file is skipped", func(t *testing.T) {
		dir := t.TempDir()
		big := make([]byte, ssoCacheMaxFileSize+1)
		for i := range big {
			big[i] = ' '
		}
		if err := os.WriteFile(filepath.Join(dir, "big.json"), big, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, _ := readSSOCacheStatuses(dir, now)
		if len(got) != 0 {
			t.Errorf("readSSOCacheStatuses() = %v, want empty", got)
		}
	})

	t.Run("same startUrl keeps the latest expiry", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"old.json": `{"startUrl": "https://a.awsapps.com/start", "expiresAt": "` + past.Format(time.RFC3339) + `"}`,
			"new.json": `{"startUrl": "https://a.awsapps.com/start/", "expiresAt": "` + future.Format(time.RFC3339) + `"}`,
		})
		got, _ := readSSOCacheStatuses(dir, now)
		st := got["https://a.awsapps.com/start"]
		if st.Status != SSOStatusValid || !st.ExpiresAt.Equal(future) {
			t.Errorf("got %+v, want valid/%v", st, future)
		}
	})

	t.Run("missing dir is ok (not logged in)", func(t *testing.T) {
		got, ok := readSSOCacheStatuses(filepath.Join(t.TempDir(), "nope"), now)
		if !ok {
			t.Fatal("readSSOCacheStatuses() ok = false, want true for not-exist")
		}
		if len(got) != 0 {
			t.Errorf("readSSOCacheStatuses() = %v, want empty", got)
		}
	})

	t.Run("unreadable dir reports not readable", func(t *testing.T) {
		// ディレクトリではなく通常ファイルを cacheDir に指定して ReadDir を
		// 失敗させる (permission に依存しないポータブルな再現方法)。
		dir := t.TempDir()
		file := filepath.Join(dir, "file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, ok := readSSOCacheStatuses(file, now)
		if ok {
			t.Fatal("readSSOCacheStatuses() ok = true, want false for unreadable dir")
		}
		if len(got) != 0 {
			t.Errorf("readSSOCacheStatuses() = %v, want empty", got)
		}
	})
}
