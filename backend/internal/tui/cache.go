package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// cacheService は ElastiCache クラスタの一覧を提供する Service。
type cacheService struct {
	list func(ctx context.Context, profile, region string) ([]aws.ElastiCacheResource, error)
}

// NewCacheService は ElastiCache の Service を返す。list には aws.ListElastiCacheResources を渡す。
func NewCacheService(list func(ctx context.Context, profile, region string) ([]aws.ElastiCacheResource, error)) Service {
	return &cacheService{list: list}
}

// ID は Web UI の serviceMeta.ts と PRD の FR-29 と同じ cache を返す。
// aws.ElastiCacheResource.ServiceName() の elasticache とは違う。
func (s *cacheService) ID() string    { return "cache" }
func (s *cacheService) Title() string { return "ElastiCache" }

// Columns は Web UI の cacheColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// 値が無いセルの表示も Web UI に合わせ、Replication Group と AZs は dash、Endpoint は空文字列にする。
// AZs はノードの AZ を ", " で連ねる。Nodes は 10 進。
func (s *cacheService) Columns() []Column {
	return []Column{
		{Title: "Cluster"},
		{Title: "State"},
		{Title: "Engine"},
		{Title: "Replication Group"},
		{Title: "Node type"},
		{Title: "Region"},
		{Title: "AZs"},
		{Title: "Nodes"},
		{Title: "Endpoint"},
	}
}

func (s *cacheService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list ElastiCache clusters: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, cacheRow{res: it, region: region})
	}
	return rows, nil
}

func (s *cacheService) SupportsSession() bool { return false }

// cacheRow は ElastiCache クラスタ 1 つの行。ID はクラスタ ID。
type cacheRow struct {
	res    aws.ElastiCacheResource
	region string
}

func (r cacheRow) ID() string   { return r.res.ID }
func (r cacheRow) Name() string { return r.res.Name }

// State は ElastiCacheResource.State の生の値 (available など) を返す。ResourceState() は aws.NormalizeState で
// available を running に変えるため、Web UI の StatusBadge と食い違うので使わない。
func (r cacheRow) State() string { return r.res.State }

// azs はノードの AZ を Web UI と同じ ", " 区切りで連ね、無ければ dash を返す。
func (r cacheRow) azs() string {
	return orDash(strings.Join(r.res.NodeAvailabilityZones, ", "))
}

func (r cacheRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Engine,
		orDash(r.res.ReplicationGroupID),
		r.res.NodeType,
		r.region,
		r.azs(),
		formatInt32(r.res.NumNodes),
		r.res.Endpoint,
	}
}

// Overview は Web UI の cacheOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Replication group と AZs は空なら dash、Nodes と Port は 10 進、他の項目は生の値にする。
func (r cacheRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Engine", Value: r.res.Engine},
		{Key: "Engine version", Value: r.res.EngineVersion},
		{Key: "Replication group", Value: orDash(r.res.ReplicationGroupID)},
		{Key: "Node type", Value: r.res.NodeType},
		{Key: "Nodes", Value: formatInt32(r.res.NumNodes)},
		{Key: "AZs", Value: r.azs()},
		{Key: "Endpoint", Value: r.res.Endpoint},
		{Key: "Port", Value: formatInt32(r.res.Port)},
		{Key: "Region", Value: r.region},
	}
}

// Tags は nil を返す。ElastiCacheResource にタグは無く、Web UI の ElastiCache の Tags タブも空である。
// TUI の Drawer は Tags タブを "No tags" と表示する。
func (r cacheRow) Tags() map[string]string { return nil }
