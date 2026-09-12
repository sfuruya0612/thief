package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

// errDatadogTestTokenBroken は保存済みトークンが壊れている状態を表すテスト用のエラー。
var errDatadogTestTokenBroken = errors.New("parse token file: unexpected end of JSON input")

// datadogOrgTokens は org ごとの保存済みトークンを表す疑似ディスク。datadogAuthDisk は
// org を問わず 1 つのトークンしか持てないため、組織一覧のログイン状態の検証には使えない。
type datadogOrgTokens struct {
	mu sync.Mutex

	// loggedIn に入っている org はトークンが保存済み。
	loggedIn map[string]bool
	// loadErr に入っている org は読み取りが壊れている (未ログインと区別する経路の検証用)。
	loadErr map[string]error
}

func (d *datadogOrgTokens) deps(t *testing.T) datadogAuthDeps {
	t.Helper()
	deps := (&datadogAuthDisk{}).deps(t)
	deps.loadToken = func(_, org string) (*datadogauth.TokenSet, bool, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		if err := d.loadErr[org]; err != nil {
			return nil, false, err
		}
		if !d.loggedIn[org] {
			return nil, false, nil
		}
		return validTestToken(t, "access-token-"+org), true, nil
	}
	return deps
}

func (d *datadogOrgTokens) logIn(org string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loggedIn[org] = true
}

const datadogOrgsBody = `{"orgs":[
  {"public_id":"PARENT1","name":"Parent Org"},
  {"public_id":"suborg1","name":"Sub Org 1"}
]}`

// newDatadogOrgsTestServer は Organizations API を orgs へ向けた Server をルート登録済みで
// 返す。呼び出し回数はキャッシュ動作の検証に使うため *int へ数える。
func newDatadogOrgsTestServer(t *testing.T, tokens *datadogOrgTokens, orgs http.HandlerFunc) *Server {
	t.Helper()

	orgsSrv := httptest.NewServer(orgs)
	t.Cleanup(orgsSrv.Close)
	orgsURL := mustParseURL(t, orgsSrv.URL)

	s := newTestServer(t)
	s.ddAuth = tokens.deps(t)
	cfg := ddclient.NewConfiguration(testDatadogSite)
	cfg.Host = orgsURL.Host
	cfg.Scheme = orgsURL.Scheme
	s.ddOrgV1 = ddclient.NewOrganizationsV1API(cfg)
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// okOrgsHandler は組織一覧を返し、呼ばれた回数を calls へ数える。
func okOrgsHandler(calls *int, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		*calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, datadogOrgsBody)
	}
}

