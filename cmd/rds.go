package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

var rdsCmd = &cobra.Command{
	Use:   "rds",
	Short: "Manage RDS",
}

var rdsInstanceCmd = &cobra.Command{
	Use:   "ls",
	Short: "List RDS instances",
	Long:  "Retrieves and displays a list of RDS DB instances in the current region.",
	RunE:  listRDSInstances,
}

var rdsClusterCmd = &cobra.Command{
	Use:     "cluster",
	Aliases: []string{"c"},
	Short:   "List RDS clusters",
	Long:    "Retrieves and displays a list of RDS DB clusters in the current region.",
	RunE:    listRDSClusters,
}

var rdsInstanceColumns = []util.Column{
	{Header: "Name"},
	{Header: "DBInstanceClass"},
	{Header: "Engine"},
	{Header: "EngineVersion"},
	{Header: "Storage"},
	{Header: "StorageType"},
	{Header: "DBInstanceStatus"},
}

var rdsClusterColumns = []util.Column{
	{Header: "Name"},
	{Header: "Engine"},
	{Header: "EngineVersion"},
	{Header: "EngineMode"},
	{Header: "Status"},
}

func listRDSInstances(cmd *cobra.Command, args []string) error {
	return runList(cmd, ListConfig[aws.RDSInstanceInfo]{
		Columns:  rdsInstanceColumns,
		EmptyMsg: "No DB instances found",
		Fetch: func(cfg *config.Config) ([]aws.RDSInstanceInfo, error) {
			client, err := aws.NewRdsClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, fmt.Errorf("create RDS client: %w", err)
			}
			return aws.DescribeDBInstances(client, aws.GenerateDescribeDBInstancesInput(&aws.RdsOpts{}))
		},
	})
}

func listRDSClusters(cmd *cobra.Command, args []string) error {
	return runList(cmd, ListConfig[aws.RDSClusterInfo]{
		Columns:  rdsClusterColumns,
		EmptyMsg: "No DB clusters found",
		Fetch: func(cfg *config.Config) ([]aws.RDSClusterInfo, error) {
			client, err := aws.NewRdsClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, fmt.Errorf("create RDS client: %w", err)
			}
			return aws.DescribeDBClusters(client, aws.GenerateDescribeDBClustersInput(&aws.RdsOpts{}))
		},
	})
}
