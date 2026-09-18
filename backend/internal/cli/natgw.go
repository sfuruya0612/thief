package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var natgwColumns = []util.Column{
	{Header: "Name"},
	{Header: "State"},
	{Header: "GatewayID"},
	{Header: "VPC"},
	{Header: "ElasticIP"},
	{Header: "LaunchTime"},
}

func newNATGatewayCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "natgw",
		Aliases: []string{"nat"},
		Short:   "List NAT Gateways",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.NATGatewayResource]{
				Columns:  natgwColumns,
				EmptyMsg: "No NAT Gateways found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.NATGatewayResource, error) {
					return awsinternal.ListNATGatewayResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}
