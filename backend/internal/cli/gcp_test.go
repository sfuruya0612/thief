package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sfuruya0612/thief/backend/internal/gcp"
	"github.com/spf13/cobra"
)

// gcsObjectsCall は fakeGCSObjectsLister が記録する呼び出しの内容。
type gcsObjectsCall struct {
	projectID, bucket, prefix, delimiter string
}

// fakeGCSObjectsLister は呼び出しの引数を calls に記録し、固定の応答を返す取得関数を返す。
func fakeGCSObjectsLister(calls *[]gcsObjectsCall, objects []gcp.ObjectInfo, prefixes []string, truncated bool) gcsObjectsLister {
	return func(_ context.Context, projectID, bucket, prefix, delimiter string) ([]gcp.ObjectInfo, []string, bool, error) {
		*calls = append(*calls, gcsObjectsCall{projectID: projectID, bucket: bucket, prefix: prefix, delimiter: delimiter})
		return objects, prefixes, truncated, nil
	}
}

// newRootCmdWithGCSObjectsLister は gcp コマンドを差し替えた取得関数で組み立て直したルートを返す。
// 標準エラー出力は stderr に集める (打ち切りの警告の検証に使う)。
func newRootCmdWithGCSObjectsLister(t *testing.T, list gcsObjectsLister, stderr *bytes.Buffer) *cobra.Command {
	t.Helper()
	root := NewRootCmd()
	for _, c := range root.Commands() {
		if c.Name() == "gcp" {
			root.RemoveCommand(c)
		}
	}
	root.AddCommand(newGCPCmdWithObjectsLister(list))
	root.SetErr(stderr)
	return root
}

// TestGCPGCSObjectsOutput は `thief gcp gcs objects` が delimiter を渡さずに一覧を取得し、
// 応答に prefixes が混ざっても出力の列と行が変わらないこと、打ち切りの警告が truncated の
// ときだけ標準エラー出力に出る (ちょうど上限で終わる false のときは出ない) ことを、
// 実コマンドツリーの実行で確かめる。
func TestGCPGCSObjectsOutput(t *testing.T) {
	// loadConfig の先の config.Load が実行環境の設定を読まないよう、参照先を空に向ける。
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	objects := []gcp.ObjectInfo{
		{Name: "reports/a.csv", Bucket: "my-bucket", Size: 10, ContentType: "text/csv", StorageClass: "STANDARD", Updated: "2026-09-28T00:00:00Z"},
		{Name: "reports/b.csv", Bucket: "my-bucket", Size: 20, ContentType: "text/csv", StorageClass: "NEARLINE", Updated: "2026-09-28T00:00:01Z"},
	}
	args := []string{"gcp", "gcs", "objects", "my-bucket", "--prefix", "reports/", "--project", "my-project", "-o", "csv"}
	wantCall := gcsObjectsCall{projectID: "my-project", bucket: "my-bucket", prefix: "reports/", delimiter: ""}
	header := "Name,Bucket,Size,ContentType,StorageClass,Updated\n"
	wantTable := header +
		"reports/a.csv,my-bucket,10,text/csv,STANDARD,2026-09-28T00:00:00Z\n" +
		"reports/b.csv,my-bucket,20,text/csv,NEARLINE,2026-09-28T00:00:01Z\n"

	tests := []struct {
		name      string
		objects   []gcp.ObjectInfo
		prefixes  []string
		truncated bool
		wantOut   string
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
			// 該当が無いときはヘッダだけを出す (本 issue の前からの挙動)。
			name:    "prints only the header when there are no objects",
			wantOut: header,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []gcsObjectsCall
			var stderr bytes.Buffer
			root := newRootCmdWithGCSObjectsLister(t, fakeGCSObjectsLister(&calls, tt.objects, tt.prefixes, tt.truncated), &stderr)
			root.SetArgs(args)

			got := captureStdout(t, root.Execute)

			if diff := cmp.Diff([]gcsObjectsCall{wantCall}, calls, cmp.AllowUnexported(gcsObjectsCall{})); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			if got != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got, tt.wantOut)
			}
			if stderr.String() != tt.wantErr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
		})
	}
}
