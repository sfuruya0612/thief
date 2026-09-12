package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
)

// datadogCostPaths は認証エラーの分類を検証する 2 つのコストエンドポイント。
var datadogCostPaths = []string{"/api/datadog/cost/historical", "/api/datadog/cost/estimated"}

// newDatadogCostTestServer は Usage Metering を usage へ向けた Server をルート登録済みで
// 返す。認証状態は disk (保存済みトークン) と staticKeys (静的キーの有無) で決める。
func newDatadogCostTestServer(t *testing.T, disk *datadogAuthDisk, staticKeys bool, usage http.HandlerFunc) *Server {
	t.Helper()

	usageSrv := httptest.NewServer(usage)
	t.Cleanup(usageSrv.Close)
	usageURL := mustParseURL(t, usageSrv.URL)

	s := newDatadogAuthTestServer(t, disk, staticKeys)
	cfg := ddclient.NewConfiguration(testDatadogSite)
	cfg.Host = usageURL.Host
	cfg.Scheme = usageURL.Scheme
	s.ddV2 = ddclient.NewUsageMeteringV2API(cfg)
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// forbiddenUsageHandler は Usage Metering が権限不足で拒否する応答を模す。
func forbiddenUsageHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	io.WriteString(w, `{"errors":["insufficient_scope"]}`)
}

// TestDatadogCostWithoutCredentialsReturnsUnauthorized は、OAuth トークンが無く静的キーも
// 未設定の状態でコストを取得すると、frontend が再ログイン導線を出せる 401
// DATADOG_NO_CREDENTIALS になることを確認する。
func TestDatadogCostWithoutCredentialsReturnsUnauthorized(t *testing.T) {
	for _, path := range datadogCostPaths {
		t.Run(path, func(t *testing.T) {
			s := newDatadogCostTestServer(t, &datadogAuthDisk{}, false, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("unexpected request to usage metering: %s", r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			})

			w := doDatadogRequest(t, s, http.MethodGet, path+"?start_month=2026-09")
			assertErrorCode(t, w, http.StatusUnauthorized, "DATADOG_NO_CREDENTIALS")
			if !strings.Contains(w.Body.String(), "no usable Datadog credentials") {
				t.Errorf("error body = %s, want it to state that no credentials are usable", w.Body.String())
			}
		})
	}
}

// TestDatadogCostRejectedCredentialsStayInternalError は、資格情報はあるが Datadog に
// 拒否される失敗を再ログイン導線の対象にしないことを確認する。OAuth のスコープ不足も
// 静的キーの失効も、ブラウザでの再ログインでは解決しないため 500 INTERNAL_ERROR のままとする。
func TestDatadogCostRejectedCredentialsStayInternalError(t *testing.T) {
	tests := []struct {
		name       string
		token      bool
		staticKeys bool
	}{
		{name: "oauth token rejected with 403 and no static keys", token: true},
		{name: "static keys rejected with 403", staticKeys: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, path := range datadogCostPaths {
				t.Run(path, func(t *testing.T) {
					disk := &datadogAuthDisk{}
					if tt.token {
						disk.token = validTestToken(t, "valid-token")
					}
					s := newDatadogCostTestServer(t, disk, tt.staticKeys, forbiddenUsageHandler)

					w := doDatadogRequest(t, s, http.MethodGet, path+"?start_month=2026-09")
					assertErrorCode(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
				})
			}
		})
	}
}
