package aws

import (
	"reflect"
	"testing"

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
		want            WAFResource
	}{
		{
			name:            "regional",
			id:              "acl-1",
			aclName:         "edge-acl",
			scope:           waftypes.ScopeRegional,
			ruleCount:       3,
			associatedCount: 1,
			tags:            map[string]string{"Env": "prod"},
			want: WAFResource{
				ID:              "acl-1",
				Name:            "edge-acl",
				State:           "active",
				Scope:           "REGIONAL",
				RuleCount:       3,
				AssociatedCount: 1,
				Tags:            map[string]string{"Env": "prod"},
			},
		},
		{
			name:            "cloudfront",
			id:              "acl-2",
			aclName:         "cf-acl",
			scope:           waftypes.ScopeCloudfront,
			ruleCount:       0,
			associatedCount: 0,
			tags:            map[string]string{},
			want: WAFResource{
				ID:              "acl-2",
				Name:            "cf-acl",
				State:           "active",
				Scope:           "CLOUDFRONT",
				RuleCount:       0,
				AssociatedCount: 0,
				Tags:            map[string]string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newWAFResource(tt.id, tt.aclName, tt.scope, tt.ruleCount, tt.associatedCount, tt.tags)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
