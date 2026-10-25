package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestNATGatewayService_IDTitleAndColumns(t *testing.T) {
	svc := NewNATGatewayService(nil)
	if svc.ID() != "natgw" || svc.Title() != "NAT Gateway" {
		t.Errorf("ID/Title = %q/%q, want natgw/NAT Gateway", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("NAT Gateway has no interactive session")
	}
	// Web UI の natgwColumns の header の並び。
	want := []Column{
		{Title: "Name"}, {Title: "State"}, {Title: "Gateway ID"}, {Title: "VPC"}, {Title: "Elastic IP"},
		{Title: "Region"}, {Title: "Uptime"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestNATGatewayRow_CellsAndOverview(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	r := natgwRow{region: "ap-northeast-1", now: now, res: aws.NATGatewayResource{
		ID: "nat-0abc", Name: "public-a", State: "available", VpcID: "vpc-1", SubnetID: "subnet-1",
		ElasticIP: "203.0.113.10", Tags: map[string]string{"Name": "public-a", "Env": "prod"},
		LaunchTime: now.Add(-(2*24*time.Hour + 3*time.Hour)),
	}}

	wantCells := []string{"public-a", "available", "nat-0abc", "vpc-1", "203.0.113.10", "ap-northeast-1", "2d 3h"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&natgwService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&natgwService{}).Columns()))
	}
	// Web UI の natgwOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "nat-0abc"}, {"VPC", "vpc-1"}, {"Subnet", "subnet-1"}, {"Elastic IP", "203.0.113.10"},
		{"Region", "ap-northeast-1"}, {"Uptime", "2d 3h"}, {"Launched", "2026-10-21"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	// State は NATGatewayResource.State の生の値。ResourceState() は available を running に変えるが、それは使わない。
	if r.ID() != "nat-0abc" || r.Name() != "public-a" || r.State() != "available" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Name": "public-a", "Env": "prod"}) {
		t.Errorf("Tags = %v, want NATGatewayResource.Tags", got)
	}
}

// TestNATGatewayRow_EmptyValuesFollowWebUI は LaunchTime がゼロ値のとき Uptime (列と Overview) と Launched (Overview) が dash、
// Name、VPC、Elastic IP は空文字列になることを確かめる。
func TestNATGatewayRow_EmptyValuesFollowWebUI(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	r := natgwRow{region: "ap-northeast-1", now: now, res: aws.NATGatewayResource{ID: "nat-1", State: "pending"}}

	wantCells := []string{"", "pending", "nat-1", "", "", "ap-northeast-1", dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "nat-1"}, {"VPC", ""}, {"Subnet", ""}, {"Elastic IP", ""}, {"Region", "ap-northeast-1"},
		{"Uptime", dash}, {"Launched", dash},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil when the resource has none", r.Tags())
	}
	// Name タグの無い資源では Name() も空で、Drawer の見出し (refreshDrawer) も空になる。
	if r.Name() != "" {
		t.Errorf("Name = %q, want empty when the resource has no Name tag", r.Name())
	}
}

// TestNATGatewayService_LoadPassesNowToRows は Load が now を各行に渡し、Uptime が now と LaunchTime から
// 計算されることを確かめる (now が渡らないと実資源の Uptime が 0h 0m になる)。
func TestNATGatewayService_LoadPassesNowToRows(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	svc := &natgwService{
		list: func(context.Context, string, string) ([]aws.NATGatewayResource, error) {
			return []aws.NATGatewayResource{{ID: "nat-1", State: "available", LaunchTime: now.Add(-(2*24*time.Hour + 3*time.Hour))}}, nil
		},
		now: func() time.Time { return now },
	}
	rows, err := svc.Load(context.Background(), "p", "ap-northeast-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if got := rows[0].Cells()[6]; got != "2d 3h" {
		t.Errorf("Uptime = %q, want \"2d 3h\" computed from the injected now", got)
	}
}

func TestNATGatewayService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewNATGatewayService(func(context.Context, string, string) ([]aws.NATGatewayResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list NAT gateways: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewNATGatewayService(func(_ context.Context, profile, region string) ([]aws.NATGatewayResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.NATGatewayResource{{ID: "nat-1", Name: "a"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "nat-1" || rows[0].Name() != "a" || rows[0].Overview()[4].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[4].Value)
	}
}
