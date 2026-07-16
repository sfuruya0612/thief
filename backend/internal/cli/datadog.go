package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

// datadogMonthFormat は年月指定 (YYYY-MM) のフォーマット。
const datadogMonthFormat = "2006-01"

var datadogCostColumns = []util.Column{
	{Header: "Month"},
	{Header: "AccountName"},
	{Header: "OrgName"},
	{Header: "ProductName"},
	{Header: "ChangeType"},
	{Header: "Cost"},
}

func newDatadogCmd() *cobra.Command {
	datadogCmd := &cobra.Command{
		Use:   "datadog",
		Short: "Show Datadog Usage",
		Long:  `Provides commands to interact with Datadog API to retrieve usage and cost information.`,
	}

	datadogCmd.PersistentFlags().StringP("site", "", "datadoghq.com", "Datadog Site")
	datadogCmd.PersistentFlags().StringP("api-key", "", "", "Datadog API Key")
	datadogCmd.PersistentFlags().StringP("app-key", "", "", "Datadog APP Key")
	datadogCmd.PersistentFlags().StringP("view", "", "summary", "String to specify whether cost is broken down at a parent-org level or at the sub-org level. Available views are summary and sub-org")
	datadogCmd.PersistentFlags().StringP("start-month", "", "", "[YYYY-MM] for cost beginning this month")
	datadogCmd.PersistentFlags().StringP("end-month", "", "", "[YYYY-MM] for cost ending this month")

	historicalCmd := &cobra.Command{
		Use:   "historical",
		Short: "Show Datadog Historical Cost",
		Long:  `Retrieves and displays historical cost data from Datadog for a specified period.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showDatadogCost(cmd, datadog.GetHistoricalCost)
		},
	}

	estimatedCmd := &cobra.Command{
		Use:   "estimated",
		Short: "Show Datadog Estimated Cost",
		Long:  `Retrieves and displays estimated cost data from Datadog for a specified period.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showDatadogCost(cmd, datadog.GetEstimatedCost)
		},
	}

	datadogCmd.AddCommand(historicalCmd, estimatedCmd)
	return datadogCmd
}

// datadogCostFetcher は historical / estimated 共通のコスト取得関数のシグネチャ。
type datadogCostFetcher func(ctx context.Context, api *datadog.UsageMeteringV2API, startMonth, endMonth, view string) ([]datadog.CostInfo, error)

// showDatadogCost は Datadog のコストデータを取得し表として出力する。
func showDatadogCost(cmd *cobra.Command, fetch datadogCostFetcher) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	if cfg.DatadogAPIKey() == "" || cfg.DatadogAppKey() == "" {
		return errors.New("datadog API key and APP key are required. Set them via flags (--api-key, --app-key), environment variables (DATADOG_API_KEY, DATADOG_APP_KEY), or config file")
	}

	if !isValidDatadogView(cfg.Datadog.View) {
		return errors.New("view must be 'summary' or 'sub-org'")
	}

	startMonth, endMonth, err := parseDatadogMonths(cfg.Datadog.StartMonth, cfg.Datadog.EndMonth)
	if err != nil {
		return fmt.Errorf("parse date: %w", err)
	}

	ddCfg := datadog.NewConfiguration(cfg.Datadog.Site)
	api := datadog.NewUsageMeteringV2API(ddCfg)
	ctx := datadog.NewContext(context.Background(), cfg.DatadogAPIKey(), cfg.DatadogAppKey())

	items, err := fetch(ctx, api, startMonth, endMonth, cfg.Datadog.View)
	if err != nil {
		return err
	}

	rows := make([][]string, len(items))
	for i, item := range items {
		rows[i] = []string{
			item.Month,
			item.AccountName,
			item.OrgName,
			item.ProductName,
			item.ChargeType,
			strconv.FormatFloat(item.Cost, 'f', -1, 64),
		}
	}

	return printRowsOrGroupBy(cfg, datadogCostColumns, rows)
}

// isValidDatadogView checks if the provided view string is valid.
func isValidDatadogView(view string) bool {
	return view == "summary" || view == "sub-org"
}

// parseDatadogMonths は start/end month を検証し、API へ渡す文字列 (YYYY-MM) を返す。
// start は必須。end は指定時のみ、その月全体を含めるため翌月の値へ変換する。
func parseDatadogMonths(startMonth, endMonth string) (string, string, error) {
	if startMonth == "" {
		return "", "", errors.New("--start-month is required")
	}

	if _, err := time.Parse(datadogMonthFormat, startMonth); err != nil {
		return "", "", fmt.Errorf("unable to parse start month: %w", err)
	}

	end := ""
	if endMonth != "" {
		parsedEnd, err := time.Parse(datadogMonthFormat, endMonth)
		if err != nil {
			return "", "", fmt.Errorf("unable to parse end month: %w", err)
		}
		// end month 全体を含めるため翌月の 1 日に変換する。
		end = parsedEnd.AddDate(0, 1, 0).Format(datadogMonthFormat)
	}

	return startMonth, end, nil
}
