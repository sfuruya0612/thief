package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
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
	{Header: "Name", Width: 40},
	{Header: "DBInstanceClass", Width: 15},
	{Header: "Engine", Width: 16},
	{Header: "EngineVersion", Width: 22},
	{Header: "Storage", Width: 8},
	{Header: "StorageType", Width: 10},
	{Header: "DBInstanceStatus", Width: 15},
}

var rdsClusterColumns = []util.Column{
	{Header: "Name", Width: 65},
	{Header: "Engine", Width: 16},
	{Header: "EngineVersion", Width: 22},
	{Header: "EngineMode", Width: 12},
	{Header: "Status", Width: 15},
}

// listRDSInstances retrieves and displays RDS DB instances.
func listRDSInstances(cmd *cobra.Command, args []string) error {
	client, err := aws.NewRdsClient(cmd.Flag("profile").Value.String(), cmd.Flag("region").Value.String())
	if err != nil {
		return fmt.Errorf("create RDS client: %w", err)
	}

	input := aws.GenerateDescribeDBInstancesInput(&aws.RdsOpts{})

	instances, err := aws.DescribeDBInstances(client, input)
	if err != nil {
		return fmt.Errorf("describe DB instances: %w", err)
	}

	if len(instances) == 0 {
		cmd.Println("No DB instances found")
		return nil
	}

	formatter := util.NewTableFormatter(rdsInstanceColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(instances)
	return nil
}

// listRDSClusters retrieves and displays RDS DB clusters.
func listRDSClusters(cmd *cobra.Command, args []string) error {
	client, err := aws.NewRdsClient(cmd.Flag("profile").Value.String(), cmd.Flag("region").Value.String())
	if err != nil {
		return fmt.Errorf("create RDS client: %w", err)
	}

	input := aws.GenerateDescribeDBClustersInput(&aws.RdsOpts{})

	clusters, err := aws.DescribeDBClusters(client, input)
	if err != nil {
		return fmt.Errorf("describe DB clusters: %w", err)
	}

	if len(clusters) == 0 {
		cmd.Println("No DB clusters found")
		return nil
	}

	formatter := util.NewTableFormatter(rdsClusterColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(clusters)
	return nil
}
