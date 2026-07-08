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

// ListProfiles parses ~/.aws/config and returns all profile names.
func ListProfiles() ([]string, error) {
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
	return parseProfileNames(string(data)), nil
}

func parseProfileNames(content string) []string {
	var profiles []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[profile ") && strings.HasSuffix(line, "]") {
			name := strings.TrimPrefix(line, "[profile ")
			name = strings.TrimSuffix(name, "]")
			profiles = append(profiles, name)
		} else if line == "[default]" {
			profiles = append(profiles, "default")
		}
	}
	return profiles
}
