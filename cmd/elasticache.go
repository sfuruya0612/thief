package cmd

import (
	"fmt"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/util"
	"github.com/spf13/cobra"
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
	{Header: "ReplicationGroupId", Width: 30},
	{Header: "CacheClusterId", Width: 30},
	{Header: "CacheNodeType", Width: 20},
	{Header: "Engine", Width: 10},
	{Header: "EngineVersion", Width: 15},
	{Header: "CacheClusterStatus", Width: 20},
}

// listElastiCacheClusters retrieves and displays ElastiCache clusters.
func listElastiCacheClusters(cmd *cobra.Command, args []string) error {
	client, err := aws.NewElasticacheClient(cmd.Flag("profile").Value.String(), cmd.Flag("region").Value.String())
	if err != nil {
		return fmt.Errorf("create ElastiCache client: %w", err)
	}

	input := aws.GenerateDescribeCacheClustersInput(&aws.ElasticacheOpts{})

	clusters, err := aws.DescribeCacheClusters(client, input)
	if err != nil {
		return fmt.Errorf("describe cache clusters: %w", err)
	}

	if len(clusters) == 0 {
		cmd.Println("No cache clusters found")
		return nil
	}

	formatter := util.NewTableFormatter(elasticacheColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(clusters)
	return nil
}
