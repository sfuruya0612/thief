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
