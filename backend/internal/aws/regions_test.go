package aws

import "testing"

func TestRegionResourceFromCode(t *testing.T) {
	tests := []struct {
		name string
		code string
		want RegionResource
	}{
		{
			name: "tokyo",
			code: "ap-northeast-1",
			want: RegionResource{Code: "ap-northeast-1", Name: "Asia Pacific (Tokyo)"},
		},
		{
			name: "us-east-1",
			code: "us-east-1",
			want: RegionResource{Code: "us-east-1", Name: "US East (N. Virginia)"},
		},
		{
			name: "eu-west-1",
			code: "eu-west-1",
			want: RegionResource{Code: "eu-west-1", Name: "Europe (Ireland)"},
		},
		{
			name: "unknown fallback",
			code: "xx-unknown-9",
			want: RegionResource{Code: "xx-unknown-9", Name: "xx-unknown-9"},
		},
		{
			name: "empty code",
			code: "",
			want: RegionResource{Code: "", Name: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := regionResourceFromCode(tt.code)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
