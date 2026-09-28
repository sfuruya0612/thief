package gcp

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
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

// objectAttrsIterator は it.Next に相当する next 関数をエントリ列から組み立てる。
// 列を使い切ったら iterator.Done を返す。
func objectAttrsIterator(entries []*storage.ObjectAttrs) func() (*storage.ObjectAttrs, error) {
	i := 0
	return func() (*storage.ObjectAttrs, error) {
		if i >= len(entries) {
			return nil, iterator.Done
		}
		attrs := entries[i]
		i++
		return attrs, nil
	}
}

func makeObjectAttrs(n int) []*storage.ObjectAttrs {
	entries := make([]*storage.ObjectAttrs, n)
	for i := range entries {
		entries[i] = &storage.ObjectAttrs{Name: fmt.Sprintf("obj-%d", i)}
	}
	return entries
}

func makeFolderAttrs(n int) []*storage.ObjectAttrs {
	entries := make([]*storage.ObjectAttrs, n)
	for i := range entries {
		entries[i] = &storage.ObjectAttrs{Prefix: fmt.Sprintf("folder-%d/", i)}
	}
	return entries
}

// TestCollectGCSObjectsLimit はオブジェクトとフォルダを合わせた打ち切りの境界を検証する。
// 打ち切るのは蓄積件数が上限に達した後にまだエントリが返るときだけで、ちょうど上限で
// 終わるとき (iterator.Done) は truncated にしない。
func TestCollectGCSObjectsLimit(t *testing.T) {
	tests := []struct {
		name          string
		entries       []*storage.ObjectAttrs
		max           int
		wantObjects   int
		wantPrefixes  int
		wantTruncated bool
	}{
		{
			name:        "objects under limit",
			entries:     makeObjectAttrs(3),
			max:         1000,
			wantObjects: 3,
		},
		{
			name:        "objects exactly at limit",
			entries:     makeObjectAttrs(1000),
			max:         1000,
			wantObjects: 1000,
		},
		{
			name:          "objects over limit",
			entries:       makeObjectAttrs(1001),
			max:           1000,
			wantObjects:   1000,
			wantTruncated: true,
		},
		{
			name:         "folders exactly at limit",
			entries:      makeFolderAttrs(1000),
			max:          1000,
			wantPrefixes: 1000,
		},
		{
			name:          "folders at limit plus one object",
			entries:       append(makeFolderAttrs(1000), makeObjectAttrs(1)...),
			max:           1000,
			wantPrefixes:  1000,
			wantTruncated: true,
		},
		{
			name:         "objects and folders exactly at limit",
			entries:      append(makeObjectAttrs(600), makeFolderAttrs(400)...),
			max:          1000,
			wantObjects:  600,
			wantPrefixes: 400,
		},
		{
			name:          "objects and folders exceed limit",
			entries:       append(makeObjectAttrs(600), makeFolderAttrs(401)...),
			max:           1000,
			wantObjects:   600,
			wantPrefixes:  400,
			wantTruncated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects, prefixes, truncated, err := collectGCSObjects(objectAttrsIterator(tt.entries), tt.max)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if len(objects) != tt.wantObjects {
				t.Errorf("len(objects) = %d, want %d", len(objects), tt.wantObjects)
			}
			if len(prefixes) != tt.wantPrefixes {
				t.Errorf("len(prefixes) = %d, want %d", len(prefixes), tt.wantPrefixes)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
		})
	}
}

// TestCollectGCSObjectsClassifiesFolders は ObjectAttrs.Prefix だけを持つ合成
// ディレクトリエントリが prefixes に入り objects に入らないことを検証する。
func TestCollectGCSObjectsClassifiesFolders(t *testing.T) {
	entries := []*storage.ObjectAttrs{
		{Name: "a.txt", Bucket: "my-bucket", Size: 10},
		{Prefix: "reports/"},
		{Name: "b.txt", Bucket: "my-bucket", Size: 20},
		{Prefix: "notes/"},
	}

	objects, prefixes, truncated, err := collectGCSObjects(objectAttrsIterator(entries), maxGCSListObjects)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if truncated {
		t.Error("truncated = true, want false")
	}
	wantObjects := []ObjectInfo{
		{Name: "a.txt", Bucket: "my-bucket", Size: 10},
		{Name: "b.txt", Bucket: "my-bucket", Size: 20},
	}
	if diff := cmp.Diff(wantObjects, objects); diff != "" {
		t.Errorf("objects mismatch (-want +got):\n%s", diff)
	}
	wantPrefixes := []string{"reports/", "notes/"}
	if diff := cmp.Diff(wantPrefixes, prefixes); diff != "" {
		t.Errorf("prefixes mismatch (-want +got):\n%s", diff)
	}
}

// TestCollectGCSObjectsNilAttrsError は next が Done でもエラーでもないのに nil を返したとき、
// 空の行を積んだり読み飛ばして回り続けたりせず、errNilObjectAttrs で止まることを検証する。
// 上限の位置で返った nil も打ち切りではなくエラーになる。
func TestCollectGCSObjectsNilAttrsError(t *testing.T) {
	tests := []struct {
		name    string
		entries []*storage.ObjectAttrs
	}{
		{
			name: "nil before limit",
			entries: []*storage.ObjectAttrs{
				{Name: "a.txt", Bucket: "my-bucket", Size: 10},
				nil,
				{Prefix: "reports/"},
			},
		},
		{
			name:    "nil at limit",
			entries: append(makeObjectAttrs(maxGCSListObjects), nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects, prefixes, truncated, err := collectGCSObjects(objectAttrsIterator(tt.entries), maxGCSListObjects)
			if !errors.Is(err, errNilObjectAttrs) {
				t.Fatalf("err = %v, want %v", err, errNilObjectAttrs)
			}
			if truncated {
				t.Error("truncated = true, want false")
			}
			if objects != nil {
				t.Errorf("objects = %v, want nil", objects)
			}
			if prefixes != nil {
				t.Errorf("prefixes = %v, want nil", prefixes)
			}
		})
	}
}

// TestCollectGCSObjectsIteratorError は next のエラーがそのまま伝播することを検証する。
func TestCollectGCSObjectsIteratorError(t *testing.T) {
	wantErr := errors.New("iterator boom")
	objects, prefixes, truncated, err := collectGCSObjects(func() (*storage.ObjectAttrs, error) {
		return nil, wantErr
	}, maxGCSListObjects)

	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if objects != nil || prefixes != nil || truncated {
		t.Errorf("objects = %v, prefixes = %v, truncated = %v, want nil, nil, false", objects, prefixes, truncated)
	}
}
