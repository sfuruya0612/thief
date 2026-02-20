package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type mockS3Client struct {
	output *s3.ListBucketsOutput
	err    error
}

func (m *mockS3Client) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	return m.output, m.err
}

func TestListBuckets(t *testing.T) {
	creationDate := time.Now()
	mockOutput := &s3.ListBucketsOutput{
		Buckets: []types.Bucket{
			{
				Name:         aws.String("test-bucket1"),
				CreationDate: &creationDate,
			},
			{
				Name:         aws.String("test-bucket2"),
				CreationDate: &creationDate,
			},
		},
	}

	client := &mockS3Client{
		output: mockOutput,
		err:    nil,
	}

	buckets, err := ListBuckets(client, GenerateListBucketsInput())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(buckets) != 2 {
		t.Fatalf("Expected 2 buckets, got %d", len(buckets))
	}

	if buckets[0].Name != "test-bucket1" {
		t.Errorf("Expected bucket name 'test-bucket1', got '%s'", buckets[0].Name)
	}

	if buckets[1].Name != "test-bucket2" {
		t.Errorf("Expected bucket name 'test-bucket2', got '%s'", buckets[1].Name)
	}
}
