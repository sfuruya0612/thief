package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newCostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Show AWS cost and forecast",
	}

	costCmd := &cobra.Command{
		Use:   "ls",
		Short: "Show cost by service",
		RunE: func(cmd *cobra.Command, args []string) error {
			includeToday, _ := cmd.Flags().GetBool("include-today")
			return runList(cmd,
				[]util.Column{{Header: "Date"}, {Header: "Service"}, {Header: "Unblended"}, {Header: "NetAmortized"}, {Header: "Unit"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.CostResource, error) {
					return awsinternal.GetCost(ctx, profile, region, includeToday)
				},
			)
		},
	}
	costCmd.Flags().Bool("include-today", false, "Include today's unblended cost")

	forecastCmd := &cobra.Command{
		Use:   "forecast",
		Short: "Show cost forecast for current month",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Period"}, {Header: "Amount"}, {Header: "Unit"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.ForecastResource, error) {
					return awsinternal.GetForecast(ctx, profile, region)
				},
			)
		},
	}

	cmd.AddCommand(costCmd, forecastCmd)
	return cmd
}
