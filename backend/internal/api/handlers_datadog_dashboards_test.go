package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
)

// datadogDashboardsBody は一覧の応答本文。
const datadogDashboardsBody = `{"dashboards":[
  {"id":"abc-123","title":"Overview","description":"main","url":"/dashboard/abc-123/overview"}
]}`

// datadogDashboardBody は詳細の応答本文。timeseries・query_value・group の入れ子・未対応の
// 4 通りを 1 つのダッシュボードに含める。
const datadogDashboardBody = `{"id":"abc-123","title":"Overview","layout_type":"ordered",
  "url":"/dashboard/abc-123/overview","widgets":[
  {"id":1,"definition":{"type":"timeseries","title":"CPU","requests":[{"q":"avg:system.cpu.user{*}"}]}},
  {"id":2,"definition":{"type":"group","title":"Group","layout_type":"ordered","widgets":[
    {"id":3,"definition":{"type":"query_value","title":"Load","requests":[{"q":"sum:system.load.1{*}"}]}}
  ]}},
  {"id":4,"definition":{"type":"toplist","title":"Top","requests":[{"q":"top(avg:system.cpu.user{*},10,'mean','desc')"}]}}
]}`

// newDatadogDashboardsTestServer は Dashboards API を dashboards へ向けた Server を
// ルート登録済みで返す。認証状態は disk (保存済みトークン) と staticKeys で決める。
func newDatadogDashboardsTestServer(t *testing.T, disk *datadogAuthDisk, staticKeys bool, dashboards http.HandlerFunc) *Server {
	t.Helper()

	srv := httptest.NewServer(dashboards)
	t.Cleanup(srv.Close)
	srvURL := mustParseURL(t, srv.URL)

	s := newDatadogAuthTestServer(t, disk, staticKeys)
	cfg := ddclient.NewConfiguration(testDatadogSite)
	cfg.Host = srvURL.Host
	cfg.Scheme = srvURL.Scheme
	s.ddDashV1 = ddclient.NewDashboardsV1API(cfg)
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// okDashboardsHandler は一覧と詳細を返し、呼ばれた回数を calls へ数える。
func okDashboardsHandler(calls *int, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/dashboard" {
			io.WriteString(w, datadogDashboardsBody)
			return
		}
		io.WriteString(w, datadogDashboardBody)
	}
}

// unreachableDashboardsHandler は Datadog を呼んではいけない経路の検証に使う。
func unreachableDashboardsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to the datadog dashboards api: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// getDatadogDashboards は一覧を取得して応答を復号する。
func getDatadogDashboards(t *testing.T, s *Server, target string) ([]ddclient.DashboardInfo, *httptest.ResponseRecorder) {
	t.Helper()
	w := doDatadogRequest(t, s, http.MethodGet, target)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboards status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var got []ddclient.DashboardInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal dashboards response: %v (body=%s)", err, w.Body.String())
	}
	return got, w
}

// TestDatadogDashboardsInvalidOrgIsRejected は、パストラバーサルの疑いがある org を
// Datadog を呼ぶ前に弾くことを確認する。
func TestDatadogDashboardsInvalidOrgIsRejected(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "the list", target: "/api/datadog/dashboards?org=../../etc/passwd"},
		{name: "the detail", target: "/api/datadog/dashboards/abc-123?org=../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newDatadogDashboardsTestServer(t, &datadogAuthDisk{}, true, unreachableDashboardsHandler(t))

			w := doDatadogRequest(t, s, http.MethodGet, tt.target)
			assertErrorCode(t, w, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

// TestDatadogDashboardsParentOrg は org が省略または空文字のとき、親組織自身のダッシュ
// ボードを返すことを確認する (issue 0171: self タブは org="" で Cost/Dashboards/Metrics
// を横断的に扱う)。Dashboards は元々 Sub Organization 専用として org の省略を 400 で
// 弾いていたが、親組織自身のタブでも使えるようこの制限を撤廃した。
func TestDatadogDashboardsParentOrg(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "the list with an omitted org", target: "/api/datadog/dashboards"},
		{name: "the list with an empty org", target: "/api/datadog/dashboards?org="},
		{name: "the detail with an omitted org", target: "/api/datadog/dashboards/abc-123"},
		{name: "the detail with an empty org", target: "/api/datadog/dashboards/abc-123?org="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			// OAuth トークンが無くても、親組織は静的キーへフォールバックできる。
			s := newDatadogDashboardsTestServer(t, &datadogAuthDisk{}, true, okDashboardsHandler(&calls, &mu))

			w := doDatadogRequest(t, s, http.MethodGet, tt.target)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
			}

			mu.Lock()
			got := calls
			mu.Unlock()
			if got != 1 {
				t.Fatalf("upstream calls = %d, want 1", got)
			}
		})
	}
}

