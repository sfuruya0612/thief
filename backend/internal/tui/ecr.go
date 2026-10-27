package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// check は真の値の表示。Web UI の ecrColumns の Scan on push と同じ文字を使う。
const check = "✓"

// ecrService は ECR リポジトリの一覧を提供する Service。
type ecrService struct {
	list   func(ctx context.Context, profile, region string) ([]aws.ECRRepoResource, error)
	images func(ctx context.Context, profile, region, repo string) ([]aws.ECRImageResource, error)
}

// NewECRService は ECR の Service を返す。list には aws.ListECRResources を渡す。
// images を省略すると Images タブを表示しない (既存のテスト用)。
func NewECRService(list func(ctx context.Context, profile, region string) ([]aws.ECRRepoResource, error), images ...func(ctx context.Context, profile, region, repo string) ([]aws.ECRImageResource, error)) Service {
	s := &ecrService{list: list}
	if len(images) > 0 {
		s.images = images[0]
	}
	return s
}

func (s *ecrService) ID() string    { return "ecr" }
func (s *ecrService) Title() string { return "ECR" }

// Columns は Web UI の ecrColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// 値が無いセルの表示も Web UI に合わせ、Created は dash、URI と Tag mutability は空文字列にする。
// Scan on push は真なら check、偽なら dash。
func (s *ecrService) Columns() []Column {
	return []Column{
		{Title: "Repository"},
		{Title: "URI"},
		{Title: "Tag mutability"},
		{Title: "Scan on push"},
		{Title: "Created"},
	}
}

func (s *ecrService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list ECR repositories: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, ecrRow{res: it, region: region})
	}
	return rows, nil
}

func (s *ecrService) SupportsSession() bool { return false }

// Tabs は Web UI の DRAWER_TABS と同じく Overview と Images を返す。ECR は Tags を持たない。
func (s *ecrService) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.images != nil {
		tabs = append(tabs, contentTabSpec("images", "Images", func(ctx context.Context, profile, region string, row Row) (TabContent, error) {
			repo := row.(ecrRow)
			images, err := s.images(ctx, profile, region, repo.Name())
			if err != nil {
				return TabContent{}, fmt.Errorf("list ECR images: %w", err)
			}
			rows := make([][]string, 0, len(images))
			for _, image := range images {
				digest := image.ImageDigest
				if len(digest) > 19 {
					digest = digest[:19]
				}
				digest += "…"
				rows = append(rows, []string{
					orDash(image.ImageTag),
					digest,
					formatBytes(image.ImageSizeBytes),
					orDash(image.PushedAt),
					orDash(image.LastPulledAt),
				})
			}
			return TabContent{Blocks: []TabBlock{{
				Title: fmt.Sprintf("Images (%d)", len(images)),
				Table: &TabTable{
					Columns: []Column{{Title: "Tag"}, {Title: "Digest"}, {Title: "Size", Align: AlignRight}, {Title: "Pushed"}, {Title: "Pulled"}},
					Rows:    rows,
				},
			}}}, nil
		}))
	}
	return tabs
}

// ecrRow は ECR リポジトリ 1 つの行。ID はリポジトリの ARN。
type ecrRow struct {
	res    aws.ECRRepoResource
	region string
}

func (r ecrRow) ID() string   { return r.res.ID }
func (r ecrRow) Name() string { return r.res.Name }

// State は ECRRepoResource.State の生の値を返す。aws.ListECRResources は State を設定しないため空で、
// Web UI の StatusBadge も空の値を表示する (ResourceState() の固定の active は Web UI が表示しない)。
func (r ecrRow) State() string { return r.res.State }

func (r ecrRow) Cells() []string {
	scan := dash
	if r.res.ScanOnPush {
		scan = check
	}
	return []string{
		r.res.Name,
		r.res.URI,
		r.res.ImageTagMutability,
		scan,
		orDash(r.res.CreatedAt),
	}
}

// Overview は Web UI の ecrOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// Scan on push は enabled か disabled、Created は空なら dash、他の項目は生の値にする。
func (r ecrRow) Overview() []KV {
	scan := "disabled"
	if r.res.ScanOnPush {
		scan = "enabled"
	}
	return []KV{
		{Key: "Repository ARN", Value: r.res.ID},
		{Key: "URI", Value: r.res.URI},
		{Key: "Tag mutability", Value: r.res.ImageTagMutability},
		{Key: "Scan on push", Value: scan},
		{Key: "Created", Value: orDash(r.res.CreatedAt)},
		{Key: "Region", Value: r.region},
	}
}

// Tags は nil を返す。ECRRepoResource にタグは無く、Web UI の ECR の Drawer にも Tags タブが無い。
// TUI の Drawer は Tags タブを "No tags" と表示する。
func (r ecrRow) Tags() map[string]string { return nil }
