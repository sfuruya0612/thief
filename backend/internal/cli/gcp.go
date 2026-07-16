package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/sfuruya0612/thief/backend/internal/config"
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
		Short: "List projects (from local cache; run 'refresh' first if empty)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gcpRunProjects(cmd)
		},
	})
	projectsCmd.AddCommand(&cobra.Command{
		Use:   "refresh",
		Short: "Refresh the local project cache from Cloud Resource Manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gcpRunProjectsRefresh(cmd)
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

	// iam サブコマンド
	iamCmd := &cobra.Command{
		Use:   "iam",
		Short: "IAM operations",
	}
	iamCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List IAM policy bindings (flattened per member)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gcpRunIAMBindings(cmd)
		},
	})

	// serviceaccounts サブコマンド
	serviceAccountsCmd := &cobra.Command{
		Use:   "serviceaccounts",
		Short: "Service Account operations",
	}
	serviceAccountsCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List service accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gcpRunServiceAccounts(cmd)
		},
	})

	cmd.AddCommand(projectsCmd, runCmd, gcsCmd, iamCmd, serviceAccountsCmd)
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
// cfg は呼び出し側でロード済みのものを受け取り、config ファイルの二重ロードを避ける。
func gcpRequireProjectID(cmd *cobra.Command, cfg *config.Config) (string, error) {
	projectID := gcpProjectID(cmd)
	if projectID == "" {
		projectID = cfg.BigQuery.ProjectID
	}
	if projectID == "" {
		return "", fmt.Errorf("project ID required: use --project or GOOGLE_CLOUD_PROJECT")
	}
	return projectID, nil
}

// gcpRunProjects はローカルキャッシュ (~/.config/thief/gcp-projects.json) からプロジェクト
// 一覧を表示する。プロジェクトの作成/削除は頻繁ではないため API を毎回呼ばない。
// キャッシュが無い場合は自動で 1 回だけ Cloud Resource Manager から取得しキャッシュを作る。
func gcpRunProjects(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	projects, _, ok, err := gcp.LoadProjectsFromDisk(dir)
	if err != nil {
		return err
	}
	if !ok {
		projects, err = gcp.RefreshProjectsOnDisk(context.Background(), dir)
		if err != nil {
			return err
		}
	}
	return printGCPProjects(cfg, projects)
}

// gcpRunProjectsRefresh は Cloud Resource Manager から最新のプロジェクト一覧を取得し、
// ローカルキャッシュを上書きする (手動更新)。
func gcpRunProjectsRefresh(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	projects, err := gcp.RefreshProjectsOnDisk(context.Background(), dir)
	if err != nil {
		return err
	}
	return printGCPProjects(cfg, projects)
}

func printGCPProjects(cfg *config.Config, projects []gcp.ProjectInfo) error {
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
	return printRowsOrGroupBy(cfg, cols, rows)
}

func gcpRunCloudRun(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID, err := gcpRequireProjectID(cmd, cfg)
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
	return printRowsOrGroupBy(cfg, cols, rows)
}

func gcpRunBuckets(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID, err := gcpRequireProjectID(cmd, cfg)
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
	return printRowsOrGroupBy(cfg, cols, rows)
}

func gcpRunIAMBindings(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID, err := gcpRequireProjectID(cmd, cfg)
	if err != nil {
		return err
	}
	bindings, err := gcp.ListIAMBindings(context.Background(), projectID)
	if err != nil {
		return err
	}
	rows := make([][]string, len(bindings))
	for i, b := range bindings {
		rows[i] = []string{b.Member, b.Role, b.ProjectID, b.ConditionTitle}
	}
	cols := []util.Column{
		{Header: "Member"},
		{Header: "Role"},
		{Header: "ProjectID"},
		{Header: "ConditionTitle"},
	}
	return printRowsOrGroupBy(cfg, cols, rows)
}

func gcpRunServiceAccounts(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID, err := gcpRequireProjectID(cmd, cfg)
	if err != nil {
		return err
	}
	accounts, err := gcp.ListServiceAccounts(context.Background(), projectID)
	if err != nil {
		return err
	}
	rows := make([][]string, len(accounts))
	for i, a := range accounts {
		rows[i] = []string{a.Email, a.DisplayName, a.Description, fmt.Sprintf("%t", a.Disabled)}
	}
	cols := []util.Column{
		{Header: "Email"},
		{Header: "DisplayName"},
		{Header: "Description"},
		{Header: "Disabled"},
	}
	return printRowsOrGroupBy(cfg, cols, rows)
}

func gcpRunObjects(cmd *cobra.Command, bucket, prefix string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	projectID, err := gcpRequireProjectID(cmd, cfg)
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
	return printRowsOrGroupBy(cfg, cols, rows)
}
