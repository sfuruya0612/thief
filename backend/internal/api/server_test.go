package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/cache"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
	"github.com/sfuruya0612/thief/backend/internal/snippet"
	"github.com/sfuruya0612/thief/backend/internal/ssoauth"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	c := cache.New[any](time.Minute)
	t.Cleanup(c.Close)
	cfg := config.Defaults()
	cfg.PriceCacheDir = t.TempDir()
	return &Server{
		cfg:           cfg,
		snippets:      snippet.NewStore(t.TempDir()),
		resourceCache: c,
		// NewServer と同様に SSO ログイン系のフィールドも初期化する (未初期化のまま
		// start / complete ハンドラへ到達すると nil ストアで panic するため)。deps は
		// 実 AWS へ接続する defaultSSOLoginDeps ではなく、呼ばれたらエラーを返す
		// ダミーにする。SSO ログイン系のテストは newSSOLoginTestServer で差し替えること。
		ssoLoginSessions: newSSOLoginSessionStore(),
		ssoLogin:         unconfiguredSSOLoginDeps(),
		// Datadog の OAuth も同様に初期化する。deps は実 Datadog や実ファイルへ
		// 触れないダミーで、Datadog 認証を伴うテストは newDatadogAuthTestServer で
		// 差し替えること。
		ddLoginSessions: newDatadogLoginSessionStore(),
		ddAuth:          unconfiguredDatadogAuthDeps(),
	}
}

// unconfiguredDatadogAuthDeps は newTestServer の既定の datadogAuthDeps。テストが誤って
// Datadog 認証系の経路を通っても実 Datadog へ接続せず、エラー応答で気づけるようにする。
func unconfiguredDatadogAuthDeps() datadogAuthDeps {
	err := errors.New("datadog auth deps are not configured in newTestServer; use newDatadogAuthTestServer")
	return datadogAuthDeps{
		loadToken:  func(string) (*datadogauth.TokenSet, bool, error) { return nil, false, err },
		saveToken:  func(string, *datadogauth.TokenSet) error { return err },
		loadClient: func(string) (*datadogauth.ClientCredentials, bool, error) { return nil, false, err },
		refreshToken: func(context.Context, string, string, string) (*datadogauth.TokenSet, error) {
			return nil, err
		},
		prepareLogin: func(context.Context, datadogauth.PrepareParams) (*datadogauth.Login, error) {
			return nil, err
		},
		completeLogin: func(context.Context, *datadogauth.Login, string, string) (*datadogauth.TokenSet, error) {
			return nil, err
		},
		logout: func(string) error { return err },
		now:    time.Now,
	}
}

// unconfiguredSSOLoginDeps は newTestServer の既定の ssoLoginDeps。テストが誤って
// SSO ログイン系エンドポイントを叩いても実 AWS へ接続せず、エラー応答で気づける
// ようにする。
func unconfiguredSSOLoginDeps() ssoLoginDeps {
	err := errors.New("sso login deps are not configured in newTestServer; use newSSOLoginTestServer")
	return ssoLoginDeps{
		resolveConfig: func(string) (*awsinternal.SSOConfig, error) { return nil, err },
		start:         func(context.Context, string, string) (*ssoauth.Session, error) { return nil, err },
		wait:          func(context.Context, *ssoauth.Session) (*ssoauth.TokenCache, error) { return nil, err },
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{name: "no parts", parts: nil, want: ""},
		{name: "single part", parts: []string{"cost"}, want: "cost"},
		{
			name:  "plain parts are joined with colon",
			parts: []string{"cost", "prod", "ap-northeast-1"},
			want:  "cost:prod:ap-northeast-1",
		},
		{
			name:  "empty parts are kept as empty segments",
			parts: []string{"cost", "prod", "", ""},
			want:  "cost:prod::",
		},
		{
			// 要素に含まれる ":" は "%3A" になるため区切りと区別できる。
			name:  "colon in a part is escaped",
			parts: []string{"cost", "a:b", "c"},
			want:  "cost:a%3Ab:c",
		},
		{
			// エスケープ文字そのものも変換されるため、エスケープ後の文字列から元の値が一意に復元できる。
			name:  "percent in a part is escaped",
			parts: []string{"cost", "a%3Ab", "c"},
			want:  "cost:a%253Ab:c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cacheKey(tt.parts...); got != tt.want {
				t.Errorf("cacheKey(%q) = %q, want %q", tt.parts, got, tt.want)
			}
		})
	}
}

