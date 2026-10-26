package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestCFNService_IDTitleAndColumns(t *testing.T) {
	svc := NewCFNService(nil, nil)
	if svc.ID() != "cfn" || svc.Title() != "CloudFormation" {
		t.Errorf("ID/Title = %q/%q, want cfn/CloudFormation", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("CloudFormation has no interactive session")
	}
	// Web UI の cfnColumns の header の並び。
	want := []Column{{Title: "Stack"}, {Title: "State"}, {Title: "Drift"}, {Title: "Created"}, {Title: "Updated"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestCFNRow_CellsAndOverview(t *testing.T) {
	r := cfnRow{region: "ap-northeast-1", res: aws.CFNStackResource{
		ID: "arn:aws:cloudformation:ap-northeast-1:123456789012:stack/app/uuid", Name: "app", State: "CREATE_COMPLETE",
		CreationTime: "2026-10-20T01:02:03Z", LastUpdatedTime: "2026-10-21T04:05:06Z", DriftStatus: "IN_SYNC",
	}}

	wantCells := []string{"app", "CREATE_COMPLETE", "IN_SYNC", "2026-10-20T01:02:03Z", "2026-10-21T04:05:06Z"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&cfnService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&cfnService{}).Columns()))
	}
	// Web UI の cfnOverviewRows から State を除いた並び。State は refreshDrawer が Name の次に置く。
	wantOverview := []KV{
		{"Stack ID", "arn:aws:cloudformation:ap-northeast-1:123456789012:stack/app/uuid"},
		{"Drift status", "IN_SYNC"}, {"Created", "2026-10-20T01:02:03Z"}, {"Updated", "2026-10-21T04:05:06Z"},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	for _, kv := range r.Overview() {
		if kv.Key == "Name" || kv.Key == "State" {
			t.Errorf("Overview must not contain %s because refreshDrawer prints it first", kv.Key)
		}
	}
	// State は StackStatus の生の値のまま。NormalizeState の変換は掛けない。
	if r.ID() != "arn:aws:cloudformation:ap-northeast-1:123456789012:stack/app/uuid" || r.Name() != "app" || r.State() != "CREATE_COMPLETE" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	// 一覧はタグを持たないので Tags() は nil。タグは FetchTags で取る。
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil", r.Tags())
	}
}

// TestCFNRow_EmptyValuesFollowWebUI は Created と Updated が空のとき列と Overview が dash になることを確かめる。
// Drift は一覧関数が既定で NOT_CHECKED を入れるため、空の扱いは無い。
func TestCFNRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := cfnRow{region: "ap-northeast-1", res: aws.CFNStackResource{ID: "arn:s", Name: "s", State: "REVIEW_IN_PROGRESS", DriftStatus: "NOT_CHECKED"}}

	wantCells := []string{"s", "REVIEW_IN_PROGRESS", "NOT_CHECKED", dash, dash}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{{"Stack ID", "arn:s"}, {"Drift status", "NOT_CHECKED"}, {"Created", dash}, {"Updated", dash}, {"Region", "ap-northeast-1"}}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
}

func TestCFNService_LoadWrapsErrorAndSetsProfileAndRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewCFNService(func(context.Context, string, string) ([]aws.CFNStackResource, error) { return nil, boom }, nil)
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list CloudFormation stacks: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewCFNService(func(_ context.Context, profile, region string) ([]aws.CFNStackResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.CFNStackResource{{ID: "arn:s", Name: "s"}}, nil
	}, nil)
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:s" || rows[0].Name() != "s" || rows[0].Overview()[4].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[4].Value)
	}
	// Load が作る行は TagFetcher を実装し、Load に渡した profile と region で detail を呼ぶ。
	if _, ok := rows[0].(TagFetcher); !ok {
		t.Fatalf("row %T must implement TagFetcher", rows[0])
	}

	// 0 件の一覧はエラーにならず空の行 (nil ではない) になる。
	svc = NewCFNService(func(context.Context, string, string) ([]aws.CFNStackResource, error) {
		return []aws.CFNStackResource{}, nil
	}, nil)
	rows, err = svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || rows == nil || len(rows) != 0 {
		t.Errorf("empty list: rows=%#v err=%v, want an empty non-nil slice and no error", rows, err)
	}
}

