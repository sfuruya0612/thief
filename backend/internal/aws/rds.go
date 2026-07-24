package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// RDSResource represents a single RDS DB instance.
type RDSResource struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	State           string            `json:"state"`
	Engine          string            `json:"engine"`
	EngineVersion   string            `json:"engine_version"`
	Class           string            `json:"class"`
	MultiAZ         bool              `json:"multi_az"`
	Endpoint        string            `json:"endpoint"`
	Port            int32             `json:"port"`
	VpcID           string            `json:"vpc_id"`
	ParameterGroups []string          `json:"parameter_groups"`
	ClusterID       string            `json:"cluster_id"`
	Tags            map[string]string `json:"tags"`
	CostMonthly     float64           `json:"cost_monthly"`
	LaunchTime      time.Time         `json:"launch_time"`
}

// RDSParameter represents a single parameter in a DB parameter group.
type RDSParameter struct {
	Name          string `json:"name"`
	Value         string `json:"value"`
	AllowedValues string `json:"allowed_values"`
	ApplyType     string `json:"apply_type"`
	DataType      string `json:"data_type"`
	Source        string `json:"source"`
	IsModifiable  bool   `json:"is_modifiable"`
	Description   string `json:"description"`
}

func (r RDSResource) ResourceID() string    { return r.ID }
func (r RDSResource) ResourceName() string  { return r.Name }
func (r RDSResource) ResourceState() string { return NormalizeState(r.State) }
func (r RDSResource) ServiceName() string   { return "rds" }

// ListRDSResources returns all RDS DB instances for the given profile/region.
func ListRDSResources(ctx context.Context, profile, region string) ([]RDSResource, error) {
	client, err := newRDSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var resources []RDSResource
	paginator := rds.NewDescribeDBInstancesPaginator(client, &rds.DescribeDBInstancesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe rds instances: %w", err)
		}
		for _, db := range page.DBInstances {
			resources = append(resources, rdsFromInstance(db))
		}
	}
	return resources, nil
}

func rdsFromInstance(db rdstypes.DBInstance) RDSResource {
	tags := rdsTagsToMap(db.TagList)
	endpoint := ""
	port := int32(0)
	if db.Endpoint != nil {
		endpoint = ptrStr(db.Endpoint.Address)
		port = ptrInt32(db.Endpoint.Port)
	}
	launch := time.Time{}
	if db.InstanceCreateTime != nil {
		launch = *db.InstanceCreateTime
	}
	groups := make([]string, 0, len(db.DBParameterGroups))
	for _, g := range db.DBParameterGroups {
		if name := ptrStr(g.DBParameterGroupName); name != "" {
			groups = append(groups, name)
		}
	}
	return RDSResource{
		ID:              ptrStr(db.DBInstanceIdentifier),
		Name:            ptrStr(db.DBInstanceIdentifier),
		State:           DisplayState(ptrStr(db.DBInstanceStatus)),
		Engine:          ptrStr(db.Engine),
		EngineVersion:   ptrStr(db.EngineVersion),
		Class:           ptrStr(db.DBInstanceClass),
		MultiAZ:         ptrBool(db.MultiAZ),
		Endpoint:        endpoint,
		Port:            port,
		VpcID:           ptrStr(db.DBSubnetGroup.VpcId),
		ParameterGroups: groups,
		ClusterID:       ptrStr(db.DBClusterIdentifier),
		Tags:            tags,
		LaunchTime:      launch,
	}
}

// rdsParameterFromSDK は DescribeDBParameters の 1 パラメータを RDSParameter に変換する。
func rdsParameterFromSDK(p rdstypes.Parameter) RDSParameter {
	return RDSParameter{
		Name:          ptrStr(p.ParameterName),
		Value:         ptrStr(p.ParameterValue),
		AllowedValues: ptrStr(p.AllowedValues),
		ApplyType:     ptrStr(p.ApplyType),
		DataType:      ptrStr(p.DataType),
		Source:        ptrStr(p.Source),
		IsModifiable:  ptrBool(p.IsModifiable),
		Description:   ptrStr(p.Description),
	}
}

