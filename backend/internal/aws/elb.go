package aws

import (
	"context"
	"fmt"
	"strings"

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
	client, err := newELBClient(ctx, profile, region)
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

// ELBListenerResource represents a single listener on a load balancer.
type ELBListenerResource struct {
	ARN             string `json:"arn"`
	LoadBalancerArn string `json:"load_balancer_arn"`
	Protocol        string `json:"protocol"`
	Port            int32  `json:"port"`
	// DefaultActionType は DefaultActions の先頭要素の Type (forward/redirect/fixed-response 等)。
	DefaultActionType string `json:"default_action_type"`
	// DefaultTargetGroupArn は DefaultActions が単一 target group への forward の場合のみ設定される。
	DefaultTargetGroupArn string `json:"default_target_group_arn"`
}

// ListELBListeners returns all listeners attached to the given load balancer.
func ListELBListeners(ctx context.Context, profile, region, lbArn string) ([]ELBListenerResource, error) {
	client, err := newELBClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var resources []ELBListenerResource
	paginator := elbv2.NewDescribeListenersPaginator(client, &elbv2.DescribeListenersInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe listeners for load balancer %s: %w", lbArn, err)
		}
		for _, l := range page.Listeners {
			resources = append(resources, elbListenerFromSDK(l))
		}
	}
	return resources, nil
}

func elbListenerFromSDK(l elbv2types.Listener) ELBListenerResource {
	actionType := ""
	targetGroupArn := ""
	if len(l.DefaultActions) > 0 {
		actionType = string(l.DefaultActions[0].Type)
		targetGroupArn = ptrStr(l.DefaultActions[0].TargetGroupArn)
	}
	return ELBListenerResource{
		ARN:                   ptrStr(l.ListenerArn),
		LoadBalancerArn:       ptrStr(l.LoadBalancerArn),
		Protocol:              string(l.Protocol),
		Port:                  ptrInt32(l.Port),
		DefaultActionType:     actionType,
		DefaultTargetGroupArn: targetGroupArn,
	}
}

// ELBRuleResource represents a single rule on a listener.
type ELBRuleResource struct {
	ARN string `json:"arn"`
	// Priority は "default" (IsDefault) または優先順位を表す数値文字列。
	Priority  string `json:"priority"`
	IsDefault bool   `json:"is_default"`
	// Conditions は "field=values" 形式の人間可読な条件一覧。
	Conditions []string `json:"conditions"`
	// ActionType は Actions の先頭要素の Type (forward/redirect/fixed-response 等)。
	ActionType string `json:"action_type"`
	// TargetGroupArn は forward 先の target group ARN (単一 forward の場合のみ)。
	TargetGroupArn string `json:"target_group_arn"`
}

// ListELBRules returns all rules on the given listener.
func ListELBRules(ctx context.Context, profile, region, listenerArn string) ([]ELBRuleResource, error) {
	client, err := newELBClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	out, err := client.DescribeRules(ctx, &elbv2.DescribeRulesInput{
		ListenerArn: aws.String(listenerArn),
	})
	if err != nil {
		return nil, fmt.Errorf("describe rules for listener %s: %w", listenerArn, err)
	}

	var resources []ELBRuleResource
	for _, rule := range out.Rules {
		resources = append(resources, elbRuleFromSDK(rule))
	}
	return resources, nil
}

func elbRuleFromSDK(rule elbv2types.Rule) ELBRuleResource {
	conditions := make([]string, 0, len(rule.Conditions))
	for _, c := range rule.Conditions {
		conditions = append(conditions, elbRuleConditionString(c))
	}
	actionType := ""
	targetGroupArn := ""
	if len(rule.Actions) > 0 {
		actionType = string(rule.Actions[0].Type)
		targetGroupArn = ptrStr(rule.Actions[0].TargetGroupArn)
	}
	return ELBRuleResource{
		ARN:            ptrStr(rule.RuleArn),
		Priority:       ptrStr(rule.Priority),
		IsDefault:      ptrBool(rule.IsDefault),
		Conditions:     conditions,
		ActionType:     actionType,
		TargetGroupArn: targetGroupArn,
	}
}