// TestDatadogDashboardsList は一覧が返す内容と、Datadog が相対パスで返す url が
// ブラウザで開ける絶対 URL になっていることを確認する。
func TestDatadogDashboardsList(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
	s := newDatadogDashboardsTestServer(t, disk, false, okDashboardsHandler(&calls, &mu))

	got, _ := getDatadogDashboards(t, s, "/api/datadog/dashboards?org=suborg1")

	want := []ddclient.DashboardInfo{{
		ID:          "abc-123",
		Title:       "Overview",
		Description: "main",
		URL:         "https://app." + testDatadogSite + "/dashboard/abc-123/overview",
	}}
	if len(got) != len(want) {
		t.Fatalf("dashboards = %+v, want %+v", got, want)
	}
	if got[0] != want[0] {
		t.Errorf("dashboards[0] = %+v, want %+v", got[0], want[0])
	}
}

// TestDatadogDashboardDetail は詳細が group の入れ子を展開し、未対応ウィジェットを
// 種別名付きで返すことを確認する。
func TestDatadogDashboardDetail(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
	s := newDatadogDashboardsTestServer(t, disk, false, okDashboardsHandler(&calls, &mu))

	w := doDatadogRequest(t, s, http.MethodGet, "/api/datadog/dashboards/abc-123?org=suborg1")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var got ddclient.DashboardDetail
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal dashboard response: %v (body=%s)", err, w.Body.String())
	}

	if want := "https://app." + testDatadogSite + "/dashboard/abc-123/overview"; got.URL != want {
		t.Errorf("detail.URL = %q, want %q", got.URL, want)
	}
	want := []struct {
		id   int64
		kind ddclient.WidgetKind
		typ  string
	}{
		{id: 1, kind: ddclient.WidgetKindTimeseries, typ: "timeseries"},
		{id: 3, kind: ddclient.WidgetKindQueryValue, typ: "query_value"},
		{id: 4, kind: ddclient.WidgetKindUnsupported, typ: "toplist"},
	}
	if len(got.Widgets) != len(want) {
		t.Fatalf("widgets = %+v, want %d widgets (the group replaced by its nested widget)", got.Widgets, len(want))
	}
	for i, w := range want {
		if got.Widgets[i].ID != w.id || got.Widgets[i].Kind != w.kind || got.Widgets[i].Type != w.typ {
			t.Errorf("widgets[%d] = %+v, want id=%d kind=%s type=%s", i, got.Widgets[i], w.id, w.kind, w.typ)
		}
	}
}

