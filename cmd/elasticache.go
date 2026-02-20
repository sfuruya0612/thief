package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

var elasticacheCmd = &cobra.Command{
	Use:   "elasticache",
	Short: "Manage ElastiCache",
}

var elasticacheListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List ElastiCache clusters",
	Long:  "Retrieves and displays a list of ElastiCache clusters in the current region.",
	RunE:  listElastiCacheClusters,
}

var elasticacheColumns = []util.Column{
	{Header: "ReplicationGroupId"},
	{Header: "CacheClusterId"},
	{Header: "CacheNodeType"},
	{Header: "Engine"},
	{Header: "EngineVersion"},
	{Header: "CacheClusterStatus"},
}

func listElastiCacheClusters(cmd *cobra.Command, args []string) error {
	return runList(cmd, ListConfig[aws.ElastiCacheClusterInfo]{
		Columns:  elasticacheColumns,
		EmptyMsg: "No cache clusters found",
		Fetch: func(cfg *config.Config) ([]aws.ElastiCacheClusterInfo, error) {
			client, err := aws.NewElasticacheClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, fmt.Errorf("create ElastiCache client: %w", err)
			}
			return aws.DescribeCacheClusters(client, aws.GenerateDescribeCacheClustersInput(&aws.ElasticacheOpts{}))
		},
	})
}
