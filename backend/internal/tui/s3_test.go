package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestS3Service_IDTitleAndColumns(t *testing.T) {
	svc := NewS3Service(nil)
	if svc.ID() != "s3" || svc.Title() != "S3" {
		t.Errorf("ID/Title = %q/%q, want s3/S3", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("S3 has no interactive session")
	}
	// Web UI の s3Columns の header の並び。
	want := []Column{{Title: "Bucket"}, {Title: "State"}, {Title: "Region"}, {Title: "Created"}, {Title: "Public"}, {Title: "Encryption"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestS3Row_CellsAndOverview(t *testing.T) {
	r := s3Row{region: "ap-northeast-1", res: aws.S3Resource{
		ID: "my-bucket", Name: "my-bucket", Region: "us-west-2", CreatedAt: "2026-10-01T00:00:00Z",
		Public: true, Encryption: "AES256",
	}}

	// Region はバケット固有の S3Resource.Region を優先する (Web UI の s3FromRaw と同じ)。
	wantCells := []string{"my-bucket", "", "us-west-2", "2026-10-01T00:00:00Z", "public", "AES256"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&s3Service{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&s3Service{}).Columns()))
	}
	// Web UI の s3OverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "my-bucket"}, {"Region", "us-west-2"}, {"Created", "2026-10-01T00:00:00Z"},
		{"Public access", "allowed"}, {"Encryption", "AES256"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "my-bucket" || r.Name() != "my-bucket" {
		t.Errorf("accessors mismatch: %q %q", r.ID(), r.Name())
	}
	// State は S3Resource.State の生の値 (ListS3Resources は設定しないので空)。ResourceState() の active ではない。
	if r.State() != "" {
		t.Errorf("State = %q, want empty (raw value, not ResourceState)", r.State())
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil (S3 has no tags in the Web UI)", r.Tags())
	}
}

// TestS3Row_EmptyAndFalseValuesFollowWebUI は Public が偽のとき列が private、Overview が blocked になり、
// Created と Encryption が空のとき列は空文字列、Overview は dash になり、Region はバケット固有の値が無いとき
// 選択中のリージョンになることを確かめる。
func TestS3Row_EmptyAndFalseValuesFollowWebUI(t *testing.T) {
	r := s3Row{region: "ap-northeast-1", res: aws.S3Resource{ID: "b", Name: "b"}}

	wantCells := []string{"b", "", "ap-northeast-1", "", "private", ""}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "b"}, {"Region", "ap-northeast-1"}, {"Created", dash}, {"Public access", "blocked"}, {"Encryption", dash},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestS3Service_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewS3Service(func(context.Context, string, string) ([]aws.S3Resource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list S3 buckets: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewS3Service(func(_ context.Context, profile, region string) ([]aws.S3Resource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.S3Resource{{ID: "b", Name: "b"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "b" || rows[0].Name() != "b" || rows[0].Overview()[1].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[1].Value)
	}
}
