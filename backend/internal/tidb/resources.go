package tidb

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"
)

// Project represents a TiDB Cloud project.
type Project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	OrgID        string    `json:"org_id"`
	ClusterCount int       `json:"cluster_count"`
	UserCount    int       `json:"user_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// Cluster represents a TiDB Cloud cluster.
type Cluster struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Region        string    `json:"region"`
	ClusterType   string    `json:"cluster_type"`
	CloudProvider string    `json:"cloud_provider"`
	CreatedAt     time.Time `json:"created_at"`
}

// Cost represents TiDB Cloud billing cost.
type Cost struct {
	BilledDate      string  `json:"billed_date"`
	ProjectName     string  `json:"project_name"`
	ClusterName     string  `json:"cluster_name"`
	ServicePathName string  `json:"service_path_name"`
	Credits         float64 `json:"credits"`
	Discounts       float64 `json:"discounts"`
	RunningTotal    float64 `json:"running_total"`
	TotalCost       float64 `json:"total_cost"`
}

type projectsResponse struct {
	Items []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		OrgID        string `json:"org_id"`
		ClusterCount int    `json:"cluster_count"`
		UserCount    int    `json:"user_count"`
		CreateTime   string `json:"create_timestamp"`
	} `json:"items"`
	Total int `json:"total"`
}

type clustersResponse struct {
	Items []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status struct {
			ClusterStatus string `json:"cluster_status"`
		} `json:"status"`
		Region        string `json:"region"`
		ClusterType   string `json:"cluster_type"`
		CloudProvider string `json:"cloud_provider"`
		CreateTime    string `json:"create_timestamp"`
	} `json:"items"`
	Total int `json:"total"`
}

// tidbListPageSize is the page size used when paginating TiDB Cloud list
// endpoints. TiDB Cloud API v1beta caps page_size at 100.
const tidbListPageSize = 100

type billResponse struct {
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

// ListProjects returns all TiDB Cloud projects.
func (c *Client) ListProjects() ([]Project, error) {
	projects := []Project{}
	for page := 1; ; page++ {
		resp, err := c.Get(fmt.Sprintf("/api/v1beta/projects?page=%d&page_size=%d", page, tidbListPageSize))
		if err != nil {
			return nil, fmt.Errorf("list tidb projects: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read tidb projects response: %w", err)
		}

		var data projectsResponse
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("parse tidb projects response: %w", err)
		}

		for _, item := range data.Items {
			t, _ := time.Parse(time.RFC3339, item.CreateTime)
			projects = append(projects, Project{
				ID:           item.ID,
				Name:         item.Name,
				OrgID:        item.OrgID,
				ClusterCount: item.ClusterCount,
				UserCount:    item.UserCount,
				CreatedAt:    t,
			})
		}

		if len(data.Items) < tidbListPageSize || len(projects) >= data.Total {
			break
		}
	}
	return projects, nil
}

// ListClusters returns all clusters for the given project.
func (c *Client) ListClusters(projectID string) ([]Cluster, error) {
	clusters := []Cluster{}
	for page := 1; ; page++ {
		resp, err := c.Get(fmt.Sprintf("/api/v1beta/projects/%s/clusters?page=%d&page_size=%d", projectID, page, tidbListPageSize))
		if err != nil {
			return nil, fmt.Errorf("list tidb clusters: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read tidb clusters response: %w", err)
		}

		var data clustersResponse
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("parse tidb clusters response: %w", err)
		}

		for _, item := range data.Items {
			t, _ := time.Parse(time.RFC3339, item.CreateTime)
			clusters = append(clusters, Cluster{
				ID:            item.ID,
				Name:          item.Name,
				Status:        item.Status.ClusterStatus,
				Region:        item.Region,
				ClusterType:   item.ClusterType,
				CloudProvider: item.CloudProvider,
				CreatedAt:     t,
			})
		}

		if len(data.Items) < tidbListPageSize || len(clusters) >= data.Total {
			break
		}
	}
	return clusters, nil
}

// GetCost returns billing cost details for the given month (YYYY-MM).
// If month is empty, the current year-month is used.
func (c *Client) GetCost(month string) ([]Cost, error) {
	if month == "" {
		month = time.Now().Format("2006-01")
	}

	resp, err := c.getBilling(fmt.Sprintf("/v1beta1/billsDetails/%s", month))
	if err != nil {
		return nil, fmt.Errorf("get tidb cost: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tidb cost response: %w", err)
	}

	var data billResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse tidb cost response: %w", err)
	}

	costs := []Cost{}
	for _, item := range data.Details {
		costs = append(costs, Cost{
			BilledDate:      item.BilledDate,
			ProjectName:     item.ProjectName,
			ClusterName:     item.ClusterName,
			ServicePathName: item.ServicePathName,
			Credits:         parseFloat(item.Credits),
			Discounts:       parseFloat(item.Discounts),
			RunningTotal:    parseFloat(item.RunningTotal),
			TotalCost:       parseFloat(item.TotalCost),
		})
	}
	return costs, nil
}

// parseFloat parses a TiDB Cloud billing amount string into a float64,
// returning 0 if the value is empty or malformed.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// GetCostRange returns billing cost details for every month in
// [startMonth, endMonth] (both YYYY-MM, inclusive). The TiDB Cloud billing
// API only accepts a single month per request, so this issues one request
// per month. If startMonth or endMonth is empty, the current year-month is
// used for that end of the range.
func (c *Client) GetCostRange(startMonth, endMonth string) ([]Cost, error) {
	now := time.Now()
	if startMonth == "" {
		startMonth = now.Format("2006-01")
	}
	if endMonth == "" {
		endMonth = now.Format("2006-01")
	}

	start, err := time.Parse("2006-01", startMonth)
	if err != nil {
		return nil, fmt.Errorf("parse start month %q: %w", startMonth, err)
	}
	end, err := time.Parse("2006-01", endMonth)
	if err != nil {
		return nil, fmt.Errorf("parse end month %q: %w", endMonth, err)
	}
	if end.Before(start) {
		start, end = end, start
	}

	costs := []Cost{}
	for m := start; !m.After(end); m = m.AddDate(0, 1, 0) {
		monthCosts, err := c.GetCost(m.Format("2006-01"))
		if err != nil {
			return nil, err
		}
		costs = append(costs, monthCosts...)
	}
	return costs, nil
}
