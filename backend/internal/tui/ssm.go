package tui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// ssmService は Parameter Store のパラメータの一覧を提供する Service。
type ssmService struct {
	list func(ctx context.Context, profile, region string) ([]aws.SSMParameterResource, error)
}

// NewSSMService は Parameter Store の Service を返す。list には aws.ListSSMParameters を渡す
// (CLI だけが使う aws.ListSSMParameterInfos ではない)。
func NewSSMService(list func(ctx context.Context, profile, region string) ([]aws.SSMParameterResource, error)) Service {
	return &ssmService{list: list}
}

func (s *ssmService) ID() string    { return "ssm" }
func (s *ssmService) Title() string { return "Parameter Store" }

// Columns は Web UI の ssmColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// State の列は無い。Last modified は空のとき dash。
func (s *ssmService) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "Type"},
		{Title: "Version"},
		{Title: "Last modified"},
	}
}

func (s *ssmService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list Parameter Store parameters: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, ssmRow{res: it, region: region})
	}
	return rows, nil
}

// SupportsSession は false を返す。Value のタブは第 1 段階では扱わない。
func (s *ssmService) SupportsSession() bool { return false }

// ssmRow はパラメータ 1 つの行。ID はパラメータの名前。値は一覧に含めない。
type ssmRow struct {
	res    aws.SSMParameterResource
	region string
}

func (r ssmRow) ID() string   { return r.res.ID }
func (r ssmRow) Name() string { return r.res.Name }

// State は SSMParameterResource.State の生の値を返す。一覧関数は設定しないため空で、Drawer の State も
// 空になる。ResourceState() の固定の active は Web UI が表示しないので使わない。
func (r ssmRow) State() string { return r.res.State }

func (r ssmRow) Cells() []string {
	return []string{
		r.res.Name,
		r.res.Type,
		strconv.FormatInt(r.res.Version, 10),
		orDash(r.res.LastModified),
	}
}

// Overview は Web UI の ssmOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) から
// Name を除いた項目。Name は Drawer の描画 (refreshDrawer) が先頭に置くため、ここに含めると 2 回出る。
func (r ssmRow) Overview() []KV {
	return []KV{
		{Key: "Type", Value: r.res.Type},
		{Key: "Tier", Value: r.res.Tier},
		{Key: "Version", Value: strconv.FormatInt(r.res.Version, 10)},
		{Key: "Last modified", Value: orDash(r.res.LastModified)},
		{Key: "Region", Value: r.region},
	}
}

// Tags は nil を返す。SSMParameterResource にタグのフィールドが無く、Web UI の Tags タブも空のため。
func (r ssmRow) Tags() map[string]string { return nil }
