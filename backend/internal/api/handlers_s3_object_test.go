package api

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

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

func TestReadS3UploadBody(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    []byte
		wantErr error
	}{
		{
			name: "empty body",
			in:   []byte{},
			want: []byte{},
		},
		{
			name: "small body",
			in:   []byte("hello"),
			want: []byte("hello"),
		},
		{
			name: "exactly at limit",
			in:   bytes.Repeat([]byte("a"), maxS3UploadSize),
			want: bytes.Repeat([]byte("a"), maxS3UploadSize),
		},
		{
			name:    "exceeds limit",
			in:      bytes.Repeat([]byte("a"), maxS3UploadSize+1),
			wantErr: errS3UploadTooLarge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readS3UploadBody(strings.NewReader(string(tt.in)))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && !bytes.Equal(got, tt.want) {
				t.Errorf("got %d bytes, want %d bytes", len(got), len(tt.want))
			}
		})
	}
}
