package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/tidb"
)

// tidbCostService は TiDB Cloud のコストを 1 か月分だけ一覧にする Service。TUI には
// 期間の入力欄が無いため、月は起動時の設定か当月で固定する。
type tidbCostService struct {
	list  func(ctx context.Context, startMonth, endMonth string) ([]tidb.Cost, error)
	month string
}

// NewTiDBCostService は TiDB の Cost の Service を返す。list には
// (*tidb.Client).GetCostRange を渡す。month が空なら当月を使う。
func NewTiDBCostService(
	list func(ctx context.Context, startMonth, endMonth string) ([]tidb.Cost, error),
	month string,
) Service {
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	return &tidbCostService{list: list, month: month}
}

func (s *tidbCostService) ID() string    { return "tidb-cost" }
func (s *tidbCostService) Title() string { return "Cost" }

// Columns は Web UI の TiDB のコスト表と同じ内訳を並べる。
func (s *tidbCostService) Columns() []Column {
	return []Column{
		{Title: "Billed date"},
		{Title: "Project"},
		{Title: "Cluster"},
		{Title: "Service"},
		{Title: "Credits", Align: AlignRight},
		{Title: "Discounts", Align: AlignRight},
		{Title: "Total", Align: AlignRight},
	}
}

func (s *tidbCostService) Load(ctx context.Context, _, _ string) ([]Row, error) {
	items, err := s.list(ctx, s.month, s.month)
	if err != nil {
		return nil, fmt.Errorf("list TiDB costs: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for i, it := range items {
		rows = append(rows, tidbCostRow{res: it, index: i})
	}
	return rows, nil
}

func (s *tidbCostService) SupportsSession() bool { return false }

// Tabs は Web UI と同じく Overview だけを返す。
func (s *tidbCostService) Tabs() []TabSpec {
	return []TabSpec{{ID: tabOverview, Title: "Overview"}}
}

// tidbCostRow は TiDB Cloud の請求 1 行。同じ組み合わせの行が複数あるため、ID には
// 一覧内の連番も含める。
type tidbCostRow struct {
	res   tidb.Cost
	index int
}

func (r tidbCostRow) ID() string {
	return fmt.Sprintf("%s/%s/%s/%s#%d", r.res.BilledDate, r.res.ProjectName, r.res.ClusterName, r.res.ServicePathName, r.index)
}

func (r tidbCostRow) Name() string  { return r.res.ClusterName }
func (r tidbCostRow) State() string { return "" }

func (r tidbCostRow) Cells() []string {
	return []string{
		orDash(r.res.BilledDate),
		orDash(r.res.ProjectName),
		orDash(r.res.ClusterName),
		orDash(r.res.ServicePathName),
		formatCost(r.res.Credits),
		formatCost(r.res.Discounts),
		formatCost(r.res.TotalCost),
	}
}

func (r tidbCostRow) Overview() []KV {
	return []KV{
		{Key: "Billed date", Value: orDash(r.res.BilledDate)},
		{Key: "Project", Value: orDash(r.res.ProjectName)},
		{Key: "Cluster", Value: orDash(r.res.ClusterName)},
		{Key: "Service", Value: orDash(r.res.ServicePathName)},
		{Key: "Credits", Value: formatCost(r.res.Credits)},
		{Key: "Discounts", Value: formatCost(r.res.Discounts)},
		{Key: "Running total", Value: formatCost(r.res.RunningTotal)},
		{Key: "Total", Value: formatCost(r.res.TotalCost)},
	}
}

func (r tidbCostRow) Tags() map[string]string { return nil }

var (
	_ Service     = (*tidbCostService)(nil)
	_ TabProvider = (*tidbCostService)(nil)
)
