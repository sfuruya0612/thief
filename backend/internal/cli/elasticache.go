package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var elasticacheColumns = []util.Column{
	{Header: "ReplicationGroupId"},
	{Header: "CacheClusterId"},
	{Header: "CacheNodeType"},
	{Header: "Engine"},
	{Header: "EngineVersion"},
	{Header: "CacheClusterStatus"},
}

func newElastiCacheCmd() *cobra.Command {
	elasticacheCmd := &cobra.Command{
		Use:   "elasticache",
		Short: "Manage ElastiCache",
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List ElastiCache clusters",
		Long:  "Retrieves and displays a list of ElastiCache clusters in the current region.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.ElastiCacheClusterInfo]{
				Columns:  elasticacheColumns,
				EmptyMsg: "No cache clusters found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ElastiCacheClusterInfo, error) {
					return awsinternal.ListElastiCacheClusterInfos(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	elasticacheCmd.AddCommand(lsCmd)
	return elasticacheCmd
}
