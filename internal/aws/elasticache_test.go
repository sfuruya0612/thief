package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	mock.Mock
}

func (m *mockElasticacheApi) DescribeCacheClusters(ctx context.Context, input *elasticache.DescribeCacheClustersInput, opts ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	args := m.Called(ctx, input, opts)
	return args.Get(0).(*elasticache.DescribeCacheClustersOutput), args.Error(1)
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
			assert.NotNil(t, input)
		})
	}
}

func TestDescribeCacheClusters(t *testing.T) {
	mockApi := new(mockElasticacheApi)
	mockApi.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(mockElasticacheOutput, nil)

	input := &elasticache.DescribeCacheClustersInput{}
	result, err := DescribeCacheClusters(mockApi, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "repl-group-1", result[0][0])
	assert.Equal(t, "my-cache-cluster", result[0][1])
	assert.Equal(t, "cache.t3.micro", result[0][2])
	assert.Equal(t, "redis", result[0][3])
	assert.Equal(t, "6.2", result[0][4])
	assert.Equal(t, "available", result[0][5])

	mockApi.AssertExpectations(t)
}

func TestDescribeCacheClusters_Error(t *testing.T) {
	mockApi := new(mockElasticacheApi)
	mockApi.On("DescribeCacheClusters", mock.Anything, mock.Anything, mock.Anything).Return(mockElasticacheOutput, errors.New("error"))

	input := &elasticache.DescribeCacheClustersInput{}
	result, err := DescribeCacheClusters(mockApi, input)
	assert.Error(t, err)
	assert.Nil(t, result)

	mockApi.AssertExpectations(t)
}
