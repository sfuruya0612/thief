package datadogauth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestValidateSite(t *testing.T) {
	tests := []struct {
		name string
		site string
		want bool
	}{
		{name: "datadoghq.com", site: "datadoghq.com", want: true},
		{name: "regional site", site: "us3.datadoghq.com", want: true},
		{name: "eu site", site: "datadoghq.eu", want: true},
		{name: "hyphenated site", site: "ddog-gov.com", want: true},
		{name: "empty", site: "", want: false},
		{name: "no dot", site: "datadoghq", want: false},
		{name: "path traversal", site: "../../etc/passwd", want: false},
		{name: "parent directory", site: "..", want: false},
		{name: "path separator", site: "datadoghq.com/../evil", want: false},
		{name: "backslash", site: `datadoghq.com\evil`, want: false},
		{name: "uppercase", site: "DataDogHQ.com", want: false},
		{name: "leading dot", site: ".datadoghq.com", want: false},
		{name: "trailing dot", site: "datadoghq.com.", want: false},
		{name: "double dot", site: "datadoghq..com", want: false},
		{name: "leading hyphen", site: "-datadoghq.com", want: false},
		{name: "space", site: "datadog hq.com", want: false},
		{name: "null byte", site: "datadoghq.com\x00", want: false},
		{name: "too long", site: strings.Repeat("a", 250) + ".com", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSite(tt.site)
			if tt.want && err != nil {
				t.Errorf("ValidateSite(%q) err = %v, want nil", tt.site, err)
			}
			if !tt.want && !errors.Is(err, ErrInvalidSite) {
				t.Errorf("ValidateSite(%q) err = %v, want %v", tt.site, err, ErrInvalidSite)
			}
		})
	}
}

// TestValidateOrg は org のバリデーションを網羅する。org は保存先のファイル名に
// 組み込まれるため、パス区切りや ".." を通してはならない。空文字は親組織を表す
// 正当な値である。
func TestValidateOrg(t *testing.T) {
	tests := []struct {
		name string
		org  string
		want bool
	}{
		{name: "empty means the parent organization", org: "", want: true},
		{name: "public id", org: "abcdef1234567890abcdef1234567890", want: true},
		{name: "uppercase", org: "SubOrg", want: false},
		{name: "mixed case", org: "subOrg1", want: false},
		{name: "hyphen", org: "sub-org", want: true},
		{name: "underscore", org: "sub_org", want: true},
		{name: "digits only", org: "12345", want: true},
		{name: "path traversal", org: "../../etc/passwd", want: false},
		{name: "parent directory", org: "..", want: false},
		{name: "single dot", org: ".", want: false},
		{name: "path separator", org: "sub/org", want: false},
		{name: "backslash", org: `sub\org`, want: false},
		{name: "dot", org: "sub.org", want: false},
		{name: "space", org: "sub org", want: false},
		{name: "null byte", org: "suborg\x00", want: false},
		{name: "leading path separator", org: "/etc/passwd", want: false},
		{name: "tilde", org: "~", want: false},
		{name: "too long", org: strings.Repeat("a", maxOrgLength+1), want: false},
		{name: "at the length limit", org: strings.Repeat("a", maxOrgLength), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOrg(tt.org)
			if tt.want && err != nil {
				t.Errorf("ValidateOrg(%q) err = %v, want nil", tt.org, err)
			}
			if !tt.want && !errors.Is(err, ErrInvalidOrg) {
				t.Errorf("ValidateOrg(%q) err = %v, want %v", tt.org, err, ErrInvalidOrg)
			}
		})
	}
}

