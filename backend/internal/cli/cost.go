package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

// costDateFormat is the standard date format used by Cost Explorer.
const costDateFormat = "2006-01-02"

func newCostCmd() *cobra.Command {
	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "AWS Cost Explorer",
		Long:  `Provides commands to interact with AWS Cost Explorer to retrieve cost and usage data.`,
	}

	costCmd.PersistentFlags().StringP("start-date", "", "", "Start date (YYYY-MM-DD)")
	costCmd.PersistentFlags().StringP("end-date", "", "", "End date (YYYY-MM-DD)")
	costCmd.PersistentFlags().StringP("metric", "m", "UnblendedCost", "Cost metric (UnblendedCost, BlendedCost, NetUnblendedCost, NetAmortizedCost, AmortizedCost, UsageQuantity, NormalizedUsageAmount)")
	costCmd.PersistentFlags().StringP("granularity", "G", "MONTHLY", "Cost granularity (MONTHLY, DAILY)")

	serviceCmd := &cobra.Command{
		Use:   "service",
		Short: "Show costs by AWS service",
		Long:  `Retrieves and displays cost data aggregated by AWS service for a specified period.`,
		RunE:  showCostByService,
	}

	accountCmd := &cobra.Command{
		Use:   "account",
		Short: "Show costs by AWS account",
		Long:  `Retrieves and displays cost data aggregated by AWS account for a specified period.`,
		RunE:  showCostByAccount,
	}

	usageTypeCmd := &cobra.Command{
		Use:   "usage-type",
		Short: "Show costs by AWS usage type",
		Long:  `Retrieves and displays cost data aggregated by AWS usage type for a specified period.`,
		RunE:  showCostByUsageType,
	}

	overviewCmd := &cobra.Command{
		Use:   "overview",
		Short: "Show cost overview",
		Long:  `Retrieves and displays an overview of costs for a specified period.`,
		RunE:  showCostOverview,
	}

	// backend 専用のサブコマンド (レガシー CLI には存在しない)
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "Show cost by service (daily, last month)",
		RunE: func(cmd *cobra.Command, args []string) error {
			includeToday, _ := cmd.Flags().GetBool("include-today")
			return runList(cmd, ListConfig[awsinternal.CostResource]{
				Columns:  []util.Column{{Header: "Date"}, {Header: "Service"}, {Header: "Unblended"}, {Header: "NetAmortized"}, {Header: "Unit"}},
				EmptyMsg: "No cost data found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.CostResource, error) {
					return awsinternal.GetCost(ctx, cfg.Profile, cfg.Region, awsinternal.CostQueryOptions{IncludeToday: includeToday})
				},
			})
		},
	}
	lsCmd.Flags().Bool("include-today", false, "Include today's unblended cost")

	forecastCmd := &cobra.Command{
		Use:   "forecast",
		Short: "Show cost forecast for current month",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.ForecastResource]{
				Columns:  []util.Column{{Header: "Period"}, {Header: "Amount"}, {Header: "Unit"}},
				EmptyMsg: "No forecast data found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ForecastResource, error) {
					return awsinternal.GetForecast(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	costCmd.AddCommand(serviceCmd, accountCmd, usageTypeCmd, overviewCmd, lsCmd, forecastCmd)
	return costCmd
}

// parseGranularity converts a string to cetypes.Granularity.
func parseGranularity(s string) (cetypes.Granularity, error) {
	switch strings.ToUpper(s) {
	case "MONTHLY":
		return cetypes.GranularityMonthly, nil
	case "DAILY":
		return cetypes.GranularityDaily, nil
	default:
		return "", fmt.Errorf("unsupported granularity %q: use MONTHLY or DAILY", s)
	}
}

// resolveDates returns start/end dates based on granularity.
// When flags are not specified:
//   - MONTHLY: 3 months ago to today
//   - DAILY:   first day of current month to today
func resolveDates(startDate, endDate string, granularity cetypes.Granularity, now time.Time) (string, string) {
	if startDate == "" {
		switch granularity {
		case cetypes.GranularityDaily:
			startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(costDateFormat)
		default:
			// MONTHLY: 3 months back from the first day of current month
			startDate = time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, now.Location()).Format(costDateFormat)
		}
	}
	if endDate == "" {
		endDate = now.Format(costDateFormat)
	}
	return startDate, endDate
}

// costQueryParams は cost サブコマンド共通のフラグを解決した値を保持する。
type costQueryParams struct {
	cfg         *config.Config
	startDate   string
	endDate     string
	granularity cetypes.Granularity
	metric      awsinternal.CostMetric
}

