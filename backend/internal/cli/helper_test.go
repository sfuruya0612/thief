package cli

import "testing"

func TestStripOneTrailingNewline(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no trailing newline", in: "secret", want: "secret"},
		{name: "lf", in: "secret\n", want: "secret"},
		{name: "crlf", in: "secret\r\n", want: "secret"},
		{name: "only one of multiple lf", in: "secret\n\n", want: "secret\n"},
		{name: "empty", in: "", want: ""},
		{name: "single lf", in: "\n", want: ""},
		{name: "internal newlines preserved", in: "a\nb\n", want: "a\nb"},
		{name: "lone cr is not stripped", in: "secret\r", want: "secret\r"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripOneTrailingNewline(tt.in); got != tt.want {
				t.Errorf("stripOneTrailingNewline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
