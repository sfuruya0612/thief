package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestS3PathStyleEnabled(t *testing.T) {
	tests := []struct {
		name string
		env  string
		set  bool
		want bool
	}{
		{name: "unset", set: false, want: false},
		{name: "empty", env: "", set: true, want: false},
		{name: "true", env: "true", set: true, want: true},
		{name: "1", env: "1", set: true, want: true},
		{name: "false", env: "false", set: true, want: false},
		{name: "invalid", env: "not-a-bool", set: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("THIEF_S3_PATH_STYLE", tt.env)
			}
			if got := s3PathStyleEnabled(); got != tt.want {
				t.Errorf("s3PathStyleEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestS3PathStyleOption(t *testing.T) {
	tests := []struct {
		name       string
		pathStyle  bool
		wantOption bool
	}{
		{name: "enabled", pathStyle: true, wantOption: true},
		{name: "disabled", pathStyle: false, wantOption: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := s3.Options{}
			s3PathStyleOption(tt.pathStyle)(&opts)
			if opts.UsePathStyle != tt.wantOption {
				t.Errorf("UsePathStyle = %v, want %v", opts.UsePathStyle, tt.wantOption)
			}
		})
	}
}
