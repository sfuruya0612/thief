package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// kinesisService は Kinesis Data Streams の一覧を提供する Service。
type kinesisService struct {
	list func(ctx context.Context, profile, region string) ([]aws.KinesisResource, error)
}

// NewKinesisService は Kinesis の Service を返す。list には aws.ListKinesisResources を渡す。
func NewKinesisService(list func(ctx context.Context, profile, region string) ([]aws.KinesisResource, error)) Service {
	return &kinesisService{list: list}
}

func (s *kinesisService) ID() string    { return "kinesis" }
func (s *kinesisService) Title() string { return "Kinesis" }

// Columns は Web UI の kinesisColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Web UI に dash を描画するセルは無く、値が無いセルは空文字列にする。Retention は "<n>h"。
func (s *kinesisService) Columns() []Column {
	return []Column{
		{Title: "Stream"},
		{Title: "State"},
		{Title: "Mode"},
		{Title: "Shards"},
		{Title: "Retention"},
		{Title: "Encryption"},
		{Title: "Region"},
	}
}

func (s *kinesisService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list Kinesis streams: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, kinesisRow{res: it, region: region})
	}
	return rows, nil
}

func (s *kinesisService) SupportsSession() bool { return false }

// kinesisRow はストリーム 1 つの行。ID はストリームの ARN。
type kinesisRow struct {
	res    aws.KinesisResource
	region string
}

func (r kinesisRow) ID() string   { return r.res.ID }
func (r kinesisRow) Name() string { return r.res.Name }

// State は KinesisResource.State の生の値 (active など) を返す。ResourceState() は aws.NormalizeState で
// active を running に変えるため、Web UI の StatusBadge と食い違うので使わない。
func (r kinesisRow) State() string { return r.res.State }

// retention は Web UI の列と Overview が共に描画する "<n>h" を返す。
func (r kinesisRow) retention() string { return formatInt32(r.res.RetentionHours) + "h" }

func (r kinesisRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Mode,
		formatInt32(r.res.ShardCount),
		r.retention(),
		r.res.EncryptionType,
		r.region,
	}
}

// Overview は Web UI の kinesisOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r kinesisRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Capacity mode", Value: r.res.Mode},
		{Key: "Shards", Value: formatInt32(r.res.ShardCount)},
		{Key: "Retention", Value: r.retention()},
		{Key: "Encryption", Value: r.res.EncryptionType},
		{Key: "Region", Value: r.region},
	}
}

// Tags は KinesisResource.Tags をそのまま返す。ListKinesisResources は設定しないため常に空で、
// Web UI の Tags タブと同じく No tags になる。
func (r kinesisRow) Tags() map[string]string { return r.res.Tags }
