package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// dash は値が無いセルの表示。Web UI の Dash() と同じ文字を使う。
const dash = "—"

// ec2Service は EC2 インスタンスの一覧を提供する Service。
type ec2Service struct {
	list func(ctx context.Context, profile, region string) ([]aws.EC2Resource, error)
	now  func() time.Time
}

// NewEC2Service は EC2 の Service を返す。list には aws.ListEC2Resources を渡す。
func NewEC2Service(list func(ctx context.Context, profile, region string) ([]aws.EC2Resource, error)) Service {
	return &ec2Service{list: list, now: time.Now}
}

func (s *ec2Service) ID() string    { return "ec2" }
func (s *ec2Service) Title() string { return "EC2" }

// Columns は Web UI の ec2Columns (frontend/src/components/tables/columns.tsx) と同じ列。
// 値が無いセルの表示も Web UI に合わせ、Private IP は空文字列、Public IP と Uptime は dash にする。
func (s *ec2Service) Columns() []Column {
	return []Column{
		{Title: "Name"},
		{Title: "State"},
		{Title: "Type"},
		{Title: "Region"},
		{Title: "AZ"},
		{Title: "Private IP"},
		{Title: "Public IP"},
		{Title: "Uptime"},
	}
}

func (s *ec2Service) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list EC2 instances: %w", err)
	}
	now := s.now()
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, ec2Row{res: it, region: region, now: now})
	}
	return rows, nil
}

func (s *ec2Service) SupportsSession() bool { return true }

// ec2Row は EC2 インスタンス 1 台の行。
type ec2Row struct {
	res    aws.EC2Resource
	region string
	now    time.Time
}

func (r ec2Row) ID() string    { return r.res.ID }
func (r ec2Row) Name() string  { return r.res.Name }
func (r ec2Row) State() string { return r.res.State }

func (r ec2Row) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.InstanceType,
		r.region,
		azSuffix(r.res.AZ),
		r.res.PrivateIP,
		orDash(r.res.PublicIP),
		orDash(formatUptime(r.res.LaunchTime, r.now)),
	}
}

// Overview は Web UI の ec2OverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
// 値が無いときの表示も Web UI に合わせ、Private IP、Public IP、Uptime、Launched は dash、
// Resource ID、Type、AZ、Region、VPC は生の値 (空なら空文字列) にする。
func (r ec2Row) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Type", Value: r.res.InstanceType},
		{Key: "AZ", Value: r.res.AZ},
		{Key: "Region", Value: r.region},
		{Key: "Private IP", Value: orDash(r.res.PrivateIP)},
		{Key: "Public IP", Value: orDash(r.res.PublicIP)},
		{Key: "VPC", Value: r.res.VpcID},
		{Key: "Uptime", Value: orDash(formatUptime(r.res.LaunchTime, r.now))},
		{Key: "Launched", Value: orDash(formatLaunched(r.res.LaunchTime))},
	}
}

func (r ec2Row) Tags() map[string]string { return r.res.Tags }

// azSuffix は AZ の末尾 1 文字 (ap-northeast-1a なら a) を返す。Web UI の一覧と同じ表示。
func azSuffix(az string) string {
	if az == "" {
		return ""
	}
	return az[len(az)-1:]
}

func orDash(s string) string {
	if s == "" {
		return dash
	}
	return s
}

// formatUptime は起動からの経過時間を Web UI の formatUptime (frontend/src/lib/format.ts) と
// 同じ書式 (1 日未満は "3h 12m"、1 日以上は "2d 3h") で返す。launch がゼロ値なら空文字。
func formatUptime(launch, now time.Time) string {
	if launch.IsZero() {
		return ""
	}
	diff := now.Sub(launch)
	if diff < 0 {
		diff = 0
	}
	totalHours := int(diff / time.Hour)
	days := totalHours / 24
	hours := totalHours % 24
	if days == 0 {
		minutes := int((diff % time.Hour) / time.Minute)
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}

// formatLaunched は起動日を UTC の YYYY-MM-DD で返す。Web UI の launchedDate と同じ。
func formatLaunched(launch time.Time) string {
	if launch.IsZero() {
		return ""
	}
	return launch.UTC().Format("2006-01-02")
}
