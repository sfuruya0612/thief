package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newECRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ecr",
		Short: "List ECR repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "URI"}, {Header: "CreatedAt"}, {Header: "Mutability"}, {Header: "ScanOnPush"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.ECRRepoResource, error) {
					return awsinternal.ListECRResources(ctx, profile, region)
				},
			)
		},
	}
	return cmd
}
