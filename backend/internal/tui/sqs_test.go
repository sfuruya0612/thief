package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestSQSService_IDTitleAndColumns(t *testing.T) {
	svc := NewSQSService(nil)
	if svc.ID() != "sqs" || svc.Title() != "SQS" {
		t.Errorf("ID/Title = %q/%q, want sqs/SQS", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("SQS has no interactive session")
	}
	// Web UI の sqsColumns の header の並び。
	want := []Column{
		{Title: "Queue"}, {Title: "State"}, {Title: "Type"}, {Title: "Available"}, {Title: "In flight"},
		{Title: "Retention"}, {Title: "Region"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestSQSRow_CellsAndOverview(t *testing.T) {
	r := sqsRow{region: "ap-northeast-1", res: aws.SQSResource{
		ID: "arn:aws:sqs:ap-northeast-1:123456789012:orders.fifo", Name: "orders.fifo", State: "active", Type: "FIFO",
		AvailableMessages: 1234567, InFlight: 12, RetentionDays: 4, Tags: map[string]string{"Env": "prod"},
	}}

	// Available は Web UI の toLocaleString と同じ桁区切り、Retention は列が "<n>d"。
	wantCells := []string{"orders.fifo", "active", "FIFO", "1,234,567", "12", "4d", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&sqsService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&sqsService{}).Columns()))
	}
	// Web UI の sqsOverviewRows の項目名の並び。Retention は Overview では "<n> days"。
	wantOverview := []KV{
		{"Resource ID", "arn:aws:sqs:ap-northeast-1:123456789012:orders.fifo"}, {"Type", "FIFO"},
		{"Messages available", "1,234,567"}, {"Messages in flight", "12"}, {"Retention", "4 days"},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "arn:aws:sqs:ap-northeast-1:123456789012:orders.fifo" || r.Name() != "orders.fifo" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod"}) {
		t.Errorf("Tags = %v, want SQSResource.Tags", got)
	}
	if r.TagsFetchFailed() {
		t.Error("TagsFetchFailed = true, want false when the resource reports success")
	}
}

// TestSQSRow_ZeroValuesFollowWebUI は件数がゼロのとき "0"、Retention が "0d" と "0 days" になることと、
// タグの取得失敗が TagsFetchFailer として伝わることを確かめる。
func TestSQSRow_ZeroValuesFollowWebUI(t *testing.T) {
	r := sqsRow{region: "ap-northeast-1", res: aws.SQSResource{ID: "arn:q", Name: "q", State: "active", Type: "Standard", TagsFetchFailed: true}}

	wantCells := []string{"q", "active", "Standard", "0", "0", "0d", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "arn:q"}, {"Type", "Standard"}, {"Messages available", "0"}, {"Messages in flight", "0"},
		{"Retention", "0 days"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil when the resource has none", r.Tags())
	}
	var row Row = r
	f, ok := row.(TagsFetchFailer)
	if !ok || !f.TagsFetchFailed() {
		t.Errorf("sqsRow must implement TagsFetchFailer and report the failure (ok=%v)", ok)
	}
}

func TestSQSService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewSQSService(func(context.Context, string, string) ([]aws.SQSResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list SQS queues: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewSQSService(func(_ context.Context, profile, region string) ([]aws.SQSResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.SQSResource{{ID: "arn:q", Name: "q"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:q" || rows[0].Name() != "q" || rows[0].Overview()[5].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[5].Value)
	}
}
