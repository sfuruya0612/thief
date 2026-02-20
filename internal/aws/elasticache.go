package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
)

// ElasticacheOpts defines options for ElastiCache API operations.
type ElasticacheOpts struct{}

// ElastiCacheClusterInfo holds display fields for an ElastiCache cluster.
type ElastiCacheClusterInfo struct {
	ReplicationGroupID string
	CacheClusterID     string
	CacheNodeType      string
	Engine             string
	EngineVersion      string
	Status             string
}

// ToRow converts ElastiCacheClusterInfo to a string slice suitable for table formatting.
func (c ElastiCacheClusterInfo) ToRow() []string {
	return []string{
		c.ReplicationGroupID, c.CacheClusterID, c.CacheNodeType,
		c.Engine, c.EngineVersion, c.Status,
	}
}

type elasticacheApi interface {
	DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
}

// NewElasticacheClient creates a new ElastiCache client using the specified AWS profile and region.
func NewElasticacheClient(profile, region string) (elasticacheApi, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create elasticache client: %w", err)
	}
	return elasticache.NewFromConfig(cfg), nil
}

// GenerateDescribeCacheClustersInput creates the input for the DescribeCacheClusters API call.
func GenerateDescribeCacheClustersInput(opts *ElasticacheOpts) *elasticache.DescribeCacheClustersInput {
	return &elasticache.DescribeCacheClustersInput{}
}

// DescribeCacheClusters calls the ElastiCache DescribeCacheClusters API and returns
// the results as a typed slice of ElastiCacheClusterInfo.
func DescribeCacheClusters(client elasticacheApi, input *elasticache.DescribeCacheClustersInput) ([]ElastiCacheClusterInfo, error) {
	o, err := client.DescribeCacheClusters(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var clusters []ElastiCacheClusterInfo
	for _, c := range o.CacheClusters {
		clusters = append(clusters, ElastiCacheClusterInfo{
			ReplicationGroupID: aws.ToString(c.ReplicationGroupId),
			CacheClusterID:     aws.ToString(c.CacheClusterId),
			CacheNodeType:      aws.ToString(c.CacheNodeType),
			Engine:             aws.ToString(c.Engine),
			EngineVersion:      aws.ToString(c.EngineVersion),
			Status:             aws.ToString(c.CacheClusterStatus),
		})
	}

	return clusters, nil
}
