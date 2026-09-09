package api

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
		Keyword:          "AmazonEC2",
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
		{name: "keyword", mutate: func(o *awsinternal.CostQueryOptions) { o.Keyword = "AmazonS3" }},
		{name: "keyword cleared", mutate: func(o *awsinternal.CostQueryOptions) { o.Keyword = "" }},
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

// TestCostCacheKeyEscapesFilterSeparator は、キーワードに区切り文字 ":" が含まれていても
// 隣接する要素との境界が保たれることを確認する。キーワードはキャッシュキー上で開始日と
// 隣接し、どちらもクエリパラメータ由来の自由入力であるため、エスケープしないと
// ("a:b", "c") と ("a", "b:c") が同一キーへ衝突する。
func TestCostCacheKeyEscapesFilterSeparator(t *testing.T) {
	const (
		profile = "prod"
		region  = "ap-northeast-1"
	)
	key := func(keyword, start string) string {
		opts := awsinternal.CostQueryOptions{
			Granularity:      "DAILY",
			GroupByDimension: "SERVICE",
			Keyword:          keyword,
			StartDate:        start,
		}
		return costCacheKey(profile, region, opts)
	}

	if a, b := key("a:b", "c"), key("a", "b:c"); a == b {
		t.Errorf("keyword=%q/start=%q and keyword=%q/start=%q share key %q", "a:b", "c", "a", "b:c", a)
	}
	if a, b := key("a:b", ""), key("a", "b"); a == b {
		t.Errorf("keyword=%q/start=%q and keyword=%q/start=%q share key %q", "a:b", "", "a", "b", a)
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
		keyword string
	}
	cases := []filterCase{
		{name: "service keyword", keyword: "AmazonEC2"},
		{name: "different keyword", keyword: "AmazonS3"},
		{name: "account keyword", keyword: "111111111111"},
		{name: "no filter", keyword: ""},
		{name: "colon in keyword", keyword: "a:b"},
	}

	s := newTestServer(t)

	// キャッシュ済みの値は組み合わせごとに一意なマーカーにする。どの組み合わせの値が
	// 返ってきたかをレスポンスから判別できるようにするため。
	marker := func(c filterCase) string { return "cached:" + c.name }

	for _, c := range cases {
		opts := awsinternal.CostQueryOptions{
			Granularity:      "DAILY",
			GroupByDimension: "SERVICE",
			Keyword:          c.keyword,
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
			q.Set("keyword", c.keyword)
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

// TestHandleCostIgnoresLegacyFilterParams は handleCost が廃止した service / account の
// クエリパラメータを読まないことを確認する。読んでいればキャッシュキーが keyword だけで
// 組んだキーと一致せず、X-Cache-Status が HIT にならない。
func TestHandleCostIgnoresLegacyFilterParams(t *testing.T) {
	const (
		profile = "prod"
		region  = "ap-northeast-1"
		keyword = "AmazonEC2"
	)

	s := newTestServer(t)
	opts := awsinternal.CostQueryOptions{
		Granularity:      "DAILY",
		GroupByDimension: "SERVICE",
		Keyword:          keyword,
		StartDate:        "2026-07-01",
		EndDate:          "2026-07-31",
	}
	s.resourceCache.Set(costCacheKey(profile, region, opts), []string{"cached"}, time.Minute)

	q := url.Values{}
	q.Set("region", region)
	q.Set("granularity", "DAILY")
	q.Set("group_by", "SERVICE")
	q.Set("keyword", keyword)
	// 廃止済みのパラメータ。読まれていれば絞り込み条件が変わる値を入れる。
	q.Set("service", "AmazonS3")
	q.Set("account", "222222222222")
	q.Set("start", "2026-07-01")
	q.Set("end", "2026-07-31")

	r := httptest.NewRequest(http.MethodGet, "/api/aws/"+profile+"/cost?"+q.Encode(), nil)
	r.SetPathValue("profile", profile)
	w := httptest.NewRecorder()
	s.handleCost(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("X-Cache-Status"); got != "HIT" {
		t.Fatalf("X-Cache-Status = %q, want HIT (handleCost still reads service/account)", got)
	}
}

// TestHandleCostSSOTokenExpired は SSO トークンが期限切れのとき handleCost と
// handleCostForecast が 401 と SSO_TOKEN_EXPIRED を返すことを検証する。
// AWS SDK は HOME 配下の ~/.aws/config と ~/.aws/sso/cache を読むため、t.Setenv で
// 期限切れトークンの fixture を注入する (このため t.Parallel とは併用できない)。
// トークンの期限切れは資格情報の解決段階で検出されるため、AWS への実リクエストは
// 発生しない。
func TestHandleCostSSOTokenExpired(t *testing.T) {
	const (
		profile  = "sso-expired"
		startURL = "https://example.awsapps.com/start"
	)
	home := t.TempDir()
	writeFile := func(rel, content string) string {
		t.Helper()
		path := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
		return path
	}

	configPath := writeFile(".aws/config", `[profile `+profile+`]
sso_start_url = `+startURL+`
sso_region = us-east-1
sso_account_id = 111111111111
sso_role_name = ReadOnly
region = ap-northeast-1
`)
	// SSO トークンキャッシュのファイル名は start URL の SHA-1 ハッシュ。過去の expiresAt を
	// 持つトークンを置くと、SDK は資格情報の解決時に期限切れエラーを返す。
	sum := sha1.Sum([]byte(startURL))
	writeFile(
		filepath.Join(".aws", "sso", "cache", hex.EncodeToString(sum[:])+".json"),
		`{"startUrl": "`+startURL+`", "region": "us-east-1",`+
			` "accessToken": "REDACTED", "expiresAt": "2020-01-02T03:04:05Z"}`,
	)
	t.Setenv("HOME", home)
	// 実行環境の環境変数が fixture を上書きしないよう明示的に差し替える。
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))

	tests := []struct {
		name   string
		path   string
		handle func(s *Server, w http.ResponseWriter, r *http.Request)
	}{
		{
			name:   "cost",
			path:   "/api/aws/profiles/" + profile + "/cost?granularity=MONTHLY",
			handle: (*Server).handleCost,
		},
		{
			name:   "cost forecast",
			path:   "/api/aws/profiles/" + profile + "/cost/forecast",
			handle: (*Server).handleCostForecast,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			r.SetPathValue("profile", profile)
			w := httptest.NewRecorder()
			tt.handle(s, w, r)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
			}
			if got := decodeErrorResponse(t, w).Code; got != "SSO_TOKEN_EXPIRED" {
				t.Errorf("code = %q, want %q", got, "SSO_TOKEN_EXPIRED")
			}
		})
	}
}
