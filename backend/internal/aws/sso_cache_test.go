package aws

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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

func TestRemoveSSOTokenCache(t *testing.T) {
	const (
		startA = "https://a.awsapps.com/start"
		startB = "https://b.awsapps.com/start"
	)
	tokenJSON := func(startURL string) string {
		return `{"startUrl": "` + startURL + `", "accessToken": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`
	}

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
	remaining := func(t *testing.T, dir string) []string {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		return names
	}
	assertRemaining := func(t *testing.T, dir string, want ...string) {
		t.Helper()
		sort.Strings(want)
		got := remaining(t, dir)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("remaining files = %v, want %v", got, want)
		}
	}

	t.Run("removes only files matching the start url", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"a.json":   tokenJSON(startA),
			"b.json":   tokenJSON(startB),
			"reg.json": `{"clientId": "cid", "clientSecret": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`,
		})
		if err := RemoveSSOTokenCache(dir, startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir, "b.json", "reg.json")
	})

	t.Run("no match returns nil and keeps files", func(t *testing.T) {
		dir := writeCache(t, map[string]string{"b.json": tokenJSON(startB)})
		if err := RemoveSSOTokenCache(dir, startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir, "b.json")
	})

	t.Run("empty start url matches nothing", func(t *testing.T) {
		// 空の start URL を渡しても、他の start URL のトークンと startUrl を持たない
		// client registration (startUrl が空として読まれる) を巻き込まないことを保証する。
		dir := writeCache(t, map[string]string{
			"a.json":   tokenJSON(startA),
			"reg.json": `{"clientId": "c", "clientSecret": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`,
		})
		if err := RemoveSSOTokenCache(dir, ""); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir, "a.json", "reg.json")
	})

	t.Run("empty cache dir returns nil", func(t *testing.T) {
		if err := RemoveSSOTokenCache(t.TempDir(), startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
	})

	t.Run("client registration without startUrl is kept", func(t *testing.T) {
		// startUrl を持たないファイルは、他のフィールドが一致対象の URL を含んでも対象外。
		dir := writeCache(t, map[string]string{
			"reg.json": `{"clientId": "` + startA + `", "clientSecret": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`,
		})
		if err := RemoveSSOTokenCache(dir, startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir, "reg.json")
	})

	t.Run("non json files are kept", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"a.json": tokenJSON(startA),
			"a.txt":  tokenJSON(startA),
			"a":      tokenJSON(startA),
		})
		if err := RemoveSSOTokenCache(dir, startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir, "a", "a.txt")
	})

	t.Run("trailing slash difference still matches", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"slash.json":   tokenJSON(startA + "/"),
			"noslash.json": tokenJSON(startA),
		})
		if err := RemoveSSOTokenCache(dir, startA+"//"); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir)
	})

	t.Run("file removed by someone else before os.Remove is success", func(t *testing.T) {
		dir := writeCache(t, map[string]string{"a.json": tokenJSON(startA)})
		orig := ssoCacheRemove
		t.Cleanup(func() { ssoCacheRemove = orig })
		ssoCacheRemove = func(name string) error {
			// 走査と削除の間に別プロセスが消した状況。実際に消してから not-exist を返す。
			if err := os.Remove(name); err != nil {
				return err
			}
			return os.Remove(name)
		}
		if err := RemoveSSOTokenCache(dir, startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir)
	})

	t.Run("broken and oversized neighbours are kept", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"a.json":      tokenJSON(startA),
			"broken.json": `{not json`,
		})
		big := make([]byte, ssoCacheMaxFileSize+1)
		for i := range big {
			big[i] = ' '
		}
		if err := os.WriteFile(filepath.Join(dir, "big.json"), big, 0o600); err != nil {
			t.Fatalf("write big: %v", err)
		}
		if err := RemoveSSOTokenCache(dir, startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
		assertRemaining(t, dir, "big.json", "broken.json")
	})

	t.Run("missing cache dir returns nil", func(t *testing.T) {
		if err := RemoveSSOTokenCache(filepath.Join(t.TempDir(), "nope"), startA); err != nil {
			t.Fatalf("RemoveSSOTokenCache() = %v, want nil", err)
		}
	})

	t.Run("unreadable cache dir returns error", func(t *testing.T) {
		// ディレクトリではなく通常ファイルを cacheDir に指定して ReadDir を
		// 失敗させる (readSSOCacheStatuses のテストと同じ、permission に依存しない再現方法)。
		dir := t.TempDir()
		file := filepath.Join(dir, "file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		err := RemoveSSOTokenCache(file, startA)
		if err == nil {
			t.Fatal("RemoveSSOTokenCache() = nil, want error for unreadable dir")
		}
		if errors.Is(err, os.ErrNotExist) {
			t.Errorf("RemoveSSOTokenCache() = %v, want an error other than not-exist", err)
		}
	})

	t.Run("continues after a failed removal and reports every failure", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"first.json":  tokenJSON(startA),
			"second.json": tokenJSON(startA + "/"),
		})
		orig := ssoCacheRemove
		t.Cleanup(func() { ssoCacheRemove = orig })
		errFirst := errors.New("first removal failed")
		ssoCacheRemove = func(name string) error {
			if filepath.Base(name) == "first.json" {
				return errFirst
			}
			return os.Remove(name)
		}
		err := RemoveSSOTokenCache(dir, startA)
		if !errors.Is(err, errFirst) {
			t.Fatalf("RemoveSSOTokenCache() = %v, want wrapping %v", err, errFirst)
		}
		if !strings.Contains(err.Error(), "first.json") {
			t.Errorf("error %q does not name the failed file", err)
		}
		// 1 件目の失敗で止まらず 2 件目は削除されている。
		assertRemaining(t, dir, "first.json")
	})

	t.Run("reports every failure when all removals fail", func(t *testing.T) {
		dir := writeCache(t, map[string]string{
			"first.json":  tokenJSON(startA),
			"second.json": tokenJSON(startA),
		})
		orig := ssoCacheRemove
		t.Cleanup(func() { ssoCacheRemove = orig })
		errFirst := errors.New("first removal failed")
		errSecond := errors.New("second removal failed")
		ssoCacheRemove = func(name string) error {
			if filepath.Base(name) == "first.json" {
				return errFirst
			}
			return errSecond
		}
		err := RemoveSSOTokenCache(dir, startA)
		if !errors.Is(err, errFirst) || !errors.Is(err, errSecond) {
			t.Fatalf("RemoveSSOTokenCache() = %v, want wrapping both %v and %v", err, errFirst, errSecond)
		}
		assertRemaining(t, dir, "first.json", "second.json")
	})
}

