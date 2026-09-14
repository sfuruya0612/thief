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

// datadogMetricsBody はメトリクスクエリの応答本文。
const datadogMetricsBody = `{"status":"ok","query":"avg:system.cpu.user{*}","series":[
  {"expression":"avg:system.cpu.user{host:web-1}","scope":"host:web-1","unit":[{"short_name":"%"}],
   "pointlist":[[1700000000000,12.5],[1700000060000,null]]}
]}`

// metricsTarget は既定の時間窓を付けたメトリクスクエリの URL を組み立てる。
func metricsTarget(query string) string {
	return "/api/datadog/metrics/query?org=suborg1&from=1700000000&to=1700003600&query=" + query
}

// newDatadogMetricsTestServer は Metrics API を metrics へ向けた Server をルート登録済みで返す。
func newDatadogMetricsTestServer(t *testing.T, disk *datadogAuthDisk, staticKeys bool, metrics http.HandlerFunc) *Server {
	t.Helper()

	srv := httptest.NewServer(metrics)
	t.Cleanup(srv.Close)
	srvURL := mustParseURL(t, srv.URL)

	s := newDatadogAuthTestServer(t, disk, staticKeys)
	cfg := ddclient.NewConfiguration(testDatadogSite)
	cfg.Host = srvURL.Host
	cfg.Scheme = srvURL.Scheme
	s.ddMetricsV1 = ddclient.NewMetricsV1API(cfg)
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// okMetricsHandler は固定の時系列を返し、呼ばれた回数を calls へ数える。
func okMetricsHandler(calls *int, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, datadogMetricsBody)
	}
}

// unreachableMetricsHandler は Datadog を呼んではいけない経路の検証に使う。
func unreachableMetricsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to the datadog metrics api: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// getDatadogMetrics はメトリクスクエリを実行して応答を復号する。
func getDatadogMetrics(t *testing.T, s *Server, target string) (ddclient.MetricQueryResult, *httptest.ResponseRecorder) {
	t.Helper()
	w := doDatadogRequest(t, s, http.MethodGet, target)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var got ddclient.MetricQueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal metrics response: %v (body=%s)", err, w.Body.String())
	}
	return got, w
}

