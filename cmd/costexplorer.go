// Package cmd implements the command line interface for thief.
package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

const (
	// dateFormat is the standard date format used in the application.
	dateFormat = "2006-01-02"
)

// costexplorerCmd represents the base command for cost explorer operations.
var costexplorerCmd = &cobra.Command{
	Use:   "cost",
	Short: "AWS Cost Explorer",
	Long:  `Provides commands to interact with AWS Cost Explorer to retrieve cost and usage data.`,
}

var costMetric string

// costByServiceCmd represents the command to show costs by AWS service.
var costByServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Show costs by AWS service",
	Long:  `Retrieves and displays cost data aggregated by AWS service for a specified period.`,
	RunE:  showCostByService,
}

// costByAccountCmd represents the command to show costs by AWS account.
var costByAccountCmd = &cobra.Command{
	Use:   "account",
	Short: "Show costs by AWS account",
	Long:  `Retrieves and displays cost data aggregated by AWS account for a specified period.`,
	RunE:  showCostByAccount,
}

// costByUsageTypeCmd represents the command to show costs by AWS usage type.
var costByUsageTypeCmd = &cobra.Command{
	Use:   "usage-type",
	Short: "Show costs by AWS usage type",
	Long:  `Retrieves and displays cost data aggregated by AWS usage type for a specified period.`,
	RunE:  showCostByUsageType,
}

// costOverviewCmd represents the command to show a cost overview.
var costOverviewCmd = &cobra.Command{
	Use:   "overview",
	Short: "Show cost overview",
	Long:  `Retrieves and displays an overview of costs for a specified period.`,
	RunE:  showCostOverview,
}

var costColumns = []util.Column{
	{Header: "Period", Width: 10},
	{Header: "Service/Account", Width: 50},
	{Header: "Amount", Width: 15},
	{Header: "Unit", Width: 5},
}

// resolveDates returns start/end dates, defaulting to current month if empty.
func resolveDates(cmd *cobra.Command) (string, string) {
	startDate := cmd.Flag("start-date").Value.String()
	endDate := cmd.Flag("end-date").Value.String()

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(dateFormat)
	}
	if endDate == "" {
		endDate = time.Now().Format(dateFormat)
	}
	return startDate, endDate
}

// printCostRows formats and prints a slice of CostDetail.
func printCostRows(cmd *cobra.Command, costs []aws.CostDetail, keyFn func(aws.CostDetail) string) error {
	cfg := config.FromContext(cmd.Context())

	var items [][]string
	for _, cost := range costs {
		items = append(items, []string{
			cost.TimePeriod,
			keyFn(cost),
			cost.Amount,
			cost.Unit,
		})
	}
	return printRowsOrGroupBy(cfg, costColumns, items)
}

// showCostByService retrieves and displays cost data aggregated by service.
func showCostByService(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	startDate, endDate := resolveDates(cmd)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostByService(startDate, endDate, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get costs by service: %w", err)
	}

	return printCostRows(cmd, costs, func(c aws.CostDetail) string { return c.ServiceName })
}

// showCostByAccount retrieves and displays cost data aggregated by account.
func showCostByAccount(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	startDate, endDate := resolveDates(cmd)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostByAccount(startDate, endDate, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get costs by account: %w", err)
	}

	return printCostRows(cmd, costs, func(c aws.CostDetail) string { return c.GroupKey })
}

// showCostByUsageType retrieves and displays cost data aggregated by usage type.
func showCostByUsageType(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	startDate, endDate := resolveDates(cmd)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostByUsageType(startDate, endDate, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get costs by usage type: %w", err)
	}

	return printCostRows(cmd, costs, func(c aws.CostDetail) string { return c.GroupKey })
}

// showCostOverview retrieves and displays an overview of costs.
func showCostOverview(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	startDate, endDate := resolveDates(cmd)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostForPeriod(startDate, endDate, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get cost for period: %w", err)
	}

	return printCostRows(cmd, costs, func(c aws.CostDetail) string { return "Total" })
}
