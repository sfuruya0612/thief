package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newLambdaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lambda",
		Short: "List Lambda functions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "State"}, {Header: "Runtime"}, {Header: "Memory(MB)"}, {Header: "Timeout(s)"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.LambdaResource, error) {
					return awsinternal.ListLambdaResources(ctx, profile, region)
				},
			)
		},
	}
}
