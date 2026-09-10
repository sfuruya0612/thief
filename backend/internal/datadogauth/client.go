package datadogauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// requestTimeout は 1 リクエストあたりの上限。ブラウザでの承認を待たない純粋な
// ネットワーク往復なので、internal/tidb のクライアントと同じ 30 秒とする。
const requestTimeout = 30 * time.Second

// maxErrorBodyBytes はエラー応答の本文をエラーメッセージへ載せる際の上限。
// 認可サーバが HTML を返した場合にエラーが際限なく長くなるのを防ぐ。
const maxErrorBodyBytes = 1024

// Client は Datadog の OAuth エンドポイント (Dynamic Client Registration、トークン) への
// HTTP 呼び出しを行う。ゼロ値で使えるので、本番の呼び出し口 (RegisterClient / ExchangeCode /
// Refresh) はゼロ値へ委譲する。
//
// HTTP を差し替えられるようにしているのは、httptest のサーバへリクエストを向ける
// RoundTripper を注入した統合テストのためである。エンドポイントの URL は site から
// 組み立てる仕様 (https://api.{site}) であり、URL 自体を差し替えると組み立ての実装が
// テストから外れてしまう。
type Client struct {
	// HTTP は nil の場合、requestTimeout を持つ既定のクライアントを使う。
	HTTP *http.Client
}

// defaultHTTPClient は Client.HTTP が nil のときに使うクライアント。
var defaultHTTPClient = &http.Client{Timeout: requestTimeout}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTPClient
}

// apiBaseURL は site から API ホストの base URL を組み立てる。
// Dynamic Client Registration とトークンエンドポイントはどちらもこのホストにある
// (認可エンドポイントだけは app. サブドメインで、authorize.go が組み立てる)。
func apiBaseURL(site string) (string, error) {
	if err := ValidateSite(site); err != nil {
		return "", err
	}
	return "https://api." + site, nil
}

// postJSON は JSON の本文を POST し、wantStatus の応答の本文を out へ復号する。
func (c Client) postJSON(ctx context.Context, url string, body any, wantStatus int, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return c.do(req, wantStatus, out)
}

// postForm は application/x-www-form-urlencoded の本文を POST し、wantStatus の応答の
// 本文を out へ復号する。
func (c Client) postForm(ctx context.Context, url, form string, wantStatus int, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(form))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return c.do(req, wantStatus, out)
}

// do はリクエストを送り、状態コードを検査してから本文を out へ復号する。
// 期待した状態コード以外は、本文を maxErrorBodyBytes まで添えたエラーにする
// (OAuth のエラー応答は RFC 6749 §5.2 の error / error_description を本文で返すため、
// 状態コードだけでは何が起きたか分からない)。
func (c Client) do(req *http.Request, wantStatus int, out any) error {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("send request to %s: %w", req.URL.Redacted(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, req.URL.Redacted(), strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response from %s: %w", req.URL.Redacted(), err)
	}
	return nil
}
