package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sfuruya0612/thief/internal/tidb"
	"github.com/sfuruya0612/thief/internal/util"
	"github.com/spf13/cobra"
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
	Use:   "cluster",
	Short: "TiDB cluster",
	Run:   listTidbProjects,
}

var tidbClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "TiDB cluster",
	Run:   listTidbProjects,
}

var tidbCostCmd = &cobra.Command{
	Use:   "cost",
	Short: "TiDB cost",
	Run:   showTidbCost,
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

func listTidbProjects(cmd *cobra.Command, args []string) {
	publicKey := cmd.Flag("public-key").Value.String()
	privateKey := cmd.Flag("private-key").Value.String()

	d := tidb.NewDigestClient(publicKey, privateKey)

	// TODO: Pagination
	endpoint := "/api/v1beta/projects?page=1&page_size=100"

	resp, err := d.Get(HOST, endpoint)
	if err != nil {
		fmt.Printf("Failed to get response: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("API error: code=%d, message=%s", resp.StatusCode, string(body))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response: %v", err)
		return
	}

	var project TidbProject
	if err := json.Unmarshal(body, &project); err != nil {
		fmt.Printf("Failed to parse JSON: %v", err)
		return
	}

	var items [][]string
	for _, i := range project.Items {
		timestamp, err := strconv.ParseInt(i.CreateTimestamp, 10, 64)
		if err != nil {
			fmt.Printf("Failed to parse timestamp: %v", err)
			return
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

	formatter := util.NewTableFormatter(tidbProjectColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(items)
}

func showTidbCost(cmd *cobra.Command, args []string) {
	publicKey := cmd.Flag("public-key").Value.String()
	privateKey := cmd.Flag("private-key").Value.String()

	bliiedMonth := cmd.Flag("billed-month").Value.String()
	if bliiedMonth == "" {
		fmt.Println("--billed-month is required")
		return
	}

	d := tidb.NewDigestClient(publicKey, privateKey)

	endpoint := fmt.Sprintf("/v1beta1/billsDetails/%s", bliiedMonth)

	resp, err := d.Get(BillingHost, endpoint)
	if err != nil {
		fmt.Printf("Failed to get response: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("API error: code=%d, message=%s", resp.StatusCode, string(body))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response: %v", err)
		return
	}

	var cost TidbCost
	if err := json.Unmarshal(body, &cost); err != nil {
		fmt.Printf("Failed to parse JSON: %v", err)
		return
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

	formatter := util.NewTableFormatter(tidbCostColumns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(items)
}

// func getTiDBCluster(cmd *cobra.Command, args []string) {
// 	apiKey := cmd.Flag("api-key").Value.String()
// 	// region := cmd.Flag("region").Value.String()

// 	endpoint := fmt.Sprintf("%s/projects", baseURL)

// 	client := &http.Client{
// 		Timeout: time.Second * 10,
// 	}

// 	req, err := http.NewRequest("GET", endpoint, nil)
// 	if err != nil {
// 		fmt.Printf("リクエストの作成に失敗: %v", err)
// 		return
// 	}

// 	req.SetBasicAuth(strings.Split(apiKey, ":")[0], strings.Split(apiKey, ":")[1])

// 	// req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
// 	// req.Header.Set("Content-Type", "application/json")
// 	// req.Header.Set("Accept", "application/json")

// 	resp, err := client.Do(req)
// 	if err != nil {
// 		fmt.Printf("API リクエストに失敗: %v", err)
// 		return
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		body, _ := io.ReadAll(resp.Body)
// 		fmt.Printf("API エラー: %s (status: %d)", string(body), resp.StatusCode)
// 		return
// 	}

// 	body, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		fmt.Printf("レスポンスの読み取りに失敗: %v", err)
// 		return
// 	}

// 	var clusters ClusterResponse
// 	if err := json.Unmarshal(body, &clusters); err != nil {
// 		fmt.Printf("JSONの解析に失敗: %v", err)
// 		return
// 	}

// 	fmt.Println(clusters)
// }
