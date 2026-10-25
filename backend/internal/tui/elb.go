package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// elbService は ELB (ALB / NLB / GWLB) の一覧を提供する Service。
type elbService struct {
	list func(ctx context.Context, profile, region string) ([]aws.ELBResource, error)
}

// NewELBService は ELB の Service を返す。list には aws.ListELBResources を渡す。
func NewELBService(list func(ctx context.Context, profile, region string) ([]aws.ELBResource, error)) Service {
	return &elbService{list: list}
}

func (s *elbService) ID() string    { return "elb" }
func (s *elbService) Title() string { return "ELB" }

// Columns は Web UI の elbColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// 値が無いセルの表示も Web UI に合わせ、AZs は dash、DNS name は空文字列にする。
func (s *elbService) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "State"},
		{Title: "Type"},
		{Title: "Scheme"},
		{Title: "Region"},
		{Title: "DNS name"},
		{Title: "AZs"},
	}
}

func (s *elbService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list ELB load balancers: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, elbRow{res: it, region: region})
	}
	return rows, nil
}

// SupportsSession は false を返す。Listeners と Targets のタブは第 1 段階では扱わない。
func (s *elbService) SupportsSession() bool { return false }

// elbRow はロードバランサ 1 つの行。ID はロードバランサの ARN。
type elbRow struct {
	res    aws.ELBResource
	region string
}

func (r elbRow) ID() string   { return r.res.ID }
func (r elbRow) Name() string { return r.res.Name }

// State は ELBResource.State の生の値 (active など) を返す。ResourceState() は aws.NormalizeState で
// active を running に変えるため、Web UI の StatusBadge と食い違うので使わない。
func (r elbRow) State() string { return r.res.State }

// azs は AZ を ", " で結合し、無ければ dash を返す (Web UI の azs.join(', ') || '—' と同じ)。
func (r elbRow) azs() string { return orDash(strings.Join(r.res.AZs, ", ")) }

func (r elbRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Type,
		r.res.Scheme,
		r.region,
		r.res.DNSName,
		r.azs(),
	}
}

// Overview は Web UI の elbOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r elbRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Type", Value: r.res.Type},
		{Key: "Scheme", Value: r.res.Scheme},
		{Key: "DNS name", Value: r.res.DNSName},
		{Key: "VPC", Value: r.res.VpcID},
		{Key: "AZs", Value: r.azs()},
		{Key: "Region", Value: r.region},
	}
}

// Tags は nil を返す。ELBResource にタグのフィールドが無く、Web UI の Tags タブも空のため。
func (r elbRow) Tags() map[string]string { return nil }
