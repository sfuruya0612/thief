package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// elbService は ELB (ALB / NLB / GWLB) の一覧を提供する Service。
type elbService struct {
	list         func(ctx context.Context, profile, region string) ([]aws.ELBResource, error)
	listeners    func(ctx context.Context, profile, region, lbArn string) ([]aws.ELBListenerResource, error)
	rules        func(ctx context.Context, profile, region, listenerArn string) ([]aws.ELBRuleResource, error)
	targetGroups func(ctx context.Context, profile, region, lbArn string) ([]aws.ELBTargetGroupResource, error)
	health       func(ctx context.Context, profile, region, tgArn string) ([]aws.ELBTargetHealthResource, error)
}

// NewELBService は ELB の Service を返す。list には aws.ListELBResources を渡す。
func NewELBService(list func(ctx context.Context, profile, region string) ([]aws.ELBResource, error)) Service {
	return &elbService{list: list}
}

// NewELBServiceWithTabs は Listeners と Targets のタブを持つ ELB の Service を返す。
func NewELBServiceWithTabs(
	list func(ctx context.Context, profile, region string) ([]aws.ELBResource, error),
	listeners func(ctx context.Context, profile, region, lbArn string) ([]aws.ELBListenerResource, error),
	rules func(ctx context.Context, profile, region, listenerArn string) ([]aws.ELBRuleResource, error),
	targetGroups func(ctx context.Context, profile, region, lbArn string) ([]aws.ELBTargetGroupResource, error),
	health func(ctx context.Context, profile, region, tgArn string) ([]aws.ELBTargetHealthResource, error),
) Service {
	return &elbService{list: list, listeners: listeners, rules: rules, targetGroups: targetGroups, health: health}
}

func (s *elbService) ID() string    { return "elb" }
func (s *elbService) Title() string { return "ELB" }

// Columns は Web UI の elbColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// 値が無いセルの表示も Web UI に合わせ、AZs は dash、DNS name は空文字列にする。
func (s *elbService) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "State"},
		{Title: "Type"},
		{Title: "Scheme"},
		{Title: "Region"},
		{Title: "DNS name"},
		{Title: "AZs"},
	}
}

func (s *elbService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list ELB load balancers: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, elbRow{res: it, region: region})
	}
	return rows, nil
}

// SupportsSession は false を返す。
func (s *elbService) SupportsSession() bool { return false }

