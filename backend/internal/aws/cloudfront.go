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
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	State       string               `json:"state"`
	DomainName  string               `json:"domain_name"`
	Aliases     []string             `json:"aliases"`
	Origins     []string             `json:"origins"`
	Behaviors   []CloudFrontBehavior `json:"behaviors"`
	Enabled     bool                 `json:"enabled"`
	PriceClass  string               `json:"price_class"`
	CostMonthly float64              `json:"cost_monthly"`
}

// CloudFrontBehavior represents a single cache behavior (additional or default) of a distribution.
type CloudFrontBehavior struct {
	PathPattern          string   `json:"path_pattern"`
	TargetOriginID       string   `json:"target_origin_id"`
	ViewerProtocolPolicy string   `json:"viewer_protocol_policy"`
	AllowedMethods       []string `json:"allowed_methods"`
	Compress             bool     `json:"compress"`
	IsDefault            bool     `json:"is_default"`
}

func (r CloudFrontResource) ResourceID() string    { return r.ID }
func (r CloudFrontResource) ResourceName() string  { return r.Name }
func (r CloudFrontResource) ResourceState() string { return NormalizeState(r.State) }
func (r CloudFrontResource) ServiceName() string   { return "cloudfront" }

// ListCloudFrontResources returns all CloudFront distributions.
// CloudFront is a global service; us-east-1 is used.
func ListCloudFrontResources(ctx context.Context, profile, _ string) ([]CloudFrontResource, error) {
	client, err := newCloudFrontClient(ctx, profile)
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
	client, err := newCloudFrontClient(ctx, profile)
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
	var aliases []string
	if d.Aliases != nil {
		aliases = append(aliases, d.Aliases.Items...)
	}
	return CloudFrontResource{
		ID:         ptrStr(d.Id),
		Name:       name,
		State:      DisplayState(ptrStr(d.Status)),
		DomainName: ptrStr(d.DomainName),
		Aliases:    aliases,
		Origins:    origins,
		Behaviors:  cloudfrontBehaviorsFromSummary(d),
		Enabled:    ptrBool(d.Enabled),
		PriceClass: string(d.PriceClass),
	}
}

// cloudfrontBehaviorsFromSummary はディストリビューションの追加ビヘイビアと既定ビヘイビアを
// 評価順 (追加が Items 順で先、既定が末尾) に並べたスライスに変換する。
// DefaultCacheBehavior と CacheBehaviors がどちらも欠けている場合は nil を返す。
func cloudfrontBehaviorsFromSummary(d cftypes.DistributionSummary) []CloudFrontBehavior {
	var behaviors []CloudFrontBehavior
	if d.CacheBehaviors != nil {
		for _, b := range d.CacheBehaviors.Items {
			behaviors = append(behaviors, CloudFrontBehavior{
				PathPattern:          ptrStr(b.PathPattern),
				TargetOriginID:       ptrStr(b.TargetOriginId),
				ViewerProtocolPolicy: string(b.ViewerProtocolPolicy),
				AllowedMethods:       allowedMethodItems(b.AllowedMethods),
				Compress:             ptrBool(b.Compress),
			})
		}
	}
	if d.DefaultCacheBehavior != nil {
		behaviors = append(behaviors, CloudFrontBehavior{
			TargetOriginID:       ptrStr(d.DefaultCacheBehavior.TargetOriginId),
			ViewerProtocolPolicy: string(d.DefaultCacheBehavior.ViewerProtocolPolicy),
			AllowedMethods:       allowedMethodItems(d.DefaultCacheBehavior.AllowedMethods),
			Compress:             ptrBool(d.DefaultCacheBehavior.Compress),
			IsDefault:            true,
		})
	}
	return behaviors
}

// allowedMethodItems は AllowedMethods から Method のスライスを string スライスに変換する。
// AllowedMethods が nil または Items が nil の場合は空スライスを返す。
func allowedMethodItems(m *cftypes.AllowedMethods) []string {
	if m == nil {
		return make([]string, 0)
	}
	methods := make([]string, 0, len(m.Items))
	for _, item := range m.Items {
		methods = append(methods, string(item))
	}
	return methods
}

// newCloudFrontClient は CloudFront API クライアントを生成する。CloudFront はグローバルサービスのため us-east-1 を使う。
func newCloudFrontClient(ctx context.Context, profile string) (*cloudfront.Client, error) {
	return NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *cloudfront.Client {
		return cloudfront.NewFromConfig(cfg)
	})
}