// TestDatadogDashboardsCaching は一覧と詳細が TTL の間キャッシュされ、org とダッシュボード
// ごとに分かれ、?refresh=true で取り直されることを確認する。
func TestDatadogDashboardsCaching(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
	s := newDatadogDashboardsTestServer(t, disk, false, okDashboardsHandler(&calls, &mu))

	upstreamCalls := func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}

	_, w := getDatadogDashboards(t, s, "/api/datadog/dashboards?org=suborg1")
	if got := w.Header().Get("X-Cache-Status"); got != "MISS" {
		t.Errorf("first X-Cache-Status = %q, want MISS", got)
	}
	_, w = getDatadogDashboards(t, s, "/api/datadog/dashboards?org=suborg1")
	if got := w.Header().Get("X-Cache-Status"); got != "HIT" {
		t.Errorf("second X-Cache-Status = %q, want HIT", got)
	}
	if got := upstreamCalls(); got != 1 {
		t.Fatalf("datadog dashboards calls = %d, want 1", got)
	}

	// org が変われば別のキャッシュになる。
	if _, w := getDatadogDashboards(t, s, "/api/datadog/dashboards?org=suborg2"); w.Header().Get("X-Cache-Status") != "MISS" {
		t.Errorf("X-Cache-Status for another org = %q, want MISS", w.Header().Get("X-Cache-Status"))
	}
	if got := upstreamCalls(); got != 2 {
		t.Fatalf("datadog dashboards calls after switching org = %d, want 2", got)
	}

	// refresh=true は Datadog から取り直す。
	if _, w := getDatadogDashboards(t, s, "/api/datadog/dashboards?org=suborg1&refresh=true"); w.Header().Get("X-Cache-Status") != "MISS" {
		t.Errorf("refreshed X-Cache-Status = %q, want MISS", w.Header().Get("X-Cache-Status"))
	}
	if got := upstreamCalls(); got != 3 {
		t.Fatalf("datadog dashboards calls after refresh = %d, want 3", got)
	}

	// 詳細はダッシュボードごとに分かれる。
	for _, target := range []string{
		"/api/datadog/dashboards/abc-123?org=suborg1",
		"/api/datadog/dashboards/abc-123?org=suborg1",
		"/api/datadog/dashboards/def-456?org=suborg1",
	} {
		if w := doDatadogRequest(t, s, http.MethodGet, target); w.Code != http.StatusOK {
			t.Fatalf("dashboard status for %q = %d, want %d (body=%s)", target, w.Code, http.StatusOK, w.Body.String())
		}
	}
	if got := upstreamCalls(); got != 5 {
		t.Errorf("datadog dashboards calls after fetching two dashboards twice = %d, want 5", got)
	}
}

// TestDatadogDashboardsErrors はエラー変換を確認する。資格情報が 1 つも無い場合だけ
// frontend が再ログイン導線を出せる 401 DATADOG_NO_CREDENTIALS とし、Datadog に拒否される
// 失敗は 500 INTERNAL_ERROR のままとする (コスト取得・組織一覧と同じ分類)。
func TestDatadogDashboardsErrors(t *testing.T) {
	targets := []string{
		"/api/datadog/dashboards?org=suborg1",
		"/api/datadog/dashboards/abc-123?org=suborg1",
	}

	t.Run("a sub organization does not fall back to the static keys", func(t *testing.T) {
		for _, target := range targets {
			t.Run(target, func(t *testing.T) {
				// 静的キーは org 非依存のグローバル設定であり、Sub Organization の文脈で
				// 使うと別の組織のダッシュボードを黙って返すことになる。
				s := newDatadogDashboardsTestServer(t, &datadogAuthDisk{}, true, unreachableDashboardsHandler(t))

				w := doDatadogRequest(t, s, http.MethodGet, target)
				assertErrorCode(t, w, http.StatusUnauthorized, "DATADOG_NO_CREDENTIALS")
				if !strings.Contains(w.Body.String(), "suborg1") {
					t.Errorf("error body = %s, want it to name the sub organization", w.Body.String())
				}
			})
		}
	})

	t.Run("a rejected token stays an internal error", func(t *testing.T) {
		for _, target := range targets {
			t.Run(target, func(t *testing.T) {
				// スコープ不足 (dashboards_read が付いていない等) はブラウザでの再ログインでは
				// 解消しないため、再ログイン導線の対象にしない。
				disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
				s := newDatadogDashboardsTestServer(t, disk, false, forbiddenUsageHandler)

				w := doDatadogRequest(t, s, http.MethodGet, target)
				assertErrorCode(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
			})
		}
	})
}

func TestDatadogDashboardURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "a relative path gets the site host",
			raw:  "/dashboard/abc-123/overview",
			want: "https://app.datadoghq.com/dashboard/abc-123/overview",
		},
		{
			name: "a query string is kept",
			raw:  "/dashboard/abc-123/overview?from_ts=1",
			want: "https://app.datadoghq.com/dashboard/abc-123/overview?from_ts=1",
		},
		{
			name: "an absolute url is kept as it is",
			raw:  "https://app.datadoghq.eu/dashboard/abc-123/overview",
			want: "https://app.datadoghq.eu/dashboard/abc-123/overview",
		},
		{name: "an empty url stays empty", raw: "", want: ""},
		{name: "an unparsable url becomes empty", raw: "://nope", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datadogDashboardURL("datadoghq.com", tt.raw); got != tt.want {
				t.Errorf("datadogDashboardURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