// Tabs は Overview、Listeners、Targets、Tags を返す。
func (s *elbService) Tabs() []TabSpec {
	tabs := []TabSpec{{ID: tabOverview, Title: "Overview"}}
	if s.listeners != nil && s.rules != nil {
		tabs = append(tabs, TabSpec{
			ID: "listeners", Title: "Listeners",
			New: func(ctx context.Context, profile, region string, row Row) Tab {
				lbArn := row.(elbRow).ID()
				tab := newListDetailTab(ctx,
					[]Column{{Title: "Protocol"}, {Title: "Port", Align: AlignRight}, {Title: "Default action"}, {Title: "Default target group"}, {Title: "ARN"}},
					func(ctx context.Context) ([]listRow, error) {
						listeners, err := s.listeners(ctx, profile, region, lbArn)
						if err != nil {
							return nil, fmt.Errorf("list ELB listeners: %w", err)
						}
						rows := make([]listRow, 0, len(listeners))
						for _, listener := range listeners {
							rows = append(rows, listRow{
								cells: []string{listener.Protocol, formatInt32(listener.Port), orDash(listener.DefaultActionType), orDash(listener.DefaultTargetGroupArn), listener.ARN},
								value: listener,
							})
						}
						return rows, nil
					},
					nil,
					func(ctx context.Context, row listRow) (*TabDetail, error) {
						listener := row.value.(aws.ELBListenerResource)
						rules, err := s.rules(ctx, profile, region, listener.ARN)
						if err != nil {
							return nil, err
						}
						rows := make([][]string, 0, len(rules))
						for _, rule := range rules {
							priority := rule.Priority
							if rule.IsDefault {
								priority = "default"
							}
							rows = append(rows, []string{priority, orDash(strings.Join(rule.Conditions, " / ")), orDash(rule.ActionType), orDash(rule.TargetGroupArn), rule.ARN})
						}
						detail := listSummary("Rules", nil, &TabTable{
							Columns: []Column{{Title: "Priority"}, {Title: "Conditions"}, {Title: "Action"}, {Title: "Target group"}, {Title: "ARN"}},
							Rows:    rows,
						})
						if len(rows) == 0 {
							detail.Text = "No rules"
						}
						return detail, nil
					},
				)
				tab.empty = "No listeners"
				return tab
			},
		})
	}
	if s.targetGroups != nil && s.health != nil {
		tabs = append(tabs, TabSpec{
			ID: "targets", Title: "Targets",
			New: func(ctx context.Context, profile, region string, row Row) Tab {
				lbArn := row.(elbRow).ID()
				tab := newListDetailTab(ctx,
					[]Column{{Title: "Name"}, {Title: "Protocol"}, {Title: "Port", Align: AlignRight}, {Title: "Target type"}, {Title: "Health check"}, {Title: "VPC"}, {Title: "ARN"}},
					func(ctx context.Context) ([]listRow, error) {
						groups, err := s.targetGroups(ctx, profile, region, lbArn)
						if err != nil {
							return nil, fmt.Errorf("list ELB target groups: %w", err)
						}
						rows := make([]listRow, 0, len(groups))
						for _, group := range groups {
							rows = append(rows, listRow{
								cells: []string{group.Name, group.Protocol, formatInt32(group.Port), group.TargetType, orDash(group.HealthCheckPath), group.VpcID, group.ARN},
								value: group,
							})
						}
						return rows, nil
					},
					nil,
					func(ctx context.Context, row listRow) (*TabDetail, error) {
						group := row.value.(aws.ELBTargetGroupResource)
						health, err := s.health(ctx, profile, region, group.ARN)
						if err != nil {
							return nil, err
						}
						rows := make([][]string, 0, len(health))
						for _, target := range health {
							rows = append(rows, []string{target.TargetID, formatInt32(target.Port), target.State, orDash(target.AvailabilityZone), orDash(target.Reason), orDash(target.Description)})
						}
						detail := listSummary("Target health", nil, &TabTable{
							Columns: []Column{{Title: "Target"}, {Title: "Port", Align: AlignRight}, {Title: "State"}, {Title: "AZ"}, {Title: "Reason"}, {Title: "Description"}},
							Rows:    rows,
						})
						if len(rows) == 0 {
							detail.Text = "No targets"
						}
						return detail, nil
					},
				)
				tab.empty = "No target groups"
				return tab
			},
		})
	}
	tabs = append(tabs, TabSpec{ID: tabTags, Title: "Tags"})
	return tabs
}

// elbRow はロードバランサ 1 つの行。ID はロードバランサの ARN。
type elbRow struct {
	res    aws.ELBResource
	region string
}

func (r elbRow) ID() string   { return r.res.ID }
func (r elbRow) Name() string { return r.res.Name }

// State は ELBResource.State の生の値 (active など) を返す。ResourceState() は aws.NormalizeState で
// active を running に変えるため、Web UI の StatusBadge と食い違うので使わない。
func (r elbRow) State() string { return r.res.State }

// azs は AZ を ", " で結合し、無ければ dash を返す (Web UI の azs.join(', ') || '—' と同じ)。
func (r elbRow) azs() string { return orDash(strings.Join(r.res.AZs, ", ")) }

func (r elbRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.Type,
		r.res.Scheme,
		r.region,
		r.res.DNSName,
		r.azs(),
	}
}

// Overview は Web UI の elbOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r elbRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Type", Value: r.res.Type},
		{Key: "Scheme", Value: r.res.Scheme},
		{Key: "DNS name", Value: r.res.DNSName},
		{Key: "VPC", Value: r.res.VpcID},
		{Key: "AZs", Value: r.azs()},
		{Key: "Region", Value: r.region},
	}
}

// Tags は nil を返す。ELBResource にタグのフィールドが無く、Web UI の Tags タブも空のため。
func (r elbRow) Tags() map[string]string { return nil }
