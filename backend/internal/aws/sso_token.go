package aws

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ssoTokenFile struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

// loadSSOAccessToken reads the SSO access token from the local token cache for the given profile.
// Returns ErrSSOTokenExpired when a token is found but has already expired.
func loadSSOAccessToken(profile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}

	cacheDir := filepath.Join(home, ".aws", "sso", "cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return "", fmt.Errorf("read sso cache dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cacheDir, entry.Name()))
		if err != nil {
			continue
		}
		var tf ssoTokenFile
		if err := json.Unmarshal(data, &tf); err != nil {
			continue
		}
		if tf.AccessToken == "" {
			continue
		}
		// Check expiry when the field is present.
		if tf.ExpiresAt != "" {
			exp, err := time.Parse(time.RFC3339, tf.ExpiresAt)
			if err == nil && time.Now().After(exp) {
				return "", fmt.Errorf("%w: token expired at %s (run: aws sso login --profile %s)",
					ErrSSOTokenExpired, exp.Format(time.RFC3339), profile)
			}
		}
		return tf.AccessToken, nil
	}
	return "", fmt.Errorf("%w: no valid SSO token found in %s (run: aws sso login --profile %s)",
		ErrSSOTokenExpired, cacheDir, profile)
}
