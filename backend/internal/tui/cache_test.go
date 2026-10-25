package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestCacheService_IDTitleAndColumns(t *testing.T) {
	svc := NewCacheService(nil)
	// ID は Web UI の serviceMeta.ts と同じ cache (ElastiCacheResource.ServiceName() の elasticache ではない)。
	if svc.ID() != "cache" || svc.Title() != "ElastiCache" {
		t.Errorf("ID/Title = %q/%q, want cache/ElastiCache", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("ElastiCache has no interactive session")
	}
	// Web UI の cacheColumns の header の並び。
	want := []Column{
		{Title: "Cluster"}, {Title: "State"}, {Title: "Engine"}, {Title: "Replication Group"}, {Title: "Node type"},
		{Title: "Region"}, {Title: "AZs"}, {Title: "Nodes"}, {Title: "Endpoint"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestCacheRow_CellsAndOverview(t *testing.T) {
	r := cacheRow{region: "ap-northeast-1", res: aws.ElastiCacheResource{
		ID: "redis-1", Name: "redis-1", State: "available", Engine: "redis", EngineVersion: "7.1.0",
		NodeType: "cache.t4g.micro", NumNodes: 2, Endpoint: "redis-1.abc.cfg.apne1.cache.amazonaws.com", Port: 6379,
		ReplicationGroupID: "rg-1", NodeAvailabilityZones: []string{"ap-northeast-1a", "ap-northeast-1c"},
	}}

	wantCells := []string{
		"redis-1", "available", "redis", "rg-1", "cache.t4g.micro", "ap-northeast-1", "ap-northeast-1a, ap-northeast-1c",
		"2", "redis-1.abc.cfg.apne1.cache.amazonaws.com",
	}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&cacheService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&cacheService{}).Columns()))
	}
	// Web UI の cacheOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "redis-1"}, {"Engine", "redis"}, {"Engine version", "7.1.0"}, {"Replication group", "rg-1"},
		{"Node type", "cache.t4g.micro"}, {"Nodes", "2"}, {"AZs", "ap-northeast-1a, ap-northeast-1c"},
		{"Endpoint", "redis-1.abc.cfg.apne1.cache.amazonaws.com"}, {"Port", "6379"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	// State は ElastiCacheResource.State の生の値。ResourceState() は available を running に変えるが、それは使わない。
	if r.ID() != "redis-1" || r.Name() != "redis-1" || r.State() != "available" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil (ElastiCache has no tags in the Web UI)", r.Tags())
	}
}

// TestCacheRow_EmptyValuesFollowWebUI は Replication Group と AZs が空のとき列と Overview が dash になり、
// Endpoint、Engine、Node type は空文字列、Nodes と Port は 0 のままになることを確かめる。
func TestCacheRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := cacheRow{region: "ap-northeast-1", res: aws.ElastiCacheResource{ID: "c", Name: "c"}}

	wantCells := []string{"c", "", "", dash, "", "ap-northeast-1", dash, "0", ""}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "c"}, {"Engine", ""}, {"Engine version", ""}, {"Replication group", dash}, {"Node type", ""},
		{"Nodes", "0"}, {"AZs", dash}, {"Endpoint", ""}, {"Port", "0"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestCacheService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewCacheService(func(context.Context, string, string) ([]aws.ElastiCacheResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list ElastiCache clusters: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewCacheService(func(_ context.Context, profile, region string) ([]aws.ElastiCacheResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.ElastiCacheResource{{ID: "c", Name: "c"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "c" || rows[0].Name() != "c" || rows[0].Overview()[9].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[9].Value)
	}
}
