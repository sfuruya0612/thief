package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

func init() {
	rootCmd.AddCommand(s3Cmd)
	s3Cmd.AddCommand(s3ListCmd)
}

var s3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "S3 commands",
}

var s3ListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List S3 buckets",
	Long:    "Retrieves and displays a list of S3 buckets in the AWS account.",
	RunE:    displayS3Buckets,
}

var s3Columns = []util.Column{
	{Header: "BucketName", Width: 50},
	{Header: "CreationDate", Width: 20},
}

func displayS3Buckets(cmd *cobra.Command, args []string) error {
	return runList(cmd, ListConfig[aws.S3BucketInfo]{
		Columns:  s3Columns,
		EmptyMsg: "No S3 buckets found",
		Fetch: func(cfg *config.Config) ([]aws.S3BucketInfo, error) {
			client, err := aws.NewS3Client(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, fmt.Errorf("create S3 client: %w", err)
			}
			return aws.ListBuckets(client, aws.GenerateListBucketsInput())
		},
	})
}
