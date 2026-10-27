package tui

import (
	"context"
	"fmt"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

// gcpServiceAccountService は Service Account の一覧を提供する Service。
type gcpServiceAccountService struct {
	list func(ctx context.Context, projectID string) ([]gcp.ServiceAccountInfo, error)
}

// NewGCPServiceAccountService は Service Account の Service を返す。list には
// gcp.ListServiceAccounts を渡す。
func NewGCPServiceAccountService(list func(ctx context.Context, projectID string) ([]gcp.ServiceAccountInfo, error)) Service {
	return &gcpServiceAccountService{list: list}
}

func (s *gcpServiceAccountService) ID() string    { return "gcpserviceaccounts" }
func (s *gcpServiceAccountService) Title() string { return "Service Accounts" }

// Columns は Web UI の serviceAccountColumns と同じ列。
func (s *gcpServiceAccountService) Columns() []Column {
	return []Column{
		{Title: "Email"},
		{Title: "Display name"},
		{Title: "Status"},
		{Title: "Description"},
	}
}

func (s *gcpServiceAccountService) Load(ctx context.Context, projectID, _ string) ([]Row, error) {
	items, err := s.list(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list service accounts: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, gcpServiceAccountRow{res: it})
	}
	return rows, nil
}

func (s *gcpServiceAccountService) SupportsSession() bool { return false }

// Tabs は Web UI と同じく Overview だけを返す。
func (s *gcpServiceAccountService) Tabs() []TabSpec {
	return []TabSpec{{ID: tabOverview, Title: "Overview"}}
}

// gcpServiceAccountRow は Service Account 1 件の行。ID はメールアドレス。
type gcpServiceAccountRow struct {
	res gcp.ServiceAccountInfo
}

func (r gcpServiceAccountRow) ID() string { return r.res.Email }

// Name は Web UI の serviceAccountFromRaw と同じく、表示名が空ならメールアドレスを返す。
func (r gcpServiceAccountRow) Name() string {
	if r.res.DisplayName != "" {
		return r.res.DisplayName
	}
	return r.res.Email
}

func (r gcpServiceAccountRow) State() string {
	if r.res.Disabled {
		return "disabled"
	}
	return "enabled"
}

func (r gcpServiceAccountRow) Cells() []string {
	return []string{
		r.res.Email,
		orDash(r.res.DisplayName),
		r.State(),
		orDash(r.res.Description),
	}
}

// Overview は Web UI の serviceAccountOverviewRows と同じ項目。
func (r gcpServiceAccountRow) Overview() []KV {
	return []KV{
		{Key: "Email", Value: r.res.Email},
		{Key: "Display name", Value: orDash(r.res.DisplayName)},
		{Key: "Description", Value: orDash(r.res.Description)},
		{Key: "Project", Value: r.res.ProjectID},
		{Key: "Unique ID", Value: r.res.UniqueID},
		{Key: "Status", Value: r.State()},
	}
}

func (r gcpServiceAccountRow) Tags() map[string]string { return nil }
