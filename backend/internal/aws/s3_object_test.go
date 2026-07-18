package aws

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func makeS3Objects(n int) []s3types.Object {
	objs := make([]s3types.Object, n)
	for i := range objs {
		objs[i] = s3types.Object{Key: aws.String(fmt.Sprintf("obj-%d", i))}
	}
	return objs
}

func TestS3ObjectFromSDK(t *testing.T) {
	fixed := time.Date(2026, 7, 8, 12, 34, 56, 0, time.UTC)
	tests := []struct {
		name string
		in   s3types.Object
		want S3ObjectResource
	}{
		{
			name: "standard",
			in: s3types.Object{
				Key:          aws.String("path/to/file.txt"),
				Size:         aws.Int64(1234),
				LastModified: &fixed,
				StorageClass: s3types.ObjectStorageClassStandard,
				ETag:         aws.String(`"abc123"`),
			},
			want: S3ObjectResource{
				Key:          "path/to/file.txt",
				Size:         1234,
				LastModified: "2026-07-08T12:34:56Z",
				StorageClass: "STANDARD",
				ETag:         `"abc123"`,
			},
		},
		{
			name: "nil size and lastmodified",
			in: s3types.Object{
				Key: aws.String("empty"),
			},
			want: S3ObjectResource{
				Key: "empty",
			},
		},
		{
			name: "glacier",
			in: s3types.Object{
				Key:          aws.String("cold.bin"),
				Size:         aws.Int64(0),
				StorageClass: s3types.ObjectStorageClassGlacier,
			},
			want: S3ObjectResource{
				Key:          "cold.bin",
				StorageClass: "GLACIER",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s3ObjectFromSDK(tt.in)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAppendS3ObjectsUpToLimit(t *testing.T) {
	tests := []struct {
		name          string
		existing      int
		objs          int
		max           int
		wantLen       int
		wantTruncated bool
	}{
		{name: "under limit", existing: 0, objs: 10, max: 1000, wantLen: 10, wantTruncated: false},
		{name: "exactly at limit", existing: 0, objs: 1000, max: 1000, wantLen: 1000, wantTruncated: false},
		{name: "exceeds limit", existing: 0, objs: 1001, max: 1000, wantLen: 1000, wantTruncated: true},
		{name: "already at limit before page", existing: 1000, objs: 5, max: 1000, wantLen: 1000, wantTruncated: true},
		{
			name:          "limit reached mid page",
			existing:      998,
			objs:          10,
			max:           1000,
			wantLen:       1000,
			wantTruncated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := make([]S3ObjectResource, tt.existing)
			got, truncated := appendS3ObjectsUpToLimit(resources, makeS3Objects(tt.objs), tt.max)
			if len(got) != tt.wantLen {
				t.Errorf("len(got) = %d, want %d", len(got), tt.wantLen)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
		})
	}
}
