package aws

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// writeAWSConfig は一時ディレクトリに ~/.aws/config 相当のファイルを作り、
// そのディレクトリ (awsDir) を返す。
func writeAWSConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir
}

// TestResolveSSOConfig は profile 名から SSO リージョンと start URL を解決する経路を、
// 新形式 (sso-session)、レガシー形式 (inline)、両者の併存、設定不備の各分岐で検証する。
func TestResolveSSOConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		profile string
		want    *SSOConfig
		wantErr error
	}{
		{
			name: "sso-session 形式を解決する",
			config: `[profile dev]
sso_session = my-sso
sso_account_id = 123456789012

[sso-session my-sso]
sso_start_url = https://example.awsapps.com/start
sso_region = ap-northeast-1
`,
			profile: "dev",
			want:    &SSOConfig{Region: "ap-northeast-1", StartURL: "https://example.awsapps.com/start"},
		},
		{
			name: "レガシー inline 形式を解決する",
			config: `[profile legacy]
sso_start_url = https://legacy.awsapps.com/start
sso_region = us-east-1
`,
			profile: "legacy",
			want:    &SSOConfig{Region: "us-east-1", StartURL: "https://legacy.awsapps.com/start"},
		},
		{
			name: "sso_session と inline が併存する場合は sso-session 側を優先する",
			config: `[profile both]
sso_session = my-sso
sso_start_url = https://inline.awsapps.com/start
sso_region = us-west-2

[sso-session my-sso]
sso_start_url = https://session.awsapps.com/start
sso_region = ap-northeast-1
`,
			profile: "both",
			want:    &SSOConfig{Region: "ap-northeast-1", StartURL: "https://session.awsapps.com/start"},
		},
		{
			name: "default セクションも解決できる",
			config: `[default]
sso_start_url = https://default.awsapps.com/start
sso_region = eu-west-1
`,
			profile: "default",
			want:    &SSOConfig{Region: "eu-west-1", StartURL: "https://default.awsapps.com/start"},
		},
		{
			name: "profile が無い場合は ErrProfileNotFound",
			config: `[profile other]
region = ap-northeast-1
`,
			profile: "missing",
			wantErr: ErrProfileNotFound,
		},
		{
			name: "SSO 設定が無い profile は ErrSSONotConfigured",
			config: `[profile plain]
region = ap-northeast-1
aws_access_key_id = AKIAEXAMPLE
`,
			profile: "plain",
			wantErr: ErrSSONotConfigured,
		},
		{
			name: "参照先の sso-session セクションが無い場合は ErrSSONotConfigured",
			config: `[profile dev]
sso_session = missing-session
`,
			profile: "dev",
			wantErr: ErrSSONotConfigured,
		},
		{
			name: "sso-session に sso_region が無い場合は ErrSSONotConfigured",
			config: `[profile dev]
sso_session = my-sso

[sso-session my-sso]
sso_start_url = https://example.awsapps.com/start
`,
			profile: "dev",
			wantErr: ErrSSONotConfigured,
		},
		{
			name: "レガシー形式で sso_region が無い場合は ErrSSONotConfigured",
			config: `[profile legacy]
sso_start_url = https://legacy.awsapps.com/start
`,
			profile: "legacy",
			wantErr: ErrSSONotConfigured,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			awsDir := writeAWSConfig(t, tt.config)
			got, err := resolveSSOConfig(awsDir, tt.profile)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("config mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestResolveSSOConfigCredentialsOnlyProfile は ~/.aws/credentials のみに定義された
// profile が「存在しない」ではなく「SSO 設定が無い」に分類されることを検証する
// (listProfiles が credentials-only の profile を一覧に含めることとの整合)。
func TestResolveSSOConfigCredentialsOnlyProfile(t *testing.T) {
	awsDir := writeAWSConfig(t, `[profile dev]
sso_start_url = https://example.awsapps.com/start
sso_region = ap-northeast-1
`)
	credentials := `[keyonly]
aws_access_key_id = AKIAEXAMPLE
aws_secret_access_key = secret
`
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(credentials), 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	got, err := resolveSSOConfig(awsDir, "keyonly")
	if !errors.Is(err, ErrSSONotConfigured) {
		t.Fatalf("err = %v, want ErrSSONotConfigured", err)
	}
	if got != nil {
		t.Errorf("config = %+v, want nil", got)
	}

	// credentials にも無い名前は引き続き ErrProfileNotFound。
	if _, err := resolveSSOConfig(awsDir, "missing"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("err = %v, want ErrProfileNotFound", err)
	}

	// config ファイル自体が無くても credentials-only の profile は同じ分類になる。
	credOnlyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(credOnlyDir, "credentials"), []byte(credentials), 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	if _, err := resolveSSOConfig(credOnlyDir, "keyonly"); !errors.Is(err, ErrSSONotConfigured) {
		t.Fatalf("err = %v, want ErrSSONotConfigured (config file absent)", err)
	}
}

// TestResolveSSOConfigCredentialsReadError は credentials の読み取り失敗 (存在しない
// 以外の理由) を config と違ってエラーにせず、分類の補助を諦めて ErrProfileNotFound に
// 落とすことを検証する (listProfiles が credentials の読み取り失敗を警告のみで続行する
// ことに合わせた意図的な非対称)。credentials パスをディレクトリにして再現する。
func TestResolveSSOConfigCredentialsReadError(t *testing.T) {
	awsDir := writeAWSConfig(t, `[profile dev]
sso_start_url = https://example.awsapps.com/start
sso_region = ap-northeast-1
`)
	if err := os.MkdirAll(filepath.Join(awsDir, "credentials"), 0700); err != nil {
		t.Fatalf("mkdir credentials dir: %v", err)
	}

	if _, err := resolveSSOConfig(awsDir, "keyonly"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("err = %v, want ErrProfileNotFound (credentials read failure is ignored)", err)
	}

	// config 側の解決は credentials の読み取り失敗に影響されない。
	got, err := resolveSSOConfig(awsDir, "dev")
	if err != nil {
		t.Fatalf("resolveSSOConfig(dev): %v", err)
	}
	want := &SSOConfig{Region: "ap-northeast-1", StartURL: "https://example.awsapps.com/start"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config mismatch (-want +got):\n%s", diff)
	}
}

// TestResolveSSOConfigPublicPath は公開関数 ResolveSSOConfig がホームディレクトリ
// 配下の ~/.aws/config を読むことを検証する (本番配線が使うのはこの経路)。
// os.UserHomeDir は Unix では $HOME を参照するため t.Setenv で差し替える。
func TestResolveSSOConfigPublicPath(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0700); err != nil {
		t.Fatalf("mkdir .aws: %v", err)
	}
	config := `[profile dev]
sso_session = my-sso

[sso-session my-sso]
sso_start_url = https://example.awsapps.com/start
sso_region = ap-northeast-1
`
	if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte(config), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HOME", home)

	got, err := ResolveSSOConfig("dev")
	if err != nil {
		t.Fatalf("ResolveSSOConfig: %v", err)
	}
	want := &SSOConfig{Region: "ap-northeast-1", StartURL: "https://example.awsapps.com/start"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config mismatch (-want +got):\n%s", diff)
	}
}

// TestResolveSSOConfigReadError は config の読み取りが存在しない以外の理由で失敗した
// 場合に、ErrProfileNotFound へ丸めず読み取りエラーとして返すことを検証する。
// config パスをディレクトリにすることで、パーミッションに依存せず決定的に再現する
// (os.ReadFile がディレクトリでエラーを返す Unix 系の挙動を前提とする)。
func TestResolveSSOConfigReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	got, err := resolveSSOConfig(dir, "dev")
	if err == nil {
		t.Fatal("err = nil, want read error")
	}
	if errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrSSONotConfigured) {
		t.Fatalf("err = %v, want plain read error (not a domain sentinel)", err)
	}
	if !strings.Contains(err.Error(), "read aws config") {
		t.Errorf("err = %v, want message containing %q", err, "read aws config")
	}
	if got != nil {
		t.Errorf("config = %+v, want nil", got)
	}
}

