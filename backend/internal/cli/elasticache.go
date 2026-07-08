package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newElastiCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "elasticache",
		Short: "List ElastiCache clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "ID"}, {Header: "State"}, {Header: "Engine"}, {Header: "Version"}, {Header: "NodeType"}, {Header: "Nodes"}, {Header: "Endpoint"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.ElastiCacheResource, error) {
					return awsinternal.ListElastiCacheResources(ctx, profile, region)
				},
			)
		},
	}
}
