package api

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPreviewExtensionAllowed(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "csv lowercase", key: "path/to/file.csv", want: true},
		{name: "txt lowercase", key: "notes.txt", want: true},
		{name: "json lowercase", key: "data.json", want: true},
		{name: "json uppercase", key: "DATA.JSON", want: true},
		{name: "csv mixed case", key: "Report.Csv", want: true},
		{name: "multi extension rejected", key: "archive.json.gz", want: false},
		{name: "unsupported extension", key: "image.png", want: false},
		{name: "no extension", key: "README", want: false},
		{name: "trailing dot", key: "weird.", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := previewExtensionAllowed(tt.key)
			if got != tt.want {
				t.Errorf("previewExtensionAllowed(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestPreviewSizeAllowed(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want bool
	}{
		{name: "just under 5 MB", size: maxPreviewSize - 1, want: true},
		{name: "exactly 5 MB is too large", size: maxPreviewSize, want: false},
		{name: "well over 5 MB", size: maxPreviewSize + 1, want: false},
		{name: "zero size", size: 0, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := previewSizeAllowed(tt.size)
			if got != tt.want {
				t.Errorf("previewSizeAllowed(%d) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestReadPreviewBody(t *testing.T) {
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
			name: "just under limit",
			in:   bytes.Repeat([]byte("a"), maxPreviewSize-1),
			want: bytes.Repeat([]byte("a"), maxPreviewSize-1),
		},
		{
			// readPreviewBody 自体は「maxPreviewSize バイトちょうど」までは許容する。
			// 「5 MB ちょうどは不可」という TODO の要件は、ハンドラの ContentLength
			// 事前判定 (>= maxPreviewSize で拒否) が担う。ここでは実読み込みの防御線
			// (メタデータ欺瞞への対策) のみを検証する。
			name: "exactly at limit is still allowed by the reader alone",
			in:   bytes.Repeat([]byte("a"), maxPreviewSize),
			want: bytes.Repeat([]byte("a"), maxPreviewSize),
		},
		{
			name:    "exceeds limit",
			in:      bytes.Repeat([]byte("a"), maxPreviewSize+1),
			wantErr: errPreviewTooLarge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readPreviewBody(strings.NewReader(string(tt.in)))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && !bytes.Equal(got, tt.want) {
				t.Errorf("got %d bytes, want %d bytes", len(got), len(tt.want))
			}
		})
	}
}

func TestBuildPreviewResponse(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		want        *PreviewResponse
		wantErr     error
	}{
		{
			name:        "valid utf-8 text",
			body:        []byte("id,name\n1,alice\n"),
			contentType: "text/csv",
			want: &PreviewResponse{
				Content:     "id,name\n1,alice\n",
				ContentType: "text/csv",
				Size:        16,
			},
		},
		{
			name:        "valid utf-8 with multibyte characters",
			body:        []byte("こんにちは"),
			contentType: "text/plain",
			want: &PreviewResponse{
				Content:     "こんにちは",
				ContentType: "text/plain",
				Size:        int64(len("こんにちは")),
			},
		},
		{
			name:    "invalid utf-8 binary content",
			body:    []byte{0xff, 0xfe, 0x00, 0x01},
			wantErr: errPreviewNotText,
		},
		{
			name:    "exceeds max preview size",
			body:    bytes.Repeat([]byte("a"), maxPreviewSize+1),
			wantErr: errPreviewTooLarge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPreviewResponse(bytes.NewReader(tt.body), tt.contentType)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if *got != *tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
