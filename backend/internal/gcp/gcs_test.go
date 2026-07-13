package gcp

import (
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
)

func TestBucketFromAttrs(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	tests := []struct {
		name string
		in   *storage.BucketAttrs
		want BucketInfo
	}{
		{
			name: "populated",
			in: &storage.BucketAttrs{
				Name:         "my-bucket",
				Location:     "ASIA-NORTHEAST1",
				StorageClass: "STANDARD",
				Created:      created,
				Updated:      updated,
			},
			want: BucketInfo{
				Name:         "my-bucket",
				Location:     "ASIA-NORTHEAST1",
				StorageClass: "STANDARD",
				CreateTime:   created.Format(time.RFC3339),
				UpdateTime:   updated.Format(time.RFC3339),
			},
		},
		{
			name: "zero_times",
			in:   &storage.BucketAttrs{Name: "empty"},
			want: BucketInfo{Name: "empty"},
		},
		{
			name: "nil",
			in:   nil,
			want: BucketInfo{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bucketFromAttrs(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestObjectFromAttrs(t *testing.T) {
	updated := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	tests := []struct {
		name string
		in   *storage.ObjectAttrs
		want ObjectInfo
	}{
		{
			name: "populated",
			in: &storage.ObjectAttrs{
				Name:         "path/to/file.txt",
				Bucket:       "my-bucket",
				Size:         1024,
				ContentType:  "text/plain",
				StorageClass: "STANDARD",
				Updated:      updated,
			},
			want: ObjectInfo{
				Name:         "path/to/file.txt",
				Bucket:       "my-bucket",
				Size:         1024,
				ContentType:  "text/plain",
				StorageClass: "STANDARD",
				Updated:      updated.Format(time.RFC3339),
			},
		},
		{
			name: "nil",
			in:   nil,
			want: ObjectInfo{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := objectFromAttrs(tt.in)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
