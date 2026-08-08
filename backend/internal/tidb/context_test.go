package tidb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// tidbCtxKey は context に載せた値を取り出して伝播を確かめるための鍵。
type tidbCtxKey struct{}

// ctxRecordingTransport は各リクエストが運ぶ context を記録し、Digest 認証の
// チャレンジ応答を模した空の JSON を返す。
//
// httptest.Server ではサーバ側の context がクライアントのものとは別になるため、
// 呼び出し側の context がリクエストに載っているかを確かめられない。RoundTripper を
// 差し替えて req.Context() を直接見る。
type ctxRecordingTransport struct {
	// contexts は観測した順のリクエストの context。RoundTrip は http.Client.Do から
	// 同期的に呼ばれるため、ロックは要らない。
	contexts []context.Context
}

func (tr *ctxRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.contexts = append(tr.contexts, req.Context())

	header := make(http.Header)
	status := http.StatusOK
	if req.Header.Get("Authorization") == "" {
		status = http.StatusUnauthorized
		header.Set("WWW-Authenticate", `Digest realm="tidbcloud", nonce="testnonce", qop="auth"`)
	}
	// 空の JSON を返す。各メソッドは 0 件として解釈し、追加のページを取りに行かない。
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    req,
	}, nil
}

// TestClientCarriesContextIntoEveryRequest は公開メソッドが受け取った context を
// すべての HTTP リクエストへ載せることを検証する。
//
// TiDB Cloud の呼び出しは Digest 認証のため、チャレンジの取得と認証付きの本番の
// 2 往復になる。http.NewRequestWithContext を http.NewRequest に戻すと Ctrl-C が
// 届かなくなるが、コンパイルも lint も通ってしまうため、往復ごとに context を確かめる。
func TestClientCarriesContextIntoEveryRequest(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, c *Client) error
	}{
		{
			name: "Get",
			call: func(ctx context.Context, c *Client) error {
				resp, err := c.Get(ctx, "/api/v1beta/projects")
				if err != nil {
					return err
				}
				return resp.Body.Close()
			},
		},
		{
			name: "ListProjects",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListProjects(ctx)
				return err
			},
		},
		{
			name: "ListClusters",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListClusters(ctx, "proj1")
				return err
			},
		},
		{
			name: "GetCost",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetCost(ctx, "2024-01")
				return err
			},
		},
		{
			name: "GetCostRange",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetCostRange(ctx, "2024-01", "2024-01")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &ctxRecordingTransport{}
			c := NewClient("public", "private")
			c.http = &http.Client{Transport: tr}

			ctx := context.WithValue(context.Background(), tidbCtxKey{}, "carried")
			if err := tt.call(ctx, c); err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}

			// チャレンジの取得と認証付きの本番リクエストで 2 往復。
			if len(tr.contexts) != 2 {
				t.Fatalf("transport saw %d requests, want 2", len(tr.contexts))
			}
			// http.Client は context を派生させることがあるため、同一性ではなく
			// 載せた値の継承で確かめる。
			for i, got := range tr.contexts {
				if v := got.Value(tidbCtxKey{}); v != "carried" {
					t.Errorf("request %d carried value = %v, want %q", i, v, "carried")
				}
			}
		})
	}
}

// TestClientGetSendsNothingWithCanceledContext は、キャンセル済みの context では
// リクエストを 1 件も送らないことを検証する。
//
// 実際の http.Transport の挙動を見るため、差し替えた RoundTripper ではなく
// httptest.Server を相手にする。
func TestClientGetSendsNothingWithCanceledContext(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	c := newTestClient(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Get(ctx, "/api/v1beta/projects"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get() error = %v, want it to wrap %v", err, context.Canceled)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("server received %d requests, want 0", got)
	}
}

// TestClientStopsIteratingAfterCancellation は、複数回のリクエストに分かれる呼び出しが
// 途中のキャンセルで打ち切られることを検証する。
//
// ListProjects と ListClusters はページを、GetCostRange は月を跨いでリクエストを繰り返す。
// どのループも ctx.Err() を自分では見ておらず、次のリクエストが context のキャンセルで
// 失敗することだけを頼りに止まる。ループの中で渡す context を取り違えると、キャンセル後も
// 最後のページまで取りに行き続ける。1 回目の応答でキャンセルし、2 回目が飛ばないことを見る。
//
// 実際の http.Transport の挙動を見るため、差し替えた RoundTripper ではなく
// httptest.Server を相手にする。RoundTripper の差し替えでは context のキャンセルを
// 自前で見ない限りリクエストが成功してしまい、打ち切りを確かめられない。
func TestClientStopsIteratingAfterCancellation(t *testing.T) {
	// fullPage は 1 ページ分 (tidbListPageSize 件) の項目と、続きがあることを示す総数を返す。
	// 項目の数がページの大きさに満たないか、累計が総数に達すると、ループはキャンセルとは
	// 無関係に自分で抜けてしまう。中身は使わないため空の項目で埋める。
	fullPage := func() string {
		items := make([]string, tidbListPageSize)
		for i := range items {
			items[i] = "{}"
		}
		return fmt.Sprintf(`{"items":[%s],"total":%d}`, strings.Join(items, ","), tidbListPageSize*10)
	}

	tests := []struct {
		name string
		// body は認証済みのリクエストへ返す応答。
		body string
		call func(ctx context.Context, c *Client) error
	}{
		{
			name: "ListProjects",
			body: fullPage(),
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListProjects(ctx)
				return err
			},
		},
		{
			name: "ListClusters",
			body: fullPage(),
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListClusters(ctx, "proj1")
				return err
			},
		},
		{
			// 月ごとに 1 リクエストであり、応答の中身は繰り返しの回数に影響しない。
			name: "GetCostRange",
			body: `{}`,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetCostRange(ctx, "2024-01", "2024-12")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			// digestChallengeHandler が 401 を返す分は数えない。数えるのは
			// 認証済みのリクエスト、つまりループが 1 周するごとに 1 件である。
			var served atomic.Int32
			srv := httptest.NewServer(digestChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
				if served.Add(1) == 1 {
					// 1 周目の応答を返す前にキャンセルする。ループは 2 周目の
					// リクエストで初めてキャンセルに気付く。
					cancel()
				}
				fmt.Fprint(w, tt.body)
			}))
			c := newTestClient(t, srv)

			err := tt.call(ctx, c)

			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s error = %v, want it to wrap %v", tt.name, err, context.Canceled)
			}
			if got := served.Load(); got != 1 {
				t.Errorf("server served %d authenticated requests, want 1; キャンセル後もループが次を取りに行っている", got)
			}
		})
	}
}
