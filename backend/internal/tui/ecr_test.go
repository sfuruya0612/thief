package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestECRService_IDTitleAndColumns(t *testing.T) {
	svc := NewECRService(nil)
	if svc.ID() != "ecr" || svc.Title() != "ECR" {
		t.Errorf("ID/Title = %q/%q, want ecr/ECR", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("ECR has no interactive session")
	}
	// Web UI の ecrColumns の header の並び。
	want := []Column{{Title: "Repository"}, {Title: "URI"}, {Title: "Tag mutability"}, {Title: "Scan on push"}, {Title: "Created"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestECRRow_CellsAndOverview(t *testing.T) {
	r := ecrRow{region: "ap-northeast-1", res: aws.ECRRepoResource{
		ID: "arn:aws:ecr:ap-northeast-1:123456789012:repository/app/web", Name: "app/web",
		URI: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/app/web", CreatedAt: "2026-10-01T00:00:00Z",
		ImageTagMutability: "IMMUTABLE", ScanOnPush: true,
	}}

	wantCells := []string{"app/web", "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/app/web", "IMMUTABLE", check, "2026-10-01T00:00:00Z"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&ecrService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&ecrService{}).Columns()))
	}
	// Web UI の ecrOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Repository ARN", "arn:aws:ecr:ap-northeast-1:123456789012:repository/app/web"},
		{"URI", "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/app/web"},
		{"Tag mutability", "IMMUTABLE"}, {"Scan on push", "enabled"}, {"Created", "2026-10-01T00:00:00Z"},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "arn:aws:ecr:ap-northeast-1:123456789012:repository/app/web" || r.Name() != "app/web" {
		t.Errorf("accessors mismatch: %q %q", r.ID(), r.Name())
	}
	// State は ECRRepoResource.State の生の値 (ListECRResources は設定しないので空)。ResourceState() の active ではない。
	if r.State() != "" {
		t.Errorf("State = %q, want empty (raw value, not ResourceState)", r.State())
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil (ECR has no tags in the Web UI)", r.Tags())
	}
}

// TestECRRow_EmptyAndFalseValuesFollowWebUI は Scan on push が偽のとき列が dash、Overview が disabled に
// なり、Created が空のとき列と Overview が dash、URI と Tag mutability は空文字列のままになることを確かめる。
func TestECRRow_EmptyAndFalseValuesFollowWebUI(t *testing.T) {
	r := ecrRow{region: "ap-northeast-1", res: aws.ECRRepoResource{ID: "arn:repo", Name: "app/batch"}}

	wantCells := []string{"app/batch", "", "", dash, dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Repository ARN", "arn:repo"}, {"URI", ""}, {"Tag mutability", ""}, {"Scan on push", "disabled"},
		{"Created", dash}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestECRService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewECRService(func(context.Context, string, string) ([]aws.ECRRepoResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list ECR repositories: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewECRService(func(_ context.Context, profile, region string) ([]aws.ECRRepoResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.ECRRepoResource{{ID: "arn:repo", Name: "app/web"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:repo" || rows[0].Name() != "app/web" || rows[0].Overview()[5].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[5].Value)
	}
}
