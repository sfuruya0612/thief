package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseProfiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []Profile
	}{
		{
			name:    "default only",
			content: "[default]\nregion = ap-northeast-1\n",
			want:    []Profile{{Name: "default"}},
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
			want: []Profile{
				{Name: "new-sso", AccountID: "111111111111", SSORoleName: "AdministratorAccess"},
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
			want: []Profile{
				{Name: "legacy-sso", AccountID: "222222222222", SSORoleName: "ReadOnlyAccess"},
			},
		},
		{
			name: "role_arn only (no account id)",
			content: `[profile assume-role]
role_arn = arn:aws:iam::333333333333:role/Example
source_profile = default
region = ap-northeast-1
`,
			want: []Profile{{Name: "assume-role"}},
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
			want: []Profile{
				{Name: "default"},
				{Name: "with-comment", AccountID: "444444444444", SSORoleName: "PowerUserAccess"},
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
			want: []Profile{{Name: "after-session"}},
		},
		{
			name: "value containing an equals sign",
			content: `[profile with-equals]
sso_account_id = 666666666666
sso_role_name = Role=WithEquals
`,
			want: []Profile{
				{Name: "with-equals", AccountID: "666666666666", SSORoleName: "Role=WithEquals"},
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
			got := parseProfiles(tt.content)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseProfiles() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
