package cli

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newDatadogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "datadog",
		Short: "Datadog cost operations",
	}
	cmd.PersistentFlags().String("start-month", "", "Start month (YYYY-MM)")
	cmd.PersistentFlags().String("end-month", "", "End month (YYYY-MM)")

	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost operations",
	}

	historicalCmd := &cobra.Command{
		Use:   "historical",
		Short: "Show historical cost",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			startMonth, _ := cmd.Flags().GetString("start-month")
			endMonth, _ := cmd.Flags().GetString("end-month")
			view, _ := cmd.Flags().GetString("view")

			ddCfg := datadog.NewConfiguration(cfg.Datadog.Site)
			ddV2 := datadog.NewUsageMeteringV2API(ddCfg)
			ddCtx := datadog.NewContext(context.Background(), cfg.DatadogAPIKey(), cfg.DatadogAppKey())

			items, err := datadog.GetHistoricalCost(ddCtx, ddV2, startMonth, endMonth, view)
			if err != nil {
				return err
			}
			rows := make([][]string, len(items))
			for i, item := range items {
				rows[i] = []string{item.Month, item.AccountName, item.OrgName, item.ProductName, item.ChargeType, fmt.Sprintf("%.4f", item.Cost)}
			}
			cols := []util.Column{{Header: "Month"}, {Header: "Account"}, {Header: "Org"}, {Header: "Product"}, {Header: "ChargeType"}, {Header: "Cost"}}
			f := util.NewTableFormatter(cols, cfg.Output)
			if !cfg.NoHeader {
				f.PrintHeader()
			}
			f.PrintRows(rows)
			return nil
		},
	}
	historicalCmd.Flags().String("view", "summary", "View type")

	costCmd.AddCommand(historicalCmd)
	cmd.AddCommand(costCmd)
	return cmd
}
