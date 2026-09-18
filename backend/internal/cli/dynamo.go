package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var dynamoColumns = []util.Column{
	{Header: "Table"},
	{Header: "State"},
	{Header: "Mode"},
	{Header: "Items"},
	{Header: "Size"},
	{Header: "GSI"},
}

func newDynamoCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "dynamo",
		Aliases: []string{"dynamodb"},
		Short:   "List DynamoDB tables",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.DynamoResource]{
				Columns:  dynamoColumns,
				EmptyMsg: "No DynamoDB tables found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.DynamoResource, error) {
					return awsinternal.ListDynamoResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}
