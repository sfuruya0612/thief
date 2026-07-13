package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// Profile は ~/.aws/config の 1 プロファイルセクションを表す。
// AccountID / SSORoleName は IAM Identity Center (SSO) プロファイルにのみ
// 設定されているキーであり、role_arn ベースや credential_process のみの
// プロファイルでは空文字になる。
type Profile struct {
	Name        string
	AccountID   string
	SSORoleName string
}

// ListProfiles parses ~/.aws/config and returns all profiles.
func ListProfiles() ([]Profile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".aws", "config"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read aws config: %w", err)
	}
	return parseProfiles(string(data)), nil
}

// parseProfiles は ~/.aws/config の内容を ini ライクにパースする。
// [profile xxx] / [default] セクションのみを対象とし、[sso-session xxx] 等
// その他のセクションに入っている間はキーを拾わない。
func parseProfiles(content string) []Profile {
	var profiles []Profile
	var current *Profile

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			switch {
			case line == "[default]":
				profiles = append(profiles, Profile{Name: "default"})
				current = &profiles[len(profiles)-1]
			case strings.HasPrefix(line, "[profile "):
				name := strings.TrimSuffix(strings.TrimPrefix(line, "[profile "), "]")
				profiles = append(profiles, Profile{Name: name})
				current = &profiles[len(profiles)-1]
			default:
				// [sso-session xxx] 等、プロファイル以外のセクション。
				current = nil
			}
			continue
		}

		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "sso_account_id":
			current.AccountID = value
		case "sso_role_name":
			current.SSORoleName = value
		}
	}
	return profiles
}
