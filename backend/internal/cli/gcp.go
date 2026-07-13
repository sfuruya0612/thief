package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

// newGCPCmd は Google Cloud 操作のルートコマンドを返す。
func newGCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gcp",
		Short: "Google Cloud operations",
	}
	cmd.PersistentFlags().String("project", "", "GCP project ID (overrides GOOGLE_CLOUD_PROJECT)")

	// projects サブコマンド
	projectsCmd := &cobra.Command{
		Use:   "projects",
		Short: "Project operations",
	}
	projectsCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gcpRunProjects(cmd)
		},
	})

	// run サブコマンド (Cloud Run)
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Cloud Run operations",
	}
	runCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List Cloud Run services and jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gcpRunCloudRun(cmd)
		},
	})

	// gcs サブコマンド (Cloud Storage)
	gcsCmd := &cobra.Command{
		Use:   "gcs",
		Short: "Cloud Storage operations",
	}
	gcsCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List buckets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gcpRunBuckets(cmd)
		},
	})
	objectsCmd := &cobra.Command{
		Use:   "objects <bucket>",
		Short: "List objects in a bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, _ := cmd.Flags().GetString("prefix")
			return gcpRunObjects(cmd, args[0], prefix)
		},
	}
	objectsCmd.Flags().String("prefix", "", "Object name prefix filter")
	gcsCmd.AddCommand(objectsCmd)

	cmd.AddCommand(projectsCmd, runCmd, gcsCmd)
	return cmd
}

// gcpProjectID は --project フラグ、環境変数、設定ファイルの順に project ID を解決する。
func gcpProjectID(cmd *cobra.Command) string {
	if f := cmd.Flag("project"); f != nil && f.Changed {
		return f.Value.String()
	}
	return os.Getenv("GOOGLE_CLOUD_PROJECT")
}

// gcpRequireProjectID は project ID の解決と必須チェックを一括で行う。
func gcpRequireProjectID(cmd *cobra.Command) (string, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return "", err
	}
	projectID := gcpProjectID(cmd)
	if projectID == "" {
		projectID = cfg.BigQuery.ProjectID
	}
	if projectID == "" {
		return "", fmt.Errorf("project ID required: use --project or GOOGLE_CLOUD_PROJECT")
	}
	return projectID, nil
}

func gcpRunProjects(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projects, err := gcp.ListProjects(context.Background())
	if err != nil {
		return err
	}
	rows := make([][]string, len(projects))
	for i, p := range projects {
		rows[i] = []string{p.ProjectID, p.Name, fmt.Sprintf("%d", p.ProjectNumber), p.State, p.CreateTime}
	}
	cols := []util.Column{
		{Header: "ProjectID"},
		{Header: "Name"},
		{Header: "ProjectNumber"},
		{Header: "State"},
		{Header: "CreateTime"},
	}
	f := util.NewTableFormatter(cols, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(rows)
	return nil
}

func gcpRunCloudRun(cmd *cobra.Command) error {
	projectID, err := gcpRequireProjectID(cmd)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	items, err := gcp.ListCloudRun(context.Background(), projectID)
	if err != nil {
		return err
	}
	rows := make([][]string, len(items))
	for i, r := range items {
		rows[i] = []string{r.Name, r.Kind, r.Region, r.URI, r.CreateTime, r.UpdateTime}
	}
	cols := []util.Column{
		{Header: "Name"},
		{Header: "Kind"},
		{Header: "Region"},
		{Header: "URI"},
		{Header: "CreateTime"},
		{Header: "UpdateTime"},
	}
	f := util.NewTableFormatter(cols, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(rows)
	return nil
}

func gcpRunBuckets(cmd *cobra.Command) error {
	projectID, err := gcpRequireProjectID(cmd)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	buckets, err := gcp.ListBuckets(context.Background(), projectID)
	if err != nil {
		return err
	}
	rows := make([][]string, len(buckets))
	for i, b := range buckets {
		rows[i] = []string{b.Name, b.Location, b.StorageClass, b.CreateTime, b.UpdateTime}
	}
	cols := []util.Column{
		{Header: "Name"},
		{Header: "Location"},
		{Header: "StorageClass"},
		{Header: "CreateTime"},
		{Header: "UpdateTime"},
	}
	f := util.NewTableFormatter(cols, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(rows)
	return nil
}

func gcpRunObjects(cmd *cobra.Command, bucket, prefix string) error {
	projectID, err := gcpRequireProjectID(cmd)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	objects, err := gcp.ListObjects(context.Background(), projectID, bucket, prefix)
	if err != nil {
		return err
	}
	rows := make([][]string, len(objects))
	for i, o := range objects {
		rows[i] = []string{o.Name, o.Bucket, fmt.Sprintf("%d", o.Size), o.ContentType, o.StorageClass, o.Updated}
	}
	cols := []util.Column{
		{Header: "Name"},
		{Header: "Bucket"},
		{Header: "Size"},
		{Header: "ContentType"},
		{Header: "StorageClass"},
		{Header: "Updated"},
	}
	f := util.NewTableFormatter(cols, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(rows)
	return nil
}