// TestDatadogMetricsQueryRequiresParams は query・時間窓が必須であること、org がパス
// トラバーサルの疑いがある値だと弾かれることを確認する。org 自体の省略・空文字は親組織
// を意味し 400 にはしない (TestDatadogMetricsQueryParentOrg を参照)。
func TestDatadogMetricsQueryRequiresParams(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "with an invalid org", target: "/api/datadog/metrics/query?org=../../etc/passwd&from=1&to=2&query=q"},
		{name: "without a query", target: "/api/datadog/metrics/query?org=suborg1&from=1&to=2"},
		{name: "with an empty query", target: "/api/datadog/metrics/query?org=suborg1&from=1&to=2&query="},
		{name: "without a from", target: "/api/datadog/metrics/query?org=suborg1&to=2&query=q"},
		{name: "without a to", target: "/api/datadog/metrics/query?org=suborg1&from=1&query=q"},
		{name: "with a non numeric from", target: "/api/datadog/metrics/query?org=suborg1&from=now&to=2&query=q"},
		{name: "with a non numeric to", target: "/api/datadog/metrics/query?org=suborg1&from=1&to=later&query=q"},
		{name: "with a from after the to", target: "/api/datadog/metrics/query?org=suborg1&from=3&to=2&query=q"},
		{name: "with a from equal to the to", target: "/api/datadog/metrics/query?org=suborg1&from=2&to=2&query=q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 静的キーを設定しても、パラメータが揃わなければ Datadog には届かない。
			s := newDatadogMetricsTestServer(t, &datadogAuthDisk{}, true, unreachableMetricsHandler(t))

			w := doDatadogRequest(t, s, http.MethodGet, tt.target)
			assertErrorCode(t, w, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

// TestDatadogMetricsQueryParentOrg は org が省略または空文字のとき、親組織自身のメトリ
// クスを返すことを確認する (issue 0171: self タブは org="" で Cost/Dashboards/Metrics を
// 横断的に扱う)。Metrics は元々 Sub Organization 専用として org の省略を 400 で弾いてい
// たが、親組織自身のタブでも使えるようこの制限を撤廃した。
func TestDatadogMetricsQueryParentOrg(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "an omitted org", target: "/api/datadog/metrics/query?from=1700000000&to=1700003600&query=q"},
		{name: "an empty org", target: "/api/datadog/metrics/query?org=&from=1700000000&to=1700003600&query=q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			// OAuth トークンが無くても、親組織は静的キーへフォールバックできる。
			s := newDatadogMetricsTestServer(t, &datadogAuthDisk{}, true, okMetricsHandler(&calls, &mu))

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

// TestDatadogMetricsQuery はクエリ結果がそのまま時系列として返ること、欠測が null の
// まま保たれることを確認する。
func TestDatadogMetricsQuery(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
	s := newDatadogMetricsTestServer(t, disk, false, okMetricsHandler(&calls, &mu))

	got, _ := getDatadogMetrics(t, s, metricsTarget("avg%3Asystem.cpu.user%7B%2A%7D"))

	if got.Query != "avg:system.cpu.user{*}" {
		t.Errorf("query = %q, want %q", got.Query, "avg:system.cpu.user{*}")
	}
	if len(got.Series) != 1 {
		t.Fatalf("series = %+v, want 1 series", got.Series)
	}
	s0 := got.Series[0]
	if s0.Name != "avg:system.cpu.user{host:web-1}" || s0.Scope != "host:web-1" || s0.Unit != "%" {
		t.Errorf("series[0] = %+v, want the expression, scope and unit to be carried over", s0)
	}
	if len(s0.Points) != 2 {
		t.Fatalf("points = %+v, want 2 points", s0.Points)
	}
	if s0.Points[0].V == nil || *s0.Points[0].V != 12.5 {
		t.Errorf("points[0] = %+v, want v=12.5", s0.Points[0])
	}
	// 欠測は null のまま返す。0 に潰すと「値が 0 だった」と読めてしまう。
	if s0.Points[1].V != nil {
		t.Errorf("points[1].V = %v, want it to stay null", *s0.Points[1].V)
	}
}

// TestDatadogMetricsQueryCaching はクエリと時間窓と org ごとにキャッシュが分かれ、
// ?refresh=true で取り直されることを確認する。
func TestDatadogMetricsQueryCaching(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
	s := newDatadogMetricsTestServer(t, disk, false, okMetricsHandler(&calls, &mu))

	upstreamCalls := func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}

	_, w := getDatadogMetrics(t, s, metricsTarget("q1"))
	if got := w.Header().Get("X-Cache-Status"); got != "MISS" {
		t.Errorf("first X-Cache-Status = %q, want MISS", got)
	}
	_, w = getDatadogMetrics(t, s, metricsTarget("q1"))
	if got := w.Header().Get("X-Cache-Status"); got != "HIT" {
		t.Errorf("second X-Cache-Status = %q, want HIT", got)
	}
	if got := upstreamCalls(); got != 1 {
		t.Fatalf("datadog metrics calls = %d, want 1", got)
	}

	// クエリが変われば別のキャッシュになる。
	if _, w := getDatadogMetrics(t, s, metricsTarget("q2")); w.Header().Get("X-Cache-Status") != "MISS" {
		t.Errorf("X-Cache-Status for another query = %q, want MISS", w.Header().Get("X-Cache-Status"))
	}
	if got := upstreamCalls(); got != 2 {
		t.Fatalf("datadog metrics calls after changing the query = %d, want 2", got)
	}

	// 時間窓が変われば結果も変わるため、別のキャッシュになる。
	target := "/api/datadog/metrics/query?org=suborg1&from=1700000000&to=1700007200&query=q1"
	if _, w := getDatadogMetrics(t, s, target); w.Header().Get("X-Cache-Status") != "MISS" {
		t.Errorf("X-Cache-Status for another window = %q, want MISS", w.Header().Get("X-Cache-Status"))
	}
	if got := upstreamCalls(); got != 3 {
		t.Fatalf("datadog metrics calls after changing the window = %d, want 3", got)
	}

	// org が変われば別のキャッシュになる。
	target = "/api/datadog/metrics/query?org=suborg2&from=1700000000&to=1700003600&query=q1"
	if _, w := getDatadogMetrics(t, s, target); w.Header().Get("X-Cache-Status") != "MISS" {
		t.Errorf("X-Cache-Status for another org = %q, want MISS", w.Header().Get("X-Cache-Status"))
	}
	if got := upstreamCalls(); got != 4 {
		t.Fatalf("datadog metrics calls after switching org = %d, want 4", got)
	}

	// refresh=true は Datadog から取り直す。
	if _, w := getDatadogMetrics(t, s, metricsTarget("q1")+"&refresh=true"); w.Header().Get("X-Cache-Status") != "MISS" {
		t.Errorf("refreshed X-Cache-Status = %q, want MISS", w.Header().Get("X-Cache-Status"))
	}
	if got := upstreamCalls(); got != 5 {
		t.Fatalf("datadog metrics calls after refresh = %d, want 5", got)
	}
}

// TestDatadogMetricsQueryErrors はエラー変換を確認する。ダッシュボード取得と同じ分類で、
// 資格情報が 1 つも無い場合だけ 401 DATADOG_NO_CREDENTIALS とする。
func TestDatadogMetricsQueryErrors(t *testing.T) {
	t.Run("a sub organization does not fall back to the static keys", func(t *testing.T) {
		// 静的キーは org 非依存のグローバル設定であり、Sub Organization の文脈で使うと
		// 別の組織のメトリクスを黙って返すことになる。
		s := newDatadogMetricsTestServer(t, &datadogAuthDisk{}, true, unreachableMetricsHandler(t))

		w := doDatadogRequest(t, s, http.MethodGet, metricsTarget("q1"))
		assertErrorCode(t, w, http.StatusUnauthorized, "DATADOG_NO_CREDENTIALS")
		if !strings.Contains(w.Body.String(), "suborg1") {
			t.Errorf("error body = %s, want it to name the sub organization", w.Body.String())
		}
	})

	t.Run("a rejected token stays an internal error", func(t *testing.T) {
		// スコープ不足 (timeseries_query が付いていない等) はブラウザでの再ログインでは
		// 解消しないため、再ログイン導線の対象にしない。
		disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
		s := newDatadogMetricsTestServer(t, disk, false, forbiddenUsageHandler)

		w := doDatadogRequest(t, s, http.MethodGet, metricsTarget("q1"))
		assertErrorCode(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
	})

	t.Run("a query error reported with http 200 stays an internal error", func(t *testing.T) {
		// Datadog はクエリ自体の誤りを HTTP 200 と status:"error" で返す。これを成功と
		// して空のグラフを描くと、値が無いのかクエリが誤っているのか区別が付かない。
		disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
		s := newDatadogMetricsTestServer(t, disk, false, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"status":"error","error":"Invalid query"}`)
		})

		w := doDatadogRequest(t, s, http.MethodGet, metricsTarget("q1"))
		assertErrorCode(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
		if !strings.Contains(w.Body.String(), "Invalid query") {
			t.Errorf("error body = %s, want it to carry the query error from datadog", w.Body.String())
		}
	})
}
