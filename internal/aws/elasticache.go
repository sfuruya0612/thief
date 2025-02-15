package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
)

type ElasticacheOpts struct {
}

type elasticacheApi interface {
	DescribeCacheClusters(ctx context.Context, params *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
}

func NewElasticacheClient(profile, region string) elasticacheApi {
	return elasticache.NewFromConfig(GetSession(profile, region))
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