// resolveCostParams は cost サブコマンド共通のフラグと設定を解決する。
func resolveCostParams(cmd *cobra.Command) (*costQueryParams, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return nil, err
	}

	granularity, err := parseGranularity(cmd.Flag("granularity").Value.String())
	if err != nil {
		return nil, err
	}

	startDate, endDate := resolveDates(
		cmd.Flag("start-date").Value.String(),
		cmd.Flag("end-date").Value.String(),
		granularity,
		time.Now(),
	)

	return &costQueryParams{
		cfg:         cfg,
		startDate:   startDate,
		endDate:     endDate,
		granularity: granularity,
		metric:      awsinternal.CostMetric(cmd.Flag("metric").Value.String()),
	}, nil
}

// buildCostMatrix はコスト明細をピボット表 (行キー x 期間) に変換する。
// 行は合計金額の降順に並べる。
func buildCostMatrix(costs []awsinternal.CostDetail, keyHeader string, keyFn func(awsinternal.CostDetail) string) ([]util.Column, [][]string) {
	// 1. ユニークな期間を収集し時系列順にソートする。
	periodSet := make(map[string]struct{})
	for _, c := range costs {
		periodSet[c.TimePeriod] = struct{}{}
	}
	periods := make([]string, 0, len(periodSet))
	for p := range periodSet {
		periods = append(periods, p)
	}
	sort.Strings(periods)

	// 2. (キー, 単位) → 期間 → 金額のマッピングを構築する。
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

	// 3. 列: keyHeader, Unit, period1, period2, ...
	columns := []util.Column{
		{Header: keyHeader},
		{Header: "Unit"},
	}
	for _, p := range periods {
		columns = append(columns, util.Column{Header: p})
	}

	// 4. 行を構築し、ソート用に合計金額を計算する。
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

	// 合計金額の降順にソートする。
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].total > rows[j].total
	})

	items := make([][]string, len(rows))
	for i, r := range rows {
		items[i] = r.row
	}

	return columns, items
}

// printCostRows formats and prints a slice of CostDetail as a matrix (pivot table).
func printCostRows(cfg *config.Config, costs []awsinternal.CostDetail, keyHeader string, keyFn func(awsinternal.CostDetail) string) error {
	columns, items := buildCostMatrix(costs, keyHeader, keyFn)
	return printRowsOrGroupBy(cfg, columns, items)
}

// showCostByService retrieves and displays cost data aggregated by service.
func showCostByService(cmd *cobra.Command, args []string) error {
	p, err := resolveCostParams(cmd)
	if err != nil {
		return err
	}

	costs, err := awsinternal.GetCostByService(context.Background(), p.cfg.Profile, p.cfg.Region, p.startDate, p.endDate, p.granularity, p.metric)
	if err != nil {
		return fmt.Errorf("get costs by service: %w", err)
	}

	return printCostRows(p.cfg, costs, "Service", func(c awsinternal.CostDetail) string { return c.ServiceName })
}

// showCostByAccount retrieves and displays cost data aggregated by account.
func showCostByAccount(cmd *cobra.Command, args []string) error {
	p, err := resolveCostParams(cmd)
	if err != nil {
		return err
	}

	costs, err := awsinternal.GetCostByAccount(context.Background(), p.cfg.Profile, p.cfg.Region, p.startDate, p.endDate, p.granularity, p.metric)
	if err != nil {
		return fmt.Errorf("get costs by account: %w", err)
	}

	return printCostRows(p.cfg, costs, "Account", func(c awsinternal.CostDetail) string { return c.GroupKey })
}

// showCostByUsageType retrieves and displays cost data aggregated by usage type.
func showCostByUsageType(cmd *cobra.Command, args []string) error {
	p, err := resolveCostParams(cmd)
	if err != nil {
		return err
	}

	costs, err := awsinternal.GetCostByUsageType(context.Background(), p.cfg.Profile, p.cfg.Region, p.startDate, p.endDate, p.granularity, p.metric)
	if err != nil {
		return fmt.Errorf("get costs by usage type: %w", err)
	}

	return printCostRows(p.cfg, costs, "UsageType", func(c awsinternal.CostDetail) string { return c.GroupKey })
}

// showCostOverview retrieves and displays an overview of costs.
func showCostOverview(cmd *cobra.Command, args []string) error {
	p, err := resolveCostParams(cmd)
	if err != nil {
		return err
	}

	costs, err := awsinternal.GetCostForPeriod(context.Background(), p.cfg.Profile, p.cfg.Region, p.startDate, p.endDate, p.granularity, p.metric)
	if err != nil {
		return fmt.Errorf("get cost for period: %w", err)
	}

	return printCostRows(p.cfg, costs, "Overview", func(c awsinternal.CostDetail) string { return "Total" })
}
