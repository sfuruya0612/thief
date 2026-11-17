package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestOpenSearchService_IDTitleAndColumns(t *testing.T) {
	svc := NewOpenSearchService(nil)
	if svc.ID() != "opensearch" || svc.Title() != "OpenSearch" {
		t.Errorf("ID/Title = %q/%q, want opensearch/OpenSearch", svc.ID(), svc.Title())
	}
	if svc.SupportsSession() {
		t.Error("OpenSearch has no interactive session")
	}
	// Web UI の openSearchColumns の header の並び。
	want := []Column{
		{Title: "Domain"}, {Title: "State"}, {Title: "Version"}, {Title: "Instance"},
		{Title: "Nodes", Align: AlignRight}, {Title: "Endpoint"}, {Title: "Region"},
	}
	if got := svc.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
}

func TestOpenSearchRow_CellsAndOverview(t *testing.T) {
	r := openSearchRow{region: "ap-northeast-1", res: aws.OpenSearchResource{
		ID:                     "arn:aws:es:ap-northeast-1:123:domain/products",
		Name:                   "products",
		State:                  "active",
		EngineVersion:          "OpenSearch_2.19",
		DataNodeType:           "m5.large.search",
		DataNodeCount:          3,
		DedicatedMasterEnabled: true,
		DedicatedMasterType:    "m5.large.search",
		DedicatedMasterCount:   3,
		EBSVolumeSizeGB:        100,
		EBSVolumeType:          "gp3",
		Endpoint:               "search-products.ap-northeast-1.es.amazonaws.com",
		VPCID:                  "vpc-0123456789abcdef0",
		EncryptionAtRest:       true,
		NodeToNodeEncryption:   true,
		EnforceHTTPS:           true,
		Tags:                   map[string]string{"Env": "prod"},
	}}

	wantCells := []string{
		"products",
		"active",
		"OpenSearch_2.19",
		"m5.large.search",
		"3",
		"search-products.ap-northeast-1.es.amazonaws.com",
		"ap-northeast-1",
	}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	if n := len(r.Cells()); n != len((&openSearchService{}).Columns()) {
		t.Errorf("Cells has %d entries but Columns has %d", n, len((&openSearchService{}).Columns()))
	}
	// Web UI の openSearchOverviewRows の項目名と表示文字列の並び。
	wantOverview := []KV{
		{"Resource ID", "arn:aws:es:ap-northeast-1:123:domain/products"},
		{"Engine version", "OpenSearch_2.19"},
		{"Data node type", "m5.large.search"},
		{"Data nodes", "3"},
		{"Dedicated master type", "m5.large.search"},
		{"Dedicated masters", "3"},
		{"EBS storage", "100 GiB gp3"},
		{"Endpoint", "search-products.ap-northeast-1.es.amazonaws.com"},
		{"VPC", "vpc-0123456789abcdef0"},
		{"Encryption at rest", "enabled"},
		{"Node-to-node encryption", "enabled"},
		{"HTTPS enforcement", "enabled"},
		{"Region", "ap-northeast-1"},
	}
	if got := r.Overview(); !reflect.DeepEqual(got, wantOverview) {
		t.Errorf("Overview = %q, want %q", got, wantOverview)
	}
	if r.ID() != "arn:aws:es:ap-northeast-1:123:domain/products" || r.Name() != "products" || r.State() != "active" {
		t.Errorf("accessors mismatch: %q %q %q", r.ID(), r.Name(), r.State())
	}
	if got := r.Tags(); !reflect.DeepEqual(got, map[string]string{"Env": "prod"}) {
		t.Errorf("Tags = %v, want OpenSearchResource.Tags", got)
	}
	if r.TagsFetchFailed() {
		t.Error("TagsFetchFailed = true, want false when the resource reports success")
	}
}

// TestOpenSearchRow_ZeroValuesFollowWebUI は EBS ボリュームと接続先が無いドメインで
// Web UI と同じダッシュ表示になり、タグの取得失敗が TagsFetchFailer として伝わることを確かめる。
func TestOpenSearchRow_ZeroValuesFollowWebUI(t *testing.T) {
	r := openSearchRow{region: "ap-northeast-1", res: aws.OpenSearchResource{
		ID: "arn:o", Name: "o", State: "creating", TagsFetchFailed: true,
	}}

	wantCells := []string{"o", "creating", "", "", "0", "—", "ap-northeast-1"}
	if got := r.Cells(); !reflect.DeepEqual(got, wantCells) {
		t.Errorf("Cells = %q, want %q", got, wantCells)
	}
	wantOverview := []KV{
		{"Resource ID", "arn:o"}, {"Engine version", ""}, {"Data node type", "—"}, {"Data nodes", "0"},
		{"Dedicated master type", "—"}, {"Dedicated masters", "—"},
		{"EBS storage", "—"}, {"Endpoint", "—"}, {"VPC", "—"},
		{"Encryption at rest", "disabled"}, {"Node-to-node encryption", "disabled"},
		{"HTTPS enforcement", "disabled"}, {"Region", "ap-northeast-1"},
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
		t.Errorf("openSearchRow must implement TagsFetchFailer and report the failure (ok=%v)", ok)
	}
}

func TestOpenSearchService_LoadWrapsErrorAndSetsRegion(t *testing.T) {
	boom := errors.New("boom")
	svc := NewOpenSearchService(func(context.Context, string, string) ([]aws.OpenSearchResource, error) {
		return nil, boom
	})
	_, err := svc.Load(context.Background(), "p", "r")
	if !errors.Is(err, boom) || err.Error() != "list OpenSearch domains: boom" {
		t.Fatalf("err = %v, want wrapped boom with the service name", err)
	}

	var calls []string
	svc = NewOpenSearchService(func(_ context.Context, profile, region string) ([]aws.OpenSearchResource, error) {
		calls = append(calls, profile+"/"+region)
		return []aws.OpenSearchResource{{ID: "arn:d", Name: "d"}}, nil
	})
	rows, err := svc.Load(context.Background(), "p", "us-west-2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if want := []string{"p/us-west-2"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("list calls = %q, want %q", calls, want)
	}
	if rows[0].ID() != "arn:d" || rows[0].Name() != "d" || rows[0].Overview()[12].Value != "us-west-2" {
		t.Errorf("row = %q %q region=%q", rows[0].ID(), rows[0].Name(), rows[0].Overview()[12].Value)
	}
}
