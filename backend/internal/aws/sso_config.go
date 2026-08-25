package aws

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// SSOConfig は profile が参照する SSO の接続設定。デバイス認可フロー
// (internal/ssoauth) の開始に必要な値だけを持つ。
type SSOConfig struct {
	Region   string
	StartURL string
}

// ResolveSSOConfig は ~/.aws/config (存否の分類には credentials も参照) から
// profile 名で SSO リージョンと start URL を
// 解決する。profile が見つからない場合は ErrProfileNotFound を、profile はあるが
// SSO の設定が無い (または不完全な) 場合は ErrSSONotConfigured を返す。いずれも
// errors.Is で判別できる。
func ResolveSSOConfig(profileName string) (*SSOConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}
	return resolveSSOConfig(filepath.Join(home, ".aws"), profileName)
}

// resolveSSOConfig は awsDir (通常 ~/.aws) 配下の config を読んで解決するコア。
// テストから設定ファイルの置き場所を注入できるよう分離してある。
//
// config に該当セクションが無くても、credentials のみで定義された profile は存在
// しうる (listProfiles は credentials-only の profile も一覧に含める)。その場合は
// 「profile が無い」ではなく「profile はあるが SSO 設定が無い」なので
// ErrSSONotConfigured に分類する。
func resolveSSOConfig(awsDir, profileName string) (*SSOConfig, error) {
	configData, err := os.ReadFile(filepath.Join(awsDir, "config"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read aws config: %w", err)
	}
	if err == nil {
		sections, ssoSessions := parseAWSConfig(string(configData))
		for _, sec := range sections {
			// 同名セクションが複数ある場合は最初の定義を採用する (listProfiles の
			// seen による重複排除と同じ優先順位)。
			if sec.Name == profileName {
				return ssoConfigFromSection(sec, ssoSessions)
			}
		}
	}

	// credentials の読み取り失敗は listProfiles と同じく、警告を出して分類の補助を
	// 諦めるだけでエラーにしない (credentials が無い環境は普通にある)。
	credData, err := os.ReadFile(filepath.Join(awsDir, "credentials"))
	if err == nil {
		if _, ok := parseCredentials(string(credData))[profileName]; ok {
			return nil, fmt.Errorf("%w: profile %q is defined only in credentials",
				ErrSSONotConfigured, profileName)
		}
	} else if !os.IsNotExist(err) {
		slog.Warn("read aws credentials failed", "err", err)
	}
	return nil, fmt.Errorf("%w: %q", ErrProfileNotFound, profileName)
}

// ssoConfigFromSection はパース済みの profile セクションから SSO 設定を組み立てる。
// sso_session と inline の sso_start_url が併存する場合は sso-session 側を優先する
// (applySSOStatus と同じ方針)。
func ssoConfigFromSection(sec profileSection, ssoSessions map[string]ssoSessionSection) (*SSOConfig, error) {
	if sec.SSOSession != "" {
		sess, ok := ssoSessions[sec.SSOSession]
		if !ok {
			return nil, fmt.Errorf("%w: profile %q references sso-session %q which is not defined",
				ErrSSONotConfigured, sec.Name, sec.SSOSession)
		}
		if sess.StartURL == "" || sess.Region == "" {
			return nil, fmt.Errorf("%w: sso-session %q is missing sso_start_url or sso_region",
				ErrSSONotConfigured, sec.SSOSession)
		}
		return &SSOConfig{Region: sess.Region, StartURL: sess.StartURL}, nil
	}

	if sec.SSOStartURL != "" {
		if sec.SSORegion == "" {
			return nil, fmt.Errorf("%w: profile %q has sso_start_url but no sso_region",
				ErrSSONotConfigured, sec.Name)
		}
		return &SSOConfig{Region: sec.SSORegion, StartURL: sec.SSOStartURL}, nil
	}

	return nil, fmt.Errorf("%w: profile %q has neither sso_session nor sso_start_url",
		ErrSSONotConfigured, sec.Name)
}
