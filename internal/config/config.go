// Package config provides application configuration loading and management.
// It replaces the Viper dependency with a lightweight implementation that reads
// values from Cobra flags, environment variables, a YAML config file, and defaults.
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// contextKey is an unexported type used as a key for storing Config in a context.
type contextKey struct{}

// Config holds all application configuration.
type Config struct {
	Profile  string `yaml:"profile"`
	Region   string `yaml:"region"`
	Output   string `yaml:"output"`
	NoHeader bool   `yaml:"no-header"`
	GroupBy  string `yaml:"group-by"`

	Datadog DatadogConfig `yaml:"datadog"`
	TiDB    TiDBConfig    `yaml:"tidb"`
}

// DatadogConfig holds Datadog-specific configuration.
type DatadogConfig struct {
	Site       string `yaml:"site"`
	APIKey     string `yaml:"api-key"`
	AppKey     string `yaml:"app-key"`
	View       string `yaml:"view"`
	StartMonth string `yaml:"start-month"`
	EndMonth   string `yaml:"end-month"`
}

// TiDBConfig holds TiDB Cloud-specific configuration.
type TiDBConfig struct {
	PublicKey   string `yaml:"public-key"`
	PrivateKey  string `yaml:"private-key"`
	BilledMonth string `yaml:"billed-month"`
}

// fileConfig mirrors Config for YAML unmarshalling from a config file.
type fileConfig struct {
	Profile  string `yaml:"profile"`
	Region   string `yaml:"region"`
	Output   string `yaml:"output"`
	NoHeader bool   `yaml:"no-header"`
	Datadog  struct {
		Site   string `yaml:"site"`
		APIKey string `yaml:"api-key"`
		AppKey string `yaml:"app-key"`
		View   string `yaml:"view"`
	} `yaml:"datadog"`
	TiDB struct {
		PublicKey  string `yaml:"public-key"`
		PrivateKey string `yaml:"private-key"`
	} `yaml:"tidb"`
}

