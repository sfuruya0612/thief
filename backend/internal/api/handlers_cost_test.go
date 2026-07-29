package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// TestCostCacheKeyDistinctPerField は、応答内容を左右する条件を 1 つだけ変えたときに
// キャッシュキーが必ず変わることを確認する。いずれかの条件が costCacheKey の引数から
// 抜け落ちると、その条件だけが異なるリクエストがキャッシュを共有して誤った結果を返す。
func TestCostCacheKeyDistinctPerField(t *testing.T) {
	const (
		baseProfile = "prod"
		baseRegion  = "ap-northeast-1"
	)
	baseOpts := awsinternal.CostQueryOptions{
		IncludeToday:     false,
		Granularity:      "DAILY",
		GroupByDimension: "SERVICE",
		ServiceFilter:    "AmazonEC2",
		AccountFilter:    "111111111111",
		StartDate:        "2026-07-01",
		EndDate:          "2026-07-31",
		Months:           3,
	}

	tests := []struct {
		name    string
		profile string
		region  string
		mutate  func(o *awsinternal.CostQueryOptions)
	}{
		{name: "profile", profile: "stg"},
		{name: "region", region: "us-east-1"},
		{name: "include today", mutate: func(o *awsinternal.CostQueryOptions) { o.IncludeToday = true }},
		{name: "granularity", mutate: func(o *awsinternal.CostQueryOptions) { o.Granularity = "MONTHLY" }},
		{name: "group by dimension", mutate: func(o *awsinternal.CostQueryOptions) { o.GroupByDimension = "LINKED_ACCOUNT" }},
		{name: "service filter", mutate: func(o *awsinternal.CostQueryOptions) { o.ServiceFilter = "AmazonS3" }},
		{name: "service filter cleared", mutate: func(o *awsinternal.CostQueryOptions) { o.ServiceFilter = "" }},
		{name: "account filter", mutate: func(o *awsinternal.CostQueryOptions) { o.AccountFilter = "222222222222" }},
		{name: "account filter cleared", mutate: func(o *awsinternal.CostQueryOptions) { o.AccountFilter = "" }},
		{name: "start date", mutate: func(o *awsinternal.CostQueryOptions) { o.StartDate = "2026-06-01" }},
		{name: "end date", mutate: func(o *awsinternal.CostQueryOptions) { o.EndDate = "2026-08-31" }},
		{name: "months", mutate: func(o *awsinternal.CostQueryOptions) { o.Months = 6 }},
	}

	baseKey := costCacheKey(baseProfile, baseRegion, baseOpts)
	// ベースとの差分だけでなく、変更後のキーどうしが衝突しないことも確認する。
	seen := map[string]string{baseKey: "base"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := baseProfile
			if tt.profile != "" {
				profile = tt.profile
			}
			region := baseRegion
			if tt.region != "" {
				region = tt.region
			}
			opts := baseOpts
			if tt.mutate != nil {
				tt.mutate(&opts)
			}

			got := costCacheKey(profile, region, opts)
			if got == baseKey {
				t.Fatalf("costCacheKey did not change when %s changed (key=%q)", tt.name, got)
			}
			if prev, ok := seen[got]; ok {
				t.Fatalf("costCacheKey collision between %q and %q (key=%q)", prev, tt.name, got)
			}
			seen[got] = tt.name
		})
	}
}

// TestCostCacheKeyEscapesFilterSeparator は、絞り込み条件に区切り文字 ":" が含まれていても
// service と account の境界が保たれることを確認する。両者はキャッシュキー上で隣接するため、
// エスケープしないと ("a:b", "c") と ("a", "b:c") が同一キーへ衝突する。
func TestCostCacheKeyEscapesFilterSeparator(t *testing.T) {
	const (
		profile = "prod"
		region  = "ap-northeast-1"
	)
	key := func(service, account string) string {
		opts := awsinternal.CostQueryOptions{
			Granularity:      "DAILY",
			GroupByDimension: "SERVICE",
			ServiceFilter:    service,
			AccountFilter:    account,
		}
		return costCacheKey(profile, region, opts)
	}

	if a, b := key("a:b", "c"), key("a", "b:c"); a == b {
		t.Errorf("service=%q/account=%q and service=%q/account=%q share key %q", "a:b", "c", "a", "b:c", a)
	}
	if a, b := key("a:b", ""), key("a", "b"); a == b {
		t.Errorf("service=%q/account=%q and service=%q/account=%q share key %q", "a:b", "", "a", "b", a)
	}
}

// TestHandleCostUsesFilterAwareCacheKey は handleCost がクエリパラメータの絞り込み条件を
// キャッシュキーへ反映していることを HTTP レイヤで確認する。
// 条件の組み合わせごとに異なる値をあらかじめキャッシュへ入れておき、各リクエストが
// 自分の組み合わせの値を受け取ることを検証する。キャッシュキーに絞り込み条件が
// 含まれていなければ、別の組み合わせの値が返るか、キャッシュミスして GetCostAndUsage を
// 呼びに行く (X-Cache-Status が HIT にならない) ため、いずれの場合も失敗する。
func TestHandleCostUsesFilterAwareCacheKey(t *testing.T) {
	const (
		profile = "prod"
		region  = "ap-northeast-1"
	)

	type filterCase struct {
		name    string
		service string
		account string
	}
	cases := []filterCase{
		{name: "service and account", service: "AmazonEC2", account: "111111111111"},
		{name: "different service", service: "AmazonS3", account: "111111111111"},
		{name: "different account", service: "AmazonEC2", account: "222222222222"},
		{name: "no filter", service: "", account: ""},
		{name: "colon in service", service: "a:b", account: "c"},
		{name: "colon in account", service: "a", account: "b:c"},
	}

	s := newTestServer(t)

	// キャッシュ済みの値は組み合わせごとに一意なマーカーにする。どの組み合わせの値が
	// 返ってきたかをレスポンスから判別できるようにするため。
	marker := func(c filterCase) string { return "cached:" + c.name }

	for _, c := range cases {
		opts := awsinternal.CostQueryOptions{
			Granularity:      "DAILY",
			GroupByDimension: "SERVICE",
			ServiceFilter:    c.service,
			AccountFilter:    c.account,
			StartDate:        "2026-07-01",
			EndDate:          "2026-07-31",
		}
		s.resourceCache.Set(costCacheKey(profile, region, opts), []string{marker(c)}, time.Minute)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := url.Values{}
			q.Set("region", region)
			q.Set("granularity", "DAILY")
			q.Set("group_by", "SERVICE")
			q.Set("service", c.service)
			q.Set("account", c.account)
			q.Set("start", "2026-07-01")
			q.Set("end", "2026-07-31")

			r := httptest.NewRequest(http.MethodGet, "/api/aws/"+profile+"/cost?"+q.Encode(), nil)
			r.SetPathValue("profile", profile)
			w := httptest.NewRecorder()
			s.handleCost(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
			}
			// MISS の場合は AWS への実リクエストが走ったことを意味する。
			if got := w.Header().Get("X-Cache-Status"); got != "HIT" {
				t.Fatalf("X-Cache-Status = %q, want HIT (cache key does not match the seeded key)", got)
			}
			var body []string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}
			if len(body) != 1 || body[0] != marker(c) {
				t.Errorf("body = %v, want [%s]", body, marker(c))
			}
		})
	}
}