// ssoCacheTestDir は files をファイル名 → 内容として一時ディレクトリに書き、そのパスを返す。
func ssoCacheTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// ssoCacheRemainingFiles は dir 直下のエントリ名を辞書順で返す。
func ssoCacheRemainingFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestListSSOTokenCache(t *testing.T) {
	const (
		startA = "https://a.awsapps.com/start"
		startB = "https://b.awsapps.com/start"
	)
	tokenJSON := func(startURL, region, token string) string {
		return `{"startUrl": "` + startURL + `", "region": "` + region + `", "accessToken": "` + token + `", "expiresAt": "2027-01-01T00:00:00Z"}`
	}
	const regJSON = `{"clientId": "cid", "clientSecret": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`

	t.Run("returns only tokens matching the start url", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{
			"a.json":   tokenJSON(startA, "ap-northeast-1", "tok-a"),
			"b.json":   tokenJSON(startB, "us-east-1", "tok-b"),
			"reg.json": regJSON,
		})
		got, err := ListSSOTokenCache(dir, startA)
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v", err)
		}
		want := []SSOCachedToken{{FileName: "a.json", StartURL: startA, Region: "ap-northeast-1", AccessToken: "tok-a", ExpiresAt: "2027-01-01T00:00:00Z"}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("tokens mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("no match returns empty non-nil slice", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{"b.json": tokenJSON(startB, "us-east-1", "tok-b")})
		got, err := ListSSOTokenCache(dir, startA)
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("ListSSOTokenCache() = %#v, want empty non-nil slice", got)
		}
	})

	t.Run("empty start url returns every token in file name order", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{
			"b.json":   tokenJSON(startB, "us-east-1", "tok-b"),
			"a.json":   tokenJSON(startA, "ap-northeast-1", "tok-a"),
			"reg.json": regJSON,
		})
		got, err := ListSSOTokenCache(dir, "")
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v", err)
		}
		want := []SSOCachedToken{
			{FileName: "a.json", StartURL: startA, Region: "ap-northeast-1", AccessToken: "tok-a", ExpiresAt: "2027-01-01T00:00:00Z"},
			{FileName: "b.json", StartURL: startB, Region: "us-east-1", AccessToken: "tok-b", ExpiresAt: "2027-01-01T00:00:00Z"},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("tokens mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("client registration without startUrl is excluded even for empty start url", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{"reg.json": regJSON})
		got, err := ListSSOTokenCache(dir, "")
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ListSSOTokenCache() = %+v, want no tokens", got)
		}
	})

	t.Run("trailing slash difference still matches", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{
			"slash.json":   tokenJSON(startA+"/", "ap-northeast-1", "tok-1"),
			"noslash.json": tokenJSON(startA, "ap-northeast-1", "tok-2"),
		})
		got, err := ListSSOTokenCache(dir, startA+"//")
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v", err)
		}
		if len(got) != 2 {
			t.Errorf("ListSSOTokenCache() returned %d tokens, want 2: %+v", len(got), got)
		}
	})

	t.Run("missing region and expiresAt are returned empty", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{
			"a.json": `{"startUrl": "` + startA + `", "accessToken": "tok-a"}`,
		})
		got, err := ListSSOTokenCache(dir, startA)
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v", err)
		}
		want := []SSOCachedToken{{FileName: "a.json", StartURL: startA, AccessToken: "tok-a"}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("tokens mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("unreadable oversized and broken files are skipped with a warning", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires extra privileges on windows")
		}
		dir := ssoCacheTestDir(t, map[string]string{
			"a.json":      tokenJSON(startA, "ap-northeast-1", "tok-a"),
			"broken.json": `{not json`,
			"a.txt":       tokenJSON(startA, "ap-northeast-1", "tok-txt"),
		})
		big := make([]byte, ssoCacheMaxFileSize+1)
		for i := range big {
			big[i] = ' '
		}
		if err := os.WriteFile(filepath.Join(dir, "big.json"), big, 0o600); err != nil {
			t.Fatalf("write big: %v", err)
		}
		// 読めないファイルは、リンク先の無いシンボリックリンクで再現する。パーミッション
		// ビットは root や一部のマウントで効かないため使わない (他の unreadable 系テストが
		// 通常ファイルを cacheDir にして os.ReadDir を失敗させるのと同じ理由)。
		if err := os.Symlink(filepath.Join(dir, "missing-target.json"), filepath.Join(dir, "dangling.json")); err != nil {
			t.Fatalf("symlink dangling: %v", err)
		}
		logs := captureDefaultLogs(t)
		got, err := ListSSOTokenCache(dir, startA)
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v", err)
		}
		if len(got) != 1 || got[0].FileName != "a.json" {
			t.Errorf("ListSSOTokenCache() = %+v, want only a.json", got)
		}
		for _, want := range []string{"skip oversized sso cache file", "read sso cache file failed", "parse sso cache file failed"} {
			if !strings.Contains(logs.String(), want) {
				t.Errorf("logs do not contain %q: %s", want, logs.String())
			}
		}
	})

	t.Run("missing cache dir returns empty slice and nil", func(t *testing.T) {
		got, err := ListSSOTokenCache(filepath.Join(t.TempDir(), "nope"), startA)
		if err != nil {
			t.Fatalf("ListSSOTokenCache() error = %v, want nil", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("ListSSOTokenCache() = %#v, want empty non-nil slice", got)
		}
	})

	t.Run("unreadable cache dir returns error", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ListSSOTokenCache(file, startA)
		if err == nil {
			t.Fatal("ListSSOTokenCache() = nil error, want error for unreadable dir")
		}
		if errors.Is(err, os.ErrNotExist) {
			t.Errorf("ListSSOTokenCache() = %v, want an error other than not-exist", err)
		}
		if got != nil {
			t.Errorf("ListSSOTokenCache() = %+v, want nil on error", got)
		}
	})
}

