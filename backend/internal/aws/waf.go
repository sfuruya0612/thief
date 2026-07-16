package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
)

// WAFResource represents a WAFv2 Web ACL.
type WAFResource struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	State           string            `json:"state"`
	Scope           string            `json:"scope"`
	RuleCount       int               `json:"rule_count"`
	AssociatedCount int               `json:"associated_count"`
	Tags            map[string]string `json:"tags"`
	CostMonthly     float64           `json:"cost_monthly"`
}

func (r WAFResource) ResourceID() string    { return r.ID }
func (r WAFResource) ResourceName() string  { return r.Name }
func (r WAFResource) ResourceState() string { return NormalizeState(r.State) }
func (r WAFResource) ServiceName() string   { return "waf" }

// ListWAFResources returns all WAFv2 Web ACLs (REGIONAL for the given region and
// CLOUDFRONT scope from us-east-1) for the given profile.
func ListWAFResources(ctx context.Context, profile, region string) ([]WAFResource, error) {
	regionalClient, err := newWAFClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	// CloudFront スコープは us-east-1 必須
	globalClient, err := newWAFClient(ctx, profile, "us-east-1")
	if err != nil {
		return nil, err
	}

	var resources []WAFResource

	regionalACLs, err := listWAFACLs(ctx, regionalClient, waftypes.ScopeRegional)
	if err != nil {
		return nil, err
	}
	resources = append(resources, regionalACLs...)

	cfACLs, err := listWAFACLs(ctx, globalClient, waftypes.ScopeCloudfront)
	if err != nil {
		return nil, err
	}
	resources = append(resources, cfACLs...)

	return resources, nil
}

func listWAFACLs(ctx context.Context, client *wafv2.Client, scope waftypes.Scope) ([]WAFResource, error) {
	var summaries []waftypes.WebACLSummary
	var nextMarker *string
	for {
		out, err := client.ListWebACLs(ctx, &wafv2.ListWebACLsInput{
			Scope:      scope,
			NextMarker: nextMarker,
		})
		if err != nil {
			return nil, fmt.Errorf("list web acls scope=%s: %w", scope, err)
		}
		summaries = append(summaries, out.WebACLs...)
		if out.NextMarker == nil || *out.NextMarker == "" {
			break
		}
		nextMarker = out.NextMarker
	}

	var resources []WAFResource
	for _, s := range summaries {
		acl, err := client.GetWebACL(ctx, &wafv2.GetWebACLInput{
			Id:    s.Id,
			Name:  s.Name,
			Scope: scope,
		})
		if err != nil {
			return nil, fmt.Errorf("get web acl %s: %w", ptrStr(s.Id), err)
		}
		ruleCount := 0
		if acl.WebACL != nil {
			ruleCount = len(acl.WebACL.Rules)
		}
		// ListResourcesForWebACL は REGIONAL でのみ有効
		associatedCount := 0
		if scope == waftypes.ScopeRegional {
			resOut, err := client.ListResourcesForWebACL(ctx, &wafv2.ListResourcesForWebACLInput{
				WebACLArn: s.ARN,
			})
			if err == nil && resOut != nil {
				associatedCount = len(resOut.ResourceArns)
			}
		}
		tags := map[string]string{}
		tagsOut, tagErr := client.ListTagsForResource(ctx, &wafv2.ListTagsForResourceInput{
			ResourceARN: s.ARN,
		})
		if tagErr == nil && tagsOut != nil && tagsOut.TagInfoForResource != nil {
			for _, t := range tagsOut.TagInfoForResource.TagList {
				tags[ptrStr(t.Key)] = ptrStr(t.Value)
			}
		}
		resources = append(resources, newWAFResource(ptrStr(s.Id), ptrStr(s.Name), scope, ruleCount, associatedCount, tags))
	}
	return resources, nil
}

func newWAFResource(id, name string, scope waftypes.Scope, ruleCount, associatedCount int, tags map[string]string) WAFResource {
	return WAFResource{
		ID:              id,
		Name:            name,
		State:           "active",
		Scope:           string(scope),
		RuleCount:       ruleCount,
		AssociatedCount: associatedCount,
		Tags:            tags,
	}
}

// newWAFClient は WAFv2 API クライアントを生成する。
func newWAFClient(ctx context.Context, profile, region string) (*wafv2.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *wafv2.Client {
		return wafv2.NewFromConfig(cfg)
	})
}