// elbRuleConditionString は RuleCondition を "field=value1,value2" 形式の文字列に変換する。
// Field ごとに実際の値を保持するフィールドが異なる (HostHeaderConfig/PathPatternConfig 等) ため、
// 表示用途に限定した最小限の変換のみ行う。
func elbRuleConditionString(c elbv2types.RuleCondition) string {
	field := ptrStr(c.Field)
	var values []string
	switch {
	case c.HostHeaderConfig != nil:
		values = c.HostHeaderConfig.Values
	case c.PathPatternConfig != nil:
		values = c.PathPatternConfig.Values
	case c.HttpRequestMethodConfig != nil:
		values = c.HttpRequestMethodConfig.Values
	case c.SourceIpConfig != nil:
		values = c.SourceIpConfig.Values
	case c.QueryStringConfig != nil:
		for _, kv := range c.QueryStringConfig.Values {
			values = append(values, fmt.Sprintf("%s=%s", ptrStr(kv.Key), ptrStr(kv.Value)))
		}
	case c.HttpHeaderConfig != nil:
		values = c.HttpHeaderConfig.Values
	}
	return fmt.Sprintf("%s=%s", field, strings.Join(values, ","))
}

// ELBTargetGroupResource represents a single target group.
type ELBTargetGroupResource struct {
	ARN              string   `json:"arn"`
	Name             string   `json:"name"`
	Protocol         string   `json:"protocol"`
	Port             int32    `json:"port"`
	TargetType       string   `json:"target_type"`
	VpcID            string   `json:"vpc_id"`
	HealthCheckPath  string   `json:"health_check_path"`
	LoadBalancerArns []string `json:"load_balancer_arns"`
}

// ListELBTargetGroups returns all target groups attached to the given load balancer.
func ListELBTargetGroups(ctx context.Context, profile, region, lbArn string) ([]ELBTargetGroupResource, error) {
	client, err := newELBClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var resources []ELBTargetGroupResource
	paginator := elbv2.NewDescribeTargetGroupsPaginator(client, &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe target groups for load balancer %s: %w", lbArn, err)
		}
		for _, tg := range page.TargetGroups {
			resources = append(resources, elbTargetGroupFromSDK(tg))
		}
	}
	return resources, nil
}

func elbTargetGroupFromSDK(tg elbv2types.TargetGroup) ELBTargetGroupResource {
	return ELBTargetGroupResource{
		ARN:              ptrStr(tg.TargetGroupArn),
		Name:             ptrStr(tg.TargetGroupName),
		Protocol:         string(tg.Protocol),
		Port:             ptrInt32(tg.Port),
		TargetType:       string(tg.TargetType),
		VpcID:            ptrStr(tg.VpcId),
		HealthCheckPath:  ptrStr(tg.HealthCheckPath),
		LoadBalancerArns: tg.LoadBalancerArns,
	}
}

// ELBTargetHealthResource represents the health of a single target within a target group.
type ELBTargetHealthResource struct {
	TargetID         string `json:"target_id"`
	Port             int32  `json:"port"`
	AvailabilityZone string `json:"availability_zone"`
	State            string `json:"state"`
	Reason           string `json:"reason"`
	Description      string `json:"description"`
}

// DescribeELBTargetHealth returns the health of every target registered with the given target group.
func DescribeELBTargetHealth(ctx context.Context, profile, region, tgArn string) ([]ELBTargetHealthResource, error) {
	client, err := newELBClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	out, err := client.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgArn),
	})
	if err != nil {
		return nil, fmt.Errorf("describe target health for target group %s: %w", tgArn, err)
	}

	var resources []ELBTargetHealthResource
	for _, d := range out.TargetHealthDescriptions {
		resources = append(resources, elbTargetHealthFromSDK(d))
	}
	return resources, nil
}

func elbTargetHealthFromSDK(d elbv2types.TargetHealthDescription) ELBTargetHealthResource {
	targetID := ""
	port := int32(0)
	az := ""
	if d.Target != nil {
		targetID = ptrStr(d.Target.Id)
		port = ptrInt32(d.Target.Port)
		az = ptrStr(d.Target.AvailabilityZone)
	}
	state := ""
	reason := ""
	description := ""
	if d.TargetHealth != nil {
		state = DisplayState(string(d.TargetHealth.State))
		reason = string(d.TargetHealth.Reason)
		description = ptrStr(d.TargetHealth.Description)
	}
	return ELBTargetHealthResource{
		TargetID:         targetID,
		Port:             port,
		AvailabilityZone: az,
		State:            state,
		Reason:           reason,
		Description:      description,
	}
}

// newELBClient は ELBv2 API クライアントを生成する。
func newELBClient(ctx context.Context, profile, region string) (*elbv2.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *elbv2.Client {
		return elbv2.NewFromConfig(cfg)
	})
}