// TestTokenAndClientPath は、org が空のときに従来のファイル名 (後方互換) を保ち、
// org を与えたときだけファイル名へ org が入ることを確かめる。
func TestTokenAndClientPath(t *testing.T) {
	dir := filepath.Join("/tmp", "thief", "datadog")

	tests := []struct {
		name       string
		site       string
		org        string
		wantToken  string
		wantClient string
		wantErr    error
	}{
		{
			name:       "no org keeps the existing file names",
			site:       "datadoghq.com",
			wantToken:  "token_datadoghq.com.json",
			wantClient: "client_datadoghq.com.json",
		},
		{
			name:       "org is part of the file names",
			site:       "datadoghq.com",
			org:        "suborg1",
			wantToken:  "token_datadoghq.com_suborg1.json",
			wantClient: "client_datadoghq.com_suborg1.json",
		},
		{
			name:       "another site and org",
			site:       "us3.datadoghq.com",
			org:        "abcdef1234567890",
			wantToken:  "token_us3.datadoghq.com_abcdef1234567890.json",
			wantClient: "client_us3.datadoghq.com_abcdef1234567890.json",
		},
		{name: "invalid site", site: "../evil", wantErr: ErrInvalidSite},
		{name: "invalid site with an org", site: "../evil", org: "suborg1", wantErr: ErrInvalidSite},
		{name: "invalid org", site: "datadoghq.com", org: "../evil", wantErr: ErrInvalidOrg},
		{name: "org with a path separator", site: "datadoghq.com", org: "sub/org", wantErr: ErrInvalidOrg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenPath, tokenErr := TokenPath(dir, tt.site, tt.org)
			clientPath, clientErr := ClientPath(dir, tt.site, tt.org)

			if tt.wantErr != nil {
				if !errors.Is(tokenErr, tt.wantErr) {
					t.Errorf("TokenPath() err = %v, want %v", tokenErr, tt.wantErr)
				}
				if !errors.Is(clientErr, tt.wantErr) {
					t.Errorf("ClientPath() err = %v, want %v", clientErr, tt.wantErr)
				}
				return
			}
			if tokenErr != nil || clientErr != nil {
				t.Fatalf("TokenPath() err = %v, ClientPath() err = %v", tokenErr, clientErr)
			}
			if want := filepath.Join(dir, tt.wantToken); tokenPath != want {
				t.Errorf("TokenPath() = %q, want %q", tokenPath, want)
			}
			if want := filepath.Join(dir, tt.wantClient); clientPath != want {
				t.Errorf("ClientPath() = %q, want %q", clientPath, want)
			}
		})
	}
}

// TestOrgCredentialsAreIndependent は、同じ site の異なる org の認証情報が互いに
// 影響しないことを確かめる。Sub Organization はデータが完全に分離されており、org を
// 取り違えると別の組織のデータを返すことになる。
func TestOrgCredentialsAreIndependent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "datadog")
	const site = "datadoghq.com"
	orgs := []string{"", "suborg1", "suborg2"}

	for _, org := range orgs {
		tok := &TokenSet{AccessToken: secret("at-" + org), ExpiresIn: 3600, IssuedAt: testNow}
		if err := SaveToken(dir, site, org, tok); err != nil {
			t.Fatalf("SaveToken(org=%q) err = %v", org, err)
		}
		creds := &ClientCredentials{ClientID: "cid-" + org, RedirectURIs: []string{testCLIRedirect}}
		if err := SaveClient(dir, site, org, creds); err != nil {
			t.Fatalf("SaveClient(org=%q) err = %v", org, err)
		}
	}

	for _, org := range orgs {
		tok, ok, err := LoadToken(dir, site, org)
		if err != nil || !ok {
			t.Fatalf("LoadToken(org=%q) = (_, %v, %v), want (_, true, nil)", org, ok, err)
		}
		if want := "at-" + org; tok.AccessTokenValue() != want {
			t.Errorf("LoadToken(org=%q) access token = %q, want %q", org, tok.AccessTokenValue(), want)
		}
		creds, ok, err := LoadClient(dir, site, org)
		if err != nil || !ok {
			t.Fatalf("LoadClient(org=%q) = (_, %v, %v), want (_, true, nil)", org, ok, err)
		}
		if want := "cid-" + org; creds.ClientID != want {
			t.Errorf("LoadClient(org=%q) client id = %q, want %q", org, creds.ClientID, want)
		}
	}

	// 1 つの org を消しても、他の org の認証情報は残る。
	if err := DeleteToken(dir, site, "suborg1"); err != nil {
		t.Fatalf("DeleteToken() err = %v", err)
	}
	if err := DeleteClient(dir, site, "suborg1"); err != nil {
		t.Fatalf("DeleteClient() err = %v", err)
	}
	if _, ok, _ := LoadToken(dir, site, "suborg1"); ok {
		t.Error("the token of suborg1 is still readable after the delete")
	}
	for _, org := range []string{"", "suborg2"} {
		if _, ok, err := LoadToken(dir, site, org); !ok || err != nil {
			t.Errorf("LoadToken(org=%q) after deleting suborg1 = (_, %v, %v), want (_, true, nil)", org, ok, err)
		}
		if _, ok, err := LoadClient(dir, site, org); !ok || err != nil {
			t.Errorf("LoadClient(org=%q) after deleting suborg1 = (_, %v, %v), want (_, true, nil)", org, ok, err)
		}
	}
}

