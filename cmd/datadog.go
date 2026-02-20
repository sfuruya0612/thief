// Package cmd implements the command line interface for thief.
package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/datadog"
	"github.com/sfuruya0612/thief/internal/util"
)

const (
	// monthYearFormat is the format for year and month.
	monthYearFormat = "2006-01"
)

// datadogCmd represents the base command for Datadog operations.
var datadogCmd = &cobra.Command{
	Use:   "datadog",
	Short: "Show Datadog Usage",
	Long:  `Provides commands to interact with Datadog API to retrieve usage and cost information.`,
}

// datadogHistoricalCostCmd represents the command to show Datadog historical cost.
var datadogHistoricalCostCmd = &cobra.Command{
	Use:   "historical",
	Short: "Show Datadog Historical Cost",
	Long:  `Retrieves and displays historical cost data from Datadog for a specified period.`,
	RunE:  showDatadogHistoricalCost,
}

// datadogEstimatedCostCmd represents the command to show Datadog estimated cost.
var datadogEstimatedCostCmd = &cobra.Command{
	Use:   "estimated",
	Short: "Show Datadog Estimated Cost",
	Long:  `Retrieves and displays estimated cost data from Datadog for a specified period.`,
	RunE:  showDatadogEstimatedCost,
}

var datadogCostColumns = []util.Column{
	{Header: "Month", Width: 8},
	{Header: "AccountName", Width: 8},
	{Header: "OrgName", Width: 25},
	{Header: "ProductName", Width: 30},
	{Header: "ChangeType", Width: 10},
	{Header: "Cost", Width: 10},
}

// showDatadogHistoricalCost retrieves and displays historical cost data from Datadog.
func showDatadogHistoricalCost(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())

	if cfg.Datadog.APIKey == "" || cfg.Datadog.AppKey == "" {
		return errors.New("datadog API key and APP key are required. Set them via flags (--api-key, --app-key), environment variables (DATADOG_API_KEY, DATADOG_APP_KEY), or config file")
	}

	if !isValidView(cfg.Datadog.View) {
		return errors.New("view must be 'summary' or 'sub-org'")
	}

	start, end, err := parseDate(cfg.Datadog.StartMonth, cfg.Datadog.EndMonth)
	if err != nil {
		return fmt.Errorf("parse date: %w", err)
	}

	ctx := datadog.GenerateDatadogContext(cfg.Datadog.APIKey, cfg.Datadog.AppKey)
	api := datadog.NewDatadogUsageMeteringApi(datadog.NewDatadogClient(cfg.Datadog.Site))
	params := datadog.GenerateGetHistoricalCostByOrgOptionalParameters(cfg.Datadog.View, end)

	resp, err := datadog.GetHistoricalCostByOrg(ctx, api, start, *params)
	if err != nil {
		return fmt.Errorf("get historical cost by org: %w", err)
	}

	return printRowsOrGroupBy(cfg, datadogCostColumns, toRows(resp))
}

// showDatadogEstimatedCost retrieves and displays estimated cost data from Datadog.
func showDatadogEstimatedCost(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())

	if cfg.Datadog.APIKey == "" || cfg.Datadog.AppKey == "" {
		return errors.New("datadog API key and APP key are required. Set them via flags (--api-key, --app-key), environment variables (DATADOG_API_KEY, DATADOG_APP_KEY), or config file")
	}

	if !isValidView(cfg.Datadog.View) {
		return errors.New("view must be 'summary' or 'sub-org'")
	}

	start, end, err := parseDate(cfg.Datadog.StartMonth, cfg.Datadog.EndMonth)
	if err != nil {
		return fmt.Errorf("parse date: %w", err)
	}

	ctx := datadog.GenerateDatadogContext(cfg.Datadog.APIKey, cfg.Datadog.AppKey)
	api := datadog.NewDatadogUsageMeteringApi(datadog.NewDatadogClient(cfg.Datadog.Site))
	params := datadog.GenerateGetEstimatedCostByOrgOptionalParameters(cfg.Datadog.View, start, end)

	resp, err := datadog.GetEstimatedCostByOrg(ctx, api, *params)
	if err != nil {
		return fmt.Errorf("get estimated cost by org: %w", err)
	}

	return printRowsOrGroupBy(cfg, datadogCostColumns, toRows(resp))
}

// isValidView checks if the provided view string is valid.
func isValidView(view string) bool {
	return view == "summary" || view == "sub-org"
}

// parseDate parses the start and end month strings into time.Time objects.
func parseDate(startMonth, endMonth string) (time.Time, time.Time, error) {
	if startMonth == "" {
		return time.Time{}, time.Time{}, errors.New("--start-month is required")
	}

	start, err := time.Parse(monthYearFormat, startMonth)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("unable to parse start month: %w", err)
	}

	var end time.Time
	if endMonth != "" {
		parsedEnd, err := time.Parse(monthYearFormat, endMonth)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("unable to parse end month: %w", err)
		}
		// Set to the first day of the next month to include the entire end month.
		end = time.Date(parsedEnd.Year(), parsedEnd.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	}

	return start, end, nil
}
