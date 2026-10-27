package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

// gcsObjectsLister は GCS のオブジェクト一覧を prefix 絞り込みで取る関数の型。
type gcsObjectsLister func(ctx context.Context, projectID, bucket, prefix, delimiter string) ([]gcp.ObjectInfo, []string, bool, error)

// gcsBucketService は Cloud Storage バケットの一覧を提供する Service。
type gcsBucketService struct {
	list        func(ctx context.Context, projectID string) ([]gcp.BucketInfo, error)
	listObjects gcsObjectsLister
}

// NewGCSBucketService は Cloud Storage の Service を返す。list には gcp.ListBuckets を渡す。
// listObjects を省略すると Objects タブを表示しない (既存のテスト用)。
func NewGCSBucketService(
	list func(ctx context.Context, projectID string) ([]gcp.BucketInfo, error),
	listObjects ...func(ctx context.Context, projectID, bucket, prefix, delimiter string) ([]gcp.ObjectInfo, []string, bool, error),
) Service {
	s := &gcsBucketService{list: list}
	if len(listObjects) > 0 {
		s.listObjects = listObjects[0]
	}
	return s
}

func (s *gcsBucketService) ID() string    { return "gcs" }
func (s *gcsBucketService) Title() string { return "Cloud Storage" }

// Columns は Web UI の gcsBucketColumns と同じ列。
func (s *gcsBucketService) Columns() []Column {
	return []Column{
		{Title: "Bucket"},
		{Title: "Location"},
		{Title: "Storage class"},
		{Title: "Created"},
	}
}

func (s *gcsBucketService) Load(ctx context.Context, projectID, _ string) ([]Row, error) {
	items, err := s.list(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list Cloud Storage buckets: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, gcsBucketRow{res: it})
	}
	return rows, nil
}

func (s *gcsBucketService) SupportsSession() bool { return false }

// Tabs は Overview と、バケット内のオブジェクトを辿る Objects を返す。
func (s *gcsBucketService) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.listObjects != nil {
		tabs = append(tabs, TabSpec{
			ID: "objects", Title: "Objects",
			New: func(ctx context.Context, projectID, _ string, row Row) Tab {
				bucket := row.(gcsBucketRow)
				return newGCSObjectsTab(ctx, projectID, bucket.Name(), s.listObjects)
			},
		})
	}
	return tabs
}

// newGCSObjectsTab は S3 の Objects タブの階層移動、prefix 検索、階層 / フラット表示を
// GCS の一覧取得関数へ接続する。GCS と S3 で取得する項目は同じ (key、size、updated、
// storage class) ため、表示のロジックは s3ObjectsTab を共有する。
func newGCSObjectsTab(
	ctx context.Context,
	projectID, bucket string,
	list gcsObjectsLister,
) Tab {
	tab := newS3ObjectsTab(ctx, projectID, "", bucket, "", func(ctx context.Context, _, _, bucket, prefix, delimiter string) ([]aws.S3ObjectResource, []string, bool, error) {
		objects, prefixes, truncated, err := list(ctx, projectID, bucket, prefix, delimiter)
		if err != nil {
			return nil, nil, false, err
		}
		out := make([]aws.S3ObjectResource, 0, len(objects))
		for _, o := range objects {
			out = append(out, aws.S3ObjectResource{
				Key:          o.Name,
				Size:         o.Size,
				LastModified: o.Updated,
				StorageClass: o.StorageClass,
			})
		}
		return out, prefixes, truncated, nil
	})
	tab.keyTitle = "Object"
	tab.updatedTitle = "Updated"
	tab.SetSize(1, 1)
	return tab
}

// gcsBucketRow は Cloud Storage バケット 1 つの行。ID はバケット名。
type gcsBucketRow struct {
	res gcp.BucketInfo
}

func (r gcsBucketRow) ID() string    { return r.res.Name }
func (r gcsBucketRow) Name() string  { return r.res.Name }
func (r gcsBucketRow) State() string { return "" }

func (r gcsBucketRow) Cells() []string {
	return []string{
		r.res.Name,
		r.res.Location,
		r.res.StorageClass,
		orDash(r.res.CreateTime),
	}
}

// Overview は Web UI の gcsBucketOverviewRows と同じ項目。
func (r gcsBucketRow) Overview() []KV {
	return []KV{
		{Key: "Bucket", Value: r.res.Name},
		{Key: "Location", Value: r.res.Location},
		{Key: "Storage class", Value: r.res.StorageClass},
		{Key: "Created", Value: orDash(r.res.CreateTime)},
	}
}

func (r gcsBucketRow) Tags() map[string]string { return nil }
