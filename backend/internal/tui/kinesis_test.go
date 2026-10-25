package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestKinesisService_IDTitleAndColumns(t *testing.T) {
	svc := NewKinesisService(nil)
	if svc.ID() != "kinesis" || svc.Title() != "Kinesis" {
		t.Errorf("ID/Title = %q/%q, want kinesis/Kinesis", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("Kinesis has no interactive session")
	}
	// Web UI の kinesisColumns の header の並び。
	want := []Column{
		{Title: "Stream"}, {Title: "State"}, {Title: "Mode"}, {Title: "Shards"}, {Title: "Retention"},
		{Title: "Encryption"}, {Title: "Region"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestKinesisRow_CellsAndOverview(t *testing.T) {
	r := kinesisRow{region: "ap-northeast-1", res: aws.KinesisResource{
		ID: "arn:aws:kinesis:ap-northeast-1:123456789012:stream/events", Name: "events", State: "active",
		Mode: "provisioned", ShardCount: 4, RetentionHours: 168, EncryptionType: "KMS", Tags: map[string]string{"Env": "prod", "Team": "data"},
	}}

	// Shards は区切り無しの 10 進、Retention は "<n>h"。
	wantCells := []string{"events", "active", "provisioned", "4", "168h", "KMS", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&kinesisService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&kinesisService{}).Columns()))
	}
	// Web UI の kinesisOverviewRows の項目名の並び。
	wantOverview := []KV{
		{"Resource ID", "arn:aws:kinesis:ap-northeast-1:123456789012:stream/events"}, {"Capacity mode", "provisioned"},
		{"Shards", "4"}, {"Retention", "168h"}, {"Encryption", "KMS"}, {"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	// State は KinesisResource.State の生の値。ResourceState() は active を running に変えるが、それは使わない。
	if r.ID() != "arn:aws:kinesis:ap-northeast-1:123456789012:stream/events" || r.Name() != "events" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	// Tags は資源の Tags をそのまま返す (ListKinesisResources はタグを設定しないため実際には空だが、配線は非空で確かめる)。
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod", "Team": "data"}) {
		t.Errorf("Tags = %v, want KinesisResource.Tags", got)
	}
}

// TestKinesisRow_OnDemandModeIsRaw は Mode の on-demand が列と Overview にそのまま出ることを確かめる
// (floci の CreateStream は provisioned しか作らないため、ここで固定する)。
func TestKinesisRow_OnDemandModeIsRaw(t *testing.T) {
	r := kinesisRow{region: "ap-northeast-1", res: aws.KinesisResource{ID: "arn:o", Name: "o", State: "active", Mode: "on-demand", ShardCount: 1, RetentionHours: 24}}

	if got := r.Cells()[2]; got != "on-demand" {
		t.Errorf("Mode cell = %q, want on-demand", got)
	}
	if got := r.Overview()[1]; got != (KV{"Capacity mode", "on-demand"}) {
		t.Errorf("Capacity mode = %q, want on-demand", got)
	}
}

// TestKinesisRow_EmptyValuesFollowWebUI は件数がゼロのとき Shards が "0"、Retention が "0h" になり、
// Mode と Encryption は空文字列のままになることを確かめる。
func TestKinesisRow_EmptyValuesFollowWebUI(t *testing.T) {
	r := kinesisRow{region: "ap-northeast-1", res: aws.KinesisResource{ID: "arn:s", Name: "s", State: "creating"}}

	wantCells := []string{"s", "creating", "", "0", "0h", "", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "arn:s"}, {"Capacity mode", ""}, {"Shards", "0"}, {"Retention", "0h"}, {"Encryption", ""},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.Tags() != nil {
		t.Errorf("Tags = %v, want nil when the resource has none", r.Tags())
	}
}

func TestKinesisService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewKinesisService(func(context.Context, string, string) ([]aws.KinesisResource, error) { return nil, boom })
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list Kinesis streams: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewKinesisService(func(_ context.Context, profile, region string) ([]aws.KinesisResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.KinesisResource{{ID: "arn:s", Name: "s"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:s" || rows[0].Name() != "s" || rows[0].Overview()[5].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[5].Value)
	}
}
