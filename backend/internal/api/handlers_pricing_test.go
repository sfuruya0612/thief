package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/pricecache"
)

func pricingRequest(t *testing.T, profile, service, region string, refresh bool) *http.Request {
	t.Helper()
	url := "/api/aws/profiles/" + profile + "/pricing?service=" + service + "&region=" + region
	if refresh {
		url += "&refresh=true"
	}
	r := httptest.NewRequest(http.MethodGet, url, nil)
	r.SetPathValue("profile", profile)
	return r
}

func TestHandlePricingValidation(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		service  string
		region   string
		wantCode int
	}{
		{name: "unknown service", service: "s3", profile: "default", region: "ap-northeast-1", wantCode: http.StatusBadRequest},
		{name: "empty service", profile: "default", service: "", region: "ap-northeast-1", wantCode: http.StatusBadRequest},
		{name: "invalid region", profile: "default", service: "ec2", region: "../etc", wantCode: http.StatusBadRequest},
		{name: "empty region", profile: "default", service: "ec2", region: "", wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			w := httptest.NewRecorder()
			s.handlePricing(w, pricingRequest(t, tt.profile, tt.service, tt.region, false))
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body=%q)", w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

func TestHandlePricingServesFromCacheWithoutFetching(t *testing.T) {
	s := newTestServer(t)
	want := []byte(`{"service":"ec2","region":"ap-northeast-1","fetched_at":"2026-07-18T09:00:00Z","partial":false,"missing_models":[],"rates":[]}`)
	if err := pricecache.Save(s.cfg.PriceCacheDir, "ec2", "ap-northeast-1", want, time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("pricecache.Save() err = %v", err)
	}

	w := httptest.NewRecorder()
	// profile "default" が実在しなくても、キャッシュヒットする限り AWS 呼び出しは
	// 発生しない (実行環境に AWS 認証情報が無くても本テストが通ることでそれを示す)。
	s.handlePricing(w, pricingRequest(t, "default", "ec2", "ap-northeast-1", false))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}
	if w.Body.String() != string(want) {
		t.Errorf("body = %s, want %s", w.Body.String(), want)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHandlePricingCacheIOErrorIsRedacted(t *testing.T) {
	s := newTestServer(t)
	// キャッシュファイルが置かれるべきパスに代わりにディレクトリを置き、
	// pricecache.Load が (miss ではなく) ハードエラーを返すようにする。
	badPath := filepath.Join(s.cfg.PriceCacheDir, "ec2", "ap-northeast-1.json")
	if err := os.MkdirAll(badPath, 0o700); err != nil {
		t.Fatalf("MkdirAll() err = %v", err)
	}

	w := httptest.NewRecorder()
	s.handlePricing(w, pricingRequest(t, "default", "ec2", "ap-northeast-1", false))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body=%q)", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	resp := decodeErrorResponse(t, w)
	if resp.Error == "" {
		t.Fatal("error message is empty")
	}
	if got := resp.Error; got == badPath || strings.Contains(got, s.cfg.PriceCacheDir) {
		t.Errorf("error message %q leaks the cache directory path %q", got, s.cfg.PriceCacheDir)
	}
}
