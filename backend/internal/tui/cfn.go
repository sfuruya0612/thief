package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// cfnService は CloudFormation のスタックの一覧を提供する Service。
type cfnService struct {
	list   func(ctx context.Context, profile, region string) ([]aws.CFNStackResource, error)
	detail func(ctx context.Context, profile, region, stackName string) (*aws.CFNStackDetail, error)
}

// NewCFNService は CloudFormation の Service を返す。list には aws.ListCFNStacks を、
// detail には aws.DescribeCFNStackDetail を渡す。detail は行を選んだときのタグの取得 (FetchTags) に使う。
func NewCFNService(
	list func(ctx context.Context, profile, region string) ([]aws.CFNStackResource, error),
	detail func(ctx context.Context, profile, region, stackName string) (*aws.CFNStackDetail, error),
) Service {
	return &cfnService{list: list, detail: detail}
}

func (s *cfnService) ID() string    { return "cfn" }
func (s *cfnService) Title() string { return "CloudFormation" }

// Columns は Web UI の cfnColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Created と Updated は空のとき dash。
func (s *cfnService) Columns() []Column {
	return []Column{
		{Title: "Stack"},
		{Title: "State"},
		{Title: "Drift"},
		{Title: "Created"},
		{Title: "Updated"},
	}
}

func (s *cfnService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list CloudFormation stacks: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, cfnRow{res: it, profile: profile, region: region, detail: s.detail})
	}
	return rows, nil
}

// SupportsSession は false を返す。
func (s *cfnService) SupportsSession() bool { return false }

// cfnRow はスタック 1 つの行。ID はスタック ID (ARN)。タグは一覧 (ListStacks) に含まれないため、
// 行を選んだときに FetchTags が detail (DescribeStacks) で取る。
type cfnRow struct {
	res     aws.CFNStackResource
	profile string
	region  string
	detail  func(ctx context.Context, profile, region, stackName string) (*aws.CFNStackDetail, error)
}

func (r cfnRow) ID() string   { return r.res.ID }
func (r cfnRow) Name() string { return r.res.Name }

// State は StackStatus の生の値 (CREATE_COMPLETE など) を返す。aws.NormalizeState はこの値を変換しない。
func (r cfnRow) State() string { return r.res.State }

func (r cfnRow) Cells() []string {
	return []string{
		r.res.Name,
		r.res.State,
		r.res.DriftStatus,
		orDash(r.res.CreationTime),
		orDash(r.res.LastUpdatedTime),
	}
}

// Overview は Web UI の cfnOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) から
// State を除いた項目。State は Drawer の描画 (refreshDrawer) が Name の次に置くため、ここに含めると 2 回出る。
func (r cfnRow) Overview() []KV {
	return []KV{
		{Key: "Stack ID", Value: r.res.ID},
		{Key: "Drift status", Value: r.res.DriftStatus},
		{Key: "Created", Value: orDash(r.res.CreationTime)},
		{Key: "Updated", Value: orDash(r.res.LastUpdatedTime)},
		{Key: "Region", Value: r.region},
	}
}

// Tags は nil を返す。一覧関数 (ListCFNStacks の cfnFromSummary) はタグを設定しないため。
// タグは FetchTags で取る。
func (r cfnRow) Tags() map[string]string { return nil }

// FetchTags はスタック名で detail (DescribeStacks) を呼び、そのタグを返す。
// Web UI の DrawerCFNTags が GET /api/aws/profiles/{profile}/cfn/stacks/{stack} で取るものと同じ。
func (r cfnRow) FetchTags(ctx context.Context) (map[string]string, error) {
	// detail の配線漏れは tea.Cmd の goroutine の panic でプロセスを落とすのではなく、取得失敗として表示する。
	if r.detail == nil {
		return nil, fmt.Errorf("describe CloudFormation stack %s: %w", r.res.Name, errCFNDetailUnavailable)
	}
	d, err := r.detail(ctx, r.profile, r.region, r.res.Name)
	if err != nil {
		return nil, fmt.Errorf("describe CloudFormation stack %s: %w", r.res.Name, err)
	}
	if d == nil {
		return nil, fmt.Errorf("describe CloudFormation stack %s: %w", r.res.Name, errCFNDetailUnavailable)
	}
	return d.Tags, nil
}

// errCFNDetailUnavailable は detail 関数が無いか、エラー無しで nil の詳細を返したときの FetchTags のエラー。
var errCFNDetailUnavailable = errors.New("stack detail is not available")
