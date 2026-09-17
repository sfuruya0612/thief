package config

import (
	"os"
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

// isolateConfigFiles は XDG_CONFIG_HOME と HOME 配下の config.yaml の探索先を一時ディレクトリへ
// 向け、環境変数 THIEF_DATADOG_OAUTH_REDIRECT_BASE を空にする。実行環境のホームに置かれた
// config.yaml や環境変数がテスト結果を左右しないようにするため。configFilePaths が最優先で
// 見るカレントディレクトリ相対の config.yaml は対象外であり、パッケージのソースディレクトリに
// そのファイルを置かないことで別途担保している。
// 戻り値は $XDG_CONFIG_HOME として使う一時ディレクトリ。
func isolateConfigFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("THIEF_DATADOG_OAUTH_REDIRECT_BASE", "")
	return dir
}

// TestLoadNormalizesDatadogOAuthRedirectBase は、Load が返す redirect base に前後の空白と
// 末尾スラッシュが残らないことを確認する。ベースは先頭にスラッシュを持つ
// DatadogOAuthCallbackPath と連結されるため、末尾スラッシュが残ると redirect_uri の
// パスが二重スラッシュになる。RFC 6749 3.1.2.3 は認可サーバが登録済みの redirect_uri と
// 単純な文字列比較で照合することを求めており、二重スラッシュは一致しない。
func TestLoadNormalizesDatadogOAuthRedirectBase(t *testing.T) {
	const want = "http://127.0.0.1:8089"

	t.Run("default", func(t *testing.T) {
		isolateConfigFiles(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := cfg.Datadog.OAuthRedirectBase; got != want {
			t.Errorf("Datadog.OAuthRedirectBase = %q, want %q", got, want)
		}
	})

	t.Run("env", func(t *testing.T) {
		tests := []struct {
			name string
			env  string
			want string
		}{
			{name: "trailing slash", env: "http://127.0.0.1:8089/", want: want},
			{name: "multiple trailing slashes", env: "http://127.0.0.1:8089///", want: want},
			{name: "surrounding spaces", env: " http://127.0.0.1:8089/ ", want: want},
			{name: "already normalized", env: "http://127.0.0.1:8089", want: want},
			// スラッシュだけの値は正規化後に空になる。空のままでは redirect_uri が
			// スキームもホストも持たない相対 URI になるため、既定値へ戻す。
			{name: "slash only falls back to the default", env: "/", want: DefaultDatadogOAuthRedirectBase},
			{name: "slashes only fall back to the default", env: "///", want: DefaultDatadogOAuthRedirectBase},
			{name: "spaces only fall back to the default", env: "   ", want: DefaultDatadogOAuthRedirectBase},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				isolateConfigFiles(t)
				t.Setenv("THIEF_DATADOG_OAUTH_REDIRECT_BASE", tt.env)
				cfg, err := Load()
				if err != nil {
					t.Fatalf("Load() error = %v", err)
				}
				if got := cfg.Datadog.OAuthRedirectBase; got != tt.want {
					t.Errorf("Datadog.OAuthRedirectBase = %q, want %q", got, tt.want)
				}
			})
		}
	})

	t.Run("yaml", func(t *testing.T) {
		dir := isolateConfigFiles(t)
		if err := os.MkdirAll(filepath.Join(dir, "thief"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		body := "datadog:\n  oauth-redirect-base: \"http://127.0.0.1:8089/\"\n"
		if err := os.WriteFile(filepath.Join(dir, "thief", "config.yaml"), []byte(body), 0o600); err != nil {
			t.Fatalf("write config.yaml: %v", err)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := cfg.Datadog.OAuthRedirectBase; got != want {
			t.Errorf("Datadog.OAuthRedirectBase = %q, want %q", got, want)
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
