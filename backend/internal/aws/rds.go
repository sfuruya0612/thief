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
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	State         string            `json:"state"`
	Engine        string            `json:"engine"`
	EngineVersion string            `json:"engine_version"`
	Class         string            `json:"class"`
	MultiAZ       bool              `json:"multi_az"`
	Endpoint      string            `json:"endpoint"`
	Port          int32             `json:"port"`
	VpcID         string            `json:"vpc_id"`
	Tags          map[string]string `json:"tags"`
	CostMonthly   float64           `json:"cost_monthly"`
	LaunchTime    time.Time         `json:"launch_time"`
}

func (r RDSResource) ResourceID() string    { return r.ID }
func (r RDSResource) ResourceName() string  { return r.Name }
func (r RDSResource) ResourceState() string { return NormalizeState(r.State) }
func (r RDSResource) ServiceName() string   { return "rds" }

// ListRDSResources returns all RDS DB instances for the given profile/region.
func ListRDSResources(ctx context.Context, profile, region string) ([]RDSResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *rds.Client {
		return rds.NewFromConfig(cfg)
	})
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
	return RDSResource{
		ID:            ptrStr(db.DBInstanceIdentifier),
		Name:          ptrStr(db.DBInstanceIdentifier),
		State:         DisplayState(ptrStr(db.DBInstanceStatus)),
		Engine:        ptrStr(db.Engine),
		EngineVersion: ptrStr(db.EngineVersion),
		Class:         ptrStr(db.DBInstanceClass),
		MultiAZ:       ptrBool(db.MultiAZ),
		Endpoint:      endpoint,
		Port:          port,
		VpcID:         ptrStr(db.DBSubnetGroup.VpcId),
		Tags:          tags,
		LaunchTime:    launch,
	}
}

func rdsTagsToMap(tags []rdstypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[ptrStr(t.Key)] = ptrStr(t.Value)
	}
	return m
}
