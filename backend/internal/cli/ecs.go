package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newECSCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ecs",
		Short: "List ECS clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "State"}, {Header: "Services"}, {Header: "Running"}, {Header: "Pending"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.ECSResource, error) {
					return awsinternal.ListECSResources(ctx, profile, region)
				},
			)
		},
	}
}
