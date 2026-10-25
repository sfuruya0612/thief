package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestCloudFrontService_IDTitleAndColumns(t *testing.T) {
	svc := NewCloudFrontService(nil)
	if svc.ID() != "cloudfront" || svc.Title() != "CloudFront" {
		t.Errorf("ID/Title = %q/%q, want cloudfront/CloudFront", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("CloudFront has no interactive session")
	}
	// Web UI の cloudfrontColumns の header の並び (Region の列は無い)。
	want := []Column{
		{Title: "Distribution"}, {Title: "State"}, {Title: "Domain"}, {Title: "Alternate domains"}, {Title: "Origins"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestCloudFrontRow_CellsAndOverview(t *testing.T) {
	r := cloudfrontRow{res: aws.CloudFrontResource{
		ID: "E1ABC", Name: "static site", State: "deployed", DomainName: "d1abc.cloudfront.net",
		Aliases: []string{"www.example.com", "example.com"}, Origins: []string{"example-bucket.s3.amazonaws.com", "api.example.com"},
		Enabled: true, PriceClass: "PriceClass_All",
	}}

	// Distribution の列は Web UI と同じくディストリビューション ID。Alternate domains と Origins は複数を ", " で結合する。
	wantCells := []string{
		"E1ABC", "deployed", "d1abc.cloudfront.net", "www.example.com, example.com",
		"example-bucket.s3.amazonaws.com, api.example.com",
	}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&cloudfrontService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&cloudfrontService{}).Columns()))
	}
	// Web UI の cloudfrontOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "E1ABC"}, {"Domain", "d1abc.cloudfront.net"}, {"Alternate domains", "www.example.com, example.com"},
		{"Origins", "example-bucket.s3.amazonaws.com, api.example.com"}, {"Enabled", "yes"}, {"Price class", "PriceClass_All"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "E1ABC" || r.Name() != "static site" || r.State() != "deployed" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	// CloudFrontResource にタグは無く、Web UI の Tags タブも空。
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil", r.Tags())
	}
}

// TestCloudFrontRow_EmptyAndFalseValuesFollowWebUI は Aliases と Origins が空のとき列と Overview が dash、
// Enabled が偽のとき Overview が no、Domain と Price class は空文字列になることを確かめる。
func TestCloudFrontRow_EmptyAndFalseValuesFollowWebUI(t *testing.T) {
	r := cloudfrontRow{res: aws.CloudFrontResource{ID: "E2DEF", Name: "E2DEF", State: "in-progress"}}

	wantCells := []string{"E2DEF", "in-progress", "", dash, dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "E2DEF"}, {"Domain", ""}, {"Alternate domains", dash}, {"Origins", dash}, {"Enabled", "no"},
		{"Price class", ""},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestCloudFrontService_LoadWrapsError(t *testing.T) {
	boom := errors.New("boom")
	svc := NewCloudFrontService(func(context.Context, string, string) ([]aws.CloudFrontResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list CloudFront distributions: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	// CloudFront はグローバルだが、一覧関数には他のサービスと同じく profile と region をそのまま渡す。
	var calls []string
	svc = NewCloudFrontService(func(_ context.Context, profile, region string) ([]aws.CloudFrontResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.CloudFrontResource{{ID: "E1ABC", Name: "site"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "E1ABC" || rows[0].Name() != "site" {
		t.Errorf("row = %q %q", rows[0].ID(), rows[0].Name())
	}
}
