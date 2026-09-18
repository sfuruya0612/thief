package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var sqsColumns = []util.Column{
	{Header: "Queue"},
	{Header: "State"},
	{Header: "Type"},
	{Header: "Available"},
	{Header: "InFlight"},
	{Header: "Retention(d)"},
}

func newSQSCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sqs",
		Short: "List SQS queues",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.SQSResource]{
				Columns:  sqsColumns,
				EmptyMsg: "No SQS queues found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.SQSResource, error) {
					return awsinternal.ListSQSResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}
