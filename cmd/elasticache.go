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
	Run:   listElastiCacheClusters,
}

var elasticacheColumns = []util.Column{
	{Header: "ReplicationGroupId", Width: 30},
	{Header: "CacheClusterId", Width: 30},
	{Header: "CacheNodeType", Width: 20},
	{Header: "Engine", Width: 10},
	{Header: "EngineVersion", Width: 15},
	{Header: "CacheClusterStatus", Width: 20},
}

func listElastiCacheClusters(cmd *cobra.Command, args []string) {
	client := aws.NewElasticacheClient(cmd.Flag("profile").Value.String(), cmd.Flag("region").Value.String())

	input := aws.GenerateDescribeCacheClustersInput(&aws.ElasticacheOpts{})

	clusters, err := aws.DescribeCacheClusters(client, input)
	if err != nil {
		fmt.Printf("Unable to describe cache clusters: %v\n", err)
		return
	}

	if len(clusters) == 0 {
		fmt.Println("No cache clusters found")
		return
	}

	formatter := util.NewTableFormatter(elasticacheColumns, cmd.Flag("output").Value.String())
	formatter.PrintHeader()
	formatter.PrintRows(clusters)
}
