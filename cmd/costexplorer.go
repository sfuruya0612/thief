// Package cmd implements the command line interface for thief.
package cmd

import (
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/util"
	"github.com/spf13/cobra"
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

// showCostByService retrieves and displays cost data aggregated by service.
func showCostByService(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()
	startDate := cmd.Flag("start-date").Value.String()
	endDate := cmd.Flag("end-date").Value.String()

	// Default to the first day of the current month to today
	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(dateFormat)
	}

	if endDate == "" {
		endDate = time.Now().Format(dateFormat)
	}

	client, err := aws.NewCostExplorerClient(profile, region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	// Get the metric type from the flag
	metric := aws.CostMetric(costMetric)

	costs, err := client.GetCostByService(startDate, endDate, metric)
	if err != nil {
		return fmt.Errorf("get costs by service: %w", err)
	}

	var items [][]string
	for _, cost := range costs {
		items = append(items, []string{
			cost.TimePeriod,
			cost.ServiceName,
			cost.Amount,
			cost.Unit,
		})
	}

	formatter := util.NewTableFormatter(costColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(items)
	return nil
}

// showCostByAccount retrieves and displays cost data aggregated by account.
func showCostByAccount(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()
	startDate := cmd.Flag("start-date").Value.String()
	endDate := cmd.Flag("end-date").Value.String()

	// Default to the first day of the current month to today
	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(dateFormat)
	}

	if endDate == "" {
		endDate = time.Now().Format(dateFormat)
	}

	client, err := aws.NewCostExplorerClient(profile, region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	// Get the metric type from the flag
	metric := aws.CostMetric(costMetric)

	costs, err := client.GetCostByAccount(startDate, endDate, metric)
	if err != nil {
		return fmt.Errorf("get costs by account: %w", err)
	}

	var items [][]string
	for _, cost := range costs {
		items = append(items, []string{
			cost.TimePeriod,
			cost.GroupKey,
			cost.Amount,
			cost.Unit,
		})
	}

	formatter := util.NewTableFormatter(costColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(items)
	return nil
}

// showCostOverview retrieves and displays an overview of costs.
func showCostOverview(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()
	startDate := cmd.Flag("start-date").Value.String()
	endDate := cmd.Flag("end-date").Value.String()

	// Default to the first day of the current month to today
	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(dateFormat)
	}

	if endDate == "" {
		endDate = time.Now().Format(dateFormat)
	}

	client, err := aws.NewCostExplorerClient(profile, region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	// Get the metric type from the flag
	metric := aws.CostMetric(costMetric)

	costs, err := client.GetCostForPeriod(startDate, endDate, metric)
	if err != nil {
		return fmt.Errorf("get cost for period: %w", err)
	}

	var items [][]string
	for _, cost := range costs {
		items = append(items, []string{
			cost.TimePeriod,
			"Total",
			cost.Amount,
			cost.Unit,
		})
	}

	formatter := util.NewTableFormatter(costColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(items)
	return nil
}
