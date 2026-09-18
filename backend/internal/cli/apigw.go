package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var apigwColumns = []util.Column{
	{Header: "API"},
	{Header: "State"},
	{Header: "Type"},
	{Header: "Stage"},
	{Header: "Endpoint"},
}

func newAPIGWCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apigw",
		Short: "List API Gateway APIs (REST/HTTP/WebSocket)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.APIGatewayResource]{
				Columns:  apigwColumns,
				EmptyMsg: "No API Gateway APIs found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.APIGatewayResource, error) {
					return awsinternal.ListAPIGatewayResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}
