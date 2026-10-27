package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

// gcpIAMService はプロジェクトの IAM ポリシーをメンバー単位に集約して表示する Service。
type gcpIAMService struct {
	list func(ctx context.Context, projectID string) ([]gcp.IAMBindingInfo, error)
}

// NewGCPIAMService は IAM の Service を返す。list には gcp.ListIAMBindings を渡す。
func NewGCPIAMService(list func(ctx context.Context, projectID string) ([]gcp.IAMBindingInfo, error)) Service {
	return &gcpIAMService{list: list}
}

func (s *gcpIAMService) ID() string    { return "gcpiam" }
func (s *gcpIAMService) Title() string { return "IAM" }

// Columns は Web UI の iamMemberColumns と同じ列。Roles は複数のロールを ", " で連結する。
func (s *gcpIAMService) Columns() []Column {
	return []Column{
		{Title: "Member"},
		{Title: "Roles"},
		{Title: "Count", Align: AlignRight},
	}
}

func (s *gcpIAMService) Load(ctx context.Context, projectID, _ string) ([]Row, error) {
	bindings, err := s.list(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list IAM bindings: %w", err)
	}
	members := groupGCPIAMBindings(bindings)
	rows := make([]Row, 0, len(members))
	for _, m := range members {
		rows = append(rows, gcpIAMRow{res: m})
	}
	return rows, nil
}

func (s *gcpIAMService) SupportsSession() bool { return false }

// Tabs は Web UI と同じく Overview だけを返す。
func (s *gcpIAMService) Tabs() []TabSpec {
	return []TabSpec{{ID: tabOverview, Title: "Overview"}}
}

// gcpIAMMember は 1 メンバーと、そのメンバーに付いているロールの一覧。
type gcpIAMMember struct {
	Member    string
	ProjectID string
	Roles     []string
}

// groupGCPIAMBindings は (メンバー, ロール) のバインディングをメンバー単位に集約する。
// メンバーとロールは表示を安定させるために昇順で並べる。
func groupGCPIAMBindings(bindings []gcp.IAMBindingInfo) []gcpIAMMember {
	byMember := map[string]*gcpIAMMember{}
	seenRole := map[string]map[string]bool{}
	for _, b := range bindings {
		if b.Member == "" {
			continue
		}
		m, ok := byMember[b.Member]
		if !ok {
			m = &gcpIAMMember{Member: b.Member, ProjectID: b.ProjectID}
			byMember[b.Member] = m
			seenRole[b.Member] = map[string]bool{}
		}
		if b.Role != "" && !seenRole[b.Member][b.Role] {
			seenRole[b.Member][b.Role] = true
			m.Roles = append(m.Roles, b.Role)
		}
	}

	members := make([]gcpIAMMember, 0, len(byMember))
	for _, m := range byMember {
		sort.Strings(m.Roles)
		members = append(members, *m)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Member < members[j].Member })
	return members
}

// gcpIAMRow は IAM のメンバー 1 人の行。ID はメンバー。
type gcpIAMRow struct {
	res gcpIAMMember
}

func (r gcpIAMRow) ID() string    { return r.res.Member }
func (r gcpIAMRow) Name() string  { return r.res.Member }
func (r gcpIAMRow) State() string { return "" }

func (r gcpIAMRow) Cells() []string {
	return []string{
		r.res.Member,
		strings.Join(r.res.Roles, ", "),
		strconv.Itoa(len(r.res.Roles)),
	}
}

// Overview は Web UI の iamMemberOverviewRows と同じ項目。
func (r gcpIAMRow) Overview() []KV {
	out := []KV{
		{Key: "Member", Value: r.res.Member},
		{Key: "Project", Value: r.res.ProjectID},
		{Key: "Roles", Value: strconv.Itoa(len(r.res.Roles))},
	}
	for i, role := range r.res.Roles {
		out = append(out, KV{Key: fmt.Sprintf("Role %d", i+1), Value: role})
	}
	return out
}

func (r gcpIAMRow) Tags() map[string]string { return nil }
