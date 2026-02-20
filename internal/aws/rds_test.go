package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

type mockRdsClient struct {
}

func (m *mockRdsClient) DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return &rds.DescribeDBInstancesOutput{
		DBInstances: []types.DBInstance{
			{
				DBInstanceIdentifier: aws.String("test-instance"),
				DBInstanceClass:      aws.String("db.t2.micro"),
				Engine:               aws.String("mysql"),
				EngineVersion:        aws.String("5.7.22"),
				AllocatedStorage:     aws.Int32(20),
				StorageType:          aws.String("gp2"),
				DBInstanceStatus:     aws.String("available"),
			},
		},
	}, nil
}

func (m *mockRdsClient) DescribeDBClusters(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	return &rds.DescribeDBClustersOutput{
		DBClusters: []types.DBCluster{
			{
				DBClusterIdentifier: aws.String("test-cluster"),
				Engine:              aws.String("aurora"),
				EngineVersion:       aws.String("5.6.10a"),
				EngineMode:          aws.String("provisioned"),
				Status:              aws.String("available"),
			},
		},
	}, nil
}

func TestDescribeDBInstances(t *testing.T) {
	mockClient := &mockRdsClient{}
	input := &rds.DescribeDBInstancesInput{}

	result, err := DescribeDBInstances(mockClient, input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(result))
	}

	i := result[0]
	if i.Name != "test-instance" {
		t.Errorf("expected Name 'test-instance', got '%s'", i.Name)
	}
	if i.DBInstanceClass != "db.t2.micro" {
		t.Errorf("expected DBInstanceClass 'db.t2.micro', got '%s'", i.DBInstanceClass)
	}
	if i.Engine != "mysql" {
		t.Errorf("expected Engine 'mysql', got '%s'", i.Engine)
	}
	if i.EngineVersion != "5.7.22" {
		t.Errorf("expected EngineVersion '5.7.22', got '%s'", i.EngineVersion)
	}
	if i.Storage != "20GB" {
		t.Errorf("expected Storage '20GB', got '%s'", i.Storage)
	}
	if i.StorageType != "gp2" {
		t.Errorf("expected StorageType 'gp2', got '%s'", i.StorageType)
	}
	if i.Status != "available" {
		t.Errorf("expected Status 'available', got '%s'", i.Status)
	}
}

func TestDescribeDBClusters(t *testing.T) {
	mockClient := &mockRdsClient{}
	input := &rds.DescribeDBClustersInput{}

	result, err := DescribeDBClusters(mockClient, input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result))
	}

	c := result[0]
	if c.Name != "test-cluster" {
		t.Errorf("expected Name 'test-cluster', got '%s'", c.Name)
	}
	if c.Engine != "aurora" {
		t.Errorf("expected Engine 'aurora', got '%s'", c.Engine)
	}
	if c.EngineVersion != "5.6.10a" {
		t.Errorf("expected EngineVersion '5.6.10a', got '%s'", c.EngineVersion)
	}
	if c.EngineMode != "provisioned" {
		t.Errorf("expected EngineMode 'provisioned', got '%s'", c.EngineMode)
	}
	if c.Status != "available" {
		t.Errorf("expected Status 'available', got '%s'", c.Status)
	}
}
