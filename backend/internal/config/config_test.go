package config

import "testing"

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
