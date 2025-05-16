package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

type RdsOpts struct {
}

type rdsApi interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	DescribeDBClusters(ctx context.Context, prams *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
}

// NewRdsClient creates a new RDS client using the specified AWS profile and region.
func NewRdsClient(profile, region string) (rdsApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create RDS client: %w", err)
	}
	return rds.NewFromConfig(cfg), nil
}

func GenerateDescribeDBInstancesInput(opts *RdsOpts) *rds.DescribeDBInstancesInput {
	return &rds.DescribeDBInstancesInput{}
}

func DescribeDBInstances(client rdsApi, input *rds.DescribeDBInstancesInput) ([][]string, error) {
	o, err := client.DescribeDBInstances(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var instances [][]string
	for _, i := range o.DBInstances {
		instance := []string{
			aws.ToString(i.DBInstanceIdentifier),
			aws.ToString(i.DBInstanceClass),
			aws.ToString(i.Engine),
			aws.ToString(i.EngineVersion),
			fmt.Sprintf("%dGB", *i.AllocatedStorage),
			aws.ToString(i.StorageType),
			aws.ToString(i.DBInstanceStatus),
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

func GenerateDescribeDBClustersInput(opts *RdsOpts) *rds.DescribeDBClustersInput {
	return &rds.DescribeDBClustersInput{}
}

func DescribeDBClusters(client rdsApi, input *rds.DescribeDBClustersInput) ([][]string, error) {
	o, err := client.DescribeDBClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var clusters [][]string
	for _, c := range o.DBClusters {
		cluster := []string{
			aws.ToString(c.DBClusterIdentifier),
			aws.ToString(c.Engine),
			aws.ToString(c.EngineVersion),
			aws.ToString(c.EngineMode),
			aws.ToString(c.Status),
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}
