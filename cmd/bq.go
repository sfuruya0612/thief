package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	bq "github.com/sfuruya0612/thief/internal/bigquery"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

func init() {
	rootCmd.AddCommand(bqCmd)

	bqCmd.AddCommand(bqDatasetCmd, bqTableCmd, bqQueryCmd)
	bqDatasetCmd.AddCommand(bqDatasetListCmd)
	bqTableCmd.AddCommand(bqTableListCmd, bqTableInfoCmd)

	// --project is a persistent flag available to all bq subcommands.
	bqCmd.PersistentFlags().StringP("project", "", "", "Google Cloud project ID (or set GOOGLE_CLOUD_PROJECT)")

	// --dataset is required for table ls.
	bqTableListCmd.Flags().StringP("dataset", "", "", "Dataset ID (required)")
	_ = bqTableListCmd.MarkFlagRequired("dataset")
}

var bqCmd = &cobra.Command{
	Use:   "bq",
	Short: "Manage BigQuery",
	Long:  "Provides commands to interact with Google BigQuery datasets, tables, and queries.",
}

var bqDatasetCmd = &cobra.Command{
	Use:   "dataset",
	Short: "Manage BigQuery datasets",
}

var bqTableCmd = &cobra.Command{
	Use:   "table",
	Short: "Manage BigQuery tables",
}

var bqDatasetListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List BigQuery datasets",
	Long:    "Retrieves and displays a list of BigQuery datasets in the specified project.",
	RunE:    listBqDatasets,
}

var bqTableListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List BigQuery tables",
	Long:    "Retrieves and displays a list of BigQuery tables in the specified dataset.",
	RunE:    listBqTables,
}

var bqTableInfoCmd = &cobra.Command{
	Use:   "info <dataset.table>",
	Short: "Show BigQuery table schema",
	Long:  "Retrieves and displays schema information for a BigQuery table.",
	Args:  cobra.ExactArgs(1),
	RunE:  showBqTableInfo,
}

var bqQueryCmd = &cobra.Command{
	Use:   "query <sql>",
	Short: "Execute a BigQuery SQL query",
	Long:  "Executes a SQL query on BigQuery and displays the results.",
	Args:  cobra.ExactArgs(1),
	RunE:  executeBqQuery,
}

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

func listBqDatasets(cmd *cobra.Command, args []string) error {
	return runList(cmd, ListConfig[bq.DatasetInfo]{
		Columns:  bqDatasetColumns,
		EmptyMsg: "No datasets found",
		Fetch: func(cfg *config.Config) ([]bq.DatasetInfo, error) {
			client, err := newBQClient(context.Background(), cfg)
			if err != nil {
				return nil, err
			}
			defer func() { _ = client.Close() }()
			return bq.ListDatasets(context.Background(), client)
		},
	})
}

func listBqTables(cmd *cobra.Command, args []string) error {
	datasetID, _ := cmd.Flags().GetString("dataset")
	return runList(cmd, ListConfig[bq.TableInfo]{
		Columns:  bqTableColumns,
		EmptyMsg: fmt.Sprintf("No tables found in dataset %s", datasetID),
		Fetch: func(cfg *config.Config) ([]bq.TableInfo, error) {
			client, err := newBQClient(context.Background(), cfg)
			if err != nil {
				return nil, err
			}
			defer func() { _ = client.Close() }()
			return bq.ListTables(context.Background(), client, datasetID)
		},
	})
}

func showBqTableInfo(cmd *cobra.Command, args []string) error {
	parts := strings.SplitN(args[0], ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("argument must be in format 'dataset.table', got %q", args[0])
	}
	datasetID, tableID := parts[0], parts[1]

	return runList(cmd, ListConfig[bq.FieldInfo]{
		Columns:  bqFieldColumns,
		EmptyMsg: "No schema fields found",
		Fetch: func(cfg *config.Config) ([]bq.FieldInfo, error) {
			client, err := newBQClient(context.Background(), cfg)
			if err != nil {
				return nil, err
			}
			defer func() { _ = client.Close() }()
			return bq.GetTableSchema(context.Background(), client, datasetID, tableID)
		},
	})
}

func executeBqQuery(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())

	client, err := newBQClient(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	colNames, rows, err := bq.ExecuteQuery(context.Background(), client, args[0])
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	if len(rows) == 0 {
		cmd.Println("Query returned no results")
		return nil
	}

	columns := make([]util.Column, len(colNames))
	for i, name := range colNames {
		columns[i] = util.Column{Header: name}
	}

	return printRowsOrGroupBy(cfg, columns, rows)
}

// newBQClient validates the project ID and creates a BigQuery client from config.
func newBQClient(ctx context.Context, cfg *config.Config) (bq.BigqueryAPI, error) {
	if cfg.BigQuery.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required. Set via --project flag or GOOGLE_CLOUD_PROJECT environment variable")
	}
	return bq.NewBigQueryClient(ctx, cfg.BigQuery.ProjectID)
}
