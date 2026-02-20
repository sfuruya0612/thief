// Package cmd implements the command line interface for thief.
package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
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
var costGranularity string

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

// parseGranularity converts a string to types.Granularity.
func parseGranularity(s string) (types.Granularity, error) {
	switch strings.ToUpper(s) {
	case "MONTHLY":
		return types.GranularityMonthly, nil
	case "DAILY":
		return types.GranularityDaily, nil
	default:
		return "", fmt.Errorf("unsupported granularity %q: use MONTHLY or DAILY", s)
	}
}

// resolveDates returns start/end dates based on granularity.
// When flags are not specified:
//   - MONTHLY: 3 months ago to today
//   - DAILY:   first day of current month to today
func resolveDates(cmd *cobra.Command, granularity types.Granularity) (string, string) {
	startDate := cmd.Flag("start-date").Value.String()
	endDate := cmd.Flag("end-date").Value.String()

	if startDate == "" {
		now := time.Now()
		switch granularity {
		case types.GranularityDaily:
			startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(dateFormat)
		default:
			// MONTHLY: 3 months back from the first day of current month
			startDate = time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, now.Location()).Format(dateFormat)
		}
	}
	if endDate == "" {
		endDate = time.Now().Format(dateFormat)
	}
	return startDate, endDate
}

// printCostRows formats and prints a slice of CostDetail as a matrix (pivot table).
// Columns: keyHeader | Unit | period1 | period2 | ...
// Rows are sorted by total amount descending.
func printCostRows(cmd *cobra.Command, costs []aws.CostDetail, keyHeader string, keyFn func(aws.CostDetail) string) error {
	cfg := config.FromContext(cmd.Context())

	// 1. Collect unique periods and sort chronologically.
	periodSet := make(map[string]struct{})
	for _, c := range costs {
		periodSet[c.TimePeriod] = struct{}{}
	}
	periods := make([]string, 0, len(periodSet))
	for p := range periodSet {
		periods = append(periods, p)
	}
	sort.Strings(periods)

	// 2. Build (key, unit) → period → amount mapping.
	type rowKey struct {
		key  string
		unit string
	}
	dataMap := make(map[rowKey]map[string]string)
	for _, c := range costs {
		rk := rowKey{key: keyFn(c), unit: c.Unit}
		if dataMap[rk] == nil {
			dataMap[rk] = make(map[string]string)
		}
		dataMap[rk][c.TimePeriod] = c.Amount
	}

	// 3. Build columns: keyHeader, Unit, period1, period2, ...
	columns := []util.Column{
		{Header: keyHeader},
		{Header: "Unit"},
	}
	for _, p := range periods {
		columns = append(columns, util.Column{Header: p})
	}

	// 4. Build rows and compute total amount for sorting.
	type rowWithTotal struct {
		row   []string
		total float64
	}
	var rows []rowWithTotal
	for rk, periodAmounts := range dataMap {
		row := []string{rk.key, rk.unit}
		var total float64
		for _, p := range periods {
			amt, ok := periodAmounts[p]
			if !ok {
				amt = "0"
			}
			row = append(row, amt)
			if v, err := strconv.ParseFloat(amt, 64); err == nil {
				total += v
			}
		}
		rows = append(rows, rowWithTotal{row: row, total: total})
	}

	// Sort by total amount descending.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].total > rows[j].total
	})

	// 5. Extract sorted rows for output.
	items := make([][]string, len(rows))
	for i, r := range rows {
		items[i] = r.row
	}

	return printRowsOrGroupBy(cfg, columns, items)
}

// showCostByService retrieves and displays cost data aggregated by service.
func showCostByService(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	granularity, err := parseGranularity(costGranularity)
	if err != nil {
		return err
	}
	startDate, endDate := resolveDates(cmd, granularity)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostByService(startDate, endDate, granularity, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get costs by service: %w", err)
	}

	return printCostRows(cmd, costs, "Service", func(c aws.CostDetail) string { return c.ServiceName })
}

// showCostByAccount retrieves and displays cost data aggregated by account.
func showCostByAccount(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	granularity, err := parseGranularity(costGranularity)
	if err != nil {
		return err
	}
	startDate, endDate := resolveDates(cmd, granularity)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostByAccount(startDate, endDate, granularity, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get costs by account: %w", err)
	}

	return printCostRows(cmd, costs, "Account", func(c aws.CostDetail) string { return c.GroupKey })
}

// showCostByUsageType retrieves and displays cost data aggregated by usage type.
func showCostByUsageType(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	granularity, err := parseGranularity(costGranularity)
	if err != nil {
		return err
	}
	startDate, endDate := resolveDates(cmd, granularity)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostByUsageType(startDate, endDate, granularity, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get costs by usage type: %w", err)
	}

	return printCostRows(cmd, costs, "UsageType", func(c aws.CostDetail) string { return c.GroupKey })
}

// showCostOverview retrieves and displays an overview of costs.
func showCostOverview(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	granularity, err := parseGranularity(costGranularity)
	if err != nil {
		return err
	}
	startDate, endDate := resolveDates(cmd, granularity)

	client, err := aws.NewCostExplorerClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CostExplorer client: %w", err)
	}

	costs, err := client.GetCostForPeriod(startDate, endDate, granularity, aws.CostMetric(costMetric))
	if err != nil {
		return fmt.Errorf("get cost for period: %w", err)
	}

	return printCostRows(cmd, costs, "Overview", func(c aws.CostDetail) string { return "Total" })
}