// TestSaveTokenRoundTrip は保存したトークンをそのまま読み戻せること、および
// ファイル権限が 0600、ディレクトリが 0700 であることを確かめる。
func TestSaveTokenRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "datadog")
	tok := &TokenSet{
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresIn:    3600,
		IssuedAt:     testNow,
		Scope:        "usage_read",
		ClientID:     "cid",
	}

	if err := SaveToken(dir, "datadoghq.com", "", tok); err != nil {
		t.Fatalf("SaveToken() err = %v", err)
	}

	got, ok, err := LoadToken(dir, "datadoghq.com", "")
	if err != nil || !ok {
		t.Fatalf("LoadToken() = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if diff := cmp.Diff(tok, got); diff != "" {
		t.Errorf("token mismatch (-want +got):\n%s", diff)
	}

	path, err := TokenPath(dir, "datadoghq.com", "")
	if err != nil {
		t.Fatalf("TokenPath() err = %v", err)
	}
	assertMode(t, path, 0o600)
	assertMode(t, dir, 0o700)

	// 上書き保存でも権限が保たれる (一時ファイル経由の rename)。
	tok.AccessToken = "at2"
	if err := SaveToken(dir, "datadoghq.com", "", tok); err != nil {
		t.Fatalf("SaveToken() overwrite err = %v", err)
	}
	assertMode(t, path, 0o600)
	got, _, err = LoadToken(dir, "datadoghq.com", "")
	if err != nil {
		t.Fatalf("LoadToken() err = %v", err)
	}
	if got.AccessTokenValue() != "at2" {
		t.Errorf("access token = %q, want %q", got.AccessTokenValue(), "at2")
	}

	// 一時ファイルが残っていないこと。
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file is left behind: %s", e.Name())
		}
	}
}

func TestSaveClientRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "datadog")
	creds := &ClientCredentials{
		ClientID:     "cid",
		ClientName:   ClientName,
		RedirectURIs: []string{"http://127.0.0.1:8400/callback", "http://127.0.0.1:8089/api/datadog/auth/callback"},
	}

	if err := SaveClient(dir, "datadoghq.com", "", creds); err != nil {
		t.Fatalf("SaveClient() err = %v", err)
	}
	got, ok, err := LoadClient(dir, "datadoghq.com", "")
	if err != nil || !ok {
		t.Fatalf("LoadClient() = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if diff := cmp.Diff(creds, got); diff != "" {
		t.Errorf("credentials mismatch (-want +got):\n%s", diff)
	}

	path, err := ClientPath(dir, "datadoghq.com", "")
	if err != nil {
		t.Fatalf("ClientPath() err = %v", err)
	}
	assertMode(t, path, 0o600)
}

