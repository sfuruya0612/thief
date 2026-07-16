package cli

import (
	"context"
	"fmt"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var s3Columns = []util.Column{
	{Header: "BucketName"},
	{Header: "CreationDate"},
}

func newS3Cmd() *cobra.Command {
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

	s3Cmd.AddCommand(lsCmd)
	return s3Cmd
}
