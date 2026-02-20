package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/tidb"
	"github.com/sfuruya0612/thief/internal/util"
)

const (
	HOST        = "https://api.tidbcloud.com"
	BillingHost = "https://billing.tidbapi.com"
)

var tidbCmd = &cobra.Command{
	Use:   "tidb",
	Short: "TiDB",
}

var tidbProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "List TiDB projects",
	Long:  "Retrieves and displays TiDB Cloud projects.",
	RunE:  listTidbProjects,
}

var tidbClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "List TiDB clusters",
	Long:  "Retrieves and displays TiDB Cloud clusters in all projects.",
	RunE:  listTidbClusters,
}

var tidbCostCmd = &cobra.Command{
	Use:   "cost",
	Short: "Show TiDB costs",
	Long:  "Retrieves and displays cost information for TiDB Cloud resources.",
	RunE:  showTidbCost,
}

type Cluster struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Region        string    `json:"region"`
	CreatedAt     time.Time `json:"created_at"`
	ClusterType   string    `json:"cluster_type"`
	CloudProvider string    `json:"cloud_provider"`
}

type ClusterResponse struct {
	Items      []Cluster `json:"items"`
	TotalCount int       `json:"total_count"`
}

type TidbProject struct {
	Items []struct {
		ID              string `json:"id"`
		OrgID           string `json:"orgId"`
		Name            string `json:"name"`
		ClusterCount    int    `json:"clusterCount"`
		UserCount       int    `json:"userCount"`
		CreateTimestamp string `json:"createTimestamp"`
	} `json:"items"`
}

var tidbProjectColumns = []util.Column{
	{Header: "Id", Width: 19},
	{Header: "OrgId", Width: 19},
	{Header: "ProjectName", Width: 25},
	{Header: "ClusterCount", Width: 12},
	{Header: "UserCount", Width: 9},
	{Header: "CreatedAt", Width: 25},
}

type TidbCost struct {
	Details []struct {
		BilledDate      string `json:"billedDate"`
		ClusterName     string `json:"clusterName"`
		Credits         string `json:"credits"`
		Discounts       string `json:"discounts"`
		ProjectName     string `json:"projectName"`
		RunningTotal    string `json:"runningTotal"`
		ServicePathName string `json:"servicePathName"`
		TotalCost       string `json:"totalCost"`
	} `json:"details"`
}

var tidbCostColumns = []util.Column{
	{Header: "BilledDate", Width: 10},
	{Header: "ProjectName", Width: 25},
	{Header: "ClusterName", Width: 19},
	{Header: "ServicePathName", Width: 50},
	{Header: "Credits", Width: 9},
	{Header: "Discounts", Width: 9},
	{Header: "RunningTotal", Width: 9},
	{Header: "TotalCost", Width: 9},
}

