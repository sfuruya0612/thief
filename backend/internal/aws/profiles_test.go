package aws

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParseAWSConfig(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		want         []profileSection
		wantSessions map[string]ssoSessionSection
	}{
		{
			name:    "default only",
			content: "[default]\nregion = ap-northeast-1\n",
			want:    []profileSection{{Name: "default", Region: "ap-northeast-1"}},
		},
		{
			name: "sso new format (sso-session reference)",
			content: `[profile new-sso]
sso_session = my-session
sso_account_id = 111111111111
sso_role_name = AdministratorAccess
region = ap-northeast-1

[sso-session my-session]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
sso_registration_scopes = sso:account:access
`,
			want: []profileSection{
				{
					Name:         "new-sso",
					Region:       "ap-northeast-1",
					SSOAccountID: "111111111111",
					SSORoleName:  "AdministratorAccess",
					SSOSession:   "my-session",
				},
			},
			wantSessions: map[string]ssoSessionSection{
				"my-session": {StartURL: "https://example.awsapps.com/start", Region: "us-east-1"},
			},
		},
		{
			name: "sso legacy format (inline sso_start_url)",
			content: `[profile legacy-sso]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
sso_account_id = 222222222222
sso_role_name = ReadOnlyAccess
region = us-east-1
`,
			want: []profileSection{
				{
					Name:         "legacy-sso",
					Region:       "us-east-1",
					SSOAccountID: "222222222222",
					SSORoleName:  "ReadOnlyAccess",
					SSOStartURL:  "https://example.awsapps.com/start",
				},
			},
		},
		{
			name: "role_arn with source_profile",
			content: `[profile assume-role]
role_arn = arn:aws:iam::333333333333:role/Example
source_profile = default
region = ap-northeast-1
`,
			want: []profileSection{
				{
					Name:    "assume-role",
					Region:  "ap-northeast-1",
					RoleArn: "arn:aws:iam::333333333333:role/Example",
				},
			},
		},
		{
			name: "credential_process and inline access key",
			content: `[profile proc]
credential_process = /usr/local/bin/get-creds

[profile inline-key]
aws_access_key_id = AKIAEXAMPLE
`,
			want: []profileSection{
				{Name: "proc", CredProcess: true},
				{Name: "inline-key", HasAccessKeyID: true},
			},
		},
		{
			name: "comments and blank lines interleaved",
			content: `# comment line
[default]
; another comment style

region = ap-northeast-1
# trailing comment

[profile with-comment]
sso_account_id = 444444444444
; sso_role_name = ShouldBeIgnoredIfCommented
sso_role_name = PowerUserAccess
`,
			want: []profileSection{
				{Name: "default", Region: "ap-northeast-1"},
				{Name: "with-comment", SSOAccountID: "444444444444", SSORoleName: "PowerUserAccess"},
			},
		},
		{
			name: "keys inside sso-session section are not attributed to profile",
			content: `[sso-session leading-session]
sso_start_url = https://example.awsapps.com/start
sso_account_id = 555555555555

[profile after-session]
region = ap-northeast-1
`,
			want: []profileSection{{Name: "after-session", Region: "ap-northeast-1"}},
			wantSessions: map[string]ssoSessionSection{
				"leading-session": {StartURL: "https://example.awsapps.com/start"},
			},
		},
		{
			name: "value containing an equals sign",
			content: `[profile with-equals]
sso_account_id = 666666666666
sso_role_name = Role=WithEquals
`,
			want: []profileSection{
				{Name: "with-equals", SSOAccountID: "666666666666", SSORoleName: "Role=WithEquals"},
			},
		},
		{
			name: "sso_session and inline sso_start_url coexist",
			content: `[profile both-sso]
sso_session = my-session
sso_start_url = https://inline.awsapps.com/start

[sso-session my-session]
sso_start_url = https://session.awsapps.com/start
`,
			want: []profileSection{
				{
					Name:        "both-sso",
					SSOSession:  "my-session",
					SSOStartURL: "https://inline.awsapps.com/start",
				},
			},
			wantSessions: map[string]ssoSessionSection{
				"my-session": {StartURL: "https://session.awsapps.com/start"},
			},
		},
		{
			name:    "empty content",
			content: "",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotSessions := parseAWSConfig(tt.content)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseAWSConfig() profiles mismatch (-want +got):\n%s", diff)
			}
			wantSessions := tt.wantSessions
			if wantSessions == nil {
				wantSessions = map[string]ssoSessionSection{}
			}
			if diff := cmp.Diff(wantSessions, gotSessions); diff != "" {
				t.Errorf("parseAWSConfig() sessions mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseCredentials(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]bool
	}{
		{
			name: "sections with and without access key",
			content: `[static]
aws_access_key_id = AKIAEXAMPLE
aws_secret_access_key = secret

[empty-section]

[default]
aws_access_key_id = AKIADEFAULT
`,
			want: map[string]bool{"static": true, "empty-section": false, "default": true},
		},
		{
			name: "commented access key is ignored",
			content: `[commented]
# aws_access_key_id = AKIAEXAMPLE
`,
			want: map[string]bool{"commented": false},
		},
		{
			name:    "empty content",
			content: "",
			want:    map[string]bool{},
		},
		{
			name: "key before any section is ignored",
			content: `aws_access_key_id = AKIAEXAMPLE
[later]
`,
			want: map[string]bool{"later": false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCredentials(tt.content)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseCredentials() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveAuthType(t *testing.T) {
	tests := []struct {
		name        string
		sec         profileSection
		hasCredsKey bool
		want        AuthType
	}{
		{name: "sso session", sec: profileSection{SSOSession: "s"}, want: AuthTypeSSO},
		{name: "sso legacy", sec: profileSection{SSOStartURL: "https://x/start"}, want: AuthTypeSSO},
		{name: "assume role", sec: profileSection{RoleArn: "arn:aws:iam::1:role/x"}, want: AuthTypeAssumeRole},
		{name: "inline access key", sec: profileSection{HasAccessKeyID: true}, want: AuthTypeAccessKey},
		{name: "credentials file access key", sec: profileSection{}, hasCredsKey: true, want: AuthTypeAccessKey},
		{name: "credential process", sec: profileSection{CredProcess: true}, want: AuthTypeCredentialProcess},
		{name: "nothing", sec: profileSection{}, want: AuthTypeUnknown},
		{name: "role wins over sso", sec: profileSection{RoleArn: "arn", SSOSession: "s"}, want: AuthTypeAssumeRole},
		{name: "sso wins over access key", sec: profileSection{SSOSession: "s", HasAccessKeyID: true}, want: AuthTypeSSO},
		{name: "access key wins over process", sec: profileSection{CredProcess: true}, hasCredsKey: true, want: AuthTypeAccessKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAuthType(tt.sec, tt.hasCredsKey); got != tt.want {
				t.Errorf("resolveAuthType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// writeAWSFixture は t.TempDir に ~/.aws 相当のディレクトリツリーを組み立てる。
func writeAWSFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

func TestListProfiles(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	future := now.Add(4 * time.Hour)
	past := now.Add(-4 * time.Hour)

	config := `[default]
region = ap-northeast-1

[profile new-sso-a]
sso_session = org
sso_account_id = 111111111111
sso_role_name = AdministratorAccess
region = ap-northeast-1

[profile new-sso-b]
sso_session = org
sso_account_id = 222222222222
sso_role_name = ReadOnlyAccess

[profile legacy-sso]
sso_start_url = https://legacy.awsapps.com/start/
sso_account_id = 333333333333
sso_role_name = PowerUserAccess
region = us-east-1

[profile dangling-sso]
sso_session = missing-session

[profile assume]
role_arn = arn:aws:iam::444444444444:role/Example
source_profile = default

[profile static]
region = us-west-2

[sso-session org]
sso_start_url = https://org.awsapps.com/start
sso_region = us-east-1
`
	credentials := `[static]
aws_access_key_id = AKIAEXAMPLE

[creds-only]
aws_access_key_id = AKIACREDSONLY

[bad.name]
aws_access_key_id = AKIABADNAME
`
	// org セッションは有効、legacy は期限切れ (trailing slash 差を吸収して合致する)。
	orgToken := `{"startUrl": "https://org.awsapps.com/start", "accessToken": "REDACTED", "expiresAt": "` + future.Format(time.RFC3339) + `"}`
	legacyToken := `{"startUrl": "https://legacy.awsapps.com/start", "accessToken": "REDACTED", "expiresAt": "` + past.Format(time.RFC3339) + `"}`

	t.Run("full fixture", func(t *testing.T) {
		dir := writeAWSFixture(t, map[string]string{
			"config":              config,
			"credentials":         credentials,
			"sso/cache/org.json":  orgToken,
			"sso/cache/leg.json":  legacyToken,
			"sso/cache/reg.json":  `{"clientId": "cid", "clientSecret": "REDACTED", "expiresAt": "2027-01-01T00:00:00Z"}`,
			"sso/cache/junk.json": `not json`,
		})
		got, err := listProfiles(dir, now)
		if err != nil {
			t.Fatalf("listProfiles() error = %v", err)
		}
		want := []Profile{
			{Name: "default", Region: "ap-northeast-1", AuthType: AuthTypeUnknown},
			{
				Name: "new-sso-a", AccountID: "111111111111", SSORoleName: "AdministratorAccess",
				Region: "ap-northeast-1", AuthType: AuthTypeSSO, SSOStatus: SSOStatusValid, SSOExpiresAt: future,
			},
			{
				Name: "new-sso-b", AccountID: "222222222222", SSORoleName: "ReadOnlyAccess",
				AuthType: AuthTypeSSO, SSOStatus: SSOStatusValid, SSOExpiresAt: future,
			},
			{
				Name: "legacy-sso", AccountID: "333333333333", SSORoleName: "PowerUserAccess",
				Region: "us-east-1", AuthType: AuthTypeSSO, SSOStatus: SSOStatusExpired, SSOExpiresAt: past,
			},
			{Name: "dangling-sso", AuthType: AuthTypeSSO},
			{Name: "assume", AuthType: AuthTypeAssumeRole},
			{Name: "static", Region: "us-west-2", AuthType: AuthTypeAccessKey},
			{Name: "creds-only", AuthType: AuthTypeAccessKey},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("listProfiles() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("credentials only (no config)", func(t *testing.T) {
		dir := writeAWSFixture(t, map[string]string{
			"credentials": "[only]\naws_access_key_id = AKIAONLY\n",
		})
		got, err := listProfiles(dir, now)
		if err != nil {
			t.Fatalf("listProfiles() error = %v", err)
		}
		want := []Profile{{Name: "only", AuthType: AuthTypeAccessKey}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("listProfiles() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("sso without cache dir is not_logged_in", func(t *testing.T) {
		dir := writeAWSFixture(t, map[string]string{
			"config": "[profile sso-only]\nsso_start_url = https://x.awsapps.com/start\n",
		})
		got, err := listProfiles(dir, now)
		if err != nil {
			t.Fatalf("listProfiles() error = %v", err)
		}
		want := []Profile{{Name: "sso-only", AuthType: AuthTypeSSO, SSOStatus: SSOStatusNotLoggedIn}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("listProfiles() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("empty aws dir", func(t *testing.T) {
		got, err := listProfiles(t.TempDir(), now)
		if err != nil {
			t.Fatalf("listProfiles() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("listProfiles() = %v, want empty", got)
		}
	})
}
