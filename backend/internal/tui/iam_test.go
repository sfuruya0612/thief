package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestIAMService_IDTitleAndColumns(t *testing.T) {
	svc := NewIAMService(nil)
	if svc.ID() != "iam" || svc.Title() != "IAM" {
		t.Errorf("ID/Title = %q/%q, want iam/IAM", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("IAM has no interactive session")
	}
	// Web UI の iamColumns の header の並び。State の列の見出しは Activity。
	want := []Column{
		{Title: "Name"}, {Title: "Kind"}, {Title: "MFA"}, {Title: "Activity"}, {Title: "Last active"},
		{Title: "Policies"}, {Title: "Groups"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestIAMRow_CellsAndOverview(t *testing.T) {
	r := iamRow{res: aws.IAMResource{
		ID: "AIDAEXAMPLE", Name: "alice", State: "", ARN: "arn:aws:iam::123456789012:user/alice", Kind: "user",
		MFAEnabled: true, LastActivity: "2026-10-20T01:02:03Z",
		Groups: []string{"admins"}, Policies: []string{"p1", "p2"},
	}}

	// MFA は列が check、Activity は IAMResource.State の生の値 (空)。Policies と Groups は件数。
	wantCells := []string{"alice", "user", check, "", "2026-10-20T01:02:03Z", "2", "1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&iamService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&iamService{}).Columns()))
	}
	// Web UI の iamOverviewRows の項目名の並び。MFA は Overview では enabled。Region は無い。
	wantOverview := []KV{
		{"Resource ID", "AIDAEXAMPLE"}, {"ARN", "arn:aws:iam::123456789012:user/alice"}, {"Kind", "user"},
		{"MFA", "enabled"}, {"Last active", "2026-10-20T01:02:03Z"}, {"Policies attached", "2"}, {"Groups", "1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	// State は IAMResource.State の生の値。ResourceState() は固定で active を返すが、Web UI は表示しないので使わない。
	if r.ID() != "AIDAEXAMPLE" || r.Name() != "alice" || r.State() != "" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	// IAMResource にタグは無く、Web UI の Tags タブも空。
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil", r.Tags())
	}
	var row Row = r
	if _, ok := row.(TagsFetchFailer); ok {
		t.Error("iamRow must not implement TagsFetchFailer because IAM has no tags")
	}
}

// TestIAMRow_EmptyAndFalseValuesFollowWebUI は MFA が偽のロードで列が cross、Overview が disabled になり、
// Last active が空のとき列と Overview が dash、Policies と Groups が無いとき 0 になることを確かめる。
func TestIAMRow_EmptyAndFalseValuesFollowWebUI(t *testing.T) {
	r := iamRow{res: aws.IAMResource{ID: "AROAEXAMPLE", Name: "deploy", ARN: "arn:aws:iam::123456789012:role/deploy", Kind: "role"}}

	wantCells := []string{"deploy", "role", cross, "", dash, "0", "0"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "AROAEXAMPLE"}, {"ARN", "arn:aws:iam::123456789012:role/deploy"}, {"Kind", "role"},
		{"MFA", "disabled"}, {"Last active", dash}, {"Policies attached", "0"}, {"Groups", "0"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

// TestIAMRow_FetchFailedFollowsIssue0222 は項目ごとの取得失敗の表記を確かめる。MFA は列では Web UI が値を
// 出さないので "fetch failed"、Overview では値の横に付くので "disabled (fetch failed)" (ListIAMResources は
// 取得失敗のとき MFAEnabled を偽のままにする)。Policies と Groups は列と Overview のどちらも件数の後ろに付く。
func TestIAMRow_FetchFailedFollowsIssue0222(t *testing.T) {
	r := iamRow{res: aws.IAMResource{
		ID: "AIDAEXAMPLE", Name: "bob", ARN: "arn:aws:iam::123456789012:user/bob", Kind: "user",
		MFAEnabledFetchFailed: true, PoliciesFetchFailed: true, GroupsFetchFailed: true,
		Policies: []string{"p1", "p2"}, Groups: []string{"g1"},
	}}

	wantCells := []string{"bob", "user", "fetch failed", "", dash, "2 (fetch failed)", "1 (fetch failed)"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "AIDAEXAMPLE"}, {"ARN", "arn:aws:iam::123456789012:user/bob"}, {"Kind", "user"},
		{"MFA", "disabled (fetch failed)"}, {"Last active", dash},
		{"Policies attached", "2 (fetch failed)"}, {"Groups", "1 (fetch failed)"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

// TestIAMRow_FetchFailedWithBackendShape は backend の iamFromUser が取得失敗のときに作る形 (MFAEnabled は偽のまま、
// Policies と Groups は nil) で、列と Overview が disabled (fetch failed) と 0 (fetch failed) になることを確かめる。
func TestIAMRow_FetchFailedWithBackendShape(t *testing.T) {
	r := iamRow{res: aws.IAMResource{
		ID: "AIDAEXAMPLE", Name: "bob", ARN: "arn:aws:iam::123456789012:user/bob", Kind: "user",
		MFAEnabledFetchFailed: true, PoliciesFetchFailed: true, GroupsFetchFailed: true,
	}}

	wantCells := []string{"bob", "user", "fetch failed", "", dash, "0 (fetch failed)", "0 (fetch failed)"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "AIDAEXAMPLE"}, {"ARN", "arn:aws:iam::123456789012:user/bob"}, {"Kind", "user"},
		{"MFA", "disabled (fetch failed)"}, {"Last active", dash},
		{"Policies attached", "0 (fetch failed)"}, {"Groups", "0 (fetch failed)"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

// TestIAMRow_MFAEnabledFetchFailed は MFAEnabled が真で MFAEnabledFetchFailed も真のとき Overview が
// enabled (fetch failed) になることを確かめる。backend の iamFromUser は失敗時に MFAEnabled を偽のままにするため
// この組合せは作らないが、mfaOverview の分岐 (issue 0222 の設計判断の「enabled (fetch failed) か disabled (fetch failed)」) を固定する。
func TestIAMRow_MFAEnabledFetchFailed(t *testing.T) {
	r := iamRow{res: aws.IAMResource{ID: "AIDAEXAMPLE", Name: "bob", Kind: "user", MFAEnabled: true, MFAEnabledFetchFailed: true}}
	if got := r.Cells()[2]; got != "fetch failed" {
		t.Errorf("MFA cell = %q, want fetch failed", got)
	}
	if got := r.Overview()[3]; got != (KV{"MFA", "enabled (fetch failed)"}) {
		t.Errorf("MFA overview = %v, want enabled (fetch failed)", got)
	}
}

func TestIAMService_LoadWrapsErrorAndPassesRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewIAMService(func(context.Context, string, string) ([]aws.IAMResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list IAM users and roles: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	// ListIAMResources はリージョンを無視するが、Load は他のサービスと同じく選択中のリージョンを渡す。
	var calls []string
	svc = NewIAMService(func(_ context.Context, profile, region string) ([]aws.IAMResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.IAMResource{{ID: "AIDAEXAMPLE", Name: "alice", Kind: "user"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "AIDAEXAMPLE" || rows[0].Name() != "alice" || len(rows[0].Overview()) != 7 {
		t.Errorf("row = %q %q overview=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview())
	}

	// 0 件の一覧はエラーにならず空の行になる。
	svc = NewIAMService(func(context.Context, string, string) ([]aws.IAMResource, error) { return []aws.IAMResource{}, nil })
	rows, err = svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 0 {
		t.Errorf("empty list: rows=%v err=%v, want no rows and no error", rows, err)
	}
}
