package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Resource represents a single S3 bucket.
type S3Resource struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Region      string  `json:"region"`
	CreatedAt   string  `json:"created_at"`
	Public      bool    `json:"public"`
	Encryption  string  `json:"encryption"`
	CostMonthly float64 `json:"cost_monthly"`
}

func (r S3Resource) ResourceID() string    { return r.ID }
func (r S3Resource) ResourceName() string  { return r.Name }
func (r S3Resource) ResourceState() string { return "active" }
func (r S3Resource) ServiceName() string   { return "s3" }

// ListS3Resources returns all S3 buckets accessible via the given profile.
// ListBuckets is called against us-east-1; per-bucket region is resolved with GetBucketLocation.
func ListS3Resources(ctx context.Context, profile, _ string) ([]S3Resource, error) {
	// S3 ListBuckets is a global operation; us-east-1 is the canonical endpoint.
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *s3.Client {
		return s3.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list s3 buckets: %w", err)
	}

	var resources []S3Resource
	for _, b := range out.Buckets {
		r := s3FromBucket(ctx, client, b)
		resources = append(resources, r)
	}
	return resources, nil
}

func s3FromBucket(ctx context.Context, client *s3.Client, b s3types.Bucket) S3Resource {
	name := ptrStr(b.Name)
	createdAt := ""
	if b.CreationDate != nil {
		createdAt = b.CreationDate.Format(time.RFC3339)
	}

	region := resolveS3Region(ctx, client, name)
	encryption := resolveS3Encryption(ctx, client, name)
	public := resolveS3Public(ctx, client, name)

	return S3Resource{
		ID:         name,
		Name:       name,
		CreatedAt:  createdAt,
		Region:     region,
		Public:     public,
		Encryption: encryption,
	}
}

func resolveS3Region(ctx context.Context, client *s3.Client, bucket string) string {
	out, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "unknown"
	}
	if out.LocationConstraint == "" {
		return "us-east-1"
	}
	return string(out.LocationConstraint)
}

func resolveS3Encryption(ctx context.Context, client *s3.Client, bucket string) string {
	out, err := client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "none"
	}
	if out.ServerSideEncryptionConfiguration != nil && len(out.ServerSideEncryptionConfiguration.Rules) > 0 {
		rule := out.ServerSideEncryptionConfiguration.Rules[0]
		if rule.ApplyServerSideEncryptionByDefault != nil {
			return string(rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
		}
	}
	return "none"
}

func resolveS3Public(ctx context.Context, client *s3.Client, bucket string) bool {
	out, err := client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{Bucket: aws.String(bucket)})
	if err != nil {
		return false
	}
	if out.PublicAccessBlockConfiguration == nil {
		return true
	}
	cfg := out.PublicAccessBlockConfiguration
	// If all block settings are enabled, the bucket is not public.
	return !(ptrBool(cfg.BlockPublicAcls) && ptrBool(cfg.BlockPublicPolicy) &&
		ptrBool(cfg.IgnorePublicAcls) && ptrBool(cfg.RestrictPublicBuckets))
}

// S3BucketInfo はレガシー CLI 互換の S3 バケット表示用フィールドを保持する。
type S3BucketInfo struct {
	Name         string
	CreationDate string
}

// ToRow converts S3BucketInfo to a string slice suitable for table formatting.
func (b S3BucketInfo) ToRow() []string {
	return []string{b.Name, b.CreationDate}
}

// ListS3BucketInfos は S3 バケット一覧を返す。
// ListS3Resources と異なりバケットごとのリージョン・暗号化・公開設定の解決を行わない (軽量)。
func ListS3BucketInfos(ctx context.Context, profile string) ([]S3BucketInfo, error) {
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *s3.Client {
		return s3.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}

	var buckets []S3BucketInfo
	for _, b := range out.Buckets {
		creationDate := ""
		if b.CreationDate != nil {
			creationDate = b.CreationDate.Format("2006-01-02 15:04:05")
		}
		buckets = append(buckets, S3BucketInfo{
			Name:         ptrStr(b.Name),
			CreationDate: creationDate,
		})
	}
	return buckets, nil
}
