package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestELBService_IDTitleAndColumns(t *testing.T) {
	svc := NewELBService(nil)
	if svc.ID() != "elb" || svc.Title() != "ELB" {
		t.Errorf("ID/Title = %q/%q, want elb/ELB", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("ELB has no interactive session")
	}
	// Web UI の elbColumns の header の並び。
	want := []Column{
		{Title: "Name"}, {Title: "State"}, {Title: "Type"}, {Title: "Scheme"}, {Title: "Region"},
		{Title: "DNS name"}, {Title: "AZs"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestELBRow_CellsAndOverview(t *testing.T) {
	r := elbRow{region: "ap-northeast-1", res: aws.ELBResource{
		ID: "arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:loadbalancer/app/web/abc", Name: "web",
		State: "active", Type: "application", Scheme: "internet-facing", DNSName: "web-1.ap-northeast-1.elb.amazonaws.com",
		VpcID: "vpc-1", AZs: []string{"ap-northeast-1a", "ap-northeast-1c"},
	}}

	wantCells := []string{
		"web", "active", "application", "internet-facing", "ap-northeast-1", "web-1.ap-northeast-1.elb.amazonaws.com",
		"ap-northeast-1a, ap-northeast-1c",
	}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&elbService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&elbService{}).Columns()))
	}
	// Web UI の elbOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:loadbalancer/app/web/abc"},
		{"Type", "application"}, {"Scheme", "internet-facing"}, {"DNS name", "web-1.ap-northeast-1.elb.amazonaws.com"},
		{"VPC", "vpc-1"}, {"AZs", "ap-northeast-1a, ap-northeast-1c"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	// State は ELBResource.State の生の値。ResourceState() は active を running に変えるが、それは使わない。
	if r.ID() != "arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:loadbalancer/app/web/abc" || r.Name() != "web" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	// ELBResource にタグは無く、Web UI の Tags タブも空。
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil", r.Tags())
	}
}

// TestELBRow_EmptyValuesFollowWebUI は AZs が空のとき列と Overview が dash、DNS name と VPC は空文字列になることを確かめる。
func TestELBRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := elbRow{region: "ap-northeast-1", res: aws.ELBResource{ID: "arn:lb", Name: "lb", State: "provisioning"}}

	wantCells := []string{"lb", "provisioning", "", "", "ap-northeast-1", "", dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "arn:lb"}, {"Type", ""}, {"Scheme", ""}, {"DNS name", ""}, {"VPC", ""}, {"AZs", dash},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestELBService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewELBService(func(context.Context, string, string) ([]aws.ELBResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list ELB load balancers: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewELBService(func(_ context.Context, profile, region string) ([]aws.ELBResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.ELBResource{{ID: "arn:lb", Name: "lb"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:lb" || rows[0].Name() != "lb" || rows[0].Overview()[6].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[6].Value)
	}
}
