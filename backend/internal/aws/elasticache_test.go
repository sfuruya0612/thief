package aws

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	ectypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
)

// mockElastiCacheDescribeClustersClient は listElastiCacheResources と
// listElastiCacheClusterInfos が要求する elastiCacheDescribeClustersClient を
// テスト用に実装する手書きモック。受け取った Input を呼び出し順に記録し、
// pages に用意したレスポンスを 1 呼び出しにつき 1 ページ返す。
type mockElastiCacheDescribeClustersClient struct {
	pages  []*elasticache.DescribeCacheClustersOutput
	inputs []*elasticache.DescribeCacheClustersInput
}

func (m *mockElastiCacheDescribeClustersClient) DescribeCacheClusters(_ context.Context, params *elasticache.DescribeCacheClustersInput, _ ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	m.inputs = append(m.inputs, params)
	idx := len(m.inputs) - 1
	if idx >= len(m.pages) {
		return nil, fmt.Errorf("unexpected DescribeCacheClusters call %d: only %d pages prepared", idx+1, len(m.pages))
	}
	return m.pages[idx], nil
}

// elastiCacheDescribeClustersPages は Marker で連結された 2 ページ構成のレスポンスを返す。
// ページネータが 2 ページ目の呼び出しでも元のパラメータを維持することを検証するために使う。
func elastiCacheDescribeClustersPages() []*elasticache.DescribeCacheClustersOutput {
	return []*elasticache.DescribeCacheClustersOutput{
		{
			CacheClusters: []ectypes.CacheCluster{{CacheClusterId: aws.String("cc-1")}},
			Marker:        aws.String("page-2"),
		},
		{
			CacheClusters: []ectypes.CacheCluster{{CacheClusterId: aws.String("cc-2")}},
		},
	}
}

// assertShowCacheNodeInfoOnAllCalls は全 2 回の呼び出しの Input で
// ShowCacheNodeInfo が true であることと、1 回目の呼び出しに Marker が無く、
// 2 回目の呼び出しに 1 ページ目の Marker が引き継がれている
// (実際にページ送りが起きた) ことを検証する。
func assertShowCacheNodeInfoOnAllCalls(t *testing.T, inputs []*elasticache.DescribeCacheClustersInput) {
	t.Helper()
	if len(inputs) != 2 {
		t.Fatalf("DescribeCacheClusters called %d times, want 2", len(inputs))
	}
	for i, in := range inputs {
		if in.ShowCacheNodeInfo == nil || !*in.ShowCacheNodeInfo {
			t.Errorf("call %d: ShowCacheNodeInfo = %v, want true", i+1, in.ShowCacheNodeInfo)
		}
	}
	if inputs[0].Marker != nil {
		t.Errorf("call 1: Marker = %q, want nil", aws.ToString(inputs[0].Marker))
	}
	if got := aws.ToString(inputs[1].Marker); got != "page-2" {
		t.Errorf("call 2: Marker = %q, want %q", got, "page-2")
	}
}

func TestListElastiCacheSetsShowCacheNodeInfo(t *testing.T) {
	tests := []struct {
		name string
		// list は検証対象の関数を呼び、返ったクラスタの ID 列を返す。
		list func(ctx context.Context, client elastiCacheDescribeClustersClient) ([]string, error)
	}{
		{
			name: "listElastiCacheResources",
			list: func(ctx context.Context, client elastiCacheDescribeClustersClient) ([]string, error) {
				resources, err := listElastiCacheResources(ctx, client)
				if err != nil {
					return nil, err
				}
				ids := make([]string, 0, len(resources))
				for _, r := range resources {
					ids = append(ids, r.ID)
				}
				return ids, nil
			},
		},
		{
			name: "listElastiCacheClusterInfos",
			list: func(ctx context.Context, client elastiCacheDescribeClustersClient) ([]string, error) {
				infos, err := listElastiCacheClusterInfos(ctx, client)
				if err != nil {
					return nil, err
				}
				ids := make([]string, 0, len(infos))
				for _, c := range infos {
					ids = append(ids, c.CacheClusterID)
				}
				return ids, nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockElastiCacheDescribeClustersClient{pages: elastiCacheDescribeClustersPages()}
			gotIDs, err := tt.list(context.Background(), mock)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertShowCacheNodeInfoOnAllCalls(t, mock.inputs)

			// 両ページのクラスタが集約されることも確認する。
			wantIDs := []string{"cc-1", "cc-2"}
			if !reflect.DeepEqual(gotIDs, wantIDs) {
				t.Errorf("cluster ids = %#v, want %#v", gotIDs, wantIDs)
			}
		})
	}
}

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
