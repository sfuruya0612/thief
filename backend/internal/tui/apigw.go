package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// apigwService は API Gateway (REST / HTTP / WebSocket) の一覧を提供する Service。
type apigwService struct {
	list func(ctx context.Context, profile, region string) ([]aws.APIGatewayResource, error)
}

// NewAPIGatewayService は API Gateway の Service を返す。list には aws.ListAPIGatewayResources を渡す。
func NewAPIGatewayService(list func(ctx context.Context, profile, region string) ([]aws.APIGatewayResource, error)) Service {
	return &apigwService{list: list}
}

func (s *apigwService) ID() string    { return "apigw" }
func (s *apigwService) Title() string { return "API Gateway" }

// Columns は Web UI の apigwColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Web UI に dash を描画するセルは無く、値が無いセルは空文字列にする。
func (s *apigwService) Columns() []Column {
	return []Column{
		{Title: "API"},
		{Title: "State"},
		{Title: "Type"},
		{Title: "Stage"},
		{Title: "Endpoint"},
		{Title: "Region"},
	}
}

func (s *apigwService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list API Gateway APIs: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, apigwRow{res: it, region: region})
	}
	return rows, nil
}

func (s *apigwService) SupportsSession() bool { return false }

// apigwRow は API 1 つの行。ID は API ID。Type (REST / HTTP / WebSocket) と Endpoint は一覧関数が作る。
type apigwRow struct {
	res    aws.APIGatewayResource
	region string
}

func (r apigwRow) ID() string    { return r.res.ID }
func (r apigwRow) Name() string  { return r.res.Name }
func (r apigwRow) State() string { return r.res.State }

func (r apigwRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Type,
		r.res.Stage,
		r.res.Endpoint,
		r.region,
	}
}

// Overview は Web UI の apigwOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r apigwRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Type", Value: r.res.Type},
		{Key: "Stage", Value: r.res.Stage},
		{Key: "Endpoint", Value: r.res.Endpoint},
		{Key: "Region", Value: r.region},
	}
}

func (r apigwRow) Tags() map[string]string { return r.res.Tags }