func TestLoadTokenErrors(t *testing.T) {
	tests := []struct {
		name    string
		site    string
		org     string
		content string // 空文字はファイルを作らないことを表す
		wantOK  bool
		wantErr error
	}{
		{name: "missing file is not an error", site: "datadoghq.com", wantOK: false},
		{name: "missing file of a sub organization is not an error", site: "datadoghq.com", org: "suborg1", wantOK: false},
		{name: "broken json", site: "datadoghq.com", content: `{"access_token":`, wantErr: errAny},
		{name: "broken json of a sub organization", site: "datadoghq.com", org: "suborg1", content: `{"access_token":`, wantErr: errAny},
		{name: "not an object", site: "datadoghq.com", content: `["at"]`, wantErr: errAny},
		{name: "wrong field type", site: "datadoghq.com", content: `{"access_token":123}`, wantErr: errAny},
		{name: "empty file", site: "datadoghq.com", content: " ", wantErr: errAny},
		{name: "no access token", site: "datadoghq.com", content: `{"expires_in":3600}`, wantErr: ErrTokenIncomplete},
		{name: "invalid site", site: "../evil", wantErr: ErrInvalidSite},
		{name: "invalid org", site: "datadoghq.com", org: "../evil", wantErr: ErrInvalidOrg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" {
				writeFile(t, filepath.Join(dir, credFileName("token_", tt.site, tt.org)), tt.content)
			}
			got, ok, err := LoadToken(dir, tt.site, tt.org)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("LoadToken() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("LoadToken() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadToken() err = %v", err)
			}
			if ok != tt.wantOK {
				t.Errorf("LoadToken() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK && got != nil {
				t.Errorf("LoadToken() token = %v, want nil", got)
			}
		})
	}
}