// Load builds a Config from the following priority order (highest to lowest):
//  1. Cobra flags (if explicitly set)
//  2. Environment variables
//  3. YAML config file
//  4. Default values
func Load(cmd *cobra.Command) (*Config, error) {
	fc, err := loadFile()
	if err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}

	cfg := &Config{
		Region:   "ap-northeast-1",
		Output:   "tab",
		NoHeader: false,
		Datadog: DatadogConfig{
			Site: "datadoghq.com",
			View: "summary",
		},
	}

	// Apply config file values.
	if fc.Profile != "" {
		cfg.Profile = fc.Profile
	}
	if fc.Region != "" {
		cfg.Region = fc.Region
	}
	if fc.Output != "" {
		cfg.Output = fc.Output
	}
	if fc.NoHeader {
		cfg.NoHeader = fc.NoHeader
	}
	if fc.Datadog.Site != "" {
		cfg.Datadog.Site = fc.Datadog.Site
	}
	if fc.Datadog.APIKey != "" {
		cfg.Datadog.APIKey = fc.Datadog.APIKey
	}
	if fc.Datadog.AppKey != "" {
		cfg.Datadog.AppKey = fc.Datadog.AppKey
	}
	if fc.Datadog.View != "" {
		cfg.Datadog.View = fc.Datadog.View
	}
	if fc.TiDB.PublicKey != "" {
		cfg.TiDB.PublicKey = fc.TiDB.PublicKey
	}
	if fc.TiDB.PrivateKey != "" {
		cfg.TiDB.PrivateKey = fc.TiDB.PrivateKey
	}

	// Apply environment variables (override config file).
	if v := envOrEmpty("AWS_PROFILE", "THIEF_PROFILE"); v != "" {
		cfg.Profile = v
	}
	if v := os.Getenv("THIEF_REGION"); v != "" {
		cfg.Region = v
	}
	if v := os.Getenv("THIEF_OUTPUT"); v != "" {
		cfg.Output = v
	}
	if v := os.Getenv("DATADOG_API_KEY"); v != "" {
		cfg.Datadog.APIKey = v
	}
	if v := os.Getenv("DATADOG_APP_KEY"); v != "" {
		cfg.Datadog.AppKey = v
	}
	if v := os.Getenv("TIDB_PUBLIC_KEY"); v != "" {
		cfg.TiDB.PublicKey = v
	}
	if v := os.Getenv("TIDB_PRIVATE_KEY"); v != "" {
		cfg.TiDB.PrivateKey = v
	}

	// Apply Cobra flags (override everything).
	if f := cmd.Flag("profile"); f != nil && f.Changed {
		cfg.Profile = f.Value.String()
	}
	if f := cmd.Flag("region"); f != nil && f.Changed {
		cfg.Region = f.Value.String()
	}
	if f := cmd.Flag("output"); f != nil && f.Changed {
		cfg.Output = f.Value.String()
	}
	if f := cmd.Flag("no-header"); f != nil && f.Changed {
		cfg.NoHeader = f.Value.String() == "true"
	}
	if f := cmd.Flag("group-by"); f != nil && f.Changed {
		cfg.GroupBy = f.Value.String()
	}
	// Datadog flag overrides (inherited persistent flags on datadogCmd).
	if f := cmd.Flag("site"); f != nil && f.Changed {
		cfg.Datadog.Site = f.Value.String()
	}
	if f := cmd.Flag("api-key"); f != nil && f.Changed {
		cfg.Datadog.APIKey = f.Value.String()
	}
	if f := cmd.Flag("app-key"); f != nil && f.Changed {
		cfg.Datadog.AppKey = f.Value.String()
	}
	if f := cmd.Flag("view"); f != nil && f.Changed {
		cfg.Datadog.View = f.Value.String()
	}
	if f := cmd.Flag("start-month"); f != nil && f.Changed {
		cfg.Datadog.StartMonth = f.Value.String()
	}
	if f := cmd.Flag("end-month"); f != nil && f.Changed {
		cfg.Datadog.EndMonth = f.Value.String()
	}
	// TiDB flag overrides (inherited persistent flags on tidbCmd).
	if f := cmd.Flag("public-key"); f != nil && f.Changed {
		cfg.TiDB.PublicKey = f.Value.String()
	}
	if f := cmd.Flag("private-key"); f != nil && f.Changed {
		cfg.TiDB.PrivateKey = f.Value.String()
	}
	if f := cmd.Flag("billed-month"); f != nil && f.Changed {
		cfg.TiDB.BilledMonth = f.Value.String()
	}

	return cfg, nil
}

// loadFile reads the first config file found in the search paths.
// Returns an empty fileConfig if no file is found (not an error).
func loadFile() (fileConfig, error) {
	paths := configFilePaths()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fileConfig{}, fmt.Errorf("read %s: %w", p, err)
		}
		var fc fileConfig
		if err := yaml.Unmarshal(data, &fc); err != nil {
			return fileConfig{}, fmt.Errorf("parse %s: %w", p, err)
		}
		return fc, nil
	}
	return fileConfig{}, nil
}

// configFilePaths returns the ordered list of config file locations to search.
func configFilePaths() []string {
	paths := []string{"config.yaml"}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "thief", "config.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "thief", "config.yaml"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".thief", "config.yaml"))
	}

	return paths
}

// envOrEmpty returns the value of the first non-empty environment variable in keys.
func envOrEmpty(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// ToContext stores cfg in ctx and returns the new context.
func ToContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey{}, cfg)
}

// FromContext retrieves the Config from ctx.
// If no Config is stored, it returns an empty Config with defaults.
func FromContext(ctx context.Context) *Config {
	if cfg, ok := ctx.Value(contextKey{}).(*Config); ok {
		return cfg
	}
	return &Config{
		Region: "ap-northeast-1",
		Output: "tab",
		Datadog: DatadogConfig{
			Site: "datadoghq.com",
			View: "summary",
		},
	}
}
