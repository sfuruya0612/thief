package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// natgwService は NAT Gateway の一覧を提供する Service。
type natgwService struct {
	list func(ctx context.Context, profile, region string) ([]aws.NATGatewayResource, error)
	now  func() time.Time
}

// NewNATGatewayService は NAT Gateway の Service を返す。list には aws.ListNATGatewayResources を渡す。
func NewNATGatewayService(list func(ctx context.Context, profile, region string) ([]aws.NATGatewayResource, error)) Service {
	return &natgwService{list: list, now: time.Now}
}

func (s *natgwService) ID() string    { return "natgw" }
func (s *natgwService) Title() string { return "NAT Gateway" }

// Columns は Web UI の natgwColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// 値が無いセルの表示も Web UI に合わせ、Uptime は dash、VPC と Elastic IP は空文字列にする。
func (s *natgwService) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "State"},
		{Title: "Gateway ID"},
		{Title: "VPC"},
		{Title: "Elastic IP"},
		{Title: "Region"},
		{Title: "Uptime"},
	}
}

func (s *natgwService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list NAT gateways: %w", err)
	}
	now := s.now()
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, natgwRow{res: it, region: region, now: now})
	}
	return rows, nil
}

func (s *natgwService) SupportsSession() bool { return false }

// natgwRow は NAT Gateway 1 つの行。ID はゲートウェイ ID、Name はタグ Name の値 (無ければ空) で、
// どちらも一覧関数が決める。
type natgwRow struct {
	res    aws.NATGatewayResource
	region string
	now    time.Time
}

func (r natgwRow) ID() string   { return r.res.ID }
func (r natgwRow) Name() string { return r.res.Name }

// State は NATGatewayResource.State の生の値 (available など) を返す。ResourceState() は aws.NormalizeState で
// available を running に変えるため、Web UI の StatusBadge と食い違うので使わない。
func (r natgwRow) State() string { return r.res.State }

func (r natgwRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.ID,
		r.res.VpcID,
		r.res.ElasticIP,
		r.region,
		orDash(formatUptime(r.res.LaunchTime, r.now)),
	}
}

// Overview は Web UI の natgwOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Uptime と Launched は空なら dash にする。
func (r natgwRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "VPC", Value: r.res.VpcID},
		{Key: "Subnet", Value: r.res.SubnetID},
		{Key: "Elastic IP", Value: r.res.ElasticIP},
		{Key: "Region", Value: r.region},
		{Key: "Uptime", Value: orDash(formatUptime(r.res.LaunchTime, r.now))},
		{Key: "Launched", Value: orDash(formatLaunched(r.res.LaunchTime))},
	}
}

func (r natgwRow) Tags() map[string]string { return r.res.Tags }
