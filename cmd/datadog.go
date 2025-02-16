package cmd

import (
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/internal/datadog"
	"github.com/sfuruya0612/thief/internal/util"
	"github.com/spf13/cobra"
)

var datadogCmd = &cobra.Command{
	Use:   "datadog",
	Short: "Show Datadog Usage",
}

var datadogHistoricalCostCmd = &cobra.Command{
	Use:   "historical",
	Short: "Show Datadog Historical Cost",
	Run:   showDatadogHistoricalCost,
}

var datadogEstimatedCostCmd = &cobra.Command{
	Use:   "estimated",
	Short: "Show Datadog Estimated Cost",
	Run:   showDatadogEstimatedCost,
}

var datadogCostColumns = []util.Column{
	{Header: "Month", Width: 8},
	{Header: "AccountName", Width: 8},
	{Header: "OrgName", Width: 25},
	{Header: "ProductName", Width: 30},
	{Header: "ChangeType", Width: 10},
	{Header: "Cost", Width: 10},
}

func showDatadogHistoricalCost(cmd *cobra.Command, args []string) {
	site := cmd.Flag("site").Value.String()
	apiKey := cmd.Flag("api-key").Value.String()
	appKey := cmd.Flag("app-key").Value.String()

	if site == "" {
		site = "datadoghq.com"
	}

	if apiKey == "" || appKey == "" {
		fmt.Println("--api-key and --app-key are required")
		return
	}

	view := cmd.Flag("view").Value.String()
	if !isValidView(view) {
		fmt.Println("--view must be 'summary' or 'sub-org'")
		return
	}

	start, end, err := parseDate(cmd.Flag("start-month").Value.String(), cmd.Flag("end-month").Value.String())
	if err != nil {
		fmt.Printf("Error parsing date: %v\n", err)
		return
	}

	ctx := datadog.GenerateDatadogContext(apiKey, appKey)
	api := datadog.NewDatadogUsageMeteringApi(datadog.NewDatadogClient(site))
	params := datadog.GenerateGetHistoricalCostByOrgOptionalParameters(view, end)

	resp, err := datadog.GetHistoricalCostByOrg(ctx, api, start, *params)
	if err != nil {
		fmt.Printf("Error getting historical cost by org: %v\n", err)
		return
	}

	formatter := util.NewTableFormatter(datadogCostColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(resp)
}

func showDatadogEstimatedCost(cmd *cobra.Command, args []string) {
	site := cmd.Flag("site").Value.String()
	apiKey := cmd.Flag("api-key").Value.String()
	appKey := cmd.Flag("app-key").Value.String()

	if site == "" {
		site = "datadoghq.com"
	}

	if apiKey == "" || appKey == "" {
		fmt.Println("--api-key and --app-key are required")
		return
	}

	view := cmd.Flag("view").Value.String()
	if !isValidView(view) {
		fmt.Println("--view must be 'summary' or 'sub-org'")
		return
	}

	start, end, err := parseDate(cmd.Flag("start-month").Value.String(), cmd.Flag("end-month").Value.String())
	if err != nil {
		fmt.Printf("Error parsing date: %v\n", err)
		return
	}

	ctx := datadog.GenerateDatadogContext(apiKey, appKey)
	api := datadog.NewDatadogUsageMeteringApi(datadog.NewDatadogClient(site))
	params := datadog.GenerateGetEstimatedCostByOrgOptionalParameters(view, start, end)

	resp, err := datadog.GetEstimatedCostByOrg(ctx, api, *params)
	if err != nil {
		fmt.Printf("Error getting estimated cost by org: %v\n", err)
		return
	}

	formatter := util.NewTableFormatter(datadogCostColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(resp)
}

func isValidView(view string) bool {
	return view == "summary" || view == "sub-org"
}

func parseDate(startMonth, endMonth string) (time.Time, time.Time, error) {
	if startMonth == "" {
		fmt.Println("--start-month is required")
	}

	start, err := time.Parse("2006-01", startMonth)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("Unable to parse start month: %v", err)
	}

	var end time.Time
	if endMonth != "" {
		end, err = time.Parse("2006-01", endMonth)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("Unable to parse end month: %v", err)
		}
		end = time.Date(end.Year(), end.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	}

	return start, end, nil
}