// TestCFNRow_FetchTagsCallsDetailWithStackName は FetchTags が行の profile、region、スタック名で detail を呼び、
// CFNStackDetail.Tags を返し、失敗をラップすることを確かめる。
func TestCFNRow_FetchTagsCallsDetailWithStackName(t *testing.T) {
	var calls []string
	detail := func(_ context.Context, profile, region, stackName string) (*aws.CFNStackDetail, error) {
		calls = append(calls, profile+"/"+region+"/"+stackName)
		return &aws.CFNStackDetail{StackName: stackName, Tags: map[string]string{"Env": "prod", "Name": stackName}}, nil
	}
	svc := NewCFNService(func(context.Context, string, string) ([]aws.CFNStackResource, error) {
		return []aws.CFNStackResource{{ID: "arn:app", Name: "app"}}, nil
	}, detail)
	rows, err := svc.Load(context.Background(), "prof", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	tags, err := rows[0].(TagFetcher).FetchTags(context.Background())
	if err != nil {
		t.Fatalf("FetchTags: %v", err)
	}
	if want := map[string]string{"Env": "prod", "Name": "app"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
	if want := []string{"prof/us-west-2/app"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("detail calls = %q, want %q", calls, want)
	}

	boom := errors.New("boom")
	r := cfnRow{profile: "prof", region: "r", res: aws.CFNStackResource{ID: "arn:app", Name: "app"},
		detail: func(context.Context, string, string, string) (*aws.CFNStackDetail, error) { return nil, boom }}
	tags, err = r.FetchTags(context.Background())
	if tags != nil || !errors.Is(err, boom) || err.Error() != "describe CloudFormation stack app: boom" {
		t.Fatalf("tags=%v err=%v, want nil tags and wrapped boom with the stack name", tags, err)
	}

	// タグの無いスタックは空の map (nil でも可) を返し、エラーにしない。
	for name, detailTags := range map[string]map[string]string{"empty": {}, "nil": nil} {
		r.detail = func(context.Context, string, string, string) (*aws.CFNStackDetail, error) {
			return &aws.CFNStackDetail{StackName: "app", Tags: detailTags}, nil
		}
		tags, err = r.FetchTags(context.Background())
		if err != nil || len(tags) != 0 {
			t.Fatalf("%s tags: tags=%v err=%v, want no tags and nil error", name, tags, err)
		}
	}
}

// TestCFNRow_FetchTagsWithoutDetailFails は detail が無い行と、エラー無しで nil の詳細を返す detail で、
// FetchTags が panic せずにエラーを返すことを確かめる (配線漏れを Tags (fetch failed) の表示にするため)。
func TestCFNRow_FetchTagsWithoutDetailFails(t *testing.T) {
	tests := []struct {
		name   string
		detail func(context.Context, string, string, string) (*aws.CFNStackDetail, error)
	}{
		{name: "nil detail func", detail: nil},
		{name: "nil detail result", detail: func(context.Context, string, string, string) (*aws.CFNStackDetail, error) { return nil, nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := cfnRow{profile: "p", region: "r", res: aws.CFNStackResource{ID: "arn:app", Name: "app"}, detail: tt.detail}
			tags, err := r.FetchTags(context.Background())
			if tags != nil || !errors.Is(err, errCFNDetailUnavailable) {
				t.Fatalf("tags=%v err=%v, want nil tags and errCFNDetailUnavailable", tags, err)
			}
			if err.Error() != "describe CloudFormation stack app: stack detail is not available" {
				t.Errorf("err = %q, want the stack name in the message", err)
			}
		})
	}
}
