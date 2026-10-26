package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestSecretsService_IDTitleAndColumns(t *testing.T) {
	svc := NewSecretsService(nil)
	if svc.ID() != "secrets" || svc.Title() != "Secrets Manager" {
		t.Errorf("ID/Title = %q/%q, want secrets/Secrets Manager", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("Secrets Manager has no interactive session")
	}
	// Web UI の secretColumns の header の並び。State の列は無い。
	want := []Column{{Title: "Name"}, {Title: "Description"}, {Title: "Last changed"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestSecretsRow_CellsAndOverview(t *testing.T) {
	r := secretsRow{region: "ap-northeast-1", res: aws.SecretResource{
		ID: "arn:aws:secretsmanager:ap-northeast-1:123456789012:secret:db-abc123", Name: "db", State: "",
		Description: "database credentials", LastChanged: "2026-10-20T01:02:03Z",
	}}

	wantCells := []string{"db", "database credentials", "2026-10-20T01:02:03Z"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&secretsService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&secretsService{}).Columns()))
	}
	// Web UI の secretOverviewRows から Name を除いた並び。Name は refreshDrawer が先頭に置く。
	wantOverview := []KV{
		{"Description", "database credentials"}, {"Last changed", "2026-10-20T01:02:03Z"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	for _, kv := range r.Overview() {
		if kv.Key == "Name" {
			t.Error("Overview must not contain Name because refreshDrawer prints it first")
		}
	}
	// State は SecretResource.State の生の値 (空)。ResourceState() の固定の active は使わない。
	if r.ID() != "arn:aws:secretsmanager:ap-northeast-1:123456789012:secret:db-abc123" || r.Name() != "db" || r.State() != "" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	// SecretResource にタグは無く、Web UI の Tags タブも空。取得失敗の見出しも出さないので TagsFetchFailer は実装しない。
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil", r.Tags())
	}
	if _, ok := Row(r).(TagsFetchFailer); ok {
		t.Error("secretsRow must not implement TagsFetchFailer")
	}
}

// TestSecretsRow_EmptyValuesFollowWebUI は Description と Last changed が空のとき列と Overview が dash になることを確かめる。
func TestSecretsRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := secretsRow{region: "ap-northeast-1", res: aws.SecretResource{ID: "arn:s", Name: "s"}}

	wantCells := []string{"s", dash, dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{{"Description", dash}, {"Last changed", dash}, {"Region", "ap-northeast-1"}}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestSecretsService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewSecretsService(func(context.Context, string, string) ([]aws.SecretResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list Secrets Manager secrets: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewSecretsService(func(_ context.Context, profile, region string) ([]aws.SecretResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.SecretResource{{ID: "arn:s", Name: "s"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:s" || rows[0].Name() != "s" || rows[0].Overview()[2].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[2].Value)
	}

	// 0 件の一覧はエラーにならず空の行になる。
	svc = NewSecretsService(func(context.Context, string, string) ([]aws.SecretResource, error) {
		return []aws.SecretResource{}, nil
	})
	rows, err = svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 0 {
		t.Errorf("empty list: rows=%v err=%v, want no rows and no error", rows, err)
	}
}
