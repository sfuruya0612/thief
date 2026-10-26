package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestSSMService_IDTitleAndColumns(t *testing.T) {
	svc := NewSSMService(nil)
	if svc.ID() != "ssm" || svc.Title() != "Parameter Store" {
		t.Errorf("ID/Title = %q/%q, want ssm/Parameter Store", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("Parameter Store has no interactive session")
	}
	// Web UI の ssmColumns の header の並び。State の列は無い。
	want := []Column{{Title: "Name"}, {Title: "Type"}, {Title: "Version"}, {Title: "Last modified"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestSSMRow_CellsAndOverview(t *testing.T) {
	r := ssmRow{region: "ap-northeast-1", res: aws.SSMParameterResource{
		ID: "/app/db-password", Name: "/app/db-password", State: "", Type: "SecureString", Tier: "Standard",
		Version: 3, LastModified: "2026-10-20T01:02:03Z",
	}}

	wantCells := []string{"/app/db-password", "SecureString", "3", "2026-10-20T01:02:03Z"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&ssmService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&ssmService{}).Columns()))
	}
	// Web UI の ssmOverviewRows から Name を除いた並び。Name は refreshDrawer が先頭に置く。
	wantOverview := []KV{
		{"Type", "SecureString"}, {"Tier", "Standard"}, {"Version", "3"}, {"Last modified", "2026-10-20T01:02:03Z"},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	for _, kv := range r.Overview() {
		if kv.Key == "Name" {
			t.Error("Overview must not contain Name because refreshDrawer prints it first")
		}
	}
	// State は SSMParameterResource.State の生の値 (空)。ResourceState() の固定の active は使わない。
	if r.ID() != "/app/db-password" || r.Name() != "/app/db-password" || r.State() != "" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	// SSMParameterResource にタグは無く、Web UI の Tags タブも空。取得失敗の見出しも出さないので TagsFetchFailer は実装しない。
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil", r.Tags())
	}
	if _, ok := Row(r).(TagsFetchFailer); ok {
		t.Error("ssmRow must not implement TagsFetchFailer")
	}
}

// TestSSMRow_EmptyValuesFollowWebUI は Last modified が空のとき列と Overview が dash になり、Type と Tier は
// 空文字列のまま、Version は 0 になることを確かめる。
func TestSSMRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := ssmRow{region: "ap-northeast-1", res: aws.SSMParameterResource{ID: "/p", Name: "/p"}}

	wantCells := []string{"/p", "", "0", dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{{"Type", ""}, {"Tier", ""}, {"Version", "0"}, {"Last modified", dash}, {"Region", "ap-northeast-1"}}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestSSMService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewSSMService(func(context.Context, string, string) ([]aws.SSMParameterResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list Parameter Store parameters: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewSSMService(func(_ context.Context, profile, region string) ([]aws.SSMParameterResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.SSMParameterResource{{ID: "/p", Name: "/p"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "/p" || rows[0].Name() != "/p" || rows[0].Overview()[4].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[4].Value)
	}

	// 0 件の一覧はエラーにならず空の行になる。
	svc = NewSSMService(func(context.Context, string, string) ([]aws.SSMParameterResource, error) {
		return []aws.SSMParameterResource{}, nil
	})
	rows, err = svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 0 {
		t.Errorf("empty list: rows=%v err=%v, want no rows and no error", rows, err)
	}
}
