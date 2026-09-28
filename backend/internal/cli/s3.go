package cli

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var s3Columns = []util.Column{
	{Header: "BucketName"},
	{Header: "CreationDate"},
}

var s3ObjectColumns = []util.Column{
	{Header: "Name"},
	{Header: "Size"},
	{Header: "LastModified"},
	{Header: "StorageClass"},
	{Header: "ETag"},
}

// s3ObjectsLister は `s3 objects` が呼ぶ一覧取得関数の型。テストで AWS への接続を差し替え、
// フラグから取得関数の引数と出力までの結線を検証できるようにする (elbOps と同じ形)。
type s3ObjectsLister func(ctx context.Context, profile, region, bucket, prefix, delimiter string) ([]awsinternal.S3ObjectResource, []string, bool, error)

func newS3Cmd() *cobra.Command {
	return newS3CmdWithObjectsLister(awsinternal.ListS3Objects)
}

// newS3CmdWithObjectsLister は objects の取得関数を差し替えられる形で s3 コマンドを組み立てる。
func newS3CmdWithObjectsLister(listObjects s3ObjectsLister) *cobra.Command {
	s3Cmd := &cobra.Command{
		Use:   "s3",
		Short: "S3 commands",
	}

	lsCmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List S3 buckets",
		Long:    "Retrieves and displays a list of S3 buckets in the AWS account.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.S3BucketInfo]{
				Columns:  s3Columns,
				EmptyMsg: "No S3 buckets found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.S3BucketInfo, error) {
					buckets, err := awsinternal.ListS3BucketInfos(ctx, cfg.Profile)
					if err != nil {
						return nil, fmt.Errorf("list S3 buckets: %w", err)
					}
					return buckets, nil
				},
			})
		},
	}

	objectsCmd := &cobra.Command{
		Use:   "objects <bucket>",
		Short: "List objects in an S3 bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, _ := cmd.Flags().GetString("prefix")
			return s3RunObjects(cmd, listObjects, args[0], prefix)
		},
	}
	objectsCmd.Flags().String("prefix", "", "Object key prefix filter")

	downloadCmd := &cobra.Command{
		Use:   "download <bucket> <key>",
		Short: "Download an S3 object to a local file",
		Long: "Downloads an S3 object. --output-file defaults to the base name of the key " +
			"in the current directory. An existing file is overwritten.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFile, _ := cmd.Flags().GetString("output-file")
			return s3RunDownload(cmd, args[0], args[1], outputFile)
		},
	}
	downloadCmd.Flags().String("output-file", "", "Local file path (default: base name of the key)")

	uploadCmd := &cobra.Command{
		Use:   "upload <bucket> <key> <file>",
		Short: "Upload a local file to an S3 object",
		Long:  "Uploads a local file. The Content-Type is inferred from the file extension.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s3RunUpload(cmd, args[0], args[1], args[2])
		},
	}

	s3Cmd.AddCommand(lsCmd, objectsCmd, downloadCmd, uploadCmd)
	return s3Cmd
}

// resolveDownloadPath は `download` の書き出し先を解決する。
// --output-file が未指定なら key の base name をカレントディレクトリに使う。key が "/" で
// 終わる場合は base name が決まらないためエラーにする。
func resolveDownloadPath(key, outputFile string) (string, error) {
	if outputFile != "" {
		return outputFile, nil
	}
	if strings.HasSuffix(key, "/") {
		return "", fmt.Errorf("cannot derive a file name from key %q: specify --output-file", key)
	}
	base := path.Base(key)
	if base == "" || base == "." || base == "/" {
		return "", fmt.Errorf("cannot derive a file name from key %q: specify --output-file", key)
	}
	return base, nil
}

// contentTypeForFile はローカルファイルの拡張子から Content-Type を推定する。
// 判定できない場合は空文字を返す (PutObject 側で ContentType を付けない)。
func contentTypeForFile(filePath string) string {
	return mime.TypeByExtension(filepath.Ext(filePath))
}

// s3RunObjects はオブジェクトの平らな一覧を出力する。CLI は階層表示を扱わないため
// delimiter に空文字を渡し、応答の prefixes は使わない。
func s3RunObjects(cmd *cobra.Command, listObjects s3ObjectsLister, bucket, prefix string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	objects, _, truncated, err := listObjects(commandContext(cmd), cfg.Profile, cfg.Region, bucket, prefix, "")
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		cmd.Println("No S3 objects found")
		return nil
	}
	rows := make([][]string, len(objects))
	for i, o := range objects {
		rows[i] = []string{o.Key, fmt.Sprintf("%d", o.Size), o.LastModified, o.StorageClass, o.ETag}
	}
	if err := printRowsOrGroupBy(cfg, s3ObjectColumns, rows); err != nil {
		return err
	}
	if truncated {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: object list truncated, narrow down with --prefix")
	}
	return nil
}

func s3RunDownload(cmd *cobra.Command, bucket, key, outputFile string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	dest, err := resolveDownloadPath(key, outputFile)
	if err != nil {
		return err
	}
	out, err := awsinternal.GetS3Object(commandContext(cmd), cfg.Profile, cfg.Region, bucket, key)
	if err != nil {
		return err
	}
	defer out.Body.Close()

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	if _, err := io.Copy(f, out.Body); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dest, err)
	}
	cmd.Printf("Downloaded s3://%s/%s to %s\n", bucket, key, dest)
	return nil
}

func s3RunUpload(cmd *cobra.Command, bucket, key, filePath string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", filePath, err)
	}
	if err := awsinternal.PutS3Object(
		commandContext(cmd), cfg.Profile, cfg.Region, bucket, key, f, info.Size(), contentTypeForFile(filePath),
	); err != nil {
		return err
	}
	cmd.Printf("Uploaded %s to s3://%s/%s\n", filePath, bucket, key)
	return nil
}
