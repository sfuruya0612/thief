package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newLambdaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lambda",
		Short: "List Lambda functions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.LambdaResource]{
				Columns:  []util.Column{{Header: "Name"}, {Header: "State"}, {Header: "Runtime"}, {Header: "Memory(MB)"}, {Header: "Timeout(s)"}},
				EmptyMsg: "No Lambda functions found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.LambdaResource, error) {
					return awsinternal.ListLambdaResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}
