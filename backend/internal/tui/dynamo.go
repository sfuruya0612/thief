package tui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// dynamoService は DynamoDB テーブルの一覧を提供する Service。
type dynamoService struct {
	list func(ctx context.Context, profile, region string) ([]aws.DynamoResource, error)
}

// NewDynamoService は DynamoDB の Service を返す。list には aws.ListDynamoResources を渡す。
func NewDynamoService(list func(ctx context.Context, profile, region string) ([]aws.DynamoResource, error)) Service {
	return &dynamoService{list: list}
}

func (s *dynamoService) ID() string    { return "dynamo" }
func (s *dynamoService) Title() string { return "DynamoDB" }

// Columns は Web UI の dynamoColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Items は "," 区切り (formatCount)、Size は formatBytes、GSI は 10 進にし、Web UI に dash を描画するセルは無い。
func (s *dynamoService) Columns() []Column {
	return []Column{
		{Title: "Table"},
		{Title: "State"},
		{Title: "Mode"},
		{Title: "Items"},
		{Title: "Size"},
		{Title: "GSI"},
		{Title: "Region"},
	}
}

func (s *dynamoService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list DynamoDB tables: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, dynamoRow{res: it, region: region})
	}
	return rows, nil
}

func (s *dynamoService) SupportsSession() bool { return false }

// dynamoRow は DynamoDB テーブル 1 つの行。ID はテーブルの ARN。
type dynamoRow struct {
	res    aws.DynamoResource
	region string
}

func (r dynamoRow) ID() string   { return r.res.ID }
func (r dynamoRow) Name() string { return r.res.Name }

// State は DynamoResource.State の値 (aws.DisplayState で小文字にした active など) を返す。ResourceState() の aws.NormalizeState は
// 使わず、Web UI の StatusBadge と同じ値にする。
func (r dynamoRow) State() string { return r.res.State }

func (r dynamoRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Mode,
		formatCount(r.res.ItemCount),
		formatBytes(r.res.SizeBytes),
		strconv.Itoa(r.res.GSICount),
		r.region,
	}
}

// Overview は Web UI の dynamoOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r dynamoRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Capacity mode", Value: r.res.Mode},
		{Key: "Item count", Value: formatCount(r.res.ItemCount)},
		{Key: "Size", Value: formatBytes(r.res.SizeBytes)},
		{Key: "Global secondary indexes", Value: strconv.Itoa(r.res.GSICount)},
		{Key: "Region", Value: r.region},
	}
}

func (r dynamoRow) Tags() map[string]string { return r.res.Tags }

// TagsFetchFailed は aws.ListDynamoResources がタグの取得 (ListTagsOfResource) に失敗した行で真になる。
// Drawer の Tags タブは Web UI の DrawerTags と同じく "Tags (fetch failed)" の見出しを出す。
func (r dynamoRow) TagsFetchFailed() bool { return r.res.TagsFetchFailed }
