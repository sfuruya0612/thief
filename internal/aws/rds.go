package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

// RdsOpts defines options for RDS API operations.
type RdsOpts struct{}

// RDSInstanceInfo holds display fields for an RDS DB instance.
type RDSInstanceInfo struct {
	Name            string
	DBInstanceClass string
	Engine          string
	EngineVersion   string
	Storage         string
	StorageType     string
	Status          string
}

// ToRow converts RDSInstanceInfo to a string slice suitable for table formatting.
func (i RDSInstanceInfo) ToRow() []string {
	return []string{
		i.Name, i.DBInstanceClass, i.Engine, i.EngineVersion,
		i.Storage, i.StorageType, i.Status,
	}
}

// RDSClusterInfo holds display fields for an RDS DB cluster.
type RDSClusterInfo struct {
	Name          string
	Engine        string
	EngineVersion string
	EngineMode    string
	Status        string
}

// ToRow converts RDSClusterInfo to a string slice suitable for table formatting.
func (c RDSClusterInfo) ToRow() []string {
	return []string{c.Name, c.Engine, c.EngineVersion, c.EngineMode, c.Status}
}

type rdsApi interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	DescribeDBClusters(ctx context.Context, prams *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
}

// NewRdsClient creates a new RDS client using the specified AWS profile and region.
func NewRdsClient(profile, region string) (rdsApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create rds client: %w", err)
	}
	return rds.NewFromConfig(cfg), nil
}

// GenerateDescribeDBInstancesInput creates the input for the DescribeDBInstances API call.
func GenerateDescribeDBInstancesInput(opts *RdsOpts) *rds.DescribeDBInstancesInput {
	return &rds.DescribeDBInstancesInput{}
}

// DescribeDBInstances calls the RDS DescribeDBInstances API and returns the results
// as a typed slice of RDSInstanceInfo.
func DescribeDBInstances(client rdsApi, input *rds.DescribeDBInstancesInput) ([]RDSInstanceInfo, error) {
	o, err := client.DescribeDBInstances(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var instances []RDSInstanceInfo
	for _, i := range o.DBInstances {
		instances = append(instances, RDSInstanceInfo{
			Name:            aws.ToString(i.DBInstanceIdentifier),
			DBInstanceClass: aws.ToString(i.DBInstanceClass),
			Engine:          aws.ToString(i.Engine),
			EngineVersion:   aws.ToString(i.EngineVersion),
			Storage:         fmt.Sprintf("%dGB", *i.AllocatedStorage),
			StorageType:     aws.ToString(i.StorageType),
			Status:          aws.ToString(i.DBInstanceStatus),
		})
	}

	return instances, nil
}

// GenerateDescribeDBClustersInput creates the input for the DescribeDBClusters API call.
func GenerateDescribeDBClustersInput(opts *RdsOpts) *rds.DescribeDBClustersInput {
	return &rds.DescribeDBClustersInput{}
}

// DescribeDBClusters calls the RDS DescribeDBClusters API and returns the results
// as a typed slice of RDSClusterInfo.
func DescribeDBClusters(client rdsApi, input *rds.DescribeDBClustersInput) ([]RDSClusterInfo, error) {
	o, err := client.DescribeDBClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var clusters []RDSClusterInfo
	for _, c := range o.DBClusters {
		clusters = append(clusters, RDSClusterInfo{
			Name:          aws.ToString(c.DBClusterIdentifier),
			Engine:        aws.ToString(c.Engine),
			EngineVersion: aws.ToString(c.EngineVersion),
			EngineMode:    aws.ToString(c.EngineMode),
			Status:        aws.ToString(c.Status),
		})
	}

	return clusters, nil
}
