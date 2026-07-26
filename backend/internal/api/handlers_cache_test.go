package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// knownCacheKeySegments は実装時点 (2026-07-26) の既知のキャッシュキー第 1 セグメント全種と
// その分類期待値。全種は `grep -roh 'cacheKey("[a-z0-9-]*"' backend/ | sort -u` で抽出する
// (2026-07-26 抽出時点で 60 種)。owner は破棄する view、空文字は除外集合 (どの view も破棄しない)。
// 新しいセグメントを追加したら、このテーブルに分類期待値を追記する。
var knownCacheKeySegments = []struct {
	seg   string
	owner string
}{
	{seg: "apigw", owner: "aws"},
	{seg: "athena-catalogs", owner: "aws"},
	{seg: "athena-databases", owner: "aws"},
	{seg: "athena-tables", owner: "aws"},
	{seg: "athena-workgroups", owner: "aws"},
	{seg: "bq-datasets", owner: "gcp"},
	{seg: "bq-schema", owner: "gcp"},
	{seg: "bq-tables", owner: "gcp"},
	{seg: "cfn", owner: "aws"},
	{seg: "cfn-detail", owner: "aws"},
	{seg: "cfn-events", owner: "aws"},
	{seg: "cfn-resources", owner: "aws"},
	{seg: "cloudfront", owner: "aws"},
	{seg: "cost", owner: ""},
	{seg: "cost-forecast", owner: ""},
	{seg: "cwlogs-groups", owner: "aws"},
	{seg: "dd-estimated", owner: "datadog"},
	{seg: "dd-historical", owner: "datadog"},
	{seg: "dynamo", owner: "aws"},
	{seg: "dynamo-items", owner: ""},
	{seg: "dynamo-schema", owner: "aws"},
	{seg: "ec2", owner: "aws"},
	{seg: "ecr", owner: "aws"},
	{seg: "ecr-images", owner: "aws"},
	{seg: "ecs", owner: "aws"},
	{seg: "ecs-containers", owner: "aws"},
	{seg: "ecs-services", owner: "aws"},
	{seg: "ecs-tasks", owner: "aws"},
	{seg: "elasticache", owner: "aws"},
	{seg: "elasticache-parameters", owner: "aws"},
	{seg: "elb", owner: "aws"},
	{seg: "elb-listeners", owner: "aws"},
	{seg: "elb-rules", owner: "aws"},
	{seg: "elb-target-groups", owner: "aws"},
	{seg: "elb-target-health", owner: "aws"},
	{seg: "gcp-cloudrun", owner: "gcp"},
	{seg: "gcp-gcs", owner: "gcp"},
	{seg: "gcp-gcs-objects", owner: "gcp"},
	{seg: "gcp-iam", owner: "gcp"},
	{seg: "gcp-projects", owner: ""},
	{seg: "gcp-serviceaccounts", owner: "gcp"},
	{seg: "iam", owner: "aws"},
	{seg: "kinesis", owner: "aws"},
	{seg: "lambda", owner: "aws"},
	{seg: "natgw", owner: "aws"},
	{seg: "rds", owner: "aws"},
	{seg: "rds-cluster-parameters", owner: "aws"},
	{seg: "rds-parameters", owner: "aws"},
	{seg: "regions", owner: ""},
	{seg: "s3", owner: "aws"},
	{seg: "s3-objects", owner: "aws"},
	{seg: "secretsmanager-list", owner: "aws"},
	{seg: "sqs", owner: "aws"},
	{seg: "ssm-list", owner: "aws"},
	{seg: "sso", owner: "aws"},
	{seg: "tidb-clusters", owner: "tidb"},
	{seg: "tidb-cost", owner: "tidb"},
	{seg: "tidb-projects", owner: "tidb"},
	{seg: "waf", owner: "aws"},
	{seg: "waf-rules", owner: "aws"},
}

// TestViewOwnsCacheKey は既知のキャッシュキー第 1 セグメント全種の分類を固定する。
func TestViewOwnsCacheKey(t *testing.T) {
	// 抽出コマンドの結果 (60 種) とテーブルのケース数が一致することを固定する。
	const wantSegments = 60
	if len(knownCacheKeySegments) != wantSegments {
		t.Fatalf("known segments = %d, want %d (update the table when cacheKey segments change)",
			len(knownCacheKeySegments), wantSegments)
	}

	views := []string{"aws", "gcp", "datadog", "tidb"}
	for _, tt := range knownCacheKeySegments {
		key := tt.seg + ":test-profile:ap-northeast-1"
		for _, view := range views {
			want := view == tt.owner
			if got := viewOwnsCacheKey(view, key); got != want {
				t.Errorf("viewOwnsCacheKey(%q, %q) = %v, want %v", view, key, got, want)
			}
		}
		// 未知の view はどのキーも破棄しない。
		if viewOwnsCacheKey("bigquery", key) {
			t.Errorf("viewOwnsCacheKey(bigquery, %q) = true, want false", key)
		}
	}

	// セグメントのみのキー (":" なし) も第 1 セグメントとして扱う。
	if !viewOwnsCacheKey("aws", "ec2") {
		t.Errorf("viewOwnsCacheKey(aws, ec2) = false, want true")
	}
}

