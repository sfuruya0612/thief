package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// cloudfrontService は CloudFront ディストリビューションの一覧を提供する Service。
type cloudfrontService struct {
	list func(ctx context.Context, profile, region string) ([]aws.CloudFrontResource, error)
}

// NewCloudFrontService は CloudFront の Service を返す。list には aws.ListCloudFrontResources を渡す。
// 一覧関数はリージョンを無視する (CloudFront はグローバル) が、他のサービスと同じく選択中のリージョンを渡す。
func NewCloudFrontService(list func(ctx context.Context, profile, region string) ([]aws.CloudFrontResource, error)) Service {
	return &cloudfrontService{list: list}
}

func (s *cloudfrontService) ID() string    { return "cloudfront" }
func (s *cloudfrontService) Title() string { return "CloudFront" }

// Columns は Web UI の cloudfrontColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Web UI と同じく Region の列は無い。値が無いセルは Alternate domains と Origins が dash、Domain が空文字列。
func (s *cloudfrontService) Columns() []Column {
	return []Column{
		{Title: "Distribution"},
		{Title: "State"},
		{Title: "Domain"},
		{Title: "Alternate domains"},
		{Title: "Origins"},
	}
}

func (s *cloudfrontService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list CloudFront distributions: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, cloudfrontRow{res: it})
	}
	return rows, nil
}

// SupportsSession は false を返す。Behaviors のタブは第 1 段階では扱わない。
func (s *cloudfrontService) SupportsSession() bool { return false }

// cloudfrontRow はディストリビューション 1 つの行。ID はディストリビューション ID、
// Name は Comment (無ければ ID) で、どちらも一覧関数が決める。
type cloudfrontRow struct {
	res aws.CloudFrontResource
}

func (r cloudfrontRow) ID() string    { return r.res.ID }
func (r cloudfrontRow) Name() string  { return r.res.Name }
func (r cloudfrontRow) State() string { return r.res.State }

// aliases と origins は ", " で結合し、無ければ dash を返す (Web UI の join(', ') || dash と同じ)。
func (r cloudfrontRow) aliases() string { return orDash(strings.Join(r.res.Aliases, ", ")) }
func (r cloudfrontRow) origins() string { return orDash(strings.Join(r.res.Origins, ", ")) }

// Cells の先頭は Web UI の Distribution の列と同じくディストリビューション ID。
func (r cloudfrontRow) Cells() []string {
	return []string{
		r.res.ID,
		r.State(),
		r.res.DomainName,
		r.aliases(),
		r.origins(),
	}
}

// Overview は Web UI の cloudfrontOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Enabled は yes か no にする。
func (r cloudfrontRow) Overview() []KV {
	enabled := "no"
	if r.res.Enabled {
		enabled = "yes"
	}
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Domain", Value: r.res.DomainName},
		{Key: "Alternate domains", Value: r.aliases()},
		{Key: "Origins", Value: r.origins()},
		{Key: "Enabled", Value: enabled},
		{Key: "Price class", Value: r.res.PriceClass},
	}
}

// Tags は nil を返す。CloudFrontResource にタグのフィールドが無く、Web UI の Tags タブも空のため。
func (r cloudfrontRow) Tags() map[string]string { return nil }
