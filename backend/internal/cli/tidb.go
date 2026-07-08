package cli

import (
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/tidb"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newTiDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tidb",
		Short: "TiDB Cloud operations",
	}

	projectCmd := &cobra.Command{
		Use:   "project",
		Short: "Project operations",
	}
	projectCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			client := tidb.NewClient(cfg.TiDB.PublicKey, cfg.TiDBPrivateKey())
			projects, err := client.ListProjects()
			if err != nil {
				return err
			}
			rows := make([][]string, len(projects))
			for i, p := range projects {
				rows[i] = []string{p.ID, p.Name, p.OrgID, fmt.Sprintf("%d", p.ClusterCount), fmt.Sprintf("%d", p.UserCount)}
			}
			cols := []util.Column{{Header: "ID"}, {Header: "Name"}, {Header: "OrgID"}, {Header: "Clusters"}, {Header: "Users"}}
			f := util.NewTableFormatter(cols, cfg.Output)
			if !cfg.NoHeader {
				f.PrintHeader()
			}
			f.PrintRows(rows)
			return nil
		},
	})

	clusterCmd := &cobra.Command{
		Use:   "cluster <project-id>",
		Short: "List clusters",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			client := tidb.NewClient(cfg.TiDB.PublicKey, cfg.TiDBPrivateKey())
			clusters, err := client.ListClusters(args[0])
			if err != nil {
				return err
			}
			rows := make([][]string, len(clusters))
			for i, c := range clusters {
				rows[i] = []string{c.ID, c.Name, c.Status, c.Region, c.ClusterType, c.CloudProvider}
			}
			cols := []util.Column{{Header: "ID"}, {Header: "Name"}, {Header: "Status"}, {Header: "Region"}, {Header: "Type"}, {Header: "Cloud"}}
			f := util.NewTableFormatter(cols, cfg.Output)
			if !cfg.NoHeader {
				f.PrintHeader()
			}
			f.PrintRows(rows)
			return nil
		},
	}

	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Show billing cost",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			month, _ := cmd.Flags().GetString("month")
			client := tidb.NewClient(cfg.TiDB.PublicKey, cfg.TiDBPrivateKey())
			costs, err := client.GetCost(month)
			if err != nil {
				return err
			}
			rows := make([][]string, len(costs))
			for i, c := range costs {
				rows[i] = []string{c.BilledDate, fmt.Sprintf("%.4f", c.Credits), fmt.Sprintf("%.4f", c.Discounts), fmt.Sprintf("%.4f", c.TotalCost)}
			}
			cols := []util.Column{{Header: "Month"}, {Header: "Credits"}, {Header: "Discounts"}, {Header: "TotalCost"}}
			f := util.NewTableFormatter(cols, cfg.Output)
			if !cfg.NoHeader {
				f.PrintHeader()
			}
			f.PrintRows(rows)
			return nil
		},
	}
	costCmd.Flags().String("month", "", "Billing month (YYYY-MM)")

	cmd.AddCommand(projectCmd, clusterCmd, costCmd)
	return cmd
}
