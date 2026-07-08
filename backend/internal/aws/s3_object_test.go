package aws

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

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
