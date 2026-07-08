package aws

import "testing"

func TestDisplayState(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "pascal deployed", in: "Deployed", want: "deployed"},
		{name: "pascal inprogress", in: "InProgress", want: "in-progress"},
		{name: "upper active", in: "ACTIVE", want: "active"},
		{name: "underscore", in: "active_impaired", want: "active-impaired"},
		{name: "lower available", in: "available", want: "available"},
		{name: "whitespace", in: "  RUNNING  ", want: "running"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayState(tt.in)
			if got != tt.want {
				t.Errorf("DisplayState(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
