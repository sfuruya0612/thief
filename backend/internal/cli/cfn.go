package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newCFNCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cfn",
		Short: "List CloudFormation stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "State"}, {Header: "CreatedAt"}, {Header: "UpdatedAt"}, {Header: "Drift"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.CFNStackResource, error) {
					return awsinternal.ListCFNStacks(ctx, profile, region)
				},
			)
		},
	}
}