func listTidbProjects(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())

	if cfg.TiDB.PublicKey == "" || cfg.TiDB.PrivateKey == "" {
		return fmt.Errorf("TiDB public key and private key are required. Set them via flags (--public-key, --private-key), environment variables (TIDB_PUBLIC_KEY, TIDB_PRIVATE_KEY), or config file")
	}

	d := tidb.NewDigestClient(cfg.TiDB.PublicKey, cfg.TiDB.PrivateKey)

	// TODO: Pagination
	endpoint := "/api/v1beta/projects?page=1&page_size=100"

	resp, err := d.Get(HOST, endpoint)
	if err != nil {
		return fmt.Errorf("failed to get response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: code=%d, message=%s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var project TidbProject
	if err := json.Unmarshal(body, &project); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	var items [][]string
	for _, i := range project.Items {
		timestamp, err := strconv.ParseInt(i.CreateTimestamp, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse timestamp: %w", err)
		}

		items = append(items, []string{
			i.ID,
			i.OrgID,
			i.Name,
			fmt.Sprintf("%d", i.ClusterCount),
			fmt.Sprintf("%d", i.UserCount),
			time.Unix(timestamp, 0).Format(time.RFC3339),
		})
	}

	return printRowsOrGroupBy(cfg, tidbProjectColumns, items)
}

func listTidbClusters(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())

	if cfg.TiDB.PublicKey == "" || cfg.TiDB.PrivateKey == "" {
		return fmt.Errorf("TiDB public key and private key are required. Set them via flags (--public-key, --private-key), environment variables (TIDB_PUBLIC_KEY, TIDB_PRIVATE_KEY), or config file")
	}

	d := tidb.NewDigestClient(cfg.TiDB.PublicKey, cfg.TiDB.PrivateKey)

	// First get all projects
	endpoint := "/api/v1beta/projects?page=1&page_size=100"

	resp, err := d.Get(HOST, endpoint)
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error when fetching projects: code=%d, message=%s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read projects response: %w", err)
	}

	var project TidbProject
	if err := json.Unmarshal(body, &project); err != nil {
		return fmt.Errorf("failed to parse projects JSON: %w", err)
	}

	// Now fetch clusters for each project
	var allClusters []Cluster

	for _, p := range project.Items {
		clusterEndpoint := fmt.Sprintf("/api/v1beta/projects/%s/clusters?page=1&page_size=100", p.ID)
		clusterResp, err := d.Get(HOST, clusterEndpoint)
		if err != nil {
			return fmt.Errorf("failed to get clusters for project %s: %w", p.ID, err)
		}
		defer func() { _ = clusterResp.Body.Close() }()

		if clusterResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(clusterResp.Body)
			return fmt.Errorf("API error when fetching clusters: code=%d, message=%s", clusterResp.StatusCode, string(body))
		}

		body, err := io.ReadAll(clusterResp.Body)
		if err != nil {
			return fmt.Errorf("failed to read clusters response: %w", err)
		}

		var resp ClusterResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("failed to parse clusters JSON: %w", err)
		}

		allClusters = append(allClusters, resp.Items...)
	}

	// Define columns for clusters
	tidbClusterColumns := []util.Column{
		{Header: "Id", Width: 19},
		{Header: "Name", Width: 25},
		{Header: "Status", Width: 12},
		{Header: "Region", Width: 15},
		{Header: "CloudProvider", Width: 15},
		{Header: "ClusterType", Width: 15},
		{Header: "CreatedAt", Width: 25},
	}

	// Format data for output
	var items [][]string
	for _, cluster := range allClusters {
		items = append(items, []string{
			cluster.ID,
			cluster.Name,
			cluster.Status,
			cluster.Region,
			cluster.CloudProvider,
			cluster.ClusterType,
			cluster.CreatedAt.Format(time.RFC3339),
		})
	}

	return printRowsOrGroupBy(cfg, tidbClusterColumns, items)
}

func showTidbCost(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())

	if cfg.TiDB.PublicKey == "" || cfg.TiDB.PrivateKey == "" {
		return fmt.Errorf("TiDB public key and private key are required. Set them via flags (--public-key, --private-key), environment variables (TIDB_PUBLIC_KEY, TIDB_PRIVATE_KEY), or config file")
	}

	if cfg.TiDB.BilledMonth == "" {
		return fmt.Errorf("billed-month is required")
	}

	d := tidb.NewDigestClient(cfg.TiDB.PublicKey, cfg.TiDB.PrivateKey)

	endpoint := fmt.Sprintf("/v1beta1/billsDetails/%s", cfg.TiDB.BilledMonth)

	resp, err := d.Get(BillingHost, endpoint)
	if err != nil {
		return fmt.Errorf("failed to get response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: code=%d, message=%s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var cost TidbCost
	if err := json.Unmarshal(body, &cost); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	var items [][]string
	for _, i := range cost.Details {
		items = append(items, []string{
			i.BilledDate,
			i.ProjectName,
			i.ClusterName,
			i.ServicePathName,
			i.Credits,
			i.Discounts,
			i.RunningTotal,
			i.TotalCost,
		})
	}

	return printRowsOrGroupBy(cfg, tidbCostColumns, items)
}