// TestHandleCacheInvalidate は view 単位の一括破棄と検証エラーを確認する。
func TestHandleCacheInvalidate(t *testing.T) {
	post := func(t *testing.T, s *Server, query string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/api/cache/invalidate"+query, nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, r)
		return w
	}
	seed := func(t *testing.T, s *Server, keys ...string) {
		t.Helper()
		for _, k := range keys {
			s.resourceCache.Set(k, "cached", time.Minute)
		}
	}
	assertCached := func(t *testing.T, s *Server, key string, want bool) {
		t.Helper()
		if _, ok := s.resourceCache.Get(key); ok != want {
			t.Errorf("cache entry %q exists = %v, want %v", key, ok, want)
		}
	}

	t.Run("view=aws は AWS のエントリだけ破棄し除外集合と他 provider を残す", func(t *testing.T) {
		s := newTestServer(t)
		s.mux = http.NewServeMux()
		s.registerRoutes()
		seed(t, s,
			"ec2:prof:region", "rds-cluster-parameters:prof:region:cl", "sso:prof",
			"cost:prof:region", "cost-forecast:prof:region", "regions:prof",
			"gcp-projects", "dynamo-items:prof:region:t:q",
			"gcp-gcs:proj", "bq-datasets:proj", "dd-estimated:org", "tidb-clusters:org",
		)

		w := post(t, s, "?view=aws")
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
		}
		assertCached(t, s, "ec2:prof:region", false)
		assertCached(t, s, "rds-cluster-parameters:prof:region:cl", false)
		assertCached(t, s, "sso:prof", false)
		// 除外集合は残る。
		assertCached(t, s, "cost:prof:region", true)
		assertCached(t, s, "cost-forecast:prof:region", true)
		assertCached(t, s, "regions:prof", true)
		assertCached(t, s, "gcp-projects", true)
		assertCached(t, s, "dynamo-items:prof:region:t:q", true)
		// 他 provider のエントリは残る。
		assertCached(t, s, "gcp-gcs:proj", true)
		assertCached(t, s, "bq-datasets:proj", true)
		assertCached(t, s, "dd-estimated:org", true)
		assertCached(t, s, "tidb-clusters:org", true)
	})

	t.Run("view=gcp は gcp- と bq- の両方を破棄する", func(t *testing.T) {
		s := newTestServer(t)
		s.mux = http.NewServeMux()
		s.registerRoutes()
		seed(t, s, "gcp-gcs:proj", "bq-datasets:proj", "gcp-projects", "ec2:prof:region")

		w := post(t, s, "?view=gcp")
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
		}
		assertCached(t, s, "gcp-gcs:proj", false)
		assertCached(t, s, "bq-datasets:proj", false)
		assertCached(t, s, "gcp-projects", true)
		assertCached(t, s, "ec2:prof:region", true)
	})

	t.Run("view の欠落と 4 値以外は 400", func(t *testing.T) {
		s := newTestServer(t)
		s.mux = http.NewServeMux()
		s.registerRoutes()

		tests := []struct {
			name    string
			query   string
			wantMsg string
		}{
			{name: "欠落", query: "", wantMsg: "view query parameter is required"},
			{name: "不正値", query: "?view=bigquery", wantMsg: "view must be one of aws, gcp, datadog, tidb"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := post(t, s, tt.query)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
				}
				if got := w.Body.String(); !strings.Contains(got, tt.wantMsg) {
					t.Errorf("body = %q, want it to contain %q", got, tt.wantMsg)
				}
			})
		}
	})

	t.Run("破棄 POST 後の同一キーの GET は MISS になる", func(t *testing.T) {
		s := newTestServer(t)
		s.mux = http.NewServeMux()
		s.registerRoutes()

		calls := 0
		get := func() *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			s.serveCached(w, r, "ec2:prof:region", time.Minute, writeInternalFromError, func() (any, error) {
				calls++
				return []string{"i-1"}, nil
			})
			return w
		}

		if got := get().Header().Get("X-Cache-Status"); got != "MISS" {
			t.Fatalf("X-Cache-Status = %q, want MISS", got)
		}
		if got := get().Header().Get("X-Cache-Status"); got != "HIT" {
			t.Fatalf("X-Cache-Status = %q, want HIT", got)
		}

		w := post(t, s, "?view=aws")
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
		}

		if got := get().Header().Get("X-Cache-Status"); got != "MISS" {
			t.Errorf("X-Cache-Status = %q, want MISS after invalidate", got)
		}
		if calls != 2 {
			t.Errorf("loader calls = %d, want 2", calls)
		}
	})
}
