package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
)

var mockElasticacheOutput = &elasticache.DescribeCacheClustersOutput{
	CacheClusters: []types.CacheCluster{
		{
			ReplicationGroupId: aws.String("repl-group-1"),
			CacheClusterId:     aws.String("my-cache-cluster"),
			CacheNodeType:      aws.String("cache.t3.micro"),
			Engine:             aws.String("redis"),
			EngineVersion:      aws.String("6.2"),
			CacheClusterStatus: aws.String("available"),
		},
	},
}

type mockElasticacheApi struct {
	output *elasticache.DescribeCacheClustersOutput
	err    error
}

func (m *mockElasticacheApi) DescribeCacheClusters(ctx context.Context, input *elasticache.DescribeCacheClustersInput, opts ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	return m.output, m.err
}

func TestGenerateDescribeCacheClustersInput(t *testing.T) {
	tests := []struct {
		name string
		opts *ElasticacheOpts
	}{
		{
			name: "default options",
			opts: &ElasticacheOpts{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := GenerateDescribeCacheClustersInput(tt.opts)
			if input == nil {
				t.Fatal("expected non-nil input, got nil")
			}
		})
	}
}

func TestDescribeCacheClusters(t *testing.T) {
	mockApi := &mockElasticacheApi{
		output: mockElasticacheOutput,
		err:    nil,
	}

	input := &elasticache.DescribeCacheClustersInput{}
	result, err := DescribeCacheClusters(mockApi, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[0].ReplicationGroupID != "repl-group-1" {
		t.Errorf("expected ReplicationGroupID 'repl-group-1', got '%s'", result[0].ReplicationGroupID)
	}
	if result[0].CacheClusterID != "my-cache-cluster" {
		t.Errorf("expected CacheClusterID 'my-cache-cluster', got '%s'", result[0].CacheClusterID)
	}
	if result[0].CacheNodeType != "cache.t3.micro" {
		t.Errorf("expected CacheNodeType 'cache.t3.micro', got '%s'", result[0].CacheNodeType)
	}
	if result[0].Engine != "redis" {
		t.Errorf("expected Engine 'redis', got '%s'", result[0].Engine)
	}
	if result[0].EngineVersion != "6.2" {
		t.Errorf("expected EngineVersion '6.2', got '%s'", result[0].EngineVersion)
	}
	if result[0].Status != "available" {
		t.Errorf("expected Status 'available', got '%s'", result[0].Status)
	}
}

func TestDescribeCacheClusters_Error(t *testing.T) {
	mockApi := &mockElasticacheApi{
		output: mockElasticacheOutput,
		err:    errors.New("error"),
	}

	input := &elasticache.DescribeCacheClustersInput{}
	result, err := DescribeCacheClusters(mockApi, input)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}