// TestCacheKeyDistinctForDifferentParts は、区切り文字を含む値を渡しても引数の組み合わせが
// 異なれば必ず異なるキーになることを確認する。Cost Explorer の絞り込み (service / account) や
// DynamoDB の属性値のように自由入力が隣接する箇所では、衝突するとキャッシュ経由で
// 別の絞り込み結果が返ってしまう。
func TestCacheKeyDistinctForDifferentParts(t *testing.T) {
	groups := [][]string{
		{"cost", "a:b", "c"},
		{"cost", "a", "b:c"},
		{"cost", "a:b:c"},
		{"cost", "a", "b", "c"},
		{"cost", "a:b", ""},
		{"cost", "a", "b", ""},
		{"cost", "", "a:b"},
		{"cost", "", "a", "b"},
	}
	seen := make(map[string][]string, len(groups))
	for _, parts := range groups {
		key := cacheKey(parts...)
		if prev, ok := seen[key]; ok {
			t.Errorf("cacheKey collision: %q and %q both produce %q", prev, parts, key)
			continue
		}
		seen[key] = parts
	}
}

// TestCacheKeyPrefixForInvalidate は、末尾要素を空文字にしたキーが
// resourceCache.InvalidatePrefix (strings.HasPrefix による前方一致削除) のプレフィックスとして
// 機能し続けることを確認する。handlers_s3_object.go / handlers_gcp.go がこの用法に依存している。
func TestCacheKeyPrefixForInvalidate(t *testing.T) {
	const (
		profile = "prod"
		region  = "ap-northeast-1"
		bucket  = "my-bucket"
	)
	prefixKey := cacheKey("s3-objects", profile, region, bucket, "")

	matches := []string{
		cacheKey("s3-objects", profile, region, bucket, ""),
		cacheKey("s3-objects", profile, region, bucket, "logs/"),
		cacheKey("s3-objects", profile, region, bucket, "logs/2026:07/"),
	}
	for _, key := range matches {
		if !strings.HasPrefix(key, prefixKey) {
			t.Errorf("key %q does not have prefix %q", key, prefixKey)
		}
	}

	// バケット名が前方一致してしまう別バケットのキーは削除対象に含まれてはならない。
	nonMatches := []string{
		cacheKey("s3-objects", profile, region, bucket+"-2", "logs/"),
		cacheKey("s3-objects", profile, region, bucket+":x", "logs/"),
		cacheKey("s3-objects", profile, "us-east-1", bucket, "logs/"),
	}
	for _, key := range nonMatches {
		if strings.HasPrefix(key, prefixKey) {
			t.Errorf("key %q unexpectedly has prefix %q", key, prefixKey)
		}
	}
}

func TestServeCachedMissThenHit(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	load := func() (any, error) {
		calls++
		return []string{"a", "b"}, nil
	}

	do := func(url string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		s.serveCached(w, r, "test-key", time.Minute, writeInternalFromError, load)
		return w
	}

	w := do("/test")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("X-Cache-Status"); got != "MISS" {
		t.Errorf("X-Cache-Status = %q, want MISS", got)
	}
	var body []string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body) != 2 || body[0] != "a" {
		t.Errorf("body = %v, want [a b]", body)
	}

	w = do("/test")
	if got := w.Header().Get("X-Cache-Status"); got != "HIT" {
		t.Errorf("X-Cache-Status = %q, want HIT", got)
	}
	if calls != 1 {
		t.Errorf("loader calls = %d, want 1 (cached)", calls)
	}

	// refresh=true はキャッシュを無効化して再ロードする。
	w = do("/test?refresh=true")
	if got := w.Header().Get("X-Cache-Status"); got != "MISS" {
		t.Errorf("X-Cache-Status = %q, want MISS after refresh", got)
	}
	if calls != 2 {
		t.Errorf("loader calls = %d, want 2 after refresh", calls)
	}
}

func TestServeCachedLoaderError(t *testing.T) {
	s := newTestServer(t)
	loadErr := errors.New("boom")

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	var gotErr error
	s.serveCached(w, r, "err-key", time.Minute, func(w http.ResponseWriter, err error) {
		gotErr = err
		writeInternalError(w, err.Error())
	}, func() (any, error) {
		return nil, loadErr
	})

	if !errors.Is(gotErr, loadErr) {
		t.Errorf("onErr err = %v, want %v", gotErr, loadErr)
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if got := w.Header().Get("X-Cache-Status"); got != "" {
		t.Errorf("X-Cache-Status = %q, want empty on error", got)
	}
}
