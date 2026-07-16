package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// NATGatewayResource represents a NAT Gateway.
type NATGatewayResource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	State       string            `json:"state"`
	VpcID       string            `json:"vpc_id"`
	SubnetID    string            `json:"subnet_id"`
	ElasticIP   string            `json:"elastic_ip"`
	Tags        map[string]string `json:"tags"`
	CostMonthly float64           `json:"cost_monthly"`
	LaunchTime  time.Time         `json:"launch_time"`
}

func (r NATGatewayResource) ResourceID() string    { return r.ID }
func (r NATGatewayResource) ResourceName() string  { return r.Name }
func (r NATGatewayResource) ResourceState() string { return NormalizeState(r.State) }
func (r NATGatewayResource) ServiceName() string   { return "natgw" }

// ListNATGatewayResources returns all NAT Gateways for the given profile/region.
func ListNATGatewayResources(ctx context.Context, profile, region string) ([]NATGatewayResource, error) {
	client, err := newEC2Client(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var resources []NATGatewayResource
	paginator := ec2.NewDescribeNatGatewaysPaginator(client, &ec2.DescribeNatGatewaysInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe nat gateways: %w", err)
		}
		for _, ng := range page.NatGateways {
			resources = append(resources, natgwFromGateway(ng))
		}
	}
	return resources, nil
}

func natgwFromGateway(ng ec2types.NatGateway) NATGatewayResource {
	tags := tagsToMap(ng.Tags)
	eip := ""
	if len(ng.NatGatewayAddresses) > 0 {
		eip = ptrStr(ng.NatGatewayAddresses[0].PublicIp)
	}
	created := time.Time{}
	if ng.CreateTime != nil {
		created = *ng.CreateTime
	}
	return NATGatewayResource{
		ID:         ptrStr(ng.NatGatewayId),
		Name:       tags["Name"],
		State:      DisplayState(string(ng.State)),
		VpcID:      ptrStr(ng.VpcId),
		SubnetID:   ptrStr(ng.SubnetId),
		ElasticIP:  eip,
		Tags:       tags,
		LaunchTime: created,
	}
}
