package datadogauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// recordingTransport は本番コードが組み立てた https://api.{site}/... 宛のリクエストを
// httptest のサーバへ向け直しつつ、向け直す前の URL を記録する。
// URL の組み立て自体をテストの対象から外さないために、URL ではなく HTTP クライアントを
// 差し替える形にしている。
type recordingTransport struct {
	base *url.URL

	mu   sync.Mutex
	urls []string
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.urls = append(t.urls, req.URL.String())
	t.mu.Unlock()

	r := req.Clone(req.Context())
	r.URL.Scheme = t.base.Scheme
	r.URL.Host = t.base.Host
	r.Host = ""
	return http.DefaultTransport.RoundTrip(r)
}

func (t *recordingTransport) requested() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.urls...)
}

// newTestClient は srv へリクエストを向ける Client と、向け直す前の URL を読むための
// transport を返す。
func newTestClient(t *testing.T, srv *httptest.Server) (Client, *recordingTransport) {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	tr := &recordingTransport{base: u}
	return Client{HTTP: &http.Client{Transport: tr}}, tr
}

// httpObservation は httptest のハンドラが観測したリクエスト。観測値はチャネルで
// テストへ渡し、ハンドラの goroutine とテストの goroutine の順序を確定させる。
type httpObservation struct {
	path        string
	contentType string
	body        []byte
}

// newStubServer は status と body を返すだけのサーバと、観測値のチャネルを返す。
func newStubServer(t *testing.T, status int, body string) (*httptest.Server, chan httpObservation) {
	t.Helper()
	observed := make(chan httpObservation, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		observed <- httpObservation{path: r.URL.Path, contentType: r.Header.Get("Content-Type"), body: raw}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write response body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, observed
}

// receiveObservation は観測値を 1 件取り出す。1 件も無ければテストを失敗させる。
func receiveObservation(t *testing.T, ch chan httpObservation) httpObservation {
	t.Helper()
	select {
	case o := <-ch:
		return o
	default:
		t.Fatal("the stub server did not receive a request")
		return httpObservation{}
	}
}