// ListRDSParameters は指定した DB パラメータグループの全パラメータを返す。
func ListRDSParameters(ctx context.Context, profile, region, group string) ([]RDSParameter, error) {
	client, err := newRDSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var params []RDSParameter
	paginator := rds.NewDescribeDBParametersPaginator(client, &rds.DescribeDBParametersInput{
		DBParameterGroupName: aws.String(group),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe rds parameters for %s: %w", group, err)
		}
		for _, p := range page.Parameters {
			params = append(params, rdsParameterFromSDK(p))
		}
	}
	return params, nil
}

func rdsTagsToMap(tags []rdstypes.Tag) map[string]string {
	return tagsToMapFunc(tags, func(t rdstypes.Tag) (*string, *string) { return t.Key, t.Value })
}

// RDSInstanceInfo はレガシー CLI 互換の RDS インスタンス表示用フィールドを保持する。
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

// RDSClusterInfo はレガシー CLI 互換の RDS クラスタ表示用フィールドを保持する。
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

// ListRDSInstanceInfos は RDS DB インスタンス一覧をレガシー CLI 互換フィールドで返す。
func ListRDSInstanceInfos(ctx context.Context, profile, region string) ([]RDSInstanceInfo, error) {
	client, err := newRDSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var instances []RDSInstanceInfo
	paginator := rds.NewDescribeDBInstancesPaginator(client, &rds.DescribeDBInstancesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe rds instances: %w", err)
		}
		for _, db := range page.DBInstances {
			instances = append(instances, RDSInstanceInfo{
				Name:            ptrStr(db.DBInstanceIdentifier),
				DBInstanceClass: ptrStr(db.DBInstanceClass),
				Engine:          ptrStr(db.Engine),
				EngineVersion:   ptrStr(db.EngineVersion),
				Storage:         fmt.Sprintf("%dGB", ptrInt32(db.AllocatedStorage)),
				StorageType:     ptrStr(db.StorageType),
				Status:          ptrStr(db.DBInstanceStatus),
			})
		}
	}
	return instances, nil
}

// ListRDSClusterInfos は RDS DB クラスタ一覧をレガシー CLI 互換フィールドで返す。
func ListRDSClusterInfos(ctx context.Context, profile, region string) ([]RDSClusterInfo, error) {
	client, err := newRDSClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var clusters []RDSClusterInfo
	paginator := rds.NewDescribeDBClustersPaginator(client, &rds.DescribeDBClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe rds clusters: %w", err)
		}
		for _, c := range page.DBClusters {
			clusters = append(clusters, RDSClusterInfo{
				Name:          ptrStr(c.DBClusterIdentifier),
				Engine:        ptrStr(c.Engine),
				EngineVersion: ptrStr(c.EngineVersion),
				EngineMode:    ptrStr(c.EngineMode),
				Status:        ptrStr(c.Status),
			})
		}
	}
	return clusters, nil
}

// RDSParameterInfo はレガシー CLI 互換の RDS パラメータ表示用フィールドを保持する。
type RDSParameterInfo struct {
	Name         string
	Value        string
	ApplyType    string
	DataType     string
	IsModifiable string
	Source       string
}

// ToRow converts RDSParameterInfo to a string slice suitable for table formatting.
func (p RDSParameterInfo) ToRow() []string {
	return []string{p.Name, p.Value, p.ApplyType, p.DataType, p.IsModifiable, p.Source}
}

// ListRDSParameterInfos は指定した DB パラメータグループのパラメータをレガシー CLI 互換フィールドで返す。
func ListRDSParameterInfos(ctx context.Context, profile, region, group string) ([]RDSParameterInfo, error) {
	params, err := ListRDSParameters(ctx, profile, region, group)
	if err != nil {
		return nil, err
	}
	infos := make([]RDSParameterInfo, 0, len(params))
	for _, p := range params {
		infos = append(infos, RDSParameterInfo{
			Name:         p.Name,
			Value:        p.Value,
			ApplyType:    p.ApplyType,
			DataType:     p.DataType,
			IsModifiable: fmt.Sprintf("%v", p.IsModifiable),
			Source:       p.Source,
		})
	}
	return infos, nil
}

// newRDSClient は RDS API クライアントを生成する。
func newRDSClient(ctx context.Context, profile, region string) (*rds.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *rds.Client {
		return rds.NewFromConfig(cfg)
	})
}
