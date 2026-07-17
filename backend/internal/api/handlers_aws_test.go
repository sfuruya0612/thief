package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestHandleListProfiles は /api/aws/profiles の JSON 契約を検証する。
// ListProfiles は os.UserHomeDir 経由で ~/.aws を読むため、t.Setenv で HOME を
// 差し替えて fixture を注入する (このため t.Parallel とは併用できない)。
func TestHandleListProfiles(t *testing.T) {
	writeHome := func(t *testing.T, files map[string]string) string {
		t.Helper()
		home := t.TempDir()
		for rel, content := range files {
			path := filepath.Join(home, rel)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", rel, err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write %s: %v", rel, err)
			}
		}
		return home
	}

	do := func(t *testing.T) []map[string]any {
		t.Helper()
		s := newTestServer(t)
		r := httptest.NewRequest(http.MethodGet, "/api/aws/profiles", nil)
		w := httptest.NewRecorder()
		s.handleListProfiles(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
		}
		var body []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		return body
	}

	t.Run("sso and access key profiles", func(t *testing.T) {
		home := writeHome(t, map[string]string{
			".aws/config": `[profile sso-prof]
sso_session = org
sso_account_id = 111111111111
sso_role_name = AdministratorAccess
region = ap-northeast-1

[sso-session org]
sso_start_url = https://org.awsapps.com/start
sso_region = us-east-1
`,
			".aws/credentials": "[static]\naws_access_key_id = AKIAEXAMPLE\n",
			".aws/sso/cache/token.json": `{"startUrl": "https://org.awsapps.com/start",` +
				` "accessToken": "REDACTED", "expiresAt": "2099-01-02T03:04:05Z"}`,
		})
		t.Setenv("HOME", home)

		body := do(t)
		want := []map[string]any{
			{
				"name":           "sso-prof",
				"account_id":     "111111111111",
				"sso_role_name":  "AdministratorAccess",
				"region":         "ap-northeast-1",
				"auth_type":      "sso",
				"sso_status":     "valid",
				"sso_expires_at": "2099-01-02T03:04:05Z",
			},
			{
				"name":      "static",
				"auth_type": "access_key",
			},
		}
		if diff := cmp.Diff(want, body); diff != "" {
			t.Errorf("body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("cache read failure still returns 200 without sso_status", func(t *testing.T) {
		home := writeHome(t, map[string]string{
			".aws/config": `[profile sso-prof]
sso_start_url = https://org.awsapps.com/start
`,
			// sso/cache をディレクトリではなく通常ファイルにして ReadDir を失敗させる。
			".aws/sso/cache": "not a directory",
		})
		t.Setenv("HOME", home)

		body := do(t)
		want := []map[string]any{
			{"name": "sso-prof", "auth_type": "sso"},
		}
		if diff := cmp.Diff(want, body); diff != "" {
			t.Errorf("body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("no config files returns empty list", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		body := do(t)
		if len(body) != 0 {
			t.Errorf("body = %v, want empty", body)
		}
	})
}
