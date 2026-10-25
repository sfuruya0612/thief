package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestLambdaService_IDTitleAndColumns(t *testing.T) {
	svc := NewLambdaService(nil)
	if svc.ID() != "lambda" || svc.Title() != "Lambda" {
		t.Errorf("ID/Title = %q/%q, want lambda/Lambda", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("Lambda has no interactive session")
	}
	// Web UI の lambdaColumns の header の並び。
	want := []Column{{Title: "Function"}, {Title: "State"}, {Title: "Runtime"}, {Title: "Region"}, {Title: "Memory"}, {Title: "Timeout"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestLambdaRow_CellsAndOverview(t *testing.T) {
	r := lambdaRow{region: "ap-northeast-1", res: aws.LambdaResource{
		ID: "arn:aws:lambda:ap-northeast-1:123456789012:function:hello", Name: "hello", State: "active",
		Runtime: "python3.12", MemoryMB: 128, TimeoutSec: 30, Handler: "index.handler",
		Role: "arn:aws:iam::123456789012:role/lambda-role", Tags: map[string]string{"Env": "prod"},
	}}

	wantCells := []string{"hello", "active", "python3.12", "ap-northeast-1", "128 MB", "30s"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&lambdaService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&lambdaService{}).Columns()))
	}
	// Web UI の lambdaOverviewRows の項目名の並び (Name と State は Drawer の描画が先頭に置く)。
	wantOverview := []KV{
		{"Resource ID", "arn:aws:lambda:ap-northeast-1:123456789012:function:hello"},
		{"Runtime", "python3.12"}, {"Memory", "128 MB"}, {"Timeout", "30s"},
		{"Handler", "index.handler"}, {"Role", "arn:aws:iam::123456789012:role/lambda-role"},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "arn:aws:lambda:ap-northeast-1:123456789012:function:hello" || r.Name() != "hello" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod"}) {
		t.Errorf("Tags = %v, want LambdaResource.Tags", got)
	}
}

// TestLambdaRow_EmptyValuesFollowWebUI は Web UI の lambdaColumns と lambdaOverviewRows に dash を
// 描画する項目が無く、Runtime、Handler、Role が空なら空文字列のままで、数値は 0 でも "0 MB" と "0s" に
// なることを確かめる。タグが無い関数の Tags は nil。
func TestLambdaRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := lambdaRow{region: "ap-northeast-1", res: aws.LambdaResource{ID: "arn:fn", Name: "hello", State: "pending"}}

	wantCells := []string{"hello", "pending", "", "ap-northeast-1", "0 MB", "0s"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "arn:fn"}, {"Runtime", ""}, {"Memory", "0 MB"}, {"Timeout", "0s"},
		{"Handler", ""}, {"Role", ""}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if got := r.Tags(); len(got) != 0 {
		t.Errorf("Tags = %v, want empty", got)
	}
}

func TestLambdaService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewLambdaService(func(context.Context, string, string) ([]aws.LambdaResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list Lambda functions: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewLambdaService(func(_ context.Context, profile, region string) ([]aws.LambdaResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.LambdaResource{{ID: "arn:fn", Name: "hello", State: "active"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:fn" || rows[0].Name() != "hello" || rows[0].Cells()[3] != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Cells()[3])
	}
}
