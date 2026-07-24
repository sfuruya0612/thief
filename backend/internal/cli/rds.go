package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

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

var rdsParameterColumns = []util.Column{
	{Header: "Name"},
	{Header: "Value"},
	{Header: "ApplyType"},
	{Header: "DataType"},
	{Header: "IsModifiable"},
	{Header: "Source"},
}

func newRDSCmd() *cobra.Command {
	rdsCmd := &cobra.Command{
		Use:   "rds",
		Short: "Manage RDS",
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List RDS instances",
		Long:  "Retrieves and displays a list of RDS DB instances in the current region.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.RDSInstanceInfo]{
				Columns:  rdsInstanceColumns,
				EmptyMsg: "No DB instances found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.RDSInstanceInfo, error) {
					return awsinternal.ListRDSInstanceInfos(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	clusterCmd := &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"c"},
		Short:   "List RDS clusters",
		Long:    "Retrieves and displays a list of RDS DB clusters in the current region.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.RDSClusterInfo]{
				Columns:  rdsClusterColumns,
				EmptyMsg: "No DB clusters found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.RDSClusterInfo, error) {
					return awsinternal.ListRDSClusterInfos(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	parametersCmd := &cobra.Command{
		Use:   "parameters",
		Short: "List parameters in an RDS DB parameter group",
		Long:  "Retrieves and displays all parameters in the specified DB parameter group.",
		RunE: func(cmd *cobra.Command, args []string) error {
			group, err := cmd.Flags().GetString("group")
			if err != nil {
				return err
			}
			return runList(cmd, ListConfig[awsinternal.RDSParameterInfo]{
				Columns:  rdsParameterColumns,
				EmptyMsg: "No parameters found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.RDSParameterInfo, error) {
					return awsinternal.ListRDSParameterInfos(ctx, cfg.Profile, cfg.Region, group)
				},
			})
		},
	}
	parametersCmd.Flags().StringP("group", "", "", "DB parameter group name")

	clusterParametersCmd := &cobra.Command{
		Use:   "cluster-parameters",
		Short: "List parameters in an RDS DB cluster parameter group",
		Long:  "Retrieves and displays all parameters in the DB cluster parameter group of the specified DB cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := cmd.Flags().GetString("cluster")
			if err != nil {
				return err
			}
			return runList(cmd, ListConfig[awsinternal.RDSParameterInfo]{
				Columns:  rdsParameterColumns,
				EmptyMsg: "No parameters found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.RDSParameterInfo, error) {
					return awsinternal.ListRDSClusterParameterInfos(ctx, cfg.Profile, cfg.Region, cluster)
				},
			})
		},
	}
	clusterParametersCmd.Flags().StringP("cluster", "", "", "DB cluster identifier")

	rdsCmd.AddCommand(lsCmd, clusterCmd, parametersCmd, clusterParametersCmd)
	return rdsCmd
}
