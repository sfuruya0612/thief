// Package config provides application configuration loading.
// Priority order (high → low): CLI flag > env > YAML > default.
package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type contextKey struct{}

// Config holds all application configuration.
type Config struct {
	Profile  string `yaml:"profile"`
	Region   string `yaml:"region"`
	Output   string `yaml:"output"`
	NoHeader bool   `yaml:"no-header"`
	GroupBy  string `yaml:"-"`

	// ListenAddr は API サーバの listen アドレス。`thief server` サブコマンド専用で
	// 他のサブコマンドからは参照されない。
	ListenAddr string `yaml:"listen-addr"`

	// WebOrigins はブラウザからの WebSocket アップグレード (EC2 Session / ECS Exec) を
	// 許可するオリジンパターン。API サーバ専用。
	WebOrigins []string `yaml:"-"`

	BigQuery BigQueryConfig
	Datadog  DatadogConfig `yaml:"datadog"`
	TiDB     TiDBConfig    `yaml:"tidb"`
}

// BigQueryConfig holds BigQuery-specific configuration.
type BigQueryConfig struct {
	ProjectID string `yaml:"project-id"`
}

// DatadogConfig holds Datadog-specific configuration.
type DatadogConfig struct {
	Site       string   `yaml:"site"`
	APIKey     redacted `yaml:"api-key"`
	AppKey     redacted `yaml:"app-key"`
	View       string   `yaml:"view"`
	StartMonth string   `yaml:"-"`
	EndMonth   string   `yaml:"-"`
}

// TiDBConfig holds TiDB Cloud-specific configuration.
type TiDBConfig struct {
	PublicKey   string   `yaml:"public-key"`
	PrivateKey  redacted `yaml:"private-key"`
	BilledMonth string   `yaml:"-"`
}

// redacted is a string type that masks itself in log output.
type redacted string

func (r redacted) String() string       { return "***" }
func (r redacted) LogValue() slog.Value { return slog.StringValue("***") }
func (r redacted) value() string        { return string(r) }

// fileConfig mirrors top-level fields for YAML unmarshalling.
type fileConfig struct {
	Profile    string `yaml:"profile"`
	Region     string `yaml:"region"`
	Output     string `yaml:"output"`
	NoHeader   bool   `yaml:"no-header"`
	ListenAddr string `yaml:"listen-addr"`
	BigQuery   struct {
		ProjectID string `yaml:"project-id"`
	} `yaml:"bigquery"`
	Datadog struct {
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

// defaultWebOrigins は frontend dev server (mise run frontend:run) のポートに合わせた
// WebSocket 許可オリジンのデフォルト値。
var defaultWebOrigins = []string{"localhost:8082", "127.0.0.1:8082"}

// Defaults returns a Config with built-in default values.
func Defaults() *Config {
	return &Config{
		Region:     "ap-northeast-1",
		Output:     "tab",
		ListenAddr: "127.0.0.1:8080",
		WebOrigins: defaultWebOrigins,
		Datadog: DatadogConfig{
			Site: "datadoghq.com",
			View: "summary",
		},
	}
}

// Load builds Config from env + YAML + defaults (no CLI flags).
// Used by the API server at startup.
func Load() (*Config, error) {
	fc, err := loadFile()
	if err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}
	cfg := Defaults()
	applyFile(cfg, fc)
	applyEnv(cfg)
	return cfg, nil
}

func applyFile(cfg *Config, fc fileConfig) {
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
		cfg.NoHeader = true
	}
	if fc.ListenAddr != "" {
		cfg.ListenAddr = fc.ListenAddr
	}
	if fc.BigQuery.ProjectID != "" {
		cfg.BigQuery.ProjectID = fc.BigQuery.ProjectID
	}
	if fc.Datadog.Site != "" {
		cfg.Datadog.Site = fc.Datadog.Site
	}
	if fc.Datadog.APIKey != "" {
		cfg.Datadog.APIKey = redacted(fc.Datadog.APIKey)
	}
	if fc.Datadog.AppKey != "" {
		cfg.Datadog.AppKey = redacted(fc.Datadog.AppKey)
	}
	if fc.Datadog.View != "" {
		cfg.Datadog.View = fc.Datadog.View
	}
	if fc.TiDB.PublicKey != "" {
		cfg.TiDB.PublicKey = fc.TiDB.PublicKey
	}
	if fc.TiDB.PrivateKey != "" {
		cfg.TiDB.PrivateKey = redacted(fc.TiDB.PrivateKey)
	}
}

func applyEnv(cfg *Config) {
	// AWS: use official AWS env vars only (THIEF_PROFILE/THIEF_REGION removed).
	if v := firstEnv("AWS_PROFILE"); v != "" {
		cfg.Profile = v
	}
	if v := firstEnv("AWS_REGION", "AWS_DEFAULT_REGION"); v != "" {
		cfg.Region = v
	}
	if v := os.Getenv("THIEF_OUTPUT"); v != "" {
		cfg.Output = v
	}
	if v := os.Getenv("THIEF_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("THIEF_WEB_ORIGINS"); v != "" {
		origins := strings.Split(v, ",")
		for i, o := range origins {
			origins[i] = strings.TrimSpace(o)
		}
		cfg.WebOrigins = origins
	}
	if v := os.Getenv("GOOGLE_CLOUD_PROJECT"); v != "" {
		cfg.BigQuery.ProjectID = v
	}
	if v := os.Getenv("DATADOG_API_KEY"); v != "" {
		cfg.Datadog.APIKey = redacted(v)
	}
	if v := os.Getenv("DATADOG_APP_KEY"); v != "" {
		cfg.Datadog.AppKey = redacted(v)
	}
	if v := os.Getenv("TIDB_PUBLIC_KEY"); v != "" {
		cfg.TiDB.PublicKey = v
	}
	if v := os.Getenv("TIDB_PRIVATE_KEY"); v != "" {
		cfg.TiDB.PrivateKey = redacted(v)
	}
}

func loadFile() (fileConfig, error) {
	for _, p := range configFilePaths() {
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

// Dir returns the directory used for thief's persistent local state
// (config.yaml と同じ場所: $XDG_CONFIG_HOME/thief または ~/.config/thief)。
// gcp プロジェクト一覧のローカルキャッシュ等、config.yaml 以外のファイルもここに置く。
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "thief"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "thief"), nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// DatadogAPIKey returns the plaintext Datadog API key.
func (c *Config) DatadogAPIKey() string { return c.Datadog.APIKey.value() }

// DatadogAppKey returns the plaintext Datadog App key.
func (c *Config) DatadogAppKey() string { return c.Datadog.AppKey.value() }

// TiDBPrivateKey returns the plaintext TiDB private key.
func (c *Config) TiDBPrivateKey() string { return c.TiDB.PrivateKey.value() }

// ToContext stores cfg in ctx.
func ToContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey{}, cfg)
}

// FromContext retrieves the Config from ctx, returning defaults if absent.
func FromContext(ctx context.Context) *Config {
	if cfg, ok := ctx.Value(contextKey{}).(*Config); ok {
		return cfg
	}
	return Defaults()
}
