package tidb

import (
	"encoding/json"
	"fmt"
	"io"
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
}

type billResponse struct {
	Overview []struct {
		BilledMonth  string  `json:"billed_month"`
		Credits      float64 `json:"credits"`
		Discounts    float64 `json:"discounts"`
		RunningTotal float64 `json:"running_total"`
		TotalCost    float64 `json:"total_cost"`
	} `json:"overview"`
	SummaryByProject []struct {
		ProjectName string  `json:"project_name"`
		TotalCost   float64 `json:"total_cost"`
	} `json:"summary_by_project"`
}

// ListProjects returns all TiDB Cloud projects.
func (c *Client) ListProjects() ([]Project, error) {
	resp, err := c.Get("/api/v1beta/projects")
	if err != nil {
		return nil, fmt.Errorf("list tidb projects: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tidb projects response: %w", err)
	}

	var data projectsResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse tidb projects response: %w", err)
	}

	var projects []Project
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
	return projects, nil
}

// ListClusters returns all clusters for the given project.
func (c *Client) ListClusters(projectID string) ([]Cluster, error) {
	resp, err := c.Get(fmt.Sprintf("/api/v1beta/projects/%s/clusters", projectID))
	if err != nil {
		return nil, fmt.Errorf("list tidb clusters: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tidb clusters response: %w", err)
	}

	var data clustersResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse tidb clusters response: %w", err)
	}

	var clusters []Cluster
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
	return clusters, nil
}

// GetCost returns billing cost for the given month (YYYY-MM).
func (c *Client) GetCost(month string) ([]Cost, error) {
	endpoint := "/api/v1beta/bills"
	if month != "" {
		endpoint += "?month=" + month
	}
	resp, err := c.Get(endpoint)
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

	var costs []Cost
	for _, item := range data.Overview {
		costs = append(costs, Cost{
			BilledDate:   item.BilledMonth,
			Credits:      item.Credits,
			Discounts:    item.Discounts,
			RunningTotal: item.RunningTotal,
			TotalCost:    item.TotalCost,
		})
	}
	return costs, nil
}
