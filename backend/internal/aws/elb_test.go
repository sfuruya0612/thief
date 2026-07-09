package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
)

func TestElbFromLB(t *testing.T) {
	tests := []struct {
		name string
		in   elbv2types.LoadBalancer
		want ELBResource
	}{
		{
			name: "active",
			in: elbv2types.LoadBalancer{
				LoadBalancerArn:  aws.String("arn:1"),
				LoadBalancerName: aws.String("web"),
				State:            &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActive},
				Type:             elbv2types.LoadBalancerTypeEnumApplication,
				Scheme:           elbv2types.LoadBalancerSchemeEnumInternetFacing,
				DNSName:          aws.String("web.example.com"),
				VpcId:            aws.String("vpc-1"),
				AvailabilityZones: []elbv2types.AvailabilityZone{
					{ZoneName: aws.String("ap-northeast-1a")},
					{ZoneName: aws.String("ap-northeast-1c")},
				},
			},
			want: ELBResource{
				ID:      "arn:1",
				Name:    "web",
				State:   "active",
				Type:    "application",
				Scheme:  "internet-facing",
				DNSName: "web.example.com",
				VpcID:   "vpc-1",
				AZs:     []string{"ap-northeast-1a", "ap-northeast-1c"},
			},
		},
		{
			name: "active_impaired uses hyphenated display",
			in: elbv2types.LoadBalancer{
				LoadBalancerArn: aws.String("arn:2"),
				State:           &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActiveImpaired},
			},
			want: ELBResource{ID: "arn:2", State: "active-impaired"},
		},
		{
			name: "nil state defaults to unknown",
			in:   elbv2types.LoadBalancer{LoadBalancerArn: aws.String("arn:3")},
			want: ELBResource{ID: "arn:3", State: "unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := elbFromLB(tt.in)
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.State != tt.want.State ||
				got.Type != tt.want.Type || got.Scheme != tt.want.Scheme ||
				got.DNSName != tt.want.DNSName || got.VpcID != tt.want.VpcID ||
				!equalStrs(got.AZs, tt.want.AZs) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestElbListenerFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   elbv2types.Listener
		want ELBListenerResource
	}{
		{
			name: "forward to single target group",
			in: elbv2types.Listener{
				ListenerArn:     aws.String("arn:listener/1"),
				LoadBalancerArn: aws.String("arn:lb/1"),
				Protocol:        elbv2types.ProtocolEnumHttps,
				Port:            aws.Int32(443),
				DefaultActions: []elbv2types.Action{
					{Type: elbv2types.ActionTypeEnumForward, TargetGroupArn: aws.String("arn:tg/1")},
				},
			},
			want: ELBListenerResource{
				ARN:                   "arn:listener/1",
				LoadBalancerArn:       "arn:lb/1",
				Protocol:              "HTTPS",
				Port:                  443,
				DefaultActionType:     "forward",
				DefaultTargetGroupArn: "arn:tg/1",
			},
		},
		{
			name: "no default actions",
			in: elbv2types.Listener{
				ListenerArn: aws.String("arn:listener/2"),
				Protocol:    elbv2types.ProtocolEnumHttp,
				Port:        aws.Int32(80),
			},
			want: ELBListenerResource{ARN: "arn:listener/2", Protocol: "HTTP", Port: 80},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := elbListenerFromSDK(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestElbRuleFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   elbv2types.Rule
		want ELBRuleResource
	}{
		{
			name: "host header condition with forward action",
			in: elbv2types.Rule{
				RuleArn:  aws.String("arn:rule/1"),
				Priority: aws.String("1"),
				Conditions: []elbv2types.RuleCondition{
					{
						Field:            aws.String("host-header"),
						HostHeaderConfig: &elbv2types.HostHeaderConditionConfig{Values: []string{"example.com"}},
					},
				},
				Actions: []elbv2types.Action{
					{Type: elbv2types.ActionTypeEnumForward, TargetGroupArn: aws.String("arn:tg/1")},
				},
			},
			want: ELBRuleResource{
				ARN:            "arn:rule/1",
				Priority:       "1",
				Conditions:     []string{"host-header=example.com"},
				ActionType:     "forward",
				TargetGroupArn: "arn:tg/1",
			},
		},
		{
			name: "default rule with no conditions",
			in: elbv2types.Rule{
				RuleArn:   aws.String("arn:rule/2"),
				Priority:  aws.String("default"),
				IsDefault: aws.Bool(true),
				Actions: []elbv2types.Action{
					{Type: elbv2types.ActionTypeEnumFixedResponse},
				},
			},
			want: ELBRuleResource{
				ARN:        "arn:rule/2",
				Priority:   "default",
				IsDefault:  true,
				Conditions: []string{},
				ActionType: "fixed-response",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := elbRuleFromSDK(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestElbTargetGroupFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   elbv2types.TargetGroup
		want ELBTargetGroupResource
	}{
		{
			name: "http target group",
			in: elbv2types.TargetGroup{
				TargetGroupArn:   aws.String("arn:tg/1"),
				TargetGroupName:  aws.String("web-tg"),
				Protocol:         elbv2types.ProtocolEnumHttp,
				Port:             aws.Int32(8080),
				TargetType:       elbv2types.TargetTypeEnumIp,
				VpcId:            aws.String("vpc-1"),
				HealthCheckPath:  aws.String("/healthz"),
				LoadBalancerArns: []string{"arn:lb/1"},
			},
			want: ELBTargetGroupResource{
				ARN:              "arn:tg/1",
				Name:             "web-tg",
				Protocol:         "HTTP",
				Port:             8080,
				TargetType:       "ip",
				VpcID:            "vpc-1",
				HealthCheckPath:  "/healthz",
				LoadBalancerArns: []string{"arn:lb/1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := elbTargetGroupFromSDK(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestElbTargetHealthFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   elbv2types.TargetHealthDescription
		want ELBTargetHealthResource
	}{
		{
			name: "healthy target",
			in: elbv2types.TargetHealthDescription{
				Target: &elbv2types.TargetDescription{
					Id:               aws.String("10.0.0.1"),
					Port:             aws.Int32(80),
					AvailabilityZone: aws.String("ap-northeast-1a"),
				},
				TargetHealth: &elbv2types.TargetHealth{
					State: elbv2types.TargetHealthStateEnumHealthy,
				},
			},
			want: ELBTargetHealthResource{
				TargetID:         "10.0.0.1",
				Port:             80,
				AvailabilityZone: "ap-northeast-1a",
				State:            "healthy",
			},
		},
		{
			name: "unhealthy target with reason",
			in: elbv2types.TargetHealthDescription{
				Target: &elbv2types.TargetDescription{Id: aws.String("10.0.0.2")},
				TargetHealth: &elbv2types.TargetHealth{
					State:       elbv2types.TargetHealthStateEnumUnhealthy,
					Reason:      elbv2types.TargetHealthReasonEnumFailedHealthChecks,
					Description: aws.String("Health checks failed"),
				},
			},
			want: ELBTargetHealthResource{
				TargetID:    "10.0.0.2",
				State:       "unhealthy",
				Reason:      "Target.FailedHealthChecks",
				Description: "Health checks failed",
			},
		},
		{
			name: "nil target and health",
			in:   elbv2types.TargetHealthDescription{},
			want: ELBTargetHealthResource{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := elbTargetHealthFromSDK(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
