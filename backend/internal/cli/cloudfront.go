package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var cloudfrontColumns = []util.Column{
	{Header: "ID"},
	{Header: "Name"},
	{Header: "State"},
	{Header: "Domain"},
	{Header: "Origins"},
	{Header: "Enabled"},
}

func newCloudFrontCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cloudfront",
		Short: "List CloudFront distributions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.CloudFrontResource]{
				Columns:  cloudfrontColumns,
				EmptyMsg: "No CloudFront distributions found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.CloudFrontResource, error) {
					return awsinternal.ListCloudFrontResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}
