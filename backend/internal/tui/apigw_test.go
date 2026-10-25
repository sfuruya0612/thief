package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestAPIGatewayService_IDTitleAndColumns(t *testing.T) {
	svc := NewAPIGatewayService(nil)
	if svc.ID() != "apigw" || svc.Title() != "API Gateway" {
		t.Errorf("ID/Title = %q/%q, want apigw/API Gateway", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("API Gateway has no interactive session")
	}
	// Web UI の apigwColumns の header の並び。
	want := []Column{
		{Title: "API"}, {Title: "State"}, {Title: "Type"}, {Title: "Stage"}, {Title: "Endpoint"}, {Title: "Region"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestAPIGatewayRow_CellsAndOverview(t *testing.T) {
	r := apigwRow{region: "ap-northeast-1", res: aws.APIGatewayResource{
		ID: "a1b2c3", Name: "orders", State: "active", Type: "REST", Stage: "prod",
		Endpoint: "https://a1b2c3.execute-api.ap-northeast-1.amazonaws.com/prod", Tags: map[string]string{"Env": "prod", "Team": "api"},
	}}

	wantCells := []string{
		"orders", "active", "REST", "prod", "https://a1b2c3.execute-api.ap-northeast-1.amazonaws.com/prod", "ap-northeast-1",
	}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&apigwService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&apigwService{}).Columns()))
	}
	// Web UI の apigwOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "a1b2c3"}, {"Type", "REST"}, {"Stage", "prod"},
		{"Endpoint", "https://a1b2c3.execute-api.ap-northeast-1.amazonaws.com/prod"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "a1b2c3" || r.Name() != "orders" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod", "Team": "api"}) {
		t.Errorf("Tags = %v, want APIGatewayResource.Tags", got)
	}
}

// TestAPIGatewayRow_EmptyValuesFollowWebUI は Stage と Endpoint が空のとき、Web UI と同じく dash にせず
// 空文字列のままになることを確かめる。
func TestAPIGatewayRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := apigwRow{region: "ap-northeast-1", res: aws.APIGatewayResource{ID: "x9", Name: "ws", State: "active", Type: "WebSocket"}}

	wantCells := []string{"ws", "active", "WebSocket", "", "", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "x9"}, {"Type", "WebSocket"}, {"Stage", ""}, {"Endpoint", ""}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil when the resource has none", r.Tags())
	}
}

func TestAPIGatewayService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewAPIGatewayService(func(context.Context, string, string) ([]aws.APIGatewayResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list API Gateway APIs: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewAPIGatewayService(func(_ context.Context, profile, region string) ([]aws.APIGatewayResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.APIGatewayResource{{ID: "a1", Name: "api"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "a1" || rows[0].Name() != "api" || rows[0].Overview()[4].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[4].Value)
	}
}
