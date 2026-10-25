package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestRDSService_IDTitleAndColumns(t *testing.T) {
	svc := NewRDSService(nil)
	if svc.ID() != "rds" || svc.Title() != "RDS" {
		t.Errorf("ID/Title = %q/%q, want rds/RDS", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("RDS has no interactive session")
	}
	// Web UI の rdsColumns の header の並び。
	want := []Column{
		{Title: "Identifier"}, {Title: "State"}, {Title: "Engine"}, {Title: "Engine Version"}, {Title: "Cluster"},
		{Title: "Class"}, {Title: "Region"}, {Title: "MultiAZ"}, {Title: "Endpoint"}, {Title: "Uptime"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestRDSRow_CellsAndOverview(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	r := rdsRow{region: "ap-northeast-1", now: now, res: aws.RDSResource{
		ID: "db-1", Name: "db-1", State: "available", Engine: "aurora-mysql", EngineVersion: "8.0.mysql_aurora.3.05.2",
		Class: "db.r6g.large", MultiAZ: true, Endpoint: "db-1.abc.ap-northeast-1.rds.amazonaws.com", Port: 3306,
		VpcID: "vpc-1", ClusterID: "cluster-1", Tags: map[string]string{"Env": "prod"},
		LaunchTime: now.Add(-(2*24*time.Hour + 3*time.Hour)),
	}}

	wantCells := []string{
		"db-1", "available", "aurora-mysql", "8.0.mysql_aurora.3.05.2", "cluster-1", "db.r6g.large", "ap-northeast-1",
		check, "db-1.abc.ap-northeast-1.rds.amazonaws.com", "2d 3h",
	}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&rdsService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&rdsService{}).Columns()))
	}
	// Web UI の rdsOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "db-1"}, {"Engine", "aurora-mysql"}, {"Engine version", "8.0.mysql_aurora.3.05.2"},
		{"Cluster", "cluster-1"}, {"Class", "db.r6g.large"}, {"Multi-AZ", "yes"},
		{"Endpoint", "db-1.abc.ap-northeast-1.rds.amazonaws.com"}, {"Port", "3306"}, {"VPC", "vpc-1"},
		{"Region", "ap-northeast-1"}, {"Uptime", "2d 3h"}, {"Launched", "2026-10-21"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	// State は RDSResource.State の生の値。ResourceState() は available を running に変えるが、それは使わない。
	if r.ID() != "db-1" || r.Name() != "db-1" || r.State() != "available" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod"}) {
		t.Errorf("Tags = %v, want RDSResource.Tags", got)
	}
}

// TestRDSRow_EmptyAndFalseValuesFollowWebUI は MultiAZ が偽のとき列が dash、Overview が no になり、
// Cluster と Uptime (列と Overview) と Launched (Overview) が空のとき dash、Endpoint と VPC は空文字列、
// Port は 0 のままになることを確かめる。
func TestRDSRow_EmptyAndFalseValuesFollowWebUI(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	// Tags は backend の tagsToMapFunc と同じく、タグが無くても nil でなく空の map にする。
	r := rdsRow{region: "ap-northeast-1", now: now, res: aws.RDSResource{ID: "db-2", Name: "db-2", State: "stopped", Tags: map[string]string{}}}

	wantCells := []string{"db-2", "stopped", "", "", dash, "", "ap-northeast-1", dash, "", dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "db-2"}, {"Engine", ""}, {"Engine version", ""}, {"Cluster", dash}, {"Class", ""},
		{"Multi-AZ", "no"}, {"Endpoint", ""}, {"Port", "0"}, {"VPC", ""}, {"Region", "ap-northeast-1"},
		{"Uptime", dash}, {"Launched", dash},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if got := r.Tags(); got == nil || len(got) != 0 {
		t.Errorf("Tags = %#v, want the empty map of the resource", got)
	}
}

func TestRDSService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewRDSService(func(context.Context, string, string) ([]aws.RDSResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list RDS instances: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewRDSService(func(_ context.Context, profile, region string) ([]aws.RDSResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.RDSResource{{ID: "db-1", Name: "db-1"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "db-1" || rows[0].Name() != "db-1" || rows[0].Overview()[9].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[9].Value)
	}
}
