package aws

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
)

// mockWAFACLDetailClient は wafACLDetailClient の手書きモック。
type mockWAFACLDetailClient struct {
	listResourcesForWebACL func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error)
	listTagsForResource    func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error)
}

func (m *mockWAFACLDetailClient) ListResourcesForWebACL(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
	return m.listResourcesForWebACL(ctx, params, optFns...)
}

func (m *mockWAFACLDetailClient) ListTagsForResource(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
	return m.listTagsForResource(ctx, params, optFns...)
}

// mockCloudFrontDistributionLister は cloudFrontDistributionLister の手書きモック。
type mockCloudFrontDistributionLister struct {
	listDistributions func(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error)
}

func (m *mockCloudFrontDistributionLister) ListDistributions(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
	return m.listDistributions(ctx, params, optFns...)
}

func TestNewWAFResource(t *testing.T) {
	tests := []struct {
		name                       string
		id                         string
		aclName                    string
		arn                        string
		scope                      waftypes.Scope
		ruleCount                  int
		associatedCount            int
		associatedCountFetchFailed bool
		tags                       map[string]string
		tagsFetchFailed            bool
		description                *string
		want                       WAFResource
	}{
		{
			name:            "regional で description が nil なら空文字",
			id:              "acl-1",
			aclName:         "edge-acl",
			arn:             "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/edge-acl/acl-1",
			scope:           waftypes.ScopeRegional,
			ruleCount:       3,
			associatedCount: 1,
			tags:            map[string]string{"Env": "prod"},
			description:     nil,
			want: WAFResource{
				ID:              "acl-1",
				Name:            "edge-acl",
				ARN:             "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/edge-acl/acl-1",
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
			arn:             "arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-2",
			scope:           waftypes.ScopeCloudfront,
			ruleCount:       0,
			associatedCount: 0,
			tags:            map[string]string{},
			description:     aws.String(""),
			want: WAFResource{
				ID:              "acl-2",
				Name:            "cf-acl",
				ARN:             "arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-2",
				State:           "active",
				Scope:           "CLOUDFRONT",
				Description:     "",
				RuleCount:       0,
				AssociatedCount: 0,
				Tags:            map[string]string{},
			},
		},
		{
			// id / name / arn / description に互いに異なる値を与え、引数順の取り違えを検出する
			name:            "description 設定あり",
			id:              "acl-3",
			aclName:         "api-acl",
			arn:             "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/api-acl/acl-3",
			scope:           waftypes.ScopeRegional,
			ruleCount:       5,
			associatedCount: 2,
			tags:            map[string]string{"Team": "platform"},
			description:     aws.String("Protects the public API"),
			want: WAFResource{
				ID:              "acl-3",
				Name:            "api-acl",
				ARN:             "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/api-acl/acl-3",
				State:           "active",
				Scope:           "REGIONAL",
				Description:     "Protects the public API",
				RuleCount:       5,
				AssociatedCount: 2,
				Tags:            map[string]string{"Team": "platform"},
			},
		},
		{
			name:                       "associated count と tags の取得失敗を反映する",
			id:                         "acl-4",
			aclName:                    "degraded-acl",
			arn:                        "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/degraded-acl/acl-4",
			scope:                      waftypes.ScopeRegional,
			ruleCount:                  1,
			associatedCount:            0,
			associatedCountFetchFailed: true,
			tags:                       map[string]string{},
			tagsFetchFailed:            true,
			description:                nil,
			want: WAFResource{
				ID:                         "acl-4",
				Name:                       "degraded-acl",
				ARN:                        "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/degraded-acl/acl-4",
				State:                      "active",
				Scope:                      "REGIONAL",
				Description:                "",
				RuleCount:                  1,
				AssociatedCount:            0,
				AssociatedCountFetchFailed: true,
				Tags:                       map[string]string{},
				TagsFetchFailed:            true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newWAFResource(tt.id, tt.aclName, tt.arn, tt.scope, tt.ruleCount, tt.associatedCount, tt.associatedCountFetchFailed, tt.tags, tt.tagsFetchFailed, tt.description)
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
			// RuleJSON はフィールド追加時に既存ケースの期待値を巨大化させないため、
			// 個別に wafRuleJSON の出力と一致するかだけを検証する (配線の検証)。
			if got.Name != tt.want.Name || got.Priority != tt.want.Priority ||
				got.Action != tt.want.Action || got.Statement != tt.want.Statement {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
			if want := wafRuleJSON(tt.rule); got.RuleJSON != want {
				t.Errorf("RuleJSON = %q want %q", got.RuleJSON, want)
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

func TestWAFResourceJSONHasNoARNKey(t *testing.T) {
	b, err := json.Marshal(WAFResource{
		ID:   "acl-1",
		Name: "edge-acl",
		ARN:  "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/edge-acl/acl-1",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"arn":`) || strings.Contains(string(b), `"ARN":`) {
		t.Errorf("json = %s, want no arn/ARN key", b)
	}
}

func TestWAFResourceJSONFetchFailedOmitempty(t *testing.T) {
	tests := []struct {
		name                       string
		associatedCountFetchFailed bool
		tagsFetchFailed            bool
		wantAssociatedKey          bool
		wantTagsKey                bool
	}{
		{
			name:                       "両方 false ならキーが省略される",
			associatedCountFetchFailed: false,
			tagsFetchFailed:            false,
			wantAssociatedKey:          false,
			wantTagsKey:                false,
		},
		{
			name:                       "両方 true ならキーが true で出力される",
			associatedCountFetchFailed: true,
			tagsFetchFailed:            true,
			wantAssociatedKey:          true,
			wantTagsKey:                true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(WAFResource{
				ID:                         "acl-1",
				Name:                       "edge-acl",
				AssociatedCountFetchFailed: tt.associatedCountFetchFailed,
				TagsFetchFailed:            tt.tagsFetchFailed,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			gotAssociatedKey := strings.Contains(string(b), `"associated_count_fetch_failed":true`)
			if gotAssociatedKey != tt.wantAssociatedKey {
				t.Errorf("json = %s, associated_count_fetch_failed key present = %v, want %v", b, gotAssociatedKey, tt.wantAssociatedKey)
			}
			gotTagsKey := strings.Contains(string(b), `"tags_fetch_failed":true`)
			if gotTagsKey != tt.wantTagsKey {
				t.Errorf("json = %s, tags_fetch_failed key present = %v, want %v", b, gotTagsKey, tt.wantTagsKey)
			}
		})
	}
}

func TestWAFRegionalResourceTypes(t *testing.T) {
	want := map[waftypes.ResourceType]bool{
		waftypes.ResourceTypeApplicationLoadBalancer: true,
		waftypes.ResourceTypeApiGateway:              true,
		waftypes.ResourceTypeAppsync:                 true,
		waftypes.ResourceTypeCognitioUserPool:        true,
		waftypes.ResourceTypeAppRunnerService:        true,
		waftypes.ResourceTypeVerifiedAccessInstance:  true,
		waftypes.ResourceTypeAmplify:                 true,
		waftypes.ResourceTypeAgentcoreGateway:        true,
	}
	got := wafRegionalResourceTypes()
	if len(got) != len(want) {
		t.Fatalf("wafRegionalResourceTypes() = %v, want set %v (length mismatch)", got, want)
	}
	for _, rt := range got {
		if !want[rt] {
			t.Errorf("wafRegionalResourceTypes() contains unexpected %q", rt)
		}
	}
}

func TestSumResourceARNs(t *testing.T) {
	tests := []struct {
		name     string
		arnLists [][]string
		want     int
	}{
		{
			name:     "全種別の件数が合算される",
			arnLists: [][]string{{"arn:1"}, {"arn:2", "arn:3"}, {"arn:4"}},
			want:     4,
		},
		{
			name:     "一部の種別が 0 件でも残りが合算される",
			arnLists: [][]string{{"arn:1"}, {}, {"arn:2"}},
			want:     2,
		},
		{
			name:     "一部の種別が nil (取得失敗) でも残りが合算される",
			arnLists: [][]string{{"arn:1"}, nil, {"arn:2", "arn:3"}},
			want:     3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sumResourceARNs(tt.arnLists); got != tt.want {
				t.Errorf("sumResourceARNs(%v) = %d, want %d", tt.arnLists, got, tt.want)
			}
		})
	}
}

func TestWAFACLDetail(t *testing.T) {
	summary := waftypes.WebACLSummary{
		Id:   aws.String("acl-1"),
		Name: aws.String("edge-acl"),
		ARN:  aws.String("arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/edge-acl/acl-1"),
	}

	t.Run("REGIONAL で関連リソースとタグの取得が両方成功する", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return &wafv2.ListResourcesForWebACLOutput{ResourceArns: []string{"arn:resource-1"}}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return &wafv2.ListTagsForResourceOutput{
					TagInfoForResource: &waftypes.TagInfoForResource{
						TagList: []waftypes.Tag{{Key: aws.String("Env"), Value: aws.String("prod")}},
					},
				}, nil
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 2)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		wantCount := len(wafRegionalResourceTypes())
		if got.AssociatedCount != wantCount {
			t.Errorf("AssociatedCount = %d, want %d", got.AssociatedCount, wantCount)
		}
		if got.AssociatedCountFetchFailed {
			t.Error("AssociatedCountFetchFailed = true, want false")
		}
		if got.TagsFetchFailed {
			t.Error("TagsFetchFailed = true, want false")
		}
		if want := (map[string]string{"Env": "prod"}); !reflect.DeepEqual(got.Tags, want) {
			t.Errorf("Tags = %#v, want %#v", got.Tags, want)
		}
	})

	t.Run("REGIONAL で一部の resource type だけ取得に失敗しても残りが合算され失敗フラグが立つ", func(t *testing.T) {
		failingType := wafRegionalResourceTypes()[0]
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				if params.ResourceType == failingType {
					return nil, errors.New("throttled")
				}
				return &wafv2.ListResourcesForWebACLOutput{ResourceArns: []string{"arn:resource-1"}}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return &wafv2.ListTagsForResourceOutput{TagInfoForResource: &waftypes.TagInfoForResource{}}, nil
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 2)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		wantCount := len(wafRegionalResourceTypes()) - 1
		if got.AssociatedCount != wantCount {
			t.Errorf("AssociatedCount = %d, want %d", got.AssociatedCount, wantCount)
		}
		if !got.AssociatedCountFetchFailed {
			t.Error("AssociatedCountFetchFailed = false, want true")
		}
	})

	t.Run("タグ取得の失敗で TagsFetchFailed が立ちタグは空のまま", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return &wafv2.ListResourcesForWebACLOutput{}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return nil, errors.New("throttled")
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 2)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		if !got.TagsFetchFailed {
			t.Error("TagsFetchFailed = false, want true")
		}
		if len(got.Tags) != 0 {
			t.Errorf("Tags = %#v, want empty", got.Tags)
		}
	})

	t.Run("resource type 取得がエラーなしで nil 応答を返すと失敗フラグが立つ", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return nil, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return &wafv2.ListTagsForResourceOutput{TagInfoForResource: &waftypes.TagInfoForResource{}}, nil
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 2)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		if got.AssociatedCount != 0 {
			t.Errorf("AssociatedCount = %d, want 0", got.AssociatedCount)
		}
		if !got.AssociatedCountFetchFailed {
			t.Error("AssociatedCountFetchFailed = false, want true")
		}
	})

	t.Run("タグ取得がエラーなしで nil 応答を返すと TagsFetchFailed が立つ", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return &wafv2.ListResourcesForWebACLOutput{}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return nil, nil
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 2)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		if !got.TagsFetchFailed {
			t.Error("TagsFetchFailed = false, want true")
		}
		if len(got.Tags) != 0 {
			t.Errorf("Tags = %#v, want empty", got.Tags)
		}
	})

	t.Run("タグ取得がエラーなしで TagInfoForResource が nil の応答を返すと TagsFetchFailed が立つ", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return &wafv2.ListResourcesForWebACLOutput{}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return &wafv2.ListTagsForResourceOutput{TagInfoForResource: nil}, nil
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 2)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		if !got.TagsFetchFailed {
			t.Error("TagsFetchFailed = false, want true")
		}
		if len(got.Tags) != 0 {
			t.Errorf("Tags = %#v, want empty", got.Tags)
		}
	})

	t.Run("CLOUDFRONT スコープでは ListResourcesForWebACL を呼ばず AssociatedCount は 0 のまま", func(t *testing.T) {
		called := false
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				called = true
				return &wafv2.ListResourcesForWebACLOutput{}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return &wafv2.ListTagsForResourceOutput{TagInfoForResource: &waftypes.TagInfoForResource{}}, nil
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeCloudfront, summary, 1)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		if called {
			t.Error("ListResourcesForWebACL was called for CLOUDFRONT scope, want not called")
		}
		if got.AssociatedCount != 0 || got.AssociatedCountFetchFailed {
			t.Errorf("AssociatedCount = %d, AssociatedCountFetchFailed = %v, want 0, false", got.AssociatedCount, got.AssociatedCountFetchFailed)
		}
	})

	t.Run("CLOUDFRONT スコープでもタグ取得の失敗で TagsFetchFailed が立つ", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return &wafv2.ListResourcesForWebACLOutput{}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return nil, errors.New("throttled")
			},
		}
		got, err := wafACLDetail(context.Background(), client, waftypes.ScopeCloudfront, summary, 1)
		if err != nil {
			t.Fatalf("wafACLDetail() error = %v", err)
		}
		if !got.TagsFetchFailed {
			t.Error("TagsFetchFailed = false, want true")
		}
		if len(got.Tags) != 0 {
			t.Errorf("Tags = %#v, want empty", got.Tags)
		}
	})

	t.Run("resource type 取得のキャンセルはエラーとして伝播する", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return nil, context.Canceled
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return &wafv2.ListTagsForResourceOutput{TagInfoForResource: &waftypes.TagInfoForResource{}}, nil
			},
		}
		_, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 1)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("wafACLDetail() error = %v, want context.Canceled", err)
		}
	})

	t.Run("タグ取得のキャンセルはエラーとして伝播する", func(t *testing.T) {
		client := &mockWAFACLDetailClient{
			listResourcesForWebACL: func(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error) {
				return &wafv2.ListResourcesForWebACLOutput{}, nil
			},
			listTagsForResource: func(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error) {
				return nil, context.Canceled
			},
		}
		_, err := wafACLDetail(context.Background(), client, waftypes.ScopeRegional, summary, 1)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("wafACLDetail() error = %v, want context.Canceled", err)
		}
	})
}

func TestCloudFrontACLCounts(t *testing.T) {
	tests := []struct {
		name      string
		summaries []cftypes.DistributionSummary
		want      map[string]int
	}{
		{
			name: "複数ディストリビューションが同じ ACL ARN を指す場合の合算",
			summaries: []cftypes.DistributionSummary{
				{WebACLId: aws.String("arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-1")},
				{WebACLId: aws.String("arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-1")},
			},
			want: map[string]int{"arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-1": 2},
		},
		{
			name: "WebACLId が nil と空文字のディストリビューションの除外",
			summaries: []cftypes.DistributionSummary{
				{WebACLId: nil},
				{WebACLId: aws.String("")},
				{WebACLId: aws.String("arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-2")},
			},
			want: map[string]int{"arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-2": 1},
		},
		{
			name:      "ディストリビューションが 0 件の場合の空マップ",
			summaries: nil,
			want:      map[string]int{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloudFrontACLCounts(tt.summaries)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cloudFrontACLCounts() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestApplyCloudFrontCounts(t *testing.T) {
	tests := []struct {
		name   string
		acls   []WAFResource
		counts map[string]int
		want   []int
	}{
		{
			name: "ARN が一致する ACL に件数が入る",
			acls: []WAFResource{
				{ID: "acl-1", ARN: "arn:1"},
				{ID: "acl-2", ARN: "arn:2"},
			},
			counts: map[string]int{"arn:1": 3, "arn:2": 1},
			want:   []int{3, 1},
		},
		{
			name: "マップに無い ARN の ACL は 0 のまま",
			acls: []WAFResource{
				{ID: "acl-1", ARN: "arn:1"},
			},
			counts: map[string]int{"arn:2": 5},
			want:   []int{0},
		},
		{
			name: "マップのキーが WAF Classic の GUID のみの場合にどの ACL も 0 のまま",
			acls: []WAFResource{
				{ID: "acl-1", ARN: "arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-1"},
			},
			counts: map[string]int{"3a1b2c3d-4e5f-6789-abcd-ef0123456789": 2},
			want:   []int{0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyCloudFrontCounts(tt.acls, tt.counts)
			got := make([]int, len(tt.acls))
			for i, acl := range tt.acls {
				got[i] = acl.AssociatedCount
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("applyCloudFrontCounts() associated counts = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyCloudFrontAssociatedCountsWithClient(t *testing.T) {
	acl := WAFResource{ID: "acl-1", ARN: "arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-1"}

	t.Run("ディストリビューション一覧の取得に成功すると関連件数が反映される", func(t *testing.T) {
		client := &mockCloudFrontDistributionLister{
			listDistributions: func(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
				return &cloudfront.ListDistributionsOutput{
					DistributionList: &cftypes.DistributionList{
						Items:       []cftypes.DistributionSummary{{WebACLId: aws.String(acl.ARN)}},
						IsTruncated: aws.Bool(false),
					},
				}, nil
			},
		}
		acls := []WAFResource{acl}
		degraded, err := applyCloudFrontAssociatedCountsWithClient(context.Background(), client, "default", acls)
		if err != nil {
			t.Fatalf("applyCloudFrontAssociatedCountsWithClient() error = %v", err)
		}
		if degraded {
			t.Error("degraded = true, want false")
		}
		if acls[0].AssociatedCount != 1 {
			t.Errorf("AssociatedCount = %d, want 1", acls[0].AssociatedCount)
		}
	})

	t.Run("ディストリビューション一覧の取得に失敗すると縮退し ACL は変更されない", func(t *testing.T) {
		client := &mockCloudFrontDistributionLister{
			listDistributions: func(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
				return nil, errors.New("throttled")
			},
		}
		acls := []WAFResource{acl}
		degraded, err := applyCloudFrontAssociatedCountsWithClient(context.Background(), client, "default", acls)
		if err != nil {
			t.Fatalf("applyCloudFrontAssociatedCountsWithClient() error = %v", err)
		}
		if !degraded {
			t.Error("degraded = false, want true")
		}
		if acls[0].AssociatedCount != 0 {
			t.Errorf("AssociatedCount = %d, want 0 (未変更)", acls[0].AssociatedCount)
		}
	})

	t.Run("ディストリビューション一覧取得のキャンセルはエラーとして伝播する", func(t *testing.T) {
		client := &mockCloudFrontDistributionLister{
			listDistributions: func(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
				return nil, context.Canceled
			},
		}
		acls := []WAFResource{acl}
		_, err := applyCloudFrontAssociatedCountsWithClient(context.Background(), client, "default", acls)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("applyCloudFrontAssociatedCountsWithClient() error = %v, want context.Canceled", err)
		}
	})

	t.Run("縮退時に markCloudFrontAssociatedCountFetchFailed を適用すると AssociatedCountFetchFailed が true になる", func(t *testing.T) {
		client := &mockCloudFrontDistributionLister{
			listDistributions: func(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
				return nil, errors.New("throttled")
			},
		}
		acls := []WAFResource{acl}
		degraded, err := applyCloudFrontAssociatedCountsWithClient(context.Background(), client, "default", acls)
		if err != nil {
			t.Fatalf("applyCloudFrontAssociatedCountsWithClient() error = %v", err)
		}
		if !degraded {
			t.Fatal("degraded = false, want true")
		}
		// waf.go の ListWAFResources が行う合成 (if degraded { markCloudFrontAssociatedCountFetchFailed(acls) }) を
		// ここで手動で再現して検証する。ListWAFResources 自体は AWS クライアント生成を要求するため直接は呼べず、
		// この合成がそちらでも壊れていないかまでは検知できない点に注意。
		markCloudFrontAssociatedCountFetchFailed(acls)
		if !acls[0].AssociatedCountFetchFailed {
			t.Error("AssociatedCountFetchFailed = false, want true")
		}
	})
}

func TestMarkCloudFrontAssociatedCountFetchFailed(t *testing.T) {
	acls := []WAFResource{
		{ID: "acl-1", AssociatedCountFetchFailed: false},
		{ID: "acl-2", AssociatedCountFetchFailed: false},
	}
	markCloudFrontAssociatedCountFetchFailed(acls)
	for _, acl := range acls {
		if !acl.AssociatedCountFetchFailed {
			t.Errorf("acl %s: AssociatedCountFetchFailed = false, want true", acl.ID)
		}
	}
}

// TestApplyCloudFrontAssociatedCounts はクライアント生成に失敗した場合の縮退を検証する。
// 存在しないプロファイル名を渡すと config.LoadDefaultConfig がネットワークアクセスなしに
// 即座に失敗するため、newCloudFrontClient のエラーパスをモック無しで再現できる。
func TestApplyCloudFrontAssociatedCounts(t *testing.T) {
	acls := []WAFResource{{ID: "acl-1", ARN: "arn:aws:wafv2:us-east-1:123456789012:global/webacl/cf-acl/acl-1"}}
	degraded, err := applyCloudFrontAssociatedCounts(context.Background(), "definitely-not-a-real-profile-xyz", acls)
	if err != nil {
		t.Fatalf("applyCloudFrontAssociatedCounts() error = %v", err)
	}
	if !degraded {
		t.Error("degraded = false, want true")
	}
	if acls[0].AssociatedCount != 0 {
		t.Errorf("AssociatedCount = %d, want 0 (未変更)", acls[0].AssociatedCount)
	}
	// waf.go の ListWAFResources が行う合成 (if degraded { markCloudFrontAssociatedCountFetchFailed(acls) }) を
	// ここで手動で再現して検証する。ListWAFResources 自体は AWS クライアント生成を要求するため直接は呼べず、
	// この合成がそちらでも壊れていないかまでは検知できない点に注意。
	markCloudFrontAssociatedCountFetchFailed(acls)
	if !acls[0].AssociatedCountFetchFailed {
		t.Error("AssociatedCountFetchFailed = false, want true")
	}
}

func TestWAFRuleJSONHasRuleJSONKey(t *testing.T) {
	b, err := json.Marshal(WAFRule{Name: "rate-limit", RuleJSON: `{"Name":"rate-limit"}`})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"rule_json":"{\"Name\":\"rate-limit\"}"`) {
		t.Errorf("json = %s, want rule_json key with value", b)
	}
}

func TestWAFRuleJSON(t *testing.T) {
	tests := []struct {
		name           string
		rule           waftypes.Rule
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "Statement と Action を持つルールで null フィールドが出力に現れない",
			rule: waftypes.Rule{
				Name:     aws.String("allow-rule"),
				Priority: 1,
				Action:   &waftypes.RuleAction{Allow: &waftypes.AllowAction{}},
				Statement: &waftypes.Statement{
					ByteMatchStatement: &waftypes.ByteMatchStatement{
						FieldToMatch: &waftypes.FieldToMatch{UriPath: &waftypes.UriPath{}},
					},
				},
				VisibilityConfig: &waftypes.VisibilityConfig{
					CloudWatchMetricsEnabled: true,
					MetricName:               aws.String("allow-rule-metric"),
					SampledRequestsEnabled:   true,
				},
			},
			wantNotContain: []string{"null"},
		},
		{
			name: "Action: {Allow: {}} の Allow キーが空オブジェクトとして残る",
			rule: waftypes.Rule{
				Name:   aws.String("allow-rule"),
				Action: &waftypes.RuleAction{Allow: &waftypes.AllowAction{}},
			},
			wantContains: []string{`"Allow":{}`},
		},
		{
			name: "OverrideAction: {None: {}} が残る",
			rule: waftypes.Rule{
				Name:           aws.String("group-rule"),
				OverrideAction: &waftypes.OverrideAction{None: &waftypes.NoneAction{}},
			},
			wantContains: []string{`"None":{}`},
		},
		{
			name: "FieldToMatch: {UriPath: {}} が残る",
			rule: waftypes.Rule{
				Name: aws.String("uri-rule"),
				Statement: &waftypes.Statement{
					ByteMatchStatement: &waftypes.ByteMatchStatement{
						FieldToMatch: &waftypes.FieldToMatch{UriPath: &waftypes.UriPath{}},
					},
				},
			},
			wantContains: []string{`"UriPath":{}`},
		},
		{
			name: "null 除去で空オブジェクトになった配列要素が残る",
			rule: waftypes.Rule{
				Name:       aws.String("label-rule"),
				RuleLabels: []waftypes.Label{{Name: nil}},
			},
			wantContains: []string{`"RuleLabels":[{}]`},
		},
		{
			name: "空配列が保持される",
			rule: waftypes.Rule{
				Name:       aws.String("empty-labels-rule"),
				RuleLabels: []waftypes.Label{},
			},
			wantContains: []string{`"RuleLabels":[]`},
		},
		{
			name: "ByteMatchStatement.SearchString が base64 文字列として出力される",
			rule: waftypes.Rule{
				Name: aws.String("byte-match-rule"),
				Statement: &waftypes.Statement{
					ByteMatchStatement: &waftypes.ByteMatchStatement{
						SearchString: []byte("test"),
					},
				},
			},
			wantContains: []string{`"SearchString":"dGVzdA=="`},
		},
		{
			name: "VisibilityConfig の SampledRequestsEnabled と MetricName のキーが出力に含まれる",
			rule: waftypes.Rule{
				Name: aws.String("visibility-rule"),
				VisibilityConfig: &waftypes.VisibilityConfig{
					CloudWatchMetricsEnabled: true,
					MetricName:               aws.String("visibility-metric"),
					SampledRequestsEnabled:   false,
				},
			},
			wantContains: []string{`"MetricName":"visibility-metric"`, `"SampledRequestsEnabled":false`},
		},
		{
			name: "非ポインタ型のゼロ値が null 除去で消えずに残る (Priority の 0 と SampledRequestsEnabled の false)",
			rule: waftypes.Rule{
				Name:     aws.String("zero-value-rule"),
				Priority: 0,
				VisibilityConfig: &waftypes.VisibilityConfig{
					SampledRequestsEnabled: false,
				},
			},
			wantContains: []string{`"Priority":0`, `"SampledRequestsEnabled":false`},
		},
		{
			name: "RateBasedStatement.Limit に float64 で正確に表現できない値を与えたとき数値がその表記のまま出力される",
			rule: waftypes.Rule{
				Name: aws.String("rate-based-rule"),
				Statement: &waftypes.Statement{
					RateBasedStatement: &waftypes.RateBasedStatement{
						Limit: aws.Int64(9007199254740993),
					},
				},
			},
			wantContains: []string{`"Limit":9007199254740993`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wafRuleJSON(tt.rule)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("wafRuleJSON() = %s, want it to contain %q", got, want)
				}
			}
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(got, notWant) {
					t.Errorf("wafRuleJSON() = %s, want it not to contain %q", got, notWant)
				}
			}
		})
	}
}
