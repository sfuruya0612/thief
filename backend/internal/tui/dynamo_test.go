package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestDynamoService_IDTitleAndColumns(t *testing.T) {
	svc := NewDynamoService(nil)
	if svc.ID() != "dynamo" || svc.Title() != "DynamoDB" {
		t.Errorf("ID/Title = %q/%q, want dynamo/DynamoDB", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("DynamoDB has no interactive session")
	}
	// Web UI の dynamoColumns の header の並び。
	want := []Column{{Title: "Table"}, {Title: "State"}, {Title: "Mode"}, {Title: "Items"}, {Title: "Size"}, {Title: "GSI"}, {Title: "Region"}}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestDynamoRow_CellsAndOverview(t *testing.T) {
	r := dynamoRow{region: "ap-northeast-1", res: aws.DynamoResource{
		ID: "arn:aws:dynamodb:ap-northeast-1:123456789012:table/users", Name: "users", State: "active", Mode: "on-demand", ItemCount: 1234567, SizeBytes: 1536, GSICount: 2,
		Tags: map[string]string{"Env": "prod"},
	}}

	wantCells := []string{"users", "active", "on-demand", "1,234,567", "1.5 KB", "2", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&dynamoService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&dynamoService{}).Columns()))
	}
	// Web UI の dynamoOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "arn:aws:dynamodb:ap-northeast-1:123456789012:table/users"}, {"Capacity mode", "on-demand"}, {"Item count", "1,234,567"}, {"Size", "1.5 KB"},
		{"Global secondary indexes", "2"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	// ID はテーブルの ARN で Name と異なる。State は DynamoResource.State の値で、ResourceState() の正規化は使わない。
	if r.ID() != "arn:aws:dynamodb:ap-northeast-1:123456789012:table/users" || r.Name() != "users" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod"}) {
		t.Errorf("Tags = %v, want DynamoResource.Tags", got)
	}
	if r.TagsFetchFailed() {
		t.Error("TagsFetchFailed should follow DynamoResource.TagsFetchFailed (false)")
	}
}

// TestDynamoRow_EmptyValuesFollowWebUI は Web UI の dynamoColumns と dynamoOverviewRows に dash を描画する
// 項目が無く、Mode が空なら空文字列、件数と容量は 0 でも "0" と "0 B" になることを確かめる。
func TestDynamoRow_EmptyValuesFollowWebUI(t *testing.T) {
	// Tags は backend の fetchDynamoTableTags と同じく、タグが無くても nil でなく空の map にする。
	r := dynamoRow{region: "ap-northeast-1", res: aws.DynamoResource{ID: "t", Name: "t", Tags: map[string]string{}}}

	wantCells := []string{"t", "", "", "0", "0 B", "0", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "t"}, {"Capacity mode", ""}, {"Item count", "0"}, {"Size", "0 B"},
		{"Global secondary indexes", "0"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if got := r.Tags(); got == nil || len(got) != 0 {
		t.Errorf("Tags = %#v, want the empty map of the resource", got)
	}
}

// TestDynamoRow_TagsFetchFailed は TagsFetchFailed が DynamoResource.TagsFetchFailed をそのまま返し、
// Row が TagsFetchFailer を満たすことを確かめる。
func TestDynamoRow_TagsFetchFailed(t *testing.T) {
	var row Row = dynamoRow{res: aws.DynamoResource{ID: "t", TagsFetchFailed: true}}
	f, ok := row.(TagsFetchFailer)
	if !ok {
		t.Fatal("dynamoRow must implement TagsFetchFailer")
	}
	if !f.TagsFetchFailed() {
		t.Error("TagsFetchFailed = false, want true")
	}
}

func TestDynamoService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewDynamoService(func(context.Context, string, string) ([]aws.DynamoResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list DynamoDB tables: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewDynamoService(func(_ context.Context, profile, region string) ([]aws.DynamoResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.DynamoResource{{ID: "t", Name: "t"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "t" || rows[0].Name() != "t" || rows[0].Overview()[5].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[5].Value)
	}
}
