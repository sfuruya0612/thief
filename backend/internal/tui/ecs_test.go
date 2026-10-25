package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestECSService_IDTitleAndColumns(t *testing.T) {
	svc := NewECSService(nil)
	if svc.ID() != "ecs" || svc.Title() != "ECS" {
		t.Errorf("ID/Title = %q/%q, want ecs/ECS", svc.ID(), svc.Title())
	}
	// ECS Exec は issue 0224 で足すまで無効。
	if svc.SupportsSession() {
		t.Error("SupportsSession must be false until ECS Exec is added")
	}
	// Web UI の ecsColumns の header の並び。
	want := []Column{{Title: "Cluster"}, {Title: "State"}, {Title: "Active svc"}, {Title: "Running"}, {Title: "Pending"}, {Title: "Registered EC2"}, {Title: "Region"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestECSRow_CellsAndOverview(t *testing.T) {
	r := ecsRow{region: "ap-northeast-1", res: aws.ECSResource{
		ID: "arn:aws:ecs:ap-northeast-1:123456789012:cluster/demo", Name: "demo", State: "active",
		ActiveServices: 3, RunningTasks: 12, PendingTasks: 1, RegisteredEC2: 2, Tags: map[string]string{"Env": "prod"},
	}}

	wantCells := []string{"demo", "active", "3", "12", "1", "2", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&ecsService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&ecsService{}).Columns()))
	}
	// Web UI の ecsOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "arn:aws:ecs:ap-northeast-1:123456789012:cluster/demo"},
		{"Active services", "3"}, {"Running tasks", "12"}, {"Pending tasks", "1"}, {"Registered EC2", "2"},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "arn:aws:ecs:ap-northeast-1:123456789012:cluster/demo" || r.Name() != "demo" {
		t.Errorf("accessors mismatch: %q %q", r.ID(), r.Name())
	}
	// State は生の active のまま。ResourceState() (NormalizeState) の running にはしない。
	if r.State() != "active" {
		t.Errorf("State = %q, want active", r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod"}) {
		t.Errorf("Tags = %v, want ECSResource.Tags", got)
	}
}

// TestECSRow_EmptyValuesFollowWebUI は Web UI の ecsColumns と ecsOverviewRows に dash を描画する項目が
// 無く、件数が 0 なら "0"、State が空なら空文字列のままになることを確かめる。タグが無いクラスタの Tags は nil。
func TestECSRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := ecsRow{region: "ap-northeast-1", res: aws.ECSResource{ID: "arn:cluster", Name: "demo"}}

	wantCells := []string{"demo", "", "0", "0", "0", "0", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "arn:cluster"}, {"Active services", "0"}, {"Running tasks", "0"}, {"Pending tasks", "0"},
		{"Registered EC2", "0"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if got := r.Tags(); len(got) != 0 {
		t.Errorf("Tags = %v, want empty", got)
	}
}

func TestECSService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewECSService(func(context.Context, string, string) ([]aws.ECSResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list ECS clusters: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewECSService(func(_ context.Context, profile, region string) ([]aws.ECSResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.ECSResource{{ID: "arn:cluster", Name: "demo", State: "active"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:cluster" || rows[0].Name() != "demo" || rows[0].Cells()[6] != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Cells()[6])
	}
	if rows[0].State() != "active" {
		t.Errorf("state should be the raw value, got %q", rows[0].State())
	}
}
