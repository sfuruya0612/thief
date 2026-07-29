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

func TestElastiCacheFromClusterReplicationGroupID(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		want string
	}{
		{
			name: "レプリケーショングループに属する Redis / Valkey ノード",
			in:   aws.String("my-redis-rg"),
			want: "my-redis-rg",
		},
		{
			name: "ReplicationGroupId が空の単一ノード Redis / Valkey",
			in:   aws.String(""),
			want: "",
		},
		{
			name: "レプリケーショングループを持たない Memcached クラスター",
			in:   nil,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ectypes.CacheCluster{
				CacheClusterId:     aws.String("cc-1"),
				ReplicationGroupId: tt.in,
			}
			got := elastiCacheFromCluster(in).ReplicationGroupID
			if got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestElastiCacheFromClusterNodeAvailabilityZones(t *testing.T) {
	tests := []struct {
		name string
		in   []ectypes.CacheNode
		want []string
	}{
		{
			name: "複数ノードがそれぞれ異なる AZ に配置されている",
			in: []ectypes.CacheNode{
				{CustomerAvailabilityZone: aws.String("ap-northeast-1a")},
				{CustomerAvailabilityZone: aws.String("ap-northeast-1c")},
				{CustomerAvailabilityZone: aws.String("ap-northeast-1d")},
			},
			want: []string{"ap-northeast-1a", "ap-northeast-1c", "ap-northeast-1d"},
		},
		{
			name: "単一ノード",
			in: []ectypes.CacheNode{
				{CustomerAvailabilityZone: aws.String("ap-northeast-1a")},
			},
			want: []string{"ap-northeast-1a"},
		},
		{
			name: "CustomerAvailabilityZone が nil のノードは除外される",
			in: []ectypes.CacheNode{
				{CustomerAvailabilityZone: aws.String("ap-northeast-1a")},
				{CustomerAvailabilityZone: nil},
				{CustomerAvailabilityZone: aws.String("ap-northeast-1c")},
			},
			want: []string{"ap-northeast-1a", "ap-northeast-1c"},
		},
		{
			// XML の空要素は SDK のデシリアライザで nil ではなく空文字列のポインタになる。
			name: "CustomerAvailabilityZone が空文字列のノードは除外される",
			in: []ectypes.CacheNode{
				{CustomerAvailabilityZone: aws.String("ap-northeast-1a")},
				{CustomerAvailabilityZone: aws.String("")},
				{CustomerAvailabilityZone: aws.String("ap-northeast-1c")},
			},
			want: []string{"ap-northeast-1a", "ap-northeast-1c"},
		},
		{
			name: "全ノードの CustomerAvailabilityZone が nil または空文字列の場合は nil になる",
			in: []ectypes.CacheNode{
				{CustomerAvailabilityZone: aws.String("")},
				{CustomerAvailabilityZone: nil},
			},
			want: nil,
		},
		{
			name: "CacheNodes が空",
			in:   nil,
			want: nil,
		},
		{
			name: "複数ノードが同一 AZ に配置されていても重複は除去されない",
			in: []ectypes.CacheNode{
				{CustomerAvailabilityZone: aws.String("ap-northeast-1a")},
				{CustomerAvailabilityZone: aws.String("ap-northeast-1a")},
			},
			want: []string{"ap-northeast-1a", "ap-northeast-1a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ectypes.CacheCluster{
				CacheClusterId: aws.String("cc-1"),
				CacheNodes:     tt.in,
			}
			got := elastiCacheFromCluster(in).NodeAvailabilityZones
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestElastiCacheClusterInfoToRow(t *testing.T) {
	tests := []struct {
		name string
		in   ElastiCacheClusterInfo
		want []string
	}{
		{
			name: "複数の AZ がカンマ区切りで連結される",
			in: ElastiCacheClusterInfo{
				ReplicationGroupID:    "my-redis-rg",
				CacheClusterID:        "my-redis-rg-001",
				CacheNodeType:         "cache.t4g.micro",
				Engine:                "redis",
				EngineVersion:         "7.1.0",
				Status:                "available",
				NodeAvailabilityZones: []string{"ap-northeast-1a", "ap-northeast-1c"},
			},
			want: []string{
				"my-redis-rg", "my-redis-rg-001", "cache.t4g.micro",
				"redis", "7.1.0", "available",
				"ap-northeast-1a,ap-northeast-1c",
			},
		},
		{
			name: "単一の AZ",
			in: ElastiCacheClusterInfo{
				ReplicationGroupID:    "",
				CacheClusterID:        "my-memcached",
				CacheNodeType:         "cache.t4g.micro",
				Engine:                "memcached",
				EngineVersion:         "1.6.22",
				Status:                "available",
				NodeAvailabilityZones: []string{"ap-northeast-1a"},
			},
			want: []string{
				"", "my-memcached", "cache.t4g.micro",
				"memcached", "1.6.22", "available",
				"ap-northeast-1a",
			},
		},
		{
			name: "AZ が空の場合は空文字列になる",
			in: ElastiCacheClusterInfo{
				CacheClusterID: "cc-1",
				Status:         "creating",
			},
			want: []string{"", "cc-1", "", "", "", "creating", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.ToRow()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
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