func TestRemoveAllSSOCache(t *testing.T) {
	const tokenJSON = `{"startUrl": "https://a.awsapps.com/start", "accessToken": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`
	const regJSON = `{"clientId": "cid", "clientSecret": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`

	t.Run("removes every regular file regardless of extension", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{
			"a.json":   tokenJSON,
			"reg.json": regJSON,
			"a.txt":    tokenJSON,
			"noext":    "x",
			"broken":   `{not json`,
		})
		if err := RemoveAllSSOCache(dir); err != nil {
			t.Fatalf("RemoveAllSSOCache() = %v, want nil", err)
		}
		if got := ssoCacheRemainingFiles(t, dir); len(got) != 0 {
			t.Errorf("remaining files = %v, want none", got)
		}
	})

	t.Run("keeps subdirectories and symlinks with a warning", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires elevated privileges on windows")
		}
		dir := ssoCacheTestDir(t, map[string]string{"a.json": tokenJSON})
		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sub", "nested.json"), []byte(tokenJSON), 0o600); err != nil {
			t.Fatalf("write nested: %v", err)
		}
		if err := os.Symlink(filepath.Join(dir, "sub", "nested.json"), filepath.Join(dir, "link.json")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		logs := captureDefaultLogs(t)
		if err := RemoveAllSSOCache(dir); err != nil {
			t.Fatalf("RemoveAllSSOCache() = %v, want nil", err)
		}
		if diff := cmp.Diff([]string{"link.json", "sub"}, ssoCacheRemainingFiles(t, dir)); diff != "" {
			t.Errorf("remaining files mismatch (-want +got):\n%s", diff)
		}
		if _, err := os.Stat(filepath.Join(dir, "sub", "nested.json")); err != nil {
			t.Errorf("nested file must be kept: %v", err)
		}
		assertWarnLogLines(t, logs.String(), "sso cache entry skipped", [][]string{
			{"name=link.json"},
			{"name=sub"},
		})
	})

	t.Run("missing cache dir returns nil", func(t *testing.T) {
		if err := RemoveAllSSOCache(filepath.Join(t.TempDir(), "nope")); err != nil {
			t.Fatalf("RemoveAllSSOCache() = %v, want nil", err)
		}
	})

	t.Run("empty cache dir returns nil", func(t *testing.T) {
		if err := RemoveAllSSOCache(t.TempDir()); err != nil {
			t.Fatalf("RemoveAllSSOCache() = %v, want nil", err)
		}
	})

	t.Run("unreadable cache dir returns error", func(t *testing.T) {
		// ディレクトリではなく通常ファイルを cacheDir に指定して ReadDir を失敗させる
		// (RemoveSSOTokenCache のテストと同じ、permission に依存しない再現方法)。
		dir := t.TempDir()
		file := filepath.Join(dir, "file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		err := RemoveAllSSOCache(file)
		if err == nil {
			t.Fatal("RemoveAllSSOCache() = nil, want error for unreadable dir")
		}
		if errors.Is(err, os.ErrNotExist) {
			t.Errorf("RemoveAllSSOCache() = %v, want an error other than not-exist", err)
		}
	})

	t.Run("file removed by someone else before os.Remove is success", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{"a.json": tokenJSON})
		orig := ssoCacheRemove
		t.Cleanup(func() { ssoCacheRemove = orig })
		ssoCacheRemove = func(name string) error {
			if err := os.Remove(name); err != nil {
				return err
			}
			return os.Remove(name)
		}
		if err := RemoveAllSSOCache(dir); err != nil {
			t.Fatalf("RemoveAllSSOCache() = %v, want nil", err)
		}
	})

	t.Run("continues after a failed removal and reports every failure", func(t *testing.T) {
		dir := ssoCacheTestDir(t, map[string]string{
			"first.json":  tokenJSON,
			"second.json": tokenJSON,
			"third.txt":   "x",
		})
		orig := ssoCacheRemove
		t.Cleanup(func() { ssoCacheRemove = orig })
		errFirst := errors.New("first removal failed")
		errThird := errors.New("third removal failed")
		ssoCacheRemove = func(name string) error {
			switch filepath.Base(name) {
			case "first.json":
				return errFirst
			case "third.txt":
				return errThird
			}
			return os.Remove(name)
		}
		err := RemoveAllSSOCache(dir)
		if !errors.Is(err, errFirst) || !errors.Is(err, errThird) {
			t.Fatalf("RemoveAllSSOCache() = %v, want wrapping both %v and %v", err, errFirst, errThird)
		}
		for _, name := range []string{"first.json", "third.txt"} {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not name the failed file %s", err, name)
			}
		}
		if diff := cmp.Diff([]string{"first.json", "third.txt"}, ssoCacheRemainingFiles(t, dir)); diff != "" {
			t.Errorf("remaining files mismatch (-want +got):\n%s", diff)
		}
	})
}
