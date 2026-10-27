package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

// datadogCostFetcher は Datadog のコスト取得関数の型。新 API の historical と
// estimated で同じ形を使う。
type datadogCostFetcher func(ctx context.Context, api *datadog.UsageMeteringV2API, startMonth, endMonth, view string) ([]datadog.CostInfo, error)

// datadogCostService は Datadog のコストを 1 か月 (または設定された期間) 分だけ一覧にする
// Service。TUI には期間の入力欄が無いため、月は起動時の設定か当月で固定する。
type datadogCostService struct {
	id    string
	title string
	auth  DatadogAuth
	api   *datadog.UsageMeteringV2API
	fetch datadogCostFetcher
	start string
	end   string
	view  string
}

// NewDatadogCostService は Datadog のコストの Service を返す。fetch には
// datadog.GetHistoricalCost か datadog.GetEstimatedCost を渡す。startMonth と endMonth が
// 空なら当月を使う。
func NewDatadogCostService(
	id, title string,
	auth DatadogAuth,
	api *datadog.UsageMeteringV2API,
	fetch func(ctx context.Context, api *datadog.UsageMeteringV2API, startMonth, endMonth, view string) ([]datadog.CostInfo, error),
	startMonth, endMonth, view string,
) Service {
	if startMonth == "" {
		startMonth = time.Now().Format("2006-01")
	}
	if endMonth == "" {
		endMonth = startMonth
	}
	return &datadogCostService{
		id:    id,
		title: title,
		auth:  auth,
		api:   api,
		fetch: fetch,
		start: startMonth,
		end:   endMonth,
		view:  view,
	}
}

func (s *datadogCostService) ID() string    { return s.id }
func (s *datadogCostService) Title() string { return s.title }

// Columns は Datadog のコストの内訳を並べる。Web UI のクロス集計の前の 1 請求行に相当する。
func (s *datadogCostService) Columns() []Column {
	return []Column{
		{Title: "Month"},
		{Title: "Account"},
		{Title: "Org"},
		{Title: "Product"},
		{Title: "Charge type"},
		{Title: "Cost", Align: AlignRight},
	}
}

func (s *datadogCostService) Load(ctx context.Context, org, _ string) ([]Row, error) {
	items, err := datadogCall(ctx, s.auth, org, func(ctx context.Context) ([]datadog.CostInfo, error) {
		return s.fetch(ctx, s.api, s.start, s.end, s.view)
	})
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(items))
	for i, it := range items {
		rows = append(rows, datadogCostRow{res: it, index: i})
	}
	return rows, nil
}

func (s *datadogCostService) SupportsSession() bool { return false }

// Tabs は Web UI と同じく Overview だけを返す。
func (s *datadogCostService) Tabs() []TabSpec {
	return []TabSpec{{ID: tabOverview, Title: "Overview"}}
}

// datadogCostRow は Datadog の請求 1 行。同じ組み合わせの行が複数あるため、ID には
// 一覧内の連番も含める。
type datadogCostRow struct {
	res   datadog.CostInfo
	index int
}

func (r datadogCostRow) ID() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s#%d",
		r.res.Month, r.res.AccountName, r.res.OrgName, r.res.ProductName, r.res.ChargeType, r.index)
}

func (r datadogCostRow) Name() string  { return r.res.ProductName }
func (r datadogCostRow) State() string { return "" }

func (r datadogCostRow) Cells() []string {
	return []string{
		r.res.Month,
		orDash(r.res.AccountName),
		orDash(r.res.OrgName),
		orDash(r.res.ProductName),
		orDash(r.res.ChargeType),
		formatCost(r.res.Cost),
	}
}

func (r datadogCostRow) Overview() []KV {
	return []KV{
		{Key: "Month", Value: r.res.Month},
		{Key: "Account", Value: orDash(r.res.AccountName)},
		{Key: "Org", Value: orDash(r.res.OrgName)},
		{Key: "Product", Value: orDash(r.res.ProductName)},
		{Key: "Charge type", Value: orDash(r.res.ChargeType)},
		{Key: "Cost", Value: formatCost(r.res.Cost)},
	}
}

func (r datadogCostRow) Tags() map[string]string { return nil }

// formatCost は金額を Web UI の Money と同じ規則 (100 超は小数 0 桁、それ以下は 2 桁、
// 整数部は 3 桁ごとの "," 区切り) で返す。
func formatCost(v float64) string {
	digits := 2
	if v > 100 {
		digits = 0
	}
	s := strconv.FormatFloat(v, 'f', digits, 64)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign = "-"
		s = s[1:]
	}
	parts := strings.SplitN(s, ".", 2)
	intPart, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "$" + sign + s
	}
	out := sign + formatCount(intPart)
	if len(parts) == 2 {
		out += "." + parts[1]
	}
	return "$" + out
}

var (
	_ Service     = (*datadogCostService)(nil)
	_ TabProvider = (*datadogCostService)(nil)
)
