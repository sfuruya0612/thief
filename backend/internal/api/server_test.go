package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/cache"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/snippet"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	c := cache.New[any](time.Minute)
	t.Cleanup(c.Close)
	return &Server{
		cfg:           config.Defaults(),
		snippets:      snippet.NewStore(t.TempDir()),
		resourceCache: c,
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
