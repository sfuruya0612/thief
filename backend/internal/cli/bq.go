package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/bigquery"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var bqDatasetColumns = []util.Column{
	{Header: "DatasetID"},
	{Header: "Location"},
	{Header: "CreationTime"},
	{Header: "LastModifiedTime"},
	{Header: "Description"},
}

var bqTableColumns = []util.Column{
	{Header: "TableID"},
	{Header: "Type"},
	{Header: "CreationTime"},
	{Header: "LastModifiedTime"},
	{Header: "NumRows"},
	{Header: "NumBytes"},
}

var bqFieldColumns = []util.Column{
	{Header: "FieldName"},
	{Header: "FieldType"},
	{Header: "Mode"},
	{Header: "Description"},
}

func newBQCmd() *cobra.Command {
	bqCmd := &cobra.Command{
		Use:   "bq",
		Short: "Manage BigQuery",
		Long:  "Provides commands to interact with Google BigQuery datasets, tables, and queries.",
	}

	// --project は全 bq サブコマンドで利用可能な永続フラグ。
	bqCmd.PersistentFlags().StringP("project", "", "", "Google Cloud project ID (or set GOOGLE_CLOUD_PROJECT)")

	datasetCmd := &cobra.Command{
		Use:   "dataset",
		Short: "Manage BigQuery datasets",
	}

	datasetListCmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List BigQuery datasets",
		Long:    "Retrieves and displays a list of BigQuery datasets in the specified project.",
		RunE:    listBqDatasets,
	}
	datasetCmd.AddCommand(datasetListCmd)

	tableCmd := &cobra.Command{
		Use:   "table",
		Short: "Manage BigQuery tables",
	}

	tableListCmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List BigQuery tables",
		Long:    "Retrieves and displays a list of BigQuery tables in the specified dataset.",
		RunE:    listBqTables,
	}
	tableListCmd.Flags().StringP("dataset", "", "", "Dataset ID (required)")
	_ = tableListCmd.MarkFlagRequired("dataset")

	tableInfoCmd := &cobra.Command{
		Use:   "info <dataset.table>",
		Short: "Show BigQuery table schema",
		Long:  "Retrieves and displays schema information for a BigQuery table.",
		Args:  cobra.ExactArgs(1),
		RunE:  showBqTableInfo,
	}
	tableCmd.AddCommand(tableListCmd, tableInfoCmd)

	queryCmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "Execute a BigQuery SQL query",
		Long:  "Executes a SQL query on BigQuery and displays the results.",
		Args:  cobra.ExactArgs(1),
		RunE:  executeBqQuery,
	}

	bqCmd.AddCommand(datasetCmd, tableCmd, queryCmd)
	return bqCmd
}

// newBQClient validates the project ID and creates a BigQuery client from config.
func newBQClient(ctx context.Context, cfg *config.Config) (*bigquery.Client, error) {
	if cfg.BigQuery.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required. Set via --project flag or GOOGLE_CLOUD_PROJECT environment variable")
	}
	return bigquery.NewClient(ctx, cfg.BigQuery.ProjectID)
}

func listBqDatasets(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := newBQClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	datasets, err := client.ListDatasets(ctx)
	if err != nil {
		return err
	}

	if len(datasets) == 0 {
		cmd.Println("No datasets found")
		return nil
	}

	rows := make([][]string, len(datasets))
	for i, d := range datasets {
		rows[i] = []string{d.DatasetID, d.Location, d.CreationTime, d.LastModifiedTime, d.Description}
	}

	return printRowsOrGroupBy(cfg, bqDatasetColumns, rows)
}

func listBqTables(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	datasetID, _ := cmd.Flags().GetString("dataset")

	ctx := context.Background()
	client, err := newBQClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	tables, err := client.ListTables(ctx, datasetID)
	if err != nil {
		return err
	}

	if len(tables) == 0 {
		cmd.Printf("No tables found in dataset %s\n", datasetID)
		return nil
	}

	rows := make([][]string, len(tables))
	for i, t := range tables {
		rows[i] = []string{
			t.TableID,
			t.Type,
			t.CreationTime,
			t.LastModifiedTime,
			fmt.Sprintf("%d", t.NumRows),
			fmt.Sprintf("%d", t.NumBytes),
		}
	}

	return printRowsOrGroupBy(cfg, bqTableColumns, rows)
}

func showBqTableInfo(cmd *cobra.Command, args []string) error {
	datasetID, tableID, err := splitBqTableRef(args[0])
	if err != nil {
		return err
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := newBQClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	fields, err := client.GetTableSchema(ctx, datasetID, tableID)
	if err != nil {
		return err
	}

	if len(fields) == 0 {
		cmd.Println("No schema fields found")
		return nil
	}

	rows := make([][]string, len(fields))
	for i, f := range fields {
		rows[i] = []string{f.Name, f.Type, f.Mode, f.Description}
	}

	return printRowsOrGroupBy(cfg, bqFieldColumns, rows)
}

// splitBqTableRef は "dataset.table" 形式の引数を分割する。
func splitBqTableRef(ref string) (string, string, error) {
	parts := strings.SplitN(ref, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("argument must be in format 'dataset.table', got %q", ref)
	}
	return parts[0], parts[1], nil
}

func executeBqQuery(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := newBQClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	result, err := client.ExecuteQueryUnrestricted(ctx, args[0])
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	if len(result.Rows) == 0 {
		cmd.Println("Query returned no results")
		return nil
	}

	columns := make([]util.Column, len(result.Columns))
	for i, name := range result.Columns {
		columns[i] = util.Column{Header: name}
	}

	return printRowsOrGroupBy(cfg, columns, result.Rows)
}
