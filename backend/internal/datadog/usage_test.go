package datadog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestV2API returns a UsageMeteringV2API pointed at the given httptest.Server.
func newTestV2API(t *testing.T, srv *httptest.Server) (*UsageMeteringV2API, context.Context) {
	t.Helper()
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := NewConfiguration("datadoghq.com")
	cfg.Host = host
	cfg.Scheme = "http"

	ctx := NewContext(context.Background(), "public", "private")
	return NewUsageMeteringV2API(cfg), ctx
}

const costByOrgBody = `{
  "data": [
    {
      "id": "1",
      "type": "costs",
      "attributes": {
        "date": "2024-01-01T00:00:00Z",
        "account_name": "acct1",
        "org_name": "org1",
        "charges": [
          {"product_name": "infra", "charge_type": "on-demand", "cost": 12.5}
        ]
      }
    }
  ]
}`

func TestGetHistoricalCost(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, costByOrgBody)
	}))
	api, ctx := newTestV2API(t, srv)

	costs, err := GetHistoricalCost(ctx, api, "2024-01", "2024-01", "")
	if err != nil {
		t.Fatalf("GetHistoricalCost() error = %v", err)
	}
	if gotPath != "/api/v2/usage/historical_cost" {
		t.Errorf("requested path = %q, want /api/v2/usage/historical_cost", gotPath)
	}
	if len(costs) != 1 {
		t.Fatalf("len(costs) = %d, want 1", len(costs))
	}
	want := CostInfo{
		Month:       "2024-01",
		AccountName: "acct1",
		OrgName:     "org1",
		ProductName: "infra",
		ChargeType:  "on-demand",
		Cost:        12.5,
	}
	if costs[0] != want {
		t.Errorf("costs[0] = %+v, want %+v", costs[0], want)
	}
}

func TestGetHistoricalCostReturnsEmptySliceWhenDataIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[]}`)
	}))
	api, ctx := newTestV2API(t, srv)

	costs, err := GetHistoricalCost(ctx, api, "2024-01", "", "")
	if err != nil {
		t.Fatalf("GetHistoricalCost() error = %v", err)
	}
	if costs == nil {
		t.Fatal("GetHistoricalCost() returned nil slice, want empty slice")
	}
	if len(costs) != 0 {
		t.Fatalf("len(costs) = %d, want 0", len(costs))
	}
}

func TestGetEstimatedCostReturnsEmptySliceWhenDataIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[]}`)
	}))
	api, ctx := newTestV2API(t, srv)

	costs, err := GetEstimatedCost(ctx, api, "2024-01", "", "")
	if err != nil {
		t.Fatalf("GetEstimatedCost() error = %v", err)
	}
	if costs == nil {
		t.Fatal("GetEstimatedCost() returned nil slice, want empty slice")
	}
	if len(costs) != 0 {
		t.Fatalf("len(costs) = %d, want 0", len(costs))
	}
}

func TestGetEstimatedCost(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, costByOrgBody)
	}))
	api, ctx := newTestV2API(t, srv)

	costs, err := GetEstimatedCost(ctx, api, "2024-01", "", "")
	if err != nil {
		t.Fatalf("GetEstimatedCost() error = %v", err)
	}
	if gotPath != "/api/v2/usage/estimated_cost" {
		t.Errorf("requested path = %q, want /api/v2/usage/estimated_cost", gotPath)
	}
	if len(costs) != 1 {
		t.Fatalf("len(costs) = %d, want 1", len(costs))
	}
	want := CostInfo{
		Month:       "2024-01",
		AccountName: "acct1",
		OrgName:     "org1",
		ProductName: "infra",
		ChargeType:  "on-demand",
		Cost:        12.5,
	}
	if costs[0] != want {
		t.Errorf("costs[0] = %+v, want %+v", costs[0], want)
	}
}

func TestGetHistoricalCostInvalidStartMonth(t *testing.T) {
	api := &UsageMeteringV2API{}
	if _, err := GetHistoricalCost(context.Background(), api, "not-a-month", "", ""); err == nil {
		t.Fatal("expected error for invalid start_month, got nil")
	}
}

func TestGetEstimatedCostInvalidEndMonth(t *testing.T) {
	api := &UsageMeteringV2API{}
	if _, err := GetEstimatedCost(context.Background(), api, "", "not-a-month", ""); err == nil {
		t.Fatal("expected error for invalid end_month, got nil")
	}
}
