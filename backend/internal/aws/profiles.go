package aws

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var validProfileRe = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)

// ValidateProfileName returns ErrInvalidProfile if the name contains
// characters outside [A-Za-z0-9_-].
func ValidateProfileName(name string) error {
	if !validProfileRe.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidProfile, name)
	}
	return nil
}

// AuthType はプロファイルの認証方式。~/.aws/config と ~/.aws/credentials の
// 静的パースによる表示用の判定であり、実際に認証が通るかは検証しない。
type AuthType string

const (
	AuthTypeSSO               AuthType = "sso"
	AuthTypeAccessKey         AuthType = "access_key"
	AuthTypeAssumeRole        AuthType = "assume_role"
	AuthTypeCredentialProcess AuthType = "credential_process"
	AuthTypeUnknown           AuthType = "unknown"
)

// SSOStatus は SSO トークンキャッシュ (~/.aws/sso/cache) 由来のローカルな
// ログイン状態。実 API 呼び出しでの検証はしない best-effort な表示用の値。
type SSOStatus string

const (
	SSOStatusValid       SSOStatus = "valid"
	SSOStatusExpired     SSOStatus = "expired"
	SSOStatusNotLoggedIn SSOStatus = "not_logged_in"
)

// Profile は一覧 API 向けに解決済みのプロファイル情報。
// AccountID / SSORoleName は IAM Identity Center (SSO) プロファイルにのみ
// 設定されているキーであり、その他のプロファイルでは空文字になる。
type Profile struct {
	Name        string
	AccountID   string
	SSORoleName string
	Region      string
	AuthType    AuthType
	// SSOStatus は AuthType が sso のときのみ設定される。sso であっても
	// 参照先 sso-session セクションの欠落やキャッシュ読み取り失敗で判定
	// できない場合は空のまま残す (JSON では欠落し、バッジ非表示になる)。
	SSOStatus SSOStatus
	// SSOExpiresAt はトークンキャッシュの expiresAt が読めたときのみ非ゼロ。
	SSOExpiresAt time.Time
}

// ListProfiles parses ~/.aws/config and ~/.aws/credentials and returns all
// profiles with statically resolved auth information.
func ListProfiles() ([]Profile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}
	return listProfiles(filepath.Join(home, ".aws"), time.Now())
}

