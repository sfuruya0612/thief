package aws

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ectypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
)

func TestElastiCacheFromClusterParameterGroup(t *testing.T) {
	tests := []struct {
		name string
		in   *ectypes.CacheParameterGroupStatus
		want string
	}{
		{
			name: "populated",
			in:   &ectypes.CacheParameterGroupStatus{CacheParameterGroupName: aws.String("default.redis7")},
			want: "default.redis7",
		},
		{
			name: "nil group",
			in:   nil,
			want: "",
		},
		{
			name: "nil name",
			in:   &ectypes.CacheParameterGroupStatus{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ectypes.CacheCluster{
				CacheClusterId:      aws.String("cc-1"),
				CacheParameterGroup: tt.in,
			}
			got := elastiCacheFromCluster(in).ParameterGroup
			if got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCacheParameterFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   ectypes.Parameter
		want ElastiCacheParameter
	}{
		{
			name: "populated",
			in: ectypes.Parameter{
				ParameterName:        aws.String("maxmemory-policy"),
				ParameterValue:       aws.String("noeviction"),
				AllowedValues:        aws.String("volatile-lru,allkeys-lru,noeviction"),
				ChangeType:           ectypes.ChangeTypeImmediate,
				DataType:             aws.String("string"),
				Source:               aws.String("system"),
				IsModifiable:         aws.Bool(true),
				MinimumEngineVersion: aws.String("2.8.6"),
				Description:          aws.String("max memory eviction policy"),
			},
			want: ElastiCacheParameter{
				Name:                 "maxmemory-policy",
				Value:                "noeviction",
				AllowedValues:        "volatile-lru,allkeys-lru,noeviction",
				ChangeType:           "immediate",
				DataType:             "string",
				Source:               "system",
				IsModifiable:         true,
				MinimumEngineVersion: "2.8.6",
				Description:          "max memory eviction policy",
			},
		},
		{
			name: "nil pointers and zero change type become empty",
			in:   ectypes.Parameter{ParameterName: aws.String("p")},
			want: ElastiCacheParameter{Name: "p"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cacheParameterFromSDK(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
