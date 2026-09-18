package cli

import (
	"strings"
	"testing"
)

func TestResolveDownloadPath(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		outputFile string
		want       string
		wantErr    bool
	}{
		{name: "derives base name from key", key: "dir/sub/file.txt", want: "file.txt"},
		{name: "key without directory", key: "file.txt", want: "file.txt"},
		{name: "explicit output file wins", key: "dir/file.txt", outputFile: "out/renamed", want: "out/renamed"},
		{name: "explicit output file allowed for directory key", key: "dir/", outputFile: "out", want: "out"},
		{name: "trailing slash cannot derive base name", key: "dir/", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDownloadPath(tt.key, tt.outputFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveDownloadPath(%q, %q) error = %v, wantErr %t", tt.key, tt.outputFile, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("resolveDownloadPath(%q, %q) = %q, want %q", tt.key, tt.outputFile, got, tt.want)
			}
		})
	}
}

func TestContentTypeForFile(t *testing.T) {
	if got := contentTypeForFile("data.json"); !strings.Contains(got, "json") {
		t.Errorf("contentTypeForFile(data.json) = %q, want it to contain json", got)
	}
	if got := contentTypeForFile("archive.unknownext"); got != "" {
		t.Errorf("contentTypeForFile(archive.unknownext) = %q, want empty", got)
	}
}
