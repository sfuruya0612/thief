package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
)

// CloudFrontResource represents a CloudFront distribution.
type CloudFrontResource struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	State       string   `json:"state"`
	DomainName  string   `json:"domain_name"`
	Origins     []string `json:"origins"`
	Enabled     bool     `json:"enabled"`
	PriceClass  string   `json:"price_class"`
	CostMonthly float64  `json:"cost_monthly"`
}

func (r CloudFrontResource) ResourceID() string    { return r.ID }
func (r CloudFrontResource) ResourceName() string  { return r.Name }
func (r CloudFrontResource) ResourceState() string { return NormalizeState(r.State) }
func (r CloudFrontResource) ServiceName() string   { return "cloudfront" }

// ListCloudFrontResources returns all CloudFront distributions.
// CloudFront is a global service; us-east-1 is used.
func ListCloudFrontResources(ctx context.Context, profile, _ string) ([]CloudFrontResource, error) {
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *cloudfront.Client {
		return cloudfront.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []CloudFrontResource
	paginator := cloudfront.NewListDistributionsPaginator(client, &cloudfront.ListDistributionsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list cloudfront distributions: %w", err)
		}
		if page.DistributionList == nil {
			continue
		}
		for _, d := range page.DistributionList.Items {
			resources = append(resources, cloudfrontFromSummary(d))
		}
	}
	return resources, nil
}

// CreateCloudFrontInvalidation submits a cache invalidation for the given distribution.
func CreateCloudFrontInvalidation(ctx context.Context, profile, distributionID string, paths []string) error {
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *cloudfront.Client {
		return cloudfront.NewFromConfig(cfg)
	})
	if err != nil {
		return err
	}
	quantity := int32(len(paths))
	_, err = client.CreateInvalidation(ctx, &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(distributionID),
		InvalidationBatch: &cftypes.InvalidationBatch{
			CallerReference: aws.String(fmt.Sprintf("thief-%d", len(paths))),
			Paths: &cftypes.Paths{
				Quantity: &quantity,
				Items:    paths,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create cloudfront invalidation: %w", err)
	}
	return nil
}

func cloudfrontFromSummary(d cftypes.DistributionSummary) CloudFrontResource {
	var origins []string
	if d.Origins != nil {
		for _, o := range d.Origins.Items {
			origins = append(origins, ptrStr(o.DomainName))
		}
	}
	name := ptrStr(d.Comment)
	if name == "" {
		name = ptrStr(d.Id)
	}
	return CloudFrontResource{
		ID:         ptrStr(d.Id),
		Name:       name,
		State:      DisplayState(ptrStr(d.Status)),
		DomainName: ptrStr(d.DomainName),
		Origins:    origins,
		Enabled:    ptrBool(d.Enabled),
		PriceClass: string(d.PriceClass),
	}
}
