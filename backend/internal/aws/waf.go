package aws

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
	Description     string            `json:"description"`
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
			tags = tagsToMapFunc(tagsOut.TagInfoForResource.TagList, func(t waftypes.Tag) (*string, *string) { return t.Key, t.Value })
		}
		resources = append(resources, newWAFResource(ptrStr(s.Id), ptrStr(s.Name), scope, ruleCount, associatedCount, tags, s.Description))
	}
	return resources, nil
}

func newWAFResource(id, name string, scope waftypes.Scope, ruleCount, associatedCount int, tags map[string]string, description *string) WAFResource {
	return WAFResource{
		ID:              id,
		Name:            name,
		State:           "active",
		Scope:           string(scope),
		Description:     ptrStr(description),
		RuleCount:       ruleCount,
		AssociatedCount: associatedCount,
		Tags:            tags,
	}
}

// WAFRule represents a rule in a WAFv2 Web ACL.
type WAFRule struct {
	Name      string `json:"name"`
	Priority  int32  `json:"priority"`
	Action    string `json:"action"`
	Statement string `json:"statement"`
}

// ListWAFRules returns the rules of the Web ACL identified by name + id + scope,
// sorted by priority ascending. scope must be REGIONAL or CLOUDFRONT (validated
// by the caller).
func ListWAFRules(ctx context.Context, profile, region, scope, name, id string) ([]WAFRule, error) {
	sc := waftypes.Scope(scope)
	client, err := newWAFClient(ctx, profile, wafClientRegion(sc, region))
	if err != nil {
		return nil, err
	}
	out, err := client.GetWebACL(ctx, &wafv2.GetWebACLInput{
		Id:    aws.String(id),
		Name:  aws.String(name),
		Scope: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("get web acl %s: %w", id, err)
	}
	rules := []WAFRule{}
	if out.WebACL != nil {
		for _, r := range out.WebACL.Rules {
			rules = append(rules, newWAFRule(r))
		}
	}
	sortWAFRules(rules)
	return rules, nil
}

// wafClientRegion は wafv2 クライアントに使うリージョンを返す。
// CLOUDFRONT スコープは us-east-1 必須 (ListWAFResources と同じ扱い)。
func wafClientRegion(scope waftypes.Scope, region string) string {
	if scope == waftypes.ScopeCloudfront {
		return "us-east-1"
	}
	return region
}

// newWAFRule は SDK の Rule を WAFRule へ変換する。
func newWAFRule(r waftypes.Rule) WAFRule {
	return WAFRule{
		Name:      ptrStr(r.Name),
		Priority:  r.Priority,
		Action:    wafRuleAction(r),
		Statement: wafRuleStatement(r.Statement),
	}
}

// wafRuleAction は Rule のアクションを表示用文字列へ要約する。
// Action は非 nil フィールド名、ルールグループ参照の OverrideAction は
// "Override: None" / "Override: Count" とする。SDK 更新で増えたフィールドや
// どちらも nil の場合は空文字に落ちる。
func wafRuleAction(r waftypes.Rule) string {
	if a := r.Action; a != nil {
		switch {
		case a.Allow != nil:
			return "Allow"
		case a.Block != nil:
			return "Block"
		case a.Count != nil:
			return "Count"
		case a.Captcha != nil:
			return "Captcha"
		case a.Challenge != nil:
			return "Challenge"
		case a.Monetize != nil:
			return "Monetize"
		}
		return ""
	}
	if o := r.OverrideAction; o != nil {
		switch {
		case o.None != nil:
			return "Override: None"
		case o.Count != nil:
			return "Override: Count"
		}
	}
	return ""
}

// wafRuleStatement は Statement の非 nil フィールドから種別名を導出する。
// ManagedRuleGroupStatement は VendorName/Name、RuleGroupReferenceStatement は
// ARN の末尾セグメント、And/Or/Not は入れ子を展開せず AND/OR/NOT とする。
// どのフィールドも非 nil でない場合は Unknown を返す (SDK 更新で種別が増えた場合の既定値)。
func wafRuleStatement(st *waftypes.Statement) string {
	if st == nil {
		return "Unknown"
	}
	switch {
	case st.ManagedRuleGroupStatement != nil:
		return ptrStr(st.ManagedRuleGroupStatement.VendorName) + "/" + ptrStr(st.ManagedRuleGroupStatement.Name)
	case st.RuleGroupReferenceStatement != nil:
		return arnLastSegment(ptrStr(st.RuleGroupReferenceStatement.ARN))
	case st.AndStatement != nil:
		return "AND"
	case st.OrStatement != nil:
		return "OR"
	case st.NotStatement != nil:
		return "NOT"
	case st.AsnMatchStatement != nil:
		return "AsnMatch"
	case st.ByteMatchStatement != nil:
		return "ByteMatch"
	case st.GeoMatchStatement != nil:
		return "GeoMatch"
	case st.IPSetReferenceStatement != nil:
		return "IPSetReference"
	case st.LabelMatchStatement != nil:
		return "LabelMatch"
	case st.RateBasedStatement != nil:
		return "RateBased"
	case st.RegexMatchStatement != nil:
		return "RegexMatch"
	case st.RegexPatternSetReferenceStatement != nil:
		return "RegexPatternSetReference"
	case st.SizeConstraintStatement != nil:
		return "SizeConstraint"
	case st.SqliMatchStatement != nil:
		return "SqliMatch"
	case st.XssMatchStatement != nil:
		return "XssMatch"
	}
	return "Unknown"
}

// arnLastSegment は ARN の末尾セグメント (最後の "/" 以降) を返す。
// "/" を含まない場合は入力をそのまま返す。
func arnLastSegment(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

// sortWAFRules は priority 昇順に整列する。GetWebACL のレスポンス配列の順序は
// SDK に文書化されていないため明示的に整列する (Priority は Web ACL 内で一意)。
func sortWAFRules(rules []WAFRule) {
	slices.SortFunc(rules, func(a, b WAFRule) int {
		return int(a.Priority) - int(b.Priority)
	})
}

// newWAFClient は WAFv2 API クライアントを生成する。
func newWAFClient(ctx context.Context, profile, region string) (*wafv2.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *wafv2.Client {
		return wafv2.NewFromConfig(cfg)
	})
}
