package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// secretsService は Secrets Manager のシークレットの一覧を提供する Service。
type secretsService struct {
	list     func(ctx context.Context, profile, region string) ([]aws.SecretResource, error)
	getValue func(ctx context.Context, profile, region, name string) (string, error)
}

// NewSecretsService は Secrets Manager の Service を返す。list には aws.ListSecretResources を渡す。
func NewSecretsService(list func(ctx context.Context, profile, region string) ([]aws.SecretResource, error), getValue ...func(ctx context.Context, profile, region, name string) (string, error)) Service {
	s := &secretsService{list: list}
	if len(getValue) > 0 {
		s.getValue = getValue[0]
	}
	return s
}

func (s *secretsService) ID() string    { return "secrets" }
func (s *secretsService) Title() string { return "Secrets Manager" }

// Columns は Web UI の secretColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// State の列は無い。Description と Last changed は空のとき dash。
func (s *secretsService) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "Description"},
		{Title: "Last changed"},
	}
}

func (s *secretsService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list Secrets Manager secrets: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, secretsRow{res: it, region: region})
	}
	return rows, nil
}

// SupportsSession は false を返す。
func (s *secretsService) SupportsSession() bool { return false }

// Tabs は Overview、Value、Tags を返す。
func (s *secretsService) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.getValue != nil {
		tabs = append(tabs, TabSpec{
			ID: "value", Title: "Value",
			New: func(ctx context.Context, profile, region string, row Row) Tab {
				secret := row.(secretsRow)
				metadata := []KV{
					{Key: "Name", Value: secret.res.Name},
					{Key: "Description", Value: orDash(secret.res.Description)},
				}
				return newValueTab(ctx, metadata, true, func(ctx context.Context) (string, error) {
					return s.getValue(ctx, profile, region, secret.res.Name)
				})
			},
		})
	}
	tabs = append(tabs, TabSpec{ID: tabTags, Title: "Tags"})
	return tabs
}

// secretsRow はシークレット 1 つの行。ID はシークレットの ARN。値は一覧に含めない。
type secretsRow struct {
	res    aws.SecretResource
	region string
}

func (r secretsRow) ID() string   { return r.res.ID }
func (r secretsRow) Name() string { return r.res.Name }

// State は SecretResource.State の生の値を返す。一覧関数は設定しないため空で、Drawer の State も空になる。
// ResourceState() の固定の active は Web UI が表示しないので使わない。
func (r secretsRow) State() string { return r.res.State }

func (r secretsRow) Cells() []string {
	return []string{
		r.res.Name,
		orDash(r.res.Description),
		orDash(r.res.LastChanged),
	}
}

// Overview は Web UI の secretOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) から
// Name を除いた項目。Name は Drawer の描画 (refreshDrawer) が先頭に置くため、ここに含めると 2 回出る。
func (r secretsRow) Overview() []KV {
	return []KV{
		{Key: "Description", Value: orDash(r.res.Description)},
		{Key: "Last changed", Value: orDash(r.res.LastChanged)},
		{Key: "Region", Value: r.region},
	}
}

// Tags は nil を返す。SecretResource にタグのフィールドが無く、Web UI の Tags タブも空のため。
func (r secretsRow) Tags() map[string]string { return nil }
