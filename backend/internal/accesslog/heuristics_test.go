package accesslog

import "testing"

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/.env", want: true},
		{path: "/.env.local", want: false},
		{path: "/.git/config", want: true},
		{path: "/wp-login.php", want: true},
		{path: "/actuator/health", want: true},
		{path: "/api/users", want: false},
		{path: "/", want: false},
		{path: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsSensitivePath(tt.path); got != tt.want {
				t.Errorf("IsSensitivePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestSensitivePathsReturnsCopy(t *testing.T) {
	paths := SensitivePaths()
	if len(paths) == 0 {
		t.Fatal("no sensitive paths")
	}
	paths[0] = "changed"
	if SensitivePaths()[0] == "changed" {
		t.Error("SensitivePaths returned the internal slice")
	}
}
