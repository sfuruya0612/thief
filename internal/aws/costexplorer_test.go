package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

func TestNewCostExplorerClient(t *testing.T) {
	t.Skip("Skipping test that requires AWS credentials")

	client, err := NewCostExplorerClient("default", "ap-northeast-1")
	if err != nil {
		t.Fatal("Failed to create CostExplorerClient:", err)
	}

	if client == nil {
		t.Fatal("Failed to create CostExplorerClient")
	}

	if client.client == nil {
		t.Fatal("CostExplorerClient's client is nil")
	}
}

func TestGetCostAndUsage(t *testing.T) {
	t.Skip("Skipping test that requires AWS credentials")

	client, err := NewCostExplorerClient("default", "ap-northeast-1")
	if err != nil {
		t.Fatal("Failed to create CostExplorerClient:", err)
	}

	startDate := "2023-01-01"
	endDate := "2023-01-31"

	groupBy := []types.GroupDefinition{
		{
			Key:  nil,
			Type: types.GroupDefinitionTypeDimension,
		},
	}

	_, err = client.GetCostAndUsage(startDate, endDate, types.GranularityMonthly, groupBy, UnblendedCost)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}
