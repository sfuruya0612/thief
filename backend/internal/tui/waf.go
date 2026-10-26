package tui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// wafService は WAF の Web ACL の一覧を提供する Service。
type wafService struct {
	list func(ctx context.Context, profile, region string) ([]aws.WAFResource, error)
}

// NewWAFService は WAF の Service を返す。list には aws.ListWAFResources を渡す。
func NewWAFService(list func(ctx context.Context, profile, region string) ([]aws.WAFResource, error)) Service {
	return &wafService{list: list}
}

func (s *wafService) ID() string    { return "waf" }
func (s *wafService) Title() string { return "WAF" }

// Columns は Web UI の wafColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Description は空のとき dash。
func (s *wafService) Columns() []Column {
	return []Column{
		{Title: "Web ACL"},
		{Title: "Description"},
		{Title: "State"},
		{Title: "Scope"},
		{Title: "Region"},
		{Title: "Rules"},
		{Title: "Associated"},
	}
}

func (s *wafService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list WAF web ACLs: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, wafRow{res: it, region: region})
	}
	return rows, nil
}

// SupportsSession は false を返す。Rules のタブは第 1 段階では扱わない。
func (s *wafService) SupportsSession() bool { return false }

// wafRow は Web ACL 1 つの行。Scope は REGIONAL か CLOUDFRONT で、CLOUDFRONT の行も Region は
// 引数のリージョンのまま (Web UI の wafFromRaw と同じ)。
type wafRow struct {
	res    aws.WAFResource
	region string
}

func (r wafRow) ID() string   { return r.res.ID }
func (r wafRow) Name() string { return r.res.Name }

// State は WAFResource.State の生の値 (active) を返す。
func (r wafRow) State() string { return r.res.State }

// associated は関連付けられた資源の件数。取得失敗のとき件数の後ろに fetch failed を付ける。
func (r wafRow) associated() string {
	v := strconv.Itoa(r.res.AssociatedCount)
	if r.res.AssociatedCountFetchFailed {
		return fetchFailed(v)
	}
	return v
}

func (r wafRow) Cells() []string {
	return []string{
		r.res.Name,
		orDash(r.res.Description),
		r.State(),
		r.res.Scope,
		r.region,
		strconv.Itoa(r.res.RuleCount),
		r.associated(),
	}
}

// Overview は Web UI の wafOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r wafRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Scope", Value: r.res.Scope},
		{Key: "Description", Value: orDash(r.res.Description)},
		{Key: "Rules", Value: strconv.Itoa(r.res.RuleCount)},
		{Key: "Associated resources", Value: r.associated()},
		{Key: "Region", Value: r.region},
	}
}

func (r wafRow) Tags() map[string]string { return r.res.Tags }

// TagsFetchFailed は ListWAFResources の内部の ListTagsForResource が失敗したかを返す (TagsFetchFailer)。
func (r wafRow) TagsFetchFailed() bool { return r.res.TagsFetchFailed }
