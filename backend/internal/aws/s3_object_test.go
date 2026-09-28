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

func makeS3CommonPrefixes(n int) []s3types.CommonPrefix {
	cps := make([]s3types.CommonPrefix, n)
	for i := range cps {
		cps[i] = s3types.CommonPrefix{Prefix: aws.String(fmt.Sprintf("folder-%d/", i))}
	}
	return cps
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

// TestAppendS3ListEntriesUpToLimit はオブジェクトとフォルダを合わせた打ち切りの境界を
// 検証する。打ち切るのは蓄積件数が上限に達した後にまだ追加するエントリがあるときだけで、
// ちょうど上限で終わるときは truncated にしない。
func TestAppendS3ListEntriesUpToLimit(t *testing.T) {
	tests := []struct {
		name            string
		existingObjects int
		existingFolders int
		objs            int
		folders         int
		max             int
		wantObjects     int
		wantFolders     int
		wantTruncated   bool
	}{
		{name: "under limit", objs: 10, max: 1000, wantObjects: 10},
		{name: "objects exactly at limit", objs: 1000, max: 1000, wantObjects: 1000},
		{name: "objects exceed limit", objs: 1001, max: 1000, wantObjects: 1000, wantTruncated: true},
		{name: "folders exactly at limit", folders: 1000, max: 1000, wantFolders: 1000},
		{
			name:          "folders at limit plus one object",
			folders:       1000,
			objs:          1,
			max:           1000,
			wantFolders:   1000,
			wantTruncated: true,
		},
		{
			name:        "objects and folders exactly at limit",
			objs:        600,
			folders:     400,
			max:         1000,
			wantObjects: 600,
			wantFolders: 400,
		},
		{
			name:          "objects and folders exceed limit",
			objs:          600,
			folders:       401,
			max:           1000,
			wantObjects:   599,
			wantFolders:   401,
			wantTruncated: true,
		},
		{
			name:            "already at limit before page",
			existingObjects: 1000,
			objs:            5,
			max:             1000,
			wantObjects:     1000,
			wantTruncated:   true,
		},
		{
			name:            "limit reached mid page",
			existingObjects: 998,
			objs:            10,
			max:             1000,
			wantObjects:     1000,
			wantTruncated:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]S3ObjectResource, tt.existingObjects)
			folders := make([]string, tt.existingFolders)
			gotObjects, gotFolders, truncated := appendS3ListEntriesUpToLimit(
				objects, folders, makeS3Objects(tt.objs), makeS3CommonPrefixes(tt.folders), tt.max,
			)
			if len(gotObjects) != tt.wantObjects {
				t.Errorf("len(objects) = %d, want %d", len(gotObjects), tt.wantObjects)
			}
			if len(gotFolders) != tt.wantFolders {
				t.Errorf("len(prefixes) = %d, want %d", len(gotFolders), tt.wantFolders)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
		})
	}
}
