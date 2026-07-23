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
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	State          string  `json:"state"`
	Engine         string  `json:"engine"`
	EngineVersion  string  `json:"engine_version"`
	NodeType       string  `json:"node_type"`
	NumNodes       int32   `json:"num_nodes"`
	Endpoint       string  `json:"endpoint"`
	Port           int32   `json:"port"`
	ParameterGroup string  `json:"parameter_group"`
	CostMonthly    float64 `json:"cost_monthly"`
}

// ElastiCacheParameter represents a single parameter in a cache parameter group.
type ElastiCacheParameter struct {
	Name                 string `json:"name"`
	Value                string `json:"value"`
	AllowedValues        string `json:"allowed_values"`
	ChangeType           string `json:"change_type"`
	DataType             string `json:"data_type"`
	Source               string `json:"source"`
	IsModifiable         bool   `json:"is_modifiable"`
	MinimumEngineVersion string `json:"minimum_engine_version"`
	Description          string `json:"description"`
}

func (r ElastiCacheResource) ResourceID() string    { return r.ID }
func (r ElastiCacheResource) ResourceName() string  { return r.Name }
func (r ElastiCacheResource) ResourceState() string { return NormalizeState(r.State) }
func (r ElastiCacheResource) ServiceName() string   { return "elasticache" }

// ListElastiCacheResources returns all ElastiCache clusters for the given profile/region.
func ListElastiCacheResources(ctx context.Context, profile, region string) ([]ElastiCacheResource, error) {
	client, err := newElastiCacheClient(ctx, profile, region)
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
	paramGroup := ""
	if c.CacheParameterGroup != nil {
		paramGroup = ptrStr(c.CacheParameterGroup.CacheParameterGroupName)
	}
	return ElastiCacheResource{
		ID:             ptrStr(c.CacheClusterId),
		Name:           ptrStr(c.CacheClusterId),
		State:          DisplayState(ptrStr(c.CacheClusterStatus)),
		Engine:         ptrStr(c.Engine),
		EngineVersion:  ptrStr(c.EngineVersion),
		NodeType:       ptrStr(c.CacheNodeType),
		NumNodes:       ptrInt32(c.NumCacheNodes),
		Endpoint:       endpoint,
		Port:           port,
		ParameterGroup: paramGroup,
	}
}

// cacheParameterFromSDK は DescribeCacheParameters の 1 パラメータを ElastiCacheParameter に変換する。
func cacheParameterFromSDK(p ectypes.Parameter) ElastiCacheParameter {
	return ElastiCacheParameter{
		Name:                 ptrStr(p.ParameterName),
		Value:                ptrStr(p.ParameterValue),
		AllowedValues:        ptrStr(p.AllowedValues),
		ChangeType:           string(p.ChangeType),
		DataType:             ptrStr(p.DataType),
		Source:               ptrStr(p.Source),
		IsModifiable:         ptrBool(p.IsModifiable),
		MinimumEngineVersion: ptrStr(p.MinimumEngineVersion),
		Description:          ptrStr(p.Description),
	}
}

// ListElastiCacheParameters は指定した Cache パラメータグループの全パラメータを返す。
func ListElastiCacheParameters(ctx context.Context, profile, region, group string) ([]ElastiCacheParameter, error) {
	client, err := newElastiCacheClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var params []ElastiCacheParameter
	paginator := elasticache.NewDescribeCacheParametersPaginator(client, &elasticache.DescribeCacheParametersInput{
		CacheParameterGroupName: aws.String(group),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe elasticache parameters for %s: %w", group, err)
		}
		for _, p := range page.Parameters {
			params = append(params, cacheParameterFromSDK(p))
		}
	}
	return params, nil
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
	client, err := newElastiCacheClient(ctx, profile, region)
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

// ElastiCacheParameterInfo はレガシー CLI 互換の ElastiCache パラメータ表示用フィールドを保持する。
type ElastiCacheParameterInfo struct {
	Name         string
	Value        string
	ChangeType   string
	DataType     string
	IsModifiable string
	Source       string
}

// ToRow converts ElastiCacheParameterInfo to a string slice suitable for table formatting.
func (p ElastiCacheParameterInfo) ToRow() []string {
	return []string{p.Name, p.Value, p.ChangeType, p.DataType, p.IsModifiable, p.Source}
}

// ListElastiCacheParameterInfos は指定した Cache パラメータグループのパラメータをレガシー CLI 互換フィールドで返す。
func ListElastiCacheParameterInfos(ctx context.Context, profile, region, group string) ([]ElastiCacheParameterInfo, error) {
	params, err := ListElastiCacheParameters(ctx, profile, region, group)
	if err != nil {
		return nil, err
	}
	infos := make([]ElastiCacheParameterInfo, 0, len(params))
	for _, p := range params {
		infos = append(infos, ElastiCacheParameterInfo{
			Name:         p.Name,
			Value:        p.Value,
			ChangeType:   p.ChangeType,
			DataType:     p.DataType,
			IsModifiable: fmt.Sprintf("%v", p.IsModifiable),
			Source:       p.Source,
		})
	}
	return infos, nil
}

// newElastiCacheClient は ElastiCache API クライアントを生成する。
func newElastiCacheClient(ctx context.Context, profile, region string) (*elasticache.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *elasticache.Client {
		return elasticache.NewFromConfig(cfg)
	})
}
