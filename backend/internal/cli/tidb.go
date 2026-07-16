package cli

import (
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/tidb"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var tidbProjectColumns = []util.Column{
	{Header: "Id"},
	{Header: "OrgId"},
	{Header: "ProjectName"},
	{Header: "ClusterCount"},
	{Header: "UserCount"},
	{Header: "CreatedAt"},
}

var tidbClusterColumns = []util.Column{
	{Header: "Id"},
	{Header: "Name"},
	{Header: "Status"},
	{Header: "Region"},
	{Header: "CloudProvider"},
	{Header: "ClusterType"},
	{Header: "CreatedAt"},
}

var tidbCostColumns = []util.Column{
	{Header: "BilledDate"},
	{Header: "ProjectName"},
	{Header: "ClusterName"},
	{Header: "ServicePathName"},
	{Header: "Credits"},
	{Header: "Discounts"},
	{Header: "RunningTotal"},
	{Header: "TotalCost"},
}

func newTiDBCmd() *cobra.Command {
	tidbCmd := &cobra.Command{
		Use:   "tidb",
		Short: "TiDB",
	}

	tidbCmd.PersistentFlags().StringP("public-key", "", "", "Public Key")
	tidbCmd.PersistentFlags().StringP("private-key", "", "", "Private Key")

	projectCmd := &cobra.Command{
		Use:   "project",
		Short: "List TiDB projects",
		Long:  "Retrieves and displays TiDB Cloud projects.",
		RunE:  listTidbProjects,
	}

	clusterCmd := &cobra.Command{
		Use:   "cluster [project-id]",
		Short: "List TiDB clusters",
		Long:  "Retrieves and displays TiDB Cloud clusters in all projects. If a project ID is given, only that project's clusters are listed.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  listTidbClusters,
	}

	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Show TiDB costs",
		Long:  "Retrieves and displays cost information for TiDB Cloud resources.",
		RunE:  showTidbCost,
	}
	costCmd.Flags().StringP("billed-month", "", "", "The month of this bill happens for the specified organization. The format is YYYY-MM, for example '2024-05'")

	tidbCmd.AddCommand(projectCmd, clusterCmd, costCmd)
	return tidbCmd
}

// newTiDBClient は設定を検証して TiDB Cloud API クライアントを生成する。
func newTiDBClient(cfg *config.Config) (*tidb.Client, error) {
	if cfg.TiDB.PublicKey == "" || cfg.TiDBPrivateKey() == "" {
		return nil, fmt.Errorf("TiDB public key and private key are required. Set them via flags (--public-key, --private-key), environment variables (TIDB_PUBLIC_KEY, TIDB_PRIVATE_KEY), or config file")
	}
	return tidb.NewClient(cfg.TiDB.PublicKey, cfg.TiDBPrivateKey()), nil
}

// tidbTimeString は時刻を RFC3339 で整形する。ゼロ値は空文字を返す。
func tidbTimeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func listTidbProjects(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	client, err := newTiDBClient(cfg)
	if err != nil {
		return err
	}

	projects, err := client.ListProjects()
	if err != nil {
		return err
	}

	var items [][]string
	for _, p := range projects {
		items = append(items, []string{
			p.ID,
			p.OrgID,
			p.Name,
			fmt.Sprintf("%d", p.ClusterCount),
			fmt.Sprintf("%d", p.UserCount),
			tidbTimeString(p.CreatedAt),
		})
	}

	return printRowsOrGroupBy(cfg, tidbProjectColumns, items)
}

func listTidbClusters(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	client, err := newTiDBClient(cfg)
	if err != nil {
		return err
	}

	// プロジェクト ID が指定されていればそのプロジェクトのみ、
	// 未指定なら全プロジェクトのクラスタを取得する。
	var projectIDs []string
	if len(args) == 1 {
		projectIDs = []string{args[0]}
	} else {
		projects, err := client.ListProjects()
		if err != nil {
			return fmt.Errorf("failed to get projects: %w", err)
		}
		for _, p := range projects {
			projectIDs = append(projectIDs, p.ID)
		}
	}

	var items [][]string
	for _, projectID := range projectIDs {
		clusters, err := client.ListClusters(projectID)
		if err != nil {
			return fmt.Errorf("failed to get clusters for project %s: %w", projectID, err)
		}
		for _, c := range clusters {
			items = append(items, []string{
				c.ID,
				c.Name,
				c.Status,
				c.Region,
				c.CloudProvider,
				c.ClusterType,
				tidbTimeString(c.CreatedAt),
			})
		}
	}

	return printRowsOrGroupBy(cfg, tidbClusterColumns, items)
}

func showTidbCost(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	client, err := newTiDBClient(cfg)
	if err != nil {
		return err
	}

	if cfg.TiDB.BilledMonth == "" {
		return fmt.Errorf("billed-month is required")
	}

	costs, err := client.GetCost(cfg.TiDB.BilledMonth)
	if err != nil {
		return err
	}

	var items [][]string
	for _, c := range costs {
		items = append(items, []string{
			c.BilledDate,
			c.ProjectName,
			c.ClusterName,
			c.ServicePathName,
			c.CreditsRaw,
			c.DiscountsRaw,
			c.RunningTotalRaw,
			c.TotalCostRaw,
		})
	}

	return printRowsOrGroupBy(cfg, tidbCostColumns, items)
}
