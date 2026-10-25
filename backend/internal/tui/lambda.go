package tui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// lambdaService は Lambda 関数の一覧を提供する Service。
type lambdaService struct {
	list func(ctx context.Context, profile, region string) ([]aws.LambdaResource, error)
}

// NewLambdaService は Lambda の Service を返す。list には aws.ListLambdaResources を渡す。
func NewLambdaService(list func(ctx context.Context, profile, region string) ([]aws.LambdaResource, error)) Service {
	return &lambdaService{list: list}
}

func (s *lambdaService) ID() string    { return "lambda" }
func (s *lambdaService) Title() string { return "Lambda" }

// Columns は Web UI の lambdaColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Web UI に dash を描画するセルは無く、値が無いセルは空文字列にする。
func (s *lambdaService) Columns() []Column {
	return []Column{
		{Title: "Function"},
		{Title: "State"},
		{Title: "Runtime"},
		{Title: "Region"},
		{Title: "Memory"},
		{Title: "Timeout"},
	}
}

func (s *lambdaService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list Lambda functions: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, lambdaRow{res: it, region: region})
	}
	return rows, nil
}

func (s *lambdaService) SupportsSession() bool { return false }

// lambdaRow は Lambda 関数 1 つの行。
type lambdaRow struct {
	res    aws.LambdaResource
	region string
}

func (r lambdaRow) ID() string    { return r.res.ID }
func (r lambdaRow) Name() string  { return r.res.Name }
func (r lambdaRow) State() string { return r.res.State }

func (r lambdaRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Runtime,
		r.region,
		formatMemoryMB(r.res.MemoryMB),
		formatTimeoutSec(r.res.TimeoutSec),
	}
}

// Overview は Web UI の lambdaOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Web UI に dash を描画する項目は無く、Runtime、Handler、Role が空なら空文字列にする。
func (r lambdaRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Runtime", Value: r.res.Runtime},
		{Key: "Memory", Value: formatMemoryMB(r.res.MemoryMB)},
		{Key: "Timeout", Value: formatTimeoutSec(r.res.TimeoutSec)},
		{Key: "Handler", Value: r.res.Handler},
		{Key: "Role", Value: r.res.Role},
		{Key: "Region", Value: r.region},
	}
}

// Tags は LambdaResource.Tags をそのまま返す。aws.ListLambdaResources はタグを設定しないため
// 常に空だが、Web UI の Tags タブも同じく空である。
func (r lambdaRow) Tags() map[string]string { return r.res.Tags }

// formatMemoryMB は Web UI の lambdaColumns の Memory のセルと同じ "<n> MB" を返す。
func formatMemoryMB(mb int32) string {
	return strconv.FormatInt(int64(mb), 10) + " MB"
}

// formatTimeoutSec は Web UI の lambdaColumns の Timeout のセルと同じ "<n>s" を返す。
func formatTimeoutSec(sec int32) string {
	return strconv.FormatInt(int64(sec), 10) + "s"
}
