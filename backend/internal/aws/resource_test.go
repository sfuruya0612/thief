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

func TestTagsToMapFunc(t *testing.T) {
	type tag struct {
		Key   *string
		Value *string
	}
	str := func(s string) *string { return &s }
	kv := func(tg tag) (*string, *string) { return tg.Key, tg.Value }

	tests := []struct {
		name string
		in   []tag
		want map[string]string
	}{
		{name: "empty", in: nil, want: map[string]string{}},
		{name: "single", in: []tag{{Key: str("Name"), Value: str("web")}}, want: map[string]string{"Name": "web"}},
		{name: "multiple", in: []tag{{Key: str("a"), Value: str("1")}, {Key: str("b"), Value: str("2")}}, want: map[string]string{"a": "1", "b": "2"}},
		{name: "nil key and value", in: []tag{{Key: nil, Value: nil}}, want: map[string]string{"": ""}},
		{name: "duplicate key last wins", in: []tag{{Key: str("a"), Value: str("1")}, {Key: str("a"), Value: str("2")}}, want: map[string]string{"a": "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsToMapFunc(tt.in, kv)
			if len(got) != len(tt.want) {
				t.Fatalf("tagsToMapFunc() = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("tagsToMapFunc()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
