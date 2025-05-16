package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
)

type ElasticacheOpts struct {
}

type elasticacheApi interface {
	DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
}

// NewElasticacheClient creates a new ElastiCache client using the specified AWS profile and region.
func NewElasticacheClient(profile, region string) (elasticacheApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create ElastiCache client: %w", err)
	}
	return elasticache.NewFromConfig(cfg), nil
}

func GenerateDescribeCacheClustersInput(opts *ElasticacheOpts) *elasticache.DescribeCacheClustersInput {
	return &elasticache.DescribeCacheClustersInput{}
}

func DescribeCacheClusters(client elasticacheApi, input *elasticache.DescribeCacheClustersInput) ([][]string, error) {
	o, err := client.DescribeCacheClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var clusters [][]string
	for _, c := range o.CacheClusters {
		cluster := []string{
			aws.ToString(c.ReplicationGroupId),
			aws.ToString(c.CacheClusterId),
			aws.ToString(c.CacheNodeType),
			aws.ToString(c.Engine),
			aws.ToString(c.EngineVersion),
			aws.ToString(c.CacheClusterStatus),
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}
