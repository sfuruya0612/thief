package api

import "testing"

func TestSanitizeContentDispositionFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple filename",
			in:   "report.txt",
			want: "report.txt",
		},
		{
			name: "extract final segment from path",
			in:   "path/to/file.csv",
			want: "file.csv",
		},
		{
			name: "strip CR LF injection",
			in:   "evil\r\nX-Injected: yes.txt",
			want: "evilX-Injected: yes.txt",
		},
		{
			name: "strip double quotes",
			in:   `weird"name".pdf`,
			want: "weirdname.pdf",
		},
		{
			name: "strip backslash",
			in:   `mix\slash.bin`,
			want: "mixslash.bin",
		},
		{
			name: "empty key fallback",
			in:   "",
			want: "download",
		},
		{
			name: "trailing slash fallback",
			in:   "dir/",
			want: "download",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeContentDispositionFilename(tt.in)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
