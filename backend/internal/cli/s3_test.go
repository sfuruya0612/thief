package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/spf13/cobra"
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

// s3ObjectsCall は fakeS3ObjectsLister が記録する呼び出しの内容。
type s3ObjectsCall struct {
	profile, region, bucket, prefix, delimiter string
}

// fakeS3ObjectsLister は呼び出しの引数を calls に記録し、固定の応答を返す取得関数を返す。
func fakeS3ObjectsLister(calls *[]s3ObjectsCall, objects []awsinternal.S3ObjectResource, prefixes []string, truncated bool) s3ObjectsLister {
	return func(_ context.Context, profile, region, bucket, prefix, delimiter string) ([]awsinternal.S3ObjectResource, []string, bool, error) {
		*calls = append(*calls, s3ObjectsCall{profile: profile, region: region, bucket: bucket, prefix: prefix, delimiter: delimiter})
		return objects, prefixes, truncated, nil
	}
}

// newRootCmdWithS3ObjectsLister は s3 コマンドを差し替えた取得関数で組み立て直したルートを返す。
// cobra の出力 (cmd.Println の空のときのメッセージ) は cobraOut に、標準エラー出力 (打ち切りの
// 警告) は stderr に集める。表は util の formatter が os.Stdout に直接書くため captureStdout で取る。
func newRootCmdWithS3ObjectsLister(t *testing.T, list s3ObjectsLister, cobraOut, stderr *bytes.Buffer) *cobra.Command {
	t.Helper()
	root := NewRootCmd()
	for _, c := range root.Commands() {
		if c.Name() == "s3" {
			root.RemoveCommand(c)
		}
	}
	root.AddCommand(newS3CmdWithObjectsLister(list))
	root.SetOut(cobraOut)
	root.SetErr(stderr)
	return root
}

// TestS3ObjectsOutput は `thief s3 objects` が delimiter を渡さずに一覧を取得し、応答に
// prefixes が混ざっても出力の列と行が変わらないこと、打ち切りの警告が truncated のときだけ
// 標準エラー出力に出ることを、実コマンドツリーの実行で確かめる。
func TestS3ObjectsOutput(t *testing.T) {
	// loadConfig の先の config.Load が実行環境の設定を読まないよう、参照先を空に向ける。
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	objects := []awsinternal.S3ObjectResource{
		{Key: "reports/a.csv", Size: 10, LastModified: "2026-09-28T00:00:00Z", StorageClass: "STANDARD", ETag: "e1"},
		{Key: "reports/b.csv", Size: 20, LastModified: "2026-09-28T00:00:01Z", StorageClass: "GLACIER", ETag: "e2"},
	}
	args := []string{"s3", "objects", "my-bucket", "--prefix", "reports/", "-p", "dev", "-r", "ap-northeast-1", "-o", "csv"}
	wantCall := s3ObjectsCall{profile: "dev", region: "ap-northeast-1", bucket: "my-bucket", prefix: "reports/", delimiter: ""}
	wantTable := "Name,Size,LastModified,StorageClass,ETag\n" +
		"reports/a.csv,10,2026-09-28T00:00:00Z,STANDARD,e1\n" +
		"reports/b.csv,20,2026-09-28T00:00:01Z,GLACIER,e2\n"

	tests := []struct {
		name      string
		objects   []awsinternal.S3ObjectResource
		prefixes  []string
		truncated bool
		wantOut   string
		wantMsg   string
		wantErr   string
	}{
		{
			name:    "lists objects as csv",
			objects: objects,
			wantOut: wantTable,
		},
		{
			// 階層モードの応答が混ざっても CLI は prefixes を使わず、列と行は変わらない。
			name:     "ignores prefixes in the response",
			objects:  objects,
			prefixes: []string{"reports/2024/"},
			wantOut:  wantTable,
		},
		{
			name:      "warns on stderr only when truncated",
			objects:   objects,
			truncated: true,
			wantOut:   wantTable,
			wantErr:   "warning: object list truncated, narrow down with --prefix\n",
		},
		{
			// 空のときのメッセージは cmd.Println (cobra の出力先) に出る。表は出ない。
			name:    "prints the empty message when there are no objects",
			wantMsg: "No S3 objects found\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []s3ObjectsCall
			var cobraOut, stderr bytes.Buffer
			root := newRootCmdWithS3ObjectsLister(t, fakeS3ObjectsLister(&calls, tt.objects, tt.prefixes, tt.truncated), &cobraOut, &stderr)
			root.SetArgs(args)

			got := captureStdout(t, root.Execute)

			if diff := cmp.Diff([]s3ObjectsCall{wantCall}, calls, cmp.AllowUnexported(s3ObjectsCall{})); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			if got != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got, tt.wantOut)
			}
			if cobraOut.String() != tt.wantMsg {
				t.Errorf("cobra out = %q, want %q", cobraOut.String(), tt.wantMsg)
			}
			if stderr.String() != tt.wantErr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
		})
	}
}
