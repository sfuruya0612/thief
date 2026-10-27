package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// s3Service は S3 バケットの一覧を提供する Service。
type s3Service struct {
	list        func(ctx context.Context, profile, region string) ([]aws.S3Resource, error)
	listObjects func(ctx context.Context, profile, region, bucket, prefix, delimiter string) ([]aws.S3ObjectResource, []string, bool, error)
}

// NewS3Service は S3 の Service を返す。list には aws.ListS3Resources を渡す。
// aws.ListS3Resources はリージョンを無視して全バケットを返すが、他のサービスと同じく選択中のリージョンを渡す。
// listObjects を省略すると Objects タブを表示しない (既存のテスト用)。
func NewS3Service(list func(ctx context.Context, profile, region string) ([]aws.S3Resource, error), listObjects ...func(ctx context.Context, profile, region, bucket, prefix, delimiter string) ([]aws.S3ObjectResource, []string, bool, error)) Service {
	s := &s3Service{list: list}
	if len(listObjects) > 0 {
		s.listObjects = listObjects[0]
	}
	return s
}

func (s *s3Service) ID() string    { return "s3" }
func (s *s3Service) Title() string { return "S3" }

// Columns は Web UI の s3Columns (frontend/src/components/tables/columns.tsx) と同じ列。
// Public は真なら "public"、偽なら "private"。Created と Encryption は空なら空文字列のままにする。
func (s *s3Service) Columns() []Column {
	return []Column{
		{Title: "Bucket"},
		{Title: "State"},
		{Title: "Region"},
		{Title: "Created"},
		{Title: "Public"},
		{Title: "Encryption"},
	}
}

func (s *s3Service) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list S3 buckets: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, s3Row{res: it, region: region})
	}
	return rows, nil
}

func (s *s3Service) SupportsSession() bool { return false }

// Tabs は Overview、Objects、Tags を返す。
func (s *s3Service) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.listObjects != nil {
		tabs = append(tabs, TabSpec{
			ID: "objects", Title: "Objects",
			New: func(ctx context.Context, profile, region string, row Row) Tab {
				bucket := row.(s3Row)
				return newS3ObjectsTab(ctx, profile, region, bucket.Name(), bucket.bucketRegion(), s.listObjects)
			},
		})
	}
	tabs = append(tabs, TabSpec{ID: tabTags, Title: "Tags"})
	return tabs
}

// s3Row は S3 バケット 1 つの行。ID はバケット名。
type s3Row struct {
	res    aws.S3Resource
	region string
}

func (r s3Row) ID() string   { return r.res.ID }
func (r s3Row) Name() string { return r.res.Name }

// State は S3Resource.State の生の値を返す。aws.ListS3Resources は State を設定しないため空で、
// Web UI の StatusBadge も空の値を表示する (ResourceState() の固定の active は Web UI が表示しない)。
func (r s3Row) State() string { return r.res.State }

// bucketRegion はバケット固有のリージョン (S3Resource.Region) を返し、空なら選択中のリージョンにする。
// Web UI の s3FromRaw (frontend/src/lib/normalize.ts) と同じ優先順位。
func (r s3Row) bucketRegion() string {
	if r.res.Region != "" {
		return r.res.Region
	}
	return r.region
}

func (r s3Row) Cells() []string {
	public := "private"
	if r.res.Public {
		public = "public"
	}
	return []string{
		r.res.Name,
		r.State(),
		r.bucketRegion(),
		r.res.CreatedAt,
		public,
		r.res.Encryption,
	}
}

// Overview は Web UI の s3OverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Public access は allowed か blocked、Created と Encryption は空なら dash にする。
func (r s3Row) Overview() []KV {
	public := "blocked"
	if r.res.Public {
		public = "allowed"
	}
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Region", Value: r.bucketRegion()},
		{Key: "Created", Value: orDash(r.res.CreatedAt)},
		{Key: "Public access", Value: public},
		{Key: "Encryption", Value: orDash(r.res.Encryption)},
	}
}

// Tags は nil を返す。S3Resource にタグは無く、Web UI の S3 の Tags タブも空である。
// TUI の Drawer は Tags タブを "No tags" と表示する。
func (r s3Row) Tags() map[string]string { return nil }