// TestResolveSSOConfigWithoutConfigFile は config ファイル自体が無い場合に
// ErrProfileNotFound として返ることを検証する (読み取りエラーに分類しない)。
func TestResolveSSOConfigWithoutConfigFile(t *testing.T) {
	got, err := resolveSSOConfig(t.TempDir(), "dev")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("err = %v, want ErrProfileNotFound", err)
	}
	if got != nil {
		t.Errorf("config = %+v, want nil", got)
	}
}

// TestResolveSSOConfigPrefersFirstSection は同名セクションが複数ある場合に最初の
// 定義が採用されること (listProfiles の重複排除と同じ優先順位) を検証する。
func TestResolveSSOConfigPrefersFirstSection(t *testing.T) {
	awsDir := writeAWSConfig(t, `[profile dup]
sso_start_url = https://first.awsapps.com/start
sso_region = ap-northeast-1

[profile dup]
sso_start_url = https://second.awsapps.com/start
sso_region = us-east-1
`)
	got, err := resolveSSOConfig(awsDir, "dup")
	if err != nil {
		t.Fatalf("resolveSSOConfig: %v", err)
	}
	want := &SSOConfig{Region: "ap-northeast-1", StartURL: "https://first.awsapps.com/start"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config mismatch (-want +got):\n%s", diff)
	}
}
