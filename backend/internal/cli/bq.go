package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/sfuruya0612/thief/backend/internal/bigquery"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newBQCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bq",
		Short: "BigQuery operations",
	}
	cmd.PersistentFlags().String("project", "", "GCP project ID (overrides GOOGLE_CLOUD_PROJECT)")

	datasetCmd := &cobra.Command{
		Use:   "dataset",
		Short: "Dataset operations",
	}
	datasetCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List datasets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return bqRunDatasets(cmd)
		},
	})

	tableCmd := &cobra.Command{
		Use:   "table <dataset>",
		Short: "Table operations",
	}
	tableCmd.AddCommand(&cobra.Command{
		Use:   "ls <dataset>",
		Short: "List tables",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return bqRunTables(cmd, args[0])
		},
	})

	queryCmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "Execute a read-only SQL query",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return bqRunQuery(cmd, args[0])
		},
	}

	cmd.AddCommand(datasetCmd, tableCmd, queryCmd)
	return cmd
}

func bqProjectID(cmd *cobra.Command) string {
	if f := cmd.Flag("project"); f != nil && f.Changed {
		return f.Value.String()
	}
	return os.Getenv("GOOGLE_CLOUD_PROJECT")
}

func bqRunDatasets(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID := bqProjectID(cmd)
	if projectID == "" {
		projectID = cfg.BigQuery.ProjectID
	}
	if projectID == "" {
		return fmt.Errorf("project ID required: use --project or GOOGLE_CLOUD_PROJECT")
	}
	client, err := bigquery.NewClient(context.Background(), projectID)
	if err != nil {
		return err
	}
	defer client.Close()

	datasets, err := client.ListDatasets(context.Background())
	if err != nil {
		return err
	}
	rows := make([][]string, len(datasets))
	for i, d := range datasets {
		rows[i] = []string{d.DatasetID, d.Location, d.CreationTime, d.LastModifiedTime, d.Description}
	}
	cols := []util.Column{{Header: "DatasetID"}, {Header: "Location"}, {Header: "Created"}, {Header: "Modified"}, {Header: "Description"}}
	f := util.NewTableFormatter(cols, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(rows)
	return nil
}

func bqRunTables(cmd *cobra.Command, datasetID string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID := bqProjectID(cmd)
	if projectID == "" {
		projectID = cfg.BigQuery.ProjectID
	}
	if projectID == "" {
		return fmt.Errorf("project ID required: use --project or GOOGLE_CLOUD_PROJECT")
	}
	client, err := bigquery.NewClient(context.Background(), projectID)
	if err != nil {
		return err
	}
	defer client.Close()

	tables, err := client.ListTables(context.Background(), datasetID)
	if err != nil {
		return err
	}
	rows := make([][]string, len(tables))
	for i, t := range tables {
		rows[i] = []string{t.TableID, t.Type, t.CreationTime, t.LastModifiedTime, fmt.Sprintf("%d", t.NumRows), fmt.Sprintf("%d", t.NumBytes)}
	}
	cols := []util.Column{{Header: "TableID"}, {Header: "Type"}, {Header: "Created"}, {Header: "Modified"}, {Header: "NumRows"}, {Header: "NumBytes"}}
	f := util.NewTableFormatter(cols, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(rows)
	return nil
}

func bqRunQuery(cmd *cobra.Command, sql string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID := bqProjectID(cmd)
	if projectID == "" {
		projectID = cfg.BigQuery.ProjectID
	}
	if projectID == "" {
		return fmt.Errorf("project ID required: use --project or GOOGLE_CLOUD_PROJECT")
	}
	client, err := bigquery.NewClient(context.Background(), projectID)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ExecuteQuery(context.Background(), sql)
	if err != nil {
		return err
	}
	cols := make([]util.Column, len(result.Columns))
	for i, c := range result.Columns {
		cols[i] = util.Column{Header: c}
	}
	f := util.NewTableFormatter(cols, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(result.Rows)
	return nil
}
