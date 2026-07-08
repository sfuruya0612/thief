package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// ELBResource represents an Elastic Load Balancer (ALB, NLB, or CLB).
type ELBResource struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Type        string   `json:"type"` // application|network|gateway
	Scheme      string   `json:"scheme"`
	DNSName     string   `json:"dns_name"`
	VpcID       string   `json:"vpc_id"`
	AZs         []string `json:"azs"`
	CostMonthly float64  `json:"cost_monthly"`
}

func (r ELBResource) ResourceID() string    { return r.ID }
func (r ELBResource) ResourceName() string  { return r.Name }
func (r ELBResource) ResourceState() string { return NormalizeState(r.State) }
func (r ELBResource) ServiceName() string   { return "elb" }

// ListELBResources returns all ALB/NLB/Gateway load balancers for the given profile/region.
func ListELBResources(ctx context.Context, profile, region string) ([]ELBResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *elbv2.Client {
		return elbv2.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []ELBResource
	paginator := elbv2.NewDescribeLoadBalancersPaginator(client, &elbv2.DescribeLoadBalancersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, lb := range page.LoadBalancers {
			resources = append(resources, elbFromLB(lb))
		}
	}
	return resources, nil
}

func elbFromLB(lb elbv2types.LoadBalancer) ELBResource {
	state := "unknown"
	if lb.State != nil {
		state = DisplayState(string(lb.State.Code))
	}
	var azs []string
	for _, az := range lb.AvailabilityZones {
		azs = append(azs, ptrStr(az.ZoneName))
	}
	return ELBResource{
		ID:      ptrStr(lb.LoadBalancerArn),
		Name:    ptrStr(lb.LoadBalancerName),
		State:   state,
		Type:    string(lb.Type),
		Scheme:  string(lb.Scheme),
		DNSName: ptrStr(lb.DNSName),
		VpcID:   ptrStr(lb.VpcId),
		AZs:     azs,
	}
}
