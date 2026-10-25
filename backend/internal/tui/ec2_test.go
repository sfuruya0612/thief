package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestFormatUptime(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		launch time.Time
		want   string
	}{
		{name: "zero", want: ""},
		{name: "minutes only", launch: now.Add(-25 * time.Minute), want: "0h 25m"},
		{name: "hours and minutes", launch: now.Add(-(3*time.Hour + 12*time.Minute)), want: "3h 12m"},
		{name: "days and hours", launch: now.Add(-(2*24*time.Hour + 3*time.Hour + 59*time.Minute)), want: "2d 3h"},
		{name: "future clamps to zero", launch: now.Add(time.Hour), want: "0h 0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUptime(tt.launch, now); got != tt.want {
				t.Errorf("formatUptime = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatLaunched(t *testing.T) {
	if got := formatLaunched(time.Time{}); got != "" {
		t.Errorf("zero = %q, want empty", got)
	}
	jst := time.FixedZone("JST", 9*3600)
	if got := formatLaunched(time.Date(2026, 1, 2, 3, 0, 0, 0, jst)); got != "2026-01-01" {
		t.Errorf("formatLaunched should use UTC, got %q", got)
	}
}

func TestEC2Row_CellsAndOverview(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	r := ec2Row{region: "ap-northeast-1", now: now, res: aws.EC2Resource{
		ID: "i-1", Name: "web", State: "running", InstanceType: "t3.micro", AZ: "ap-northeast-1c",
		PrivateIP: "10.0.0.1", LaunchTime: now.Add(-90 * time.Minute),
	}}

	wantCells := []string{"web", "running", "t3.micro", "ap-northeast-1", "c", "10.0.0.1", dash, "1h 30m"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %v, want %v", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&ec2Service{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&ec2Service{}).Columns()))
	}

	wantOverview := []KV{
		{"Resource ID", "i-1"}, {"Type", "t3.micro"}, {"AZ", "ap-northeast-1c"}, {"Region", "ap-northeast-1"},
		{"Private IP", "10.0.0.1"}, {"Public IP", dash}, {"VPC", ""}, {"Uptime", "1h 30m"}, {"Launched", "2026-10-23"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %v, want %v", got, wantOverview)
	}
	if r.State() != "running" || r.ID() != "i-1" || r.Name() != "web" {
		t.Errorf("accessors mismatch: %q %q %q", r.State(), r.ID(), r.Name())
	}
}

// TestEC2Row_EmptyValuesFollowWebUI は値が無いときの表示が Web UI (ec2Columns と ec2OverviewRows) と
// 同じであることを確かめる。列は Public IP と Uptime、Overview は Private IP、Public IP、Uptime、Launched が
// dash になり、Private IP の列、Type、AZ、VPC などそれ以外は空文字列のままになる。
func TestEC2Row_EmptyValuesFollowWebUI(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	r := ec2Row{region: "ap-northeast-1", now: now, res: aws.EC2Resource{ID: "i-1", Name: "web", State: "stopped"}}

	wantCells := []string{"web", "stopped", "", "ap-northeast-1", "", "", dash, dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "i-1"}, {"Type", ""}, {"AZ", ""}, {"Region", "ap-northeast-1"},
		{"Private IP", dash}, {"Public IP", dash}, {"VPC", ""}, {"Uptime", dash}, {"Launched", dash},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

// TestEC2Service_LoadWrapsErrorAndSetsRegion は NewEC2Service に渡した一覧関数が profile と region で
// 呼ばれ、返した EC2Resource が行になることと、失敗が包んで返ることを確かめる。
func TestEC2Service_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewEC2Service(func(context.Context, string, string) ([]aws.EC2Resource, error) { return nil, boom })
	if _, err := svc.Load(context.Background(), "p", "r"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped boom", err)
	}

	var calls []string
	svc = NewEC2Service(func(_ context.Context, profile, region string) ([]aws.EC2Resource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.EC2Resource{{ID: "i-1", Name: "web", State: "pending"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "i-1" || rows[0].Name() != "web" {
		t.Errorf("row = %q %q, want i-1 web", rows[0].ID(), rows[0].Name())
	}
	if rows[0].Cells()[3] != "us-west-2" {
		t.Errorf("region cell = %q, want us-west-2", rows[0].Cells()[3])
	}
	// State() は Web UI の StatusBadge と同じ生の値を返す。aws.NormalizeState の updating にはしない。
	if rows[0].State() != "pending" {
		t.Errorf("state should be the raw value, got %q", rows[0].State())
	}
}

func TestAzSuffix(t *testing.T) {
	if got := azSuffix(""); got != "" {
		t.Errorf("empty = %q", got)
	}
	if got := azSuffix("ap-northeast-1a"); got != "a" {
		t.Errorf("suffix = %q, want a", got)
	}
}