func TestLoadClientErrors(t *testing.T) {
	tests := []struct {
		name    string
		site    string
		org     string
		content string
		wantOK  bool
		wantErr error
	}{
		{name: "missing file is not an error", site: "datadoghq.com", wantOK: false},
		{name: "missing file of a sub organization is not an error", site: "datadoghq.com", org: "suborg1", wantOK: false},
		{name: "broken json", site: "datadoghq.com", content: `{`, wantErr: errAny},
		{name: "no client id", site: "datadoghq.com", content: `{"redirect_uris":["http://127.0.0.1:8400/callback"]}`, wantErr: ErrClientIncomplete},
		{name: "no client id in a sub organization", site: "datadoghq.com", org: "suborg1", content: `{"redirect_uris":["http://127.0.0.1:8400/callback"]}`, wantErr: ErrClientIncomplete},
		{name: "no redirect uris", site: "datadoghq.com", content: `{"client_id":"cid"}`, wantErr: ErrClientIncomplete},
		{name: "invalid site", site: "not_a_site", wantErr: ErrInvalidSite},
		{name: "invalid org", site: "datadoghq.com", org: "sub org", wantErr: ErrInvalidOrg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" {
				writeFile(t, filepath.Join(dir, credFileName("client_", tt.site, tt.org)), tt.content)
			}
			_, ok, err := LoadClient(dir, tt.site, tt.org)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("LoadClient() err = nil, want an error")
				}
				if tt.wantErr != errAny && !errors.Is(err, tt.wantErr) {
					t.Fatalf("LoadClient() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadClient() err = %v", err)
			}
			if ok != tt.wantOK {
				t.Errorf("LoadClient() ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestSaveRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()

	if err := SaveToken(dir, "datadoghq.com", "", &TokenSet{}); !errors.Is(err, ErrTokenIncomplete) {
		t.Errorf("SaveToken() err = %v, want %v", err, ErrTokenIncomplete)
	}
	if err := SaveToken(dir, "../evil", "", &TokenSet{AccessToken: "at"}); !errors.Is(err, ErrInvalidSite) {
		t.Errorf("SaveToken() err = %v, want %v", err, ErrInvalidSite)
	}
	if err := SaveToken(dir, "datadoghq.com", "../evil", &TokenSet{AccessToken: "at"}); !errors.Is(err, ErrInvalidOrg) {
		t.Errorf("SaveToken() err = %v, want %v", err, ErrInvalidOrg)
	}
	if err := SaveClient(dir, "datadoghq.com", "", &ClientCredentials{}); !errors.Is(err, ErrClientIncomplete) {
		t.Errorf("SaveClient() err = %v, want %v", err, ErrClientIncomplete)
	}
	creds := &ClientCredentials{ClientID: "cid", RedirectURIs: []string{"u"}}
	if err := SaveClient(dir, "../evil", "", creds); !errors.Is(err, ErrInvalidSite) {
		t.Errorf("SaveClient() err = %v, want %v", err, ErrInvalidSite)
	}
	if err := SaveClient(dir, "datadoghq.com", "../evil", creds); !errors.Is(err, ErrInvalidOrg) {
		t.Errorf("SaveClient() err = %v, want %v", err, ErrInvalidOrg)
	}

	// 不正な site 名や org でファイルが作られていないこと。
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("save created %d entries for rejected input, want 0", len(entries))
	}
}

func TestDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "datadog")
	const site = "datadoghq.com"

	if err := SaveToken(dir, site, "", &TokenSet{AccessToken: "at", ExpiresIn: 60, IssuedAt: time.Now()}); err != nil {
		t.Fatalf("SaveToken() err = %v", err)
	}
	if err := SaveClient(dir, site, "", &ClientCredentials{ClientID: "cid", RedirectURIs: []string{"u"}}); err != nil {
		t.Fatalf("SaveClient() err = %v", err)
	}

	if err := DeleteToken(dir, site, ""); err != nil {
		t.Fatalf("DeleteToken() err = %v", err)
	}
	if err := DeleteClient(dir, site, ""); err != nil {
		t.Fatalf("DeleteClient() err = %v", err)
	}

	if _, ok, err := LoadToken(dir, site, ""); ok || err != nil {
		t.Errorf("LoadToken() after delete = (_, %v, %v), want (_, false, nil)", ok, err)
	}
	if _, ok, err := LoadClient(dir, site, ""); ok || err != nil {
		t.Errorf("LoadClient() after delete = (_, %v, %v), want (_, false, nil)", ok, err)
	}

	// 2 回目の削除も成功する (ファイルが無いことはエラーではない)。
	if err := DeleteToken(dir, site, ""); err != nil {
		t.Errorf("DeleteToken() on a missing file err = %v, want nil", err)
	}
	if err := DeleteClient(dir, site, ""); err != nil {
		t.Errorf("DeleteClient() on a missing file err = %v, want nil", err)
	}

	if err := DeleteToken(dir, "../evil", ""); !errors.Is(err, ErrInvalidSite) {
		t.Errorf("DeleteToken() err = %v, want %v", err, ErrInvalidSite)
	}
	if err := DeleteClient(dir, "../evil", ""); !errors.Is(err, ErrInvalidSite) {
		t.Errorf("DeleteClient() err = %v, want %v", err, ErrInvalidSite)
	}
	if err := DeleteToken(dir, site, "../evil"); !errors.Is(err, ErrInvalidOrg) {
		t.Errorf("DeleteToken() err = %v, want %v", err, ErrInvalidOrg)
	}
	if err := DeleteClient(dir, site, "../evil"); !errors.Is(err, ErrInvalidOrg) {
		t.Errorf("DeleteClient() err = %v, want %v", err, ErrInvalidOrg)
	}
}

// TestDir は保存先が config.Dir() 配下の datadog ディレクトリになることを確かめる。
func TestDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() err = %v", err)
	}
	if want := filepath.Join(base, "thief", "datadog"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %v, want %v", path, got, want)
	}
}

// credFileName は保存側と同じ規則でファイル名を組み立てる。壊れたファイルを置く
// テストが、保存を経ずに読み取り対象のファイルを用意するために使う。
func credFileName(prefix, site, org string) string {
	if org == "" {
		return prefix + site + ".json"
	}
	return prefix + site + "_" + org + ".json"
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
