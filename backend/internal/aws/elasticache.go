package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	ectypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
)

// ElastiCacheResource represents a single ElastiCache cluster.
type ElastiCacheResource struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	State         string  `json:"state"`
	Engine        string  `json:"engine"`
	EngineVersion string  `json:"engine_version"`
	NodeType      string  `json:"node_type"`
	NumNodes      int32   `json:"num_nodes"`
	Endpoint      string  `json:"endpoint"`
	Port          int32   `json:"port"`
	CostMonthly   float64 `json:"cost_monthly"`
}

func (r ElastiCacheResource) ResourceID() string    { return r.ID }
func (r ElastiCacheResource) ResourceName() string  { return r.Name }
func (r ElastiCacheResource) ResourceState() string { return NormalizeState(r.State) }
func (r ElastiCacheResource) ServiceName() string   { return "elasticache" }

// ListElastiCacheResources returns all ElastiCache clusters for the given profile/region.
func ListElastiCacheResources(ctx context.Context, profile, region string) ([]ElastiCacheResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *elasticache.Client {
		return elasticache.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []ElastiCacheResource
	paginator := elasticache.NewDescribeCacheClustersPaginator(client, &elasticache.DescribeCacheClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe elasticache clusters: %w", err)
		}
		for _, c := range page.CacheClusters {
			resources = append(resources, elastiCacheFromCluster(c))
		}
	}
	return resources, nil
}

func elastiCacheFromCluster(c ectypes.CacheCluster) ElastiCacheResource {
	endpoint := ""
	port := int32(0)
	if c.ConfigurationEndpoint != nil {
		endpoint = ptrStr(c.ConfigurationEndpoint.Address)
		port = ptrInt32(c.ConfigurationEndpoint.Port)
	}
	return ElastiCacheResource{
		ID:            ptrStr(c.CacheClusterId),
		Name:          ptrStr(c.CacheClusterId),
		State:         DisplayState(ptrStr(c.CacheClusterStatus)),
		Engine:        ptrStr(c.Engine),
		EngineVersion: ptrStr(c.EngineVersion),
		NodeType:      ptrStr(c.CacheNodeType),
		NumNodes:      ptrInt32(c.NumCacheNodes),
		Endpoint:      endpoint,
		Port:          port,
	}
}

// ElastiCacheClusterInfo はレガシー CLI 互換の ElastiCache 表示用フィールドを保持する。
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

// ListElastiCacheClusterInfos は ElastiCache クラスタ一覧をレガシー CLI 互換フィールドで返す。
func ListElastiCacheClusterInfos(ctx context.Context, profile, region string) ([]ElastiCacheClusterInfo, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *elasticache.Client {
		return elasticache.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var clusters []ElastiCacheClusterInfo
	paginator := elasticache.NewDescribeCacheClustersPaginator(client, &elasticache.DescribeCacheClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe elasticache clusters: %w", err)
		}
		for _, c := range page.CacheClusters {
			clusters = append(clusters, ElastiCacheClusterInfo{
				ReplicationGroupID: ptrStr(c.ReplicationGroupId),
				CacheClusterID:     ptrStr(c.CacheClusterId),
				CacheNodeType:      ptrStr(c.CacheNodeType),
				Engine:             ptrStr(c.Engine),
				EngineVersion:      ptrStr(c.EngineVersion),
				Status:             ptrStr(c.CacheClusterStatus),
			})
		}
	}
	return clusters, nil
}
