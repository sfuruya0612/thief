package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newS3Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "s3",
		Short: "List S3 buckets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "Region"}, {Header: "CreatedAt"}, {Header: "Public"}, {Header: "Encryption"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.S3Resource, error) {
					return awsinternal.ListS3Resources(ctx, profile, region)
				},
			)
		},
	}
}
