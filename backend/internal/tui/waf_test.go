package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestWAFService_IDTitleAndColumns(t *testing.T) {
	svc := NewWAFService(nil)
	if svc.ID() != "waf" || svc.Title() != "WAF" {
		t.Errorf("ID/Title = %q/%q, want waf/WAF", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("WAF has no interactive session")
	}
	// Web UI の wafColumns の header の並び。
	want := []Column{
		{Title: "Web ACL"}, {Title: "Description"}, {Title: "State"}, {Title: "Scope"}, {Title: "Region"},
		{Title: "Rules"}, {Title: "Associated"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestWAFRow_CellsAndOverview(t *testing.T) {
	r := wafRow{region: "ap-northeast-1", res: aws.WAFResource{
		ID: "acl-1", Name: "web-acl", State: "active", Scope: "REGIONAL", Description: "edge rules",
		RuleCount: 5, AssociatedCount: 3, Tags: map[string]string{"Env": "prod"},
	}}

	wantCells := []string{"web-acl", "edge rules", "active", "REGIONAL", "ap-northeast-1", "5", "3"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&wafService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&wafService{}).Columns()))
	}
	// Web UI の wafOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "acl-1"}, {"Scope", "REGIONAL"}, {"Description", "edge rules"}, {"Rules", "5"},
		{"Associated resources", "3"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "acl-1" || r.Name() != "web-acl" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod"}) {
		t.Errorf("Tags = %v, want WAFResource.Tags", got)
	}
	if r.TagsFetchFailed() {
		t.Error("TagsFetchFailed = true, want false when the resource reports success")
	}
}

// TestWAFRow_EmptyValuesFollowWebUI は Description が空のとき列と Overview が dash になり、CLOUDFRONT の行も
// Region が引数のリージョンのままで、タグの取得失敗が TagsFetchFailer として伝わることを確かめる。
func TestWAFRow_EmptyValuesFollowWebUI(t *testing.T) {
	// Tags は backend の wafACLDetail が取得失敗のときに返すのと同じ空の非 nil map にする。
	r := wafRow{region: "ap-northeast-1", res: aws.WAFResource{ID: "acl-2", Name: "cf-acl", State: "active", Scope: "CLOUDFRONT", Tags: map[string]string{}, TagsFetchFailed: true}}

	wantCells := []string{"cf-acl", dash, "active", "CLOUDFRONT", "ap-northeast-1", "0", "0"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "acl-2"}, {"Scope", "CLOUDFRONT"}, {"Description", dash}, {"Rules", "0"},
		{"Associated resources", "0"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.Tags() == nil || len(r.Tags()) != 0 {
		t.Errorf("Tags = %v, want the resource's empty map as is", r.Tags())
	}
	var row Row = r
	f, ok := row.(TagsFetchFailer)
	if !ok || !f.TagsFetchFailed() {
		t.Errorf("wafRow must implement TagsFetchFailer and report the failure (ok=%v)", ok)
	}
}

// TestWAFRow_EmptyStateAndScopeStayEmpty は State と Scope が空のとき、列が dash ではなく空文字列のままになることを
// 確かめる (dash になるのは Description だけ)。
func TestWAFRow_EmptyStateAndScopeStayEmpty(t *testing.T) {
	r := wafRow{region: "ap-northeast-1", res: aws.WAFResource{ID: "acl-3", Name: "bare-acl"}}

	wantCells := []string{"bare-acl", dash, "", "", "ap-northeast-1", "0", "0"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "acl-3"}, {"Scope", ""}, {"Description", dash}, {"Rules", "0"},
		{"Associated resources", "0"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.State() != "" {
		t.Errorf("State = %q, want empty", r.State())
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil when the resource has none", r.Tags())
	}
}

// TestWAFRow_AssociatedFetchFailedFollowsIssue0222 は Associated の取得失敗で、列と Overview の件数の後ろに
// fetch failed が付くことを確かめる。
func TestWAFRow_AssociatedFetchFailedFollowsIssue0222(t *testing.T) {
	r := wafRow{region: "ap-northeast-1", res: aws.WAFResource{ID: "acl-3", Name: "acl", State: "active", Scope: "REGIONAL", RuleCount: 1, AssociatedCount: 3, AssociatedCountFetchFailed: true}}

	if got := r.Cells()[6]; got != "3 (fetch failed)" {
		t.Errorf("Associated cell = %q, want \"3 (fetch failed)\"", got)
	}
	if got := r.Overview()[4]; got != (KV{"Associated resources", "3 (fetch failed)"}) {
		t.Errorf("Associated resources = %q, want \"3 (fetch failed)\"", got)
	}
}

func TestWAFService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewWAFService(func(context.Context, string, string) ([]aws.WAFResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list WAF web ACLs: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewWAFService(func(_ context.Context, profile, region string) ([]aws.WAFResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.WAFResource{{ID: "acl-1", Name: "web-acl"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "acl-1" || rows[0].Name() != "web-acl" || rows[0].Overview()[5].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[5].Value)
	}

	// 0 件の一覧はエラーにならず空の行になる。
	svc = NewWAFService(func(context.Context, string, string) ([]aws.WAFResource, error) { return []aws.WAFResource{}, nil })
	rows, err = svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 0 {
		t.Errorf("empty list: rows=%v err=%v, want no rows and no error", rows, err)
	}
}
