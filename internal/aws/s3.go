package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client defines the interface for S3 API operations.
type S3Client interface {
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
}

// S3BucketInfo holds display fields for an S3 bucket.
type S3BucketInfo struct {
	Name         string
	CreationDate string
}

// ToRow converts S3BucketInfo to a string slice suitable for table formatting.
func (b S3BucketInfo) ToRow() []string {
	return []string{b.Name, b.CreationDate}
}

// NewS3Client creates a new S3 client using the specified AWS profile and region.
func NewS3Client(profile, region string) (S3Client, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}

// GenerateListBucketsInput creates input for the ListBuckets operation.
func GenerateListBucketsInput() *s3.ListBucketsInput {
	return &s3.ListBucketsInput{}
}

// ListBuckets calls the S3 ListBuckets API and returns the results
// as a typed slice of S3BucketInfo.
func ListBuckets(client S3Client, input *s3.ListBucketsInput) ([]S3BucketInfo, error) {
	resp, err := client.ListBuckets(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}

	var buckets []S3BucketInfo
	for _, bucket := range resp.Buckets {
		buckets = append(buckets, S3BucketInfo{
			Name:         aws.ToString(bucket.Name),
			CreationDate: bucket.CreationDate.Format("2006-01-02 15:04:05"),
		})
	}

	return buckets, nil
}
