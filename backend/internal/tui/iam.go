package tui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// cross は真偽値が偽のセルの表記 (Web UI の IAM の MFA の列の "✕")。
const cross = "✕"

// iamService は IAM のユーザーとロールの一覧を提供する Service。
type iamService struct {
	list func(ctx context.Context, profile, region string) ([]aws.IAMResource, error)
}

// NewIAMService は IAM の Service を返す。list には aws.ListIAMResources を渡す。
// ListIAMResources はリージョンを無視するが、Load は他のサービスと同じく選択中のリージョンを渡す。
func NewIAMService(list func(ctx context.Context, profile, region string) ([]aws.IAMResource, error)) Service {
	return &iamService{list: list}
}

func (s *iamService) ID() string    { return "iam" }
func (s *iamService) Title() string { return "IAM" }

// Columns は Web UI の iamColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// State の列の見出しは Activity。Last active は空のとき dash。
func (s *iamService) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "Kind"},
		{Title: "MFA"},
		{Title: "Activity"},
		{Title: "Last active"},
		{Title: "Policies"},
		{Title: "Groups"},
	}
}

func (s *iamService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list IAM users and roles: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, iamRow{res: it})
	}
	return rows, nil
}

func (s *iamService) SupportsSession() bool { return false }

// iamRow はユーザーかロール 1 つの行。Kind が user か role。IAM はグローバルなサービスのため
// 列にも Overview にも Region が無い。
type iamRow struct {
	res aws.IAMResource
}

func (r iamRow) ID() string   { return r.res.ID }
func (r iamRow) Name() string { return r.res.Name }

// State は IAMResource.State の生の値を返す。一覧関数は設定しないため空で、Web UI の Activity の
// StatusBadge も空になる。ResourceState() の固定の active は Web UI が表示しないので使わない。
func (r iamRow) State() string { return r.res.State }

// mfaCell は MFA の列の値。Web UI は取得失敗のとき値を出さず警告アイコンだけを出すので、
// TUI は fetch failed だけにする。
func (r iamRow) mfaCell() string {
	if r.res.MFAEnabledFetchFailed {
		return fetchFailed("")
	}
	if r.res.MFAEnabled {
		return check
	}
	return cross
}

// mfaOverview は Overview の MFA の値。Web UI は enabled か disabled の横に警告アイコンを出す。
func (r iamRow) mfaOverview() string {
	v := "disabled"
	if r.res.MFAEnabled {
		v = "enabled"
	}
	if r.res.MFAEnabledFetchFailed {
		return fetchFailed(v)
	}
	return v
}

// policies はアタッチされたポリシーの件数。取得失敗のとき件数の後ろに fetch failed を付ける。
func (r iamRow) policies() string {
	v := strconv.Itoa(len(r.res.Policies))
	if r.res.PoliciesFetchFailed {
		return fetchFailed(v)
	}
	return v
}

// groups は所属するグループの件数。取得失敗のとき件数の後ろに fetch failed を付ける。
func (r iamRow) groups() string {
	v := strconv.Itoa(len(r.res.Groups))
	if r.res.GroupsFetchFailed {
		return fetchFailed(v)
	}
	return v
}

func (r iamRow) Cells() []string {
	return []string{
		r.res.Name,
		r.res.Kind,
		r.mfaCell(),
		r.State(),
		orDash(r.res.LastActivity),
		r.policies(),
		r.groups(),
	}
}

// Overview は Web UI の iamOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r iamRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "ARN", Value: r.res.ARN},
		{Key: "Kind", Value: r.res.Kind},
		{Key: "MFA", Value: r.mfaOverview()},
		{Key: "Last active", Value: orDash(r.res.LastActivity)},
		{Key: "Policies attached", Value: r.policies()},
		{Key: "Groups", Value: r.groups()},
	}
}

// Tags は nil を返す。IAMResource にタグのフィールドが無く、Web UI の Tags タブも空のため。
func (r iamRow) Tags() map[string]string { return nil }
