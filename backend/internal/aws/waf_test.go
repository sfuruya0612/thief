package aws

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
)

func TestNewWAFResource(t *testing.T) {
	tests := []struct {
		name            string
		id              string
		aclName         string
		scope           waftypes.Scope
		ruleCount       int
		associatedCount int
		tags            map[string]string
		description     *string
		want            WAFResource
	}{
		{
			name:            "regional で description が nil なら空文字",
			id:              "acl-1",
			aclName:         "edge-acl",
			scope:           waftypes.ScopeRegional,
			ruleCount:       3,
			associatedCount: 1,
			tags:            map[string]string{"Env": "prod"},
			description:     nil,
			want: WAFResource{
				ID:              "acl-1",
				Name:            "edge-acl",
				State:           "active",
				Scope:           "REGIONAL",
				Description:     "",
				RuleCount:       3,
				AssociatedCount: 1,
				Tags:            map[string]string{"Env": "prod"},
			},
		},
		{
			name:            "cloudfront で description が空文字",
			id:              "acl-2",
			aclName:         "cf-acl",
			scope:           waftypes.ScopeCloudfront,
			ruleCount:       0,
			associatedCount: 0,
			tags:            map[string]string{},
			description:     aws.String(""),
			want: WAFResource{
				ID:              "acl-2",
				Name:            "cf-acl",
				State:           "active",
				Scope:           "CLOUDFRONT",
				Description:     "",
				RuleCount:       0,
				AssociatedCount: 0,
				Tags:            map[string]string{},
			},
		},
		{
			// id / name / description に互いに異なる値を与え、引数順の取り違えを検出する
			name:            "description 設定あり",
			id:              "acl-3",
			aclName:         "api-acl",
			scope:           waftypes.ScopeRegional,
			ruleCount:       5,
			associatedCount: 2,
			tags:            map[string]string{"Team": "platform"},
			description:     aws.String("Protects the public API"),
			want: WAFResource{
				ID:              "acl-3",
				Name:            "api-acl",
				State:           "active",
				Scope:           "REGIONAL",
				Description:     "Protects the public API",
				RuleCount:       5,
				AssociatedCount: 2,
				Tags:            map[string]string{"Team": "platform"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newWAFResource(tt.id, tt.aclName, tt.scope, tt.ruleCount, tt.associatedCount, tt.tags, tt.description)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestWAFClientRegion(t *testing.T) {
	tests := []struct {
		name   string
		scope  waftypes.Scope
		region string
		want   string
	}{
		{name: "REGIONAL は指定リージョン", scope: waftypes.ScopeRegional, region: "ap-northeast-1", want: "ap-northeast-1"},
		{name: "CLOUDFRONT は us-east-1", scope: waftypes.ScopeCloudfront, region: "ap-northeast-1", want: "us-east-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wafClientRegion(tt.scope, tt.region); got != tt.want {
				t.Errorf("wafClientRegion(%s, %s) = %q, want %q", tt.scope, tt.region, got, tt.want)
			}
		})
	}
}

func TestNewWAFRule(t *testing.T) {
	tests := []struct {
		name string
		rule waftypes.Rule
		want WAFRule
	}{
		{
			name: "Allow アクションと ByteMatch",
			rule: waftypes.Rule{
				Name:      aws.String("allow-rule"),
				Priority:  1,
				Action:    &waftypes.RuleAction{Allow: &waftypes.AllowAction{}},
				Statement: &waftypes.Statement{ByteMatchStatement: &waftypes.ByteMatchStatement{}},
			},
			want: WAFRule{Name: "allow-rule", Priority: 1, Action: "Allow", Statement: "ByteMatch"},
		},
		{
			name: "Block アクション",
			rule: waftypes.Rule{
				Name:      aws.String("block-rule"),
				Priority:  2,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{GeoMatchStatement: &waftypes.GeoMatchStatement{}},
			},
			want: WAFRule{Name: "block-rule", Priority: 2, Action: "Block", Statement: "GeoMatch"},
		},
		{
			name: "Count アクション",
			rule: waftypes.Rule{
				Name:      aws.String("count-rule"),
				Priority:  3,
				Action:    &waftypes.RuleAction{Count: &waftypes.CountAction{}},
				Statement: &waftypes.Statement{IPSetReferenceStatement: &waftypes.IPSetReferenceStatement{}},
			},
			want: WAFRule{Name: "count-rule", Priority: 3, Action: "Count", Statement: "IPSetReference"},
		},
		{
			name: "Captcha アクション",
			rule: waftypes.Rule{
				Name:      aws.String("captcha-rule"),
				Priority:  4,
				Action:    &waftypes.RuleAction{Captcha: &waftypes.CaptchaAction{}},
				Statement: &waftypes.Statement{LabelMatchStatement: &waftypes.LabelMatchStatement{}},
			},
			want: WAFRule{Name: "captcha-rule", Priority: 4, Action: "Captcha", Statement: "LabelMatch"},
		},
		{
			name: "Challenge アクション",
			rule: waftypes.Rule{
				Name:      aws.String("challenge-rule"),
				Priority:  5,
				Action:    &waftypes.RuleAction{Challenge: &waftypes.ChallengeAction{}},
				Statement: &waftypes.Statement{RateBasedStatement: &waftypes.RateBasedStatement{}},
			},
			want: WAFRule{Name: "challenge-rule", Priority: 5, Action: "Challenge", Statement: "RateBased"},
		},
		{
			name: "Monetize アクション",
			rule: waftypes.Rule{
				Name:      aws.String("monetize-rule"),
				Priority:  6,
				Action:    &waftypes.RuleAction{Monetize: &waftypes.MonetizeAction{}},
				Statement: &waftypes.Statement{RegexMatchStatement: &waftypes.RegexMatchStatement{}},
			},
			want: WAFRule{Name: "monetize-rule", Priority: 6, Action: "Monetize", Statement: "RegexMatch"},
		},
		{
			name: "マネージドルールグループ参照は Override: None と Vendor/Name",
			rule: waftypes.Rule{
				Name:     aws.String("managed-rule"),
				Priority: 7,
				OverrideAction: &waftypes.OverrideAction{
					None: &waftypes.NoneAction{},
				},
				Statement: &waftypes.Statement{
					ManagedRuleGroupStatement: &waftypes.ManagedRuleGroupStatement{
						VendorName: aws.String("AWS"),
						Name:       aws.String("AWSManagedRulesCommonRuleSet"),
					},
				},
			},
			want: WAFRule{
				Name:      "managed-rule",
				Priority:  7,
				Action:    "Override: None",
				Statement: "AWS/AWSManagedRulesCommonRuleSet",
			},
		},
		{
			name: "非マネージドルールグループ参照は Override: Count と ARN 末尾セグメント",
			rule: waftypes.Rule{
				Name:     aws.String("group-rule"),
				Priority: 8,
				OverrideAction: &waftypes.OverrideAction{
					Count: &waftypes.CountAction{},
				},
				Statement: &waftypes.Statement{
					RuleGroupReferenceStatement: &waftypes.RuleGroupReferenceStatement{
						ARN: aws.String("arn:aws:wafv2:ap-northeast-1:123456789012:regional/rulegroup/my-group/abc-123"),
					},
				},
			},
			want: WAFRule{Name: "group-rule", Priority: 8, Action: "Override: Count", Statement: "abc-123"},
		},
		{
			name: "AND 論理は入れ子を展開しない",
			rule: waftypes.Rule{
				Name:      aws.String("and-rule"),
				Priority:  9,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{AndStatement: &waftypes.AndStatement{}},
			},
			want: WAFRule{Name: "and-rule", Priority: 9, Action: "Block", Statement: "AND"},
		},
		{
			name: "OR 論理",
			rule: waftypes.Rule{
				Name:      aws.String("or-rule"),
				Priority:  10,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{OrStatement: &waftypes.OrStatement{}},
			},
			want: WAFRule{Name: "or-rule", Priority: 10, Action: "Block", Statement: "OR"},
		},
		{
			name: "NOT 論理",
			rule: waftypes.Rule{
				Name:      aws.String("not-rule"),
				Priority:  11,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{NotStatement: &waftypes.NotStatement{}},
			},
			want: WAFRule{Name: "not-rule", Priority: 11, Action: "Block", Statement: "NOT"},
		},
		{
			name: "AsnMatch",
			rule: waftypes.Rule{
				Name:      aws.String("asn-rule"),
				Priority:  12,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{AsnMatchStatement: &waftypes.AsnMatchStatement{}},
			},
			want: WAFRule{Name: "asn-rule", Priority: 12, Action: "Block", Statement: "AsnMatch"},
		},
		{
			name: "RegexPatternSetReference",
			rule: waftypes.Rule{
				Name:      aws.String("regex-set-rule"),
				Priority:  13,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{RegexPatternSetReferenceStatement: &waftypes.RegexPatternSetReferenceStatement{}},
			},
			want: WAFRule{Name: "regex-set-rule", Priority: 13, Action: "Block", Statement: "RegexPatternSetReference"},
		},
		{
			name: "SizeConstraint",
			rule: waftypes.Rule{
				Name:      aws.String("size-rule"),
				Priority:  14,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{SizeConstraintStatement: &waftypes.SizeConstraintStatement{}},
			},
			want: WAFRule{Name: "size-rule", Priority: 14, Action: "Block", Statement: "SizeConstraint"},
		},
		{
			name: "SqliMatch",
			rule: waftypes.Rule{
				Name:      aws.String("sqli-rule"),
				Priority:  15,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{SqliMatchStatement: &waftypes.SqliMatchStatement{}},
			},
			want: WAFRule{Name: "sqli-rule", Priority: 15, Action: "Block", Statement: "SqliMatch"},
		},
		{
			name: "XssMatch",
			rule: waftypes.Rule{
				Name:      aws.String("xss-rule"),
				Priority:  16,
				Action:    &waftypes.RuleAction{Block: &waftypes.BlockAction{}},
				Statement: &waftypes.Statement{XssMatchStatement: &waftypes.XssMatchStatement{}},
			},
			want: WAFRule{Name: "xss-rule", Priority: 16, Action: "Block", Statement: "XssMatch"},
		},
		{
			name: "どの Statement フィールドも非 nil でなければ Unknown、Action の中身が全て nil なら空文字",
			rule: waftypes.Rule{
				Name:      aws.String("unknown-rule"),
				Priority:  17,
				Action:    &waftypes.RuleAction{},
				Statement: &waftypes.Statement{},
			},
			want: WAFRule{Name: "unknown-rule", Priority: 17, Action: "", Statement: "Unknown"},
		},
		{
			name: "Statement 自体が nil でも Unknown",
			rule: waftypes.Rule{
				Name:     aws.String("nil-statement-rule"),
				Priority: 18,
			},
			want: WAFRule{Name: "nil-statement-rule", Priority: 18, Action: "", Statement: "Unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newWAFRule(tt.rule)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestSortWAFRules(t *testing.T) {
	rules := []WAFRule{
		{Name: "third", Priority: 30},
		{Name: "first", Priority: 1},
		{Name: "second", Priority: 10},
	}
	sortWAFRules(rules)
	want := []WAFRule{
		{Name: "first", Priority: 1},
		{Name: "second", Priority: 10},
		{Name: "third", Priority: 30},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("got %#v want %#v", rules, want)
	}
}

func TestWAFResourceJSONHasDescription(t *testing.T) {
	b, err := json.Marshal(WAFResource{
		ID:          "acl-1",
		Name:        "edge-acl",
		Description: "Protects the public API",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"description":"Protects the public API"`) {
		t.Errorf("json = %s, want description key with value", b)
	}
}