// getDatadogOrgs は組織一覧を取得して応答を復号する。
func getDatadogOrgs(t *testing.T, s *Server, target string) ([]datadogOrgResponse, *httptest.ResponseRecorder) {
	t.Helper()
	w := doDatadogRequest(t, s, http.MethodGet, target)
	if w.Code != http.StatusOK {
		t.Fatalf("orgs status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var orgs []datadogOrgResponse
	if err := json.Unmarshal(w.Body.Bytes(), &orgs); err != nil {
		t.Fatalf("unmarshal orgs response: %v (body=%s)", err, w.Body.String())
	}
	return orgs, w
}

// TestDatadogOrgsReturnsLowerCasedIdsAndLoginState は、組織一覧が小文字の識別子と表示名、
// および org 単位のログイン状態を返すことを確認する。
func TestDatadogOrgsReturnsLowerCasedIdsAndLoginState(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	// 親組織のトークンだけが保存されている状態。一覧の取得はこれで行える。
	tokens := &datadogOrgTokens{loggedIn: map[string]bool{datadogParentOrg: true, "suborg1": true}}
	s := newDatadogOrgsTestServer(t, tokens, okOrgsHandler(&calls, &mu))

	orgs, _ := getDatadogOrgs(t, s, "/api/datadog/orgs")

	want := []datadogOrgResponse{
		{ID: "parent1", Name: "Parent Org", LoggedIn: false},
		{ID: "suborg1", Name: "Sub Org 1", LoggedIn: true},
	}
	if len(orgs) != len(want) {
		t.Fatalf("orgs = %+v, want %+v", orgs, want)
	}
	for i, w := range want {
		if orgs[i] != w {
			t.Errorf("orgs[%d] = %+v, want %+v", i, orgs[i], w)
		}
	}
}

// TestDatadogOrgsCaching は、組織一覧が TTL の間キャッシュされ ?refresh=true で
// 取り直されること、そしてログイン状態だけはキャッシュされず毎回判定されることを確認する。
// ログイン状態まで一緒にキャッシュすると、ログイン直後の再取得が未ログインのままの
// 応答を返し続け、タブがログイン導線を出したままになる。
func TestDatadogOrgsCaching(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	tokens := &datadogOrgTokens{loggedIn: map[string]bool{datadogParentOrg: true}}
	s := newDatadogOrgsTestServer(t, tokens, okOrgsHandler(&calls, &mu))

	_, w := getDatadogOrgs(t, s, "/api/datadog/orgs")
	if got := w.Header().Get("X-Cache-Status"); got != "MISS" {
		t.Errorf("first X-Cache-Status = %q, want MISS", got)
	}

	// 2 回目はキャッシュから返り、Datadog は呼ばれない。
	orgs, w := getDatadogOrgs(t, s, "/api/datadog/orgs")
	if got := w.Header().Get("X-Cache-Status"); got != "HIT" {
		t.Errorf("second X-Cache-Status = %q, want HIT", got)
	}
	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 1 {
		t.Fatalf("datadog organizations calls = %d, want 1", got)
	}
	if orgs[1].LoggedIn {
		t.Fatalf("orgs[1].LoggedIn = true before logging in, want false")
	}

	// キャッシュ HIT でもログイン状態は判定し直される。
	tokens.logIn("suborg1")
	orgs, w = getDatadogOrgs(t, s, "/api/datadog/orgs")
	if got := w.Header().Get("X-Cache-Status"); got != "HIT" {
		t.Errorf("third X-Cache-Status = %q, want HIT", got)
	}
	if !orgs[1].LoggedIn {
		t.Errorf("orgs[1].LoggedIn = false after logging in to suborg1, want true")
	}

	// refresh=true は Datadog から取り直す。
	if _, w := getDatadogOrgs(t, s, "/api/datadog/orgs?refresh=true"); w.Header().Get("X-Cache-Status") != "MISS" {
		t.Errorf("refreshed X-Cache-Status = %q, want MISS", w.Header().Get("X-Cache-Status"))
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("datadog organizations calls after refresh = %d, want 2", calls)
	}
}

// TestDatadogOrgsCorruptTokenIsNotLoggedIn は、トークンファイルが壊れている org を
// 未ログインとして返すことを確認する。壊れたファイルを持つ org のタブは、ログイン導線を
// 出さないと復帰できない。
func TestDatadogOrgsCorruptTokenIsNotLoggedIn(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	tokens := &datadogOrgTokens{
		loggedIn: map[string]bool{datadogParentOrg: true, "suborg1": true},
		loadErr:  map[string]error{"suborg1": errDatadogTestTokenBroken},
	}
	logs := captureLogs(t)
	s := newDatadogOrgsTestServer(t, tokens, okOrgsHandler(&calls, &mu))

	orgs, _ := getDatadogOrgs(t, s, "/api/datadog/orgs")

	if orgs[1].LoggedIn {
		t.Errorf("orgs[1].LoggedIn = true for a broken token file, want false")
	}
	assertDatadogWarn(t, logs, "stored datadog oauth token is unusable")
}

// TestDatadogOrgsErrors は組織一覧のエラー変換を確認する。資格情報が 1 つも無い場合だけ
// frontend が再ログイン導線を出せる 401 DATADOG_NO_CREDENTIALS とし、Datadog に拒否される
// 失敗は 500 INTERNAL_ERROR のままとする (コスト取得と同じ分類)。
func TestDatadogOrgsErrors(t *testing.T) {
	tests := []struct {
		name       string
		parentAuth bool
		handler    http.HandlerFunc
		wantStatus int
		wantCode   string
	}{
		{
			name: "no credentials at all",
			handler: func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("unexpected request to the datadog organizations api: %s", r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "DATADOG_NO_CREDENTIALS",
		},
		{
			name:       "the oauth token is rejected with 403",
			parentAuth: true,
			handler:    forbiddenUsageHandler,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &datadogOrgTokens{loggedIn: map[string]bool{datadogParentOrg: tt.parentAuth}}
			s := newDatadogOrgsTestServer(t, tokens, tt.handler)

			w := doDatadogRequest(t, s, http.MethodGet, "/api/datadog/orgs")
			assertErrorCode(t, w, tt.wantStatus, tt.wantCode)
		})
	}
}
