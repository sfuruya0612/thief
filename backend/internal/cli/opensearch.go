package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var openSearchColumns = []util.Column{
	{Header: "Domain"},
	{Header: "State"},
	{Header: "Version"},
	{Header: "Instance"},
	{Header: "Nodes"},
	{Header: "Endpoint"},
}

func newOpenSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "opensearch",
		Short: "List Amazon OpenSearch Service domains",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.OpenSearchResource]{
				Columns:  openSearchColumns,
				EmptyMsg: "No OpenSearch domains found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.OpenSearchResource, error) {
					return awsinternal.ListOpenSearchResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}