// listProfiles は awsDir (通常 ~/.aws) 配下の config / credentials /
// sso/cache を読み、プロファイル一覧を組み立てる。now は SSO トークンの
// 期限判定用で、テストから固定時刻を注入できるよう引数で受ける。
//
// この一覧はローカルファイルの best-effort な静的ビューであり、
// credentials / SSO キャッシュの読み取り失敗では一覧自体を失敗させない
// (Warn ログ + 該当情報の欠落に degrade する)。error を返すのはホーム
// ディレクトリ解決の失敗と config の not-exist 以外の読み取り失敗のみ。
func listProfiles(awsDir string, now time.Time) ([]Profile, error) {
	configData, err := os.ReadFile(filepath.Join(awsDir, "config"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read aws config: %w", err)
	}
	// config が無くても credentials のみの環境はあり得るため空として続行する。
	sections, ssoSessions := parseAWSConfig(string(configData))

	// credentials は auth_type 判定の補助と credentials-only プロファイルの
	// 発見に使う。not-exist は SSO のみの環境で正常なためログしない。
	credKeys := map[string]bool{}
	credData, err := os.ReadFile(filepath.Join(awsDir, "credentials"))
	if err == nil {
		credKeys = parseCredentials(string(credData))
	} else if !os.IsNotExist(err) {
		slog.Warn("read aws credentials failed", "err", err)
	}

	cacheStatuses, cacheReadable := readSSOCacheStatuses(filepath.Join(awsDir, "sso", "cache"), now)

	seen := make(map[string]bool)
	var profiles []Profile
	for _, sec := range sections {
		if seen[sec.Name] {
			continue
		}
		seen[sec.Name] = true
		p := Profile{
			Name:        sec.Name,
			AccountID:   sec.SSOAccountID,
			SSORoleName: sec.SSORoleName,
			Region:      sec.Region,
			AuthType:    resolveAuthType(sec, credKeys[sec.Name]),
		}
		if p.AuthType == AuthTypeSSO && cacheReadable {
			applySSOStatus(&p, sec, ssoSessions, cacheStatuses)
		}
		profiles = append(profiles, p)
	}

	// credentials のみに定義されたプロファイルも一覧に含める (config 由来を先に、
	// 名前順で後置)。サブ API (identity 等) がパス検証で 400 にする名前は、
	// 一覧に載せても開けないため除外する。
	credOnly := make([]string, 0, len(credKeys))
	for name := range credKeys {
		if seen[name] {
			continue
		}
		if err := ValidateProfileName(name); err != nil {
			slog.Warn("skip credentials profile with unsupported name", "profile", name)
			continue
		}
		credOnly = append(credOnly, name)
	}
	sort.Strings(credOnly)
	for _, name := range credOnly {
		authType := AuthTypeUnknown
		if credKeys[name] {
			authType = AuthTypeAccessKey
		}
		profiles = append(profiles, Profile{Name: name, AuthType: authType})
	}
	return profiles, nil
}

// applySSOStatus は SSO プロファイルの startUrl を解決し、トークンキャッシュの
// 状態を Profile に反映する。
func applySSOStatus(p *Profile, sec profileSection, ssoSessions map[string]ssoSessionSection, statuses map[string]ssoCacheStatus) {
	startURL := sec.SSOStartURL
	if sec.SSOSession != "" {
		sess, ok := ssoSessions[sec.SSOSession]
		if !ok {
			// 参照先の sso-session セクションが無い設定不備。aws sso login でも
			// 解消しないため not_logged_in にはせず判定不能 (空) のまま残す。
			slog.Warn("sso-session section not found", "profile", sec.Name, "session", sec.SSOSession)
			return
		}
		// sso_session と inline sso_start_url が併存する場合は sso-session 側を
		// 優先する (botocore は不一致を設定エラーにするが、ここは表示用に寄せる)。
		startURL = sess.StartURL
	}
	if startURL == "" {
		return
	}
	st, ok := statuses[normalizeStartURL(startURL)]
	if !ok {
		p.SSOStatus = SSOStatusNotLoggedIn
		return
	}
	p.SSOStatus = st.Status
	p.SSOExpiresAt = st.ExpiresAt
}

// profileSection は config の 1 プロファイルセクションの生パース結果。
type profileSection struct {
	Name           string
	Region         string
	SSOAccountID   string
	SSORoleName    string
	SSOSession     string // sso_session キー (新形式 SSO)
	SSOStartURL    string // profile 直下の sso_start_url (レガシー SSO)
	RoleArn        string
	CredProcess    bool
	HasAccessKeyID bool
}

// ssoSessionSection は [sso-session xxx] セクションの生パース結果。
type ssoSessionSection struct {
	StartURL string
	Region   string
}

// parseAWSConfig は ~/.aws/config の内容を ini ライクにパースし、
// [profile xxx] / [default] セクションと [sso-session xxx] セクションの
// 両方を収集する。それ以外のセクション内のキーはどちらにも帰属させない。
func parseAWSConfig(content string) ([]profileSection, map[string]ssoSessionSection) {
	var profiles []profileSection
	sessions := make(map[string]ssoSessionSection)

	var currentProfile *profileSection
	currentSession := ""

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentProfile = nil
			currentSession = ""
			switch {
			case line == "[default]":
				profiles = append(profiles, profileSection{Name: "default"})
				currentProfile = &profiles[len(profiles)-1]
			case strings.HasPrefix(line, "[profile "):
				name := strings.TrimSuffix(strings.TrimPrefix(line, "[profile "), "]")
				profiles = append(profiles, profileSection{Name: name})
				currentProfile = &profiles[len(profiles)-1]
			case strings.HasPrefix(line, "[sso-session "):
				currentSession = strings.TrimSuffix(strings.TrimPrefix(line, "[sso-session "), "]")
				if _, ok := sessions[currentSession]; !ok {
					sessions[currentSession] = ssoSessionSection{}
				}
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch {
		case currentProfile != nil:
			switch key {
			case "region":
				currentProfile.Region = value
			case "sso_account_id":
				currentProfile.SSOAccountID = value
			case "sso_role_name":
				currentProfile.SSORoleName = value
			case "sso_session":
				currentProfile.SSOSession = value
			case "sso_start_url":
				currentProfile.SSOStartURL = value
			case "role_arn":
				currentProfile.RoleArn = value
			case "credential_process":
				currentProfile.CredProcess = value != ""
			case "aws_access_key_id":
				currentProfile.HasAccessKeyID = value != ""
			}
		case currentSession != "":
			sess := sessions[currentSession]
			switch key {
			case "sso_start_url":
				sess.StartURL = value
			case "sso_region":
				sess.Region = value
			}
			sessions[currentSession] = sess
		}
	}
	return profiles, sessions
}

// parseCredentials は ~/.aws/credentials の内容をパースし、セクション名 →
// aws_access_key_id を持つかどうかの map を返す。credentials のセクションは
// [name] 形式で、config と違い "profile " プレフィックスを付けない。
func parseCredentials(content string) map[string]bool {
	result := make(map[string]bool)
	current := ""

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			if _, ok := result[current]; !ok {
				result[current] = false
			}
			continue
		}
		if current == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "aws_access_key_id" && strings.TrimSpace(value) != "" {
			result[current] = true
		}
	}
	return result
}

// resolveAuthType はプロファイルの認証方式を表示用に判定する。
// botocore の資格情報解決順 (role_arn → web identity → sso → shared
// credentials → process) の近似であり、credential_source や
// web_identity_token_file のみで構成されたプロファイルは unknown に落ちる。
func resolveAuthType(sec profileSection, hasCredentialsKey bool) AuthType {
	switch {
	case sec.RoleArn != "":
		return AuthTypeAssumeRole
	case sec.SSOSession != "" || sec.SSOStartURL != "":
		return AuthTypeSSO
	case sec.HasAccessKeyID || hasCredentialsKey:
		return AuthTypeAccessKey
	case sec.CredProcess:
		return AuthTypeCredentialProcess
	default:
		return AuthTypeUnknown
	}
}
