package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultsPriceCacheDir(t *testing.T) {
	got := Defaults().PriceCacheDir
	want := "/tmp/thief/price"
	if got != want {
		t.Errorf("Defaults().PriceCacheDir = %q, want %q", got, want)
	}
}

func TestApplyFilePriceCacheDir(t *testing.T) {
	tests := []struct {
		name string
		fc   fileConfig
		want string
	}{
		{name: "empty leaves default", fc: fileConfig{}, want: "/tmp/thief/price"},
		{name: "override", fc: fileConfig{PriceCacheDir: "/var/lib/thief/price"}, want: "/var/lib/thief/price"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			applyFile(cfg, tt.fc)
			if cfg.PriceCacheDir != tt.want {
				t.Errorf("PriceCacheDir = %q, want %q", cfg.PriceCacheDir, tt.want)
			}
		})
	}
}

func TestApplyEnvPriceCacheDir(t *testing.T) {
	t.Setenv("THIEF_PRICE_CACHE_DIR", "/custom/price/dir")
	cfg := Defaults()
	applyEnv(cfg)
	if cfg.PriceCacheDir != "/custom/price/dir" {
		t.Errorf("PriceCacheDir = %q, want %q", cfg.PriceCacheDir, "/custom/price/dir")
	}
}

func TestDefaultsSnippetsDir(t *testing.T) {
	got := Defaults().SnippetsDir
	want := filepath.Join(".thief", "snippets")
	if got != want {
		t.Errorf("Defaults().SnippetsDir = %q, want %q", got, want)
	}
}

func TestApplyFileSnippetsDir(t *testing.T) {
	tests := []struct {
		name string
		fc   fileConfig
		want string
	}{
		{name: "empty leaves default", fc: fileConfig{}, want: filepath.Join(".thief", "snippets")},
		{name: "override", fc: fileConfig{SnippetsDir: "/var/lib/thief/snippets"}, want: "/var/lib/thief/snippets"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			applyFile(cfg, tt.fc)
			if cfg.SnippetsDir != tt.want {
				t.Errorf("SnippetsDir = %q, want %q", cfg.SnippetsDir, tt.want)
			}
		})
	}
}

func TestApplyEnvSnippetsDir(t *testing.T) {
	t.Setenv("THIEF_SNIPPETS_DIR", "/custom/snippets/dir")
	cfg := Defaults()
	applyEnv(cfg)
	if cfg.SnippetsDir != "/custom/snippets/dir" {
		t.Errorf("SnippetsDir = %q, want %q", cfg.SnippetsDir, "/custom/snippets/dir")
	}
}

// TestDatadogOAuthRedirectBase は redirect_uri の元になる設定の既定値と、設定ファイル /
// 環境変数での上書きを確認する。既定値は API サーバの既定の待ち受けアドレスと一致し、
// 何も設定しなければ Datadog に登録される redirect_uri が変わらないことを保証する。
func TestDatadogOAuthRedirectBase(t *testing.T) {
	if got := Defaults().Datadog.OAuthRedirectBase; got != DefaultDatadogOAuthRedirectBase {
		t.Errorf("Defaults().Datadog.OAuthRedirectBase = %q, want %q", got, DefaultDatadogOAuthRedirectBase)
	}
	if want := "http://127.0.0.1:8089"; DefaultDatadogOAuthRedirectBase != want {
		t.Errorf("DefaultDatadogOAuthRedirectBase = %q, want %q", DefaultDatadogOAuthRedirectBase, want)
	}

	t.Run("file overrides the default", func(t *testing.T) {
		tests := []struct {
			name string
			base string
			want string
		}{
			{name: "empty leaves default", base: "", want: DefaultDatadogOAuthRedirectBase},
			{name: "override", base: "https://thief.example.com", want: "https://thief.example.com"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var fc fileConfig
				fc.Datadog.OAuthRedirectBase = tt.base
				cfg := Defaults()
				applyFile(cfg, fc)
				if cfg.Datadog.OAuthRedirectBase != tt.want {
					t.Errorf("Datadog.OAuthRedirectBase = %q, want %q", cfg.Datadog.OAuthRedirectBase, tt.want)
				}
			})
		}
	})

	t.Run("env overrides the default", func(t *testing.T) {
		t.Setenv("THIEF_DATADOG_OAUTH_REDIRECT_BASE", "https://thief.example.com:9443")
		cfg := Defaults()
		applyEnv(cfg)
		if want := "https://thief.example.com:9443"; cfg.Datadog.OAuthRedirectBase != want {
			t.Errorf("Datadog.OAuthRedirectBase = %q, want %q", cfg.Datadog.OAuthRedirectBase, want)
		}
	})
}

// TestDatadogOAuthCLIRedirectURI は CLI 用 redirect_uri の既定値を固定する。
// この値は Datadog に登録済みの redirect_uris と突き合わされるため、変更すると
// 登録済みクライアントが使えなくなる。
func TestDatadogOAuthCLIRedirectURI(t *testing.T) {
	if want := "http://127.0.0.1:8400/callback"; DefaultDatadogOAuthCLIRedirectURI != want {
		t.Errorf("DefaultDatadogOAuthCLIRedirectURI = %q, want %q", DefaultDatadogOAuthCLIRedirectURI, want)
	}
	if want := "/api/datadog/auth/callback"; DatadogOAuthCallbackPath != want {
		t.Errorf("DatadogOAuthCallbackPath = %q, want %q", DatadogOAuthCallbackPath, want)
	}
}
