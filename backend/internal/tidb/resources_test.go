package tidb

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// newTestClient returns a Client pointed at the given httptest.Server and
// bypasses Digest Authentication (the test server does not challenge).
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient("public", "private")
	c.baseURL = srv.URL
	c.billingURL = srv.URL
	t.Cleanup(srv.Close)
	return c
}

// digestChallengeHandler wraps a handler so that requests without an
// Authorization header receive a 401 Digest challenge, mirroring the real
// TiDB Cloud API flow exercised by Client.Get.
func digestChallengeHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="tidbcloud", nonce="testnonce", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func TestListProjectsPaginatesAllPages(t *testing.T) {
	const total = 25
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1beta/projects", digestChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, pageSize := parsePageParams(t, q)

		start := (page - 1) * pageSize
		end := min(start+pageSize, total)

		var items []string
		for i := start; i < end; i++ {
			items = append(items, fmt.Sprintf(`{"id":"p%d","name":"project-%d","org_id":"o1","cluster_count":0,"user_count":0,"create_timestamp":"2024-01-01T00:00:00Z"}`, i, i))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"items":[%s],"total":%d}`, joinJSON(items), total)
	}))

	srv := httptest.NewServer(mux)
	c := newTestClient(t, srv)

	projects, err := c.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != total {
		t.Fatalf("len(projects) = %d, want %d", len(projects), total)
	}
	for i, p := range projects {
		if want := fmt.Sprintf("p%d", i); p.ID != want {
			t.Errorf("projects[%d].ID = %q, want %q", i, p.ID, want)
		}
	}
}

func TestListClustersPaginatesAllPages(t *testing.T) {
	const total = 15
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1beta/projects/proj1/clusters", digestChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, pageSize := parsePageParams(t, q)

		start := (page - 1) * pageSize
		end := min(start+pageSize, total)

		var items []string
		for i := start; i < end; i++ {
			items = append(items, fmt.Sprintf(`{"id":"c%d","name":"cluster-%d","status":{"cluster_status":"AVAILABLE"},"region":"us-east-1","cluster_type":"DEDICATED","cloud_provider":"AWS","create_timestamp":"2024-01-01T00:00:00Z"}`, i, i))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"items":[%s],"total":%d}`, joinJSON(items), total)
	}))

	srv := httptest.NewServer(mux)
	c := newTestClient(t, srv)

	clusters, err := c.ListClusters("proj1")
	if err != nil {
		t.Fatalf("ListClusters() error = %v", err)
	}
	if len(clusters) != total {
		t.Fatalf("len(clusters) = %d, want %d", len(clusters), total)
	}
}

func TestGetCostReturnsEmptySliceWhenDetailsIsEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1beta1/billsDetails/2024-01", digestChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"details":[]}`)
	}))

	srv := httptest.NewServer(mux)
	c := newTestClient(t, srv)

	costs, err := c.GetCost("2024-01")
	if err != nil {
		t.Fatalf("GetCost() error = %v", err)
	}
	if costs == nil {
		t.Fatal("GetCost() returned nil slice, want empty slice")
	}
	if len(costs) != 0 {
		t.Fatalf("len(costs) = %d, want 0", len(costs))
	}
}

func TestGetCostReturnsCosts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1beta1/billsDetails/2024-01", digestChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"details":[{"billedDate":"2024-01-01","clusterName":"cluster1","credits":"1.5","discounts":"0.5","projectName":"proj1","runningTotal":"10","servicePathName":"compute","totalCost":"11"}]}`)
	}))

	srv := httptest.NewServer(mux)
	c := newTestClient(t, srv)

	costs, err := c.GetCost("2024-01")
	if err != nil {
		t.Fatalf("GetCost() error = %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("len(costs) = %d, want 1", len(costs))
	}
	got := costs[0]
	want := Cost{
		BilledDate:      "2024-01-01",
		ProjectName:     "proj1",
		ClusterName:     "cluster1",
		ServicePathName: "compute",
		Credits:         1.5,
		Discounts:       0.5,
		RunningTotal:    10,
		TotalCost:       11,
		CreditsRaw:      "1.5",
		DiscountsRaw:    "0.5",
		RunningTotalRaw: "10",
		TotalCostRaw:    "11",
	}
	if got != want {
		t.Errorf("costs[0] = %+v, want %+v", got, want)
	}
}

func TestGetCostDefaultsToCurrentMonthWhenMonthIsEmpty(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", digestChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"details":[]}`)
	}))

	srv := httptest.NewServer(mux)
	c := newTestClient(t, srv)

	if _, err := c.GetCost(""); err != nil {
		t.Fatalf("GetCost() error = %v", err)
	}

	want := "/v1beta1/billsDetails/" + time.Now().Format("2006-01")
	if gotPath != want {
		t.Errorf("requested path = %q, want %q", gotPath, want)
	}
}

func TestGetCostRangeFetchesEachMonthAndSwapsReversedRange(t *testing.T) {
	var gotPaths []string
	mux := http.NewServeMux()
	mux.HandleFunc("/", digestChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		month := r.URL.Path[len("/v1beta1/billsDetails/"):]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"details":[{"billedDate":"%s-01","totalCost":"1"}]}`, month)
	}))

	srv := httptest.NewServer(mux)
	c := newTestClient(t, srv)

	// end より start が後ろでも自動で入れ替わり、両端を含む全月が取得されること。
	costs, err := c.GetCostRange("2024-03", "2024-01")
	if err != nil {
		t.Fatalf("GetCostRange() error = %v", err)
	}
	if len(costs) != 3 {
		t.Fatalf("len(costs) = %d, want 3", len(costs))
	}

	want := []string{
		"/v1beta1/billsDetails/2024-01",
		"/v1beta1/billsDetails/2024-02",
		"/v1beta1/billsDetails/2024-03",
	}
	if len(gotPaths) != len(want) {
		t.Fatalf("gotPaths = %v, want %v", gotPaths, want)
	}
	for i, p := range want {
		if gotPaths[i] != p {
			t.Errorf("gotPaths[%d] = %q, want %q", i, gotPaths[i], p)
		}
	}
}

func parsePageParams(t *testing.T, q url.Values) (page, pageSize int) {
	t.Helper()
	if _, err := fmt.Sscanf(q.Get("page"), "%d", &page); err != nil {
		t.Fatalf("parse page param %q: %v", q.Get("page"), err)
	}
	if _, err := fmt.Sscanf(q.Get("page_size"), "%d", &pageSize); err != nil {
		t.Fatalf("parse page_size param %q: %v", q.Get("page_size"), err)
	}
	return page, pageSize
}

func joinJSON(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ","
		}
		out += item
	}
	return out
}
