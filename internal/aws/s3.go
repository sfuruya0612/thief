package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client wraps the AWS S3 client for easier testing
type S3Client interface {
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
}

// NewS3Client creates a new S3 client
func NewS3Client(profile, region string) (S3Client, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}

// GenerateListBucketsInput creates input for ListBuckets operation
func GenerateListBucketsInput() *s3.ListBucketsInput {
	return &s3.ListBucketsInput{}
}

// ListBuckets returns a list of S3 buckets
func ListBuckets(client S3Client, input *s3.ListBucketsInput) ([][]string, error) {
	ctx := context.TODO()

	resp, err := client.ListBuckets(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}

	var buckets [][]string
	for _, bucket := range resp.Buckets {
		name := aws.ToString(bucket.Name)
		creationDate := bucket.CreationDate.Format("2006-01-02 15:04:05")

		buckets = append(buckets, []string{
			name,
			creationDate,
		})
	}

	return buckets, nil
}
