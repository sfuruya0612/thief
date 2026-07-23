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

var elasticacheParameterColumns = []util.Column{
	{Header: "Name"},
	{Header: "Value"},
	{Header: "ChangeType"},
	{Header: "DataType"},
	{Header: "IsModifiable"},
	{Header: "Source"},
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

	parametersCmd := &cobra.Command{
		Use:   "parameters",
		Short: "List parameters in a cache parameter group",
		Long:  "Retrieves and displays all parameters in the specified cache parameter group.",
		RunE: func(cmd *cobra.Command, args []string) error {
			group, err := cmd.Flags().GetString("group")
			if err != nil {
				return err
			}
			return runList(cmd, ListConfig[awsinternal.ElastiCacheParameterInfo]{
				Columns:  elasticacheParameterColumns,
				EmptyMsg: "No parameters found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ElastiCacheParameterInfo, error) {
					return awsinternal.ListElastiCacheParameterInfos(ctx, cfg.Profile, cfg.Region, group)
				},
			})
		},
	}
	parametersCmd.Flags().StringP("group", "", "", "Cache parameter group name")

	elasticacheCmd.AddCommand(lsCmd, parametersCmd)
	return elasticacheCmd
}
