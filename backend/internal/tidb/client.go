// Package tidb provides a TiDB Cloud API client with RFC 2617 Digest Authentication.
package tidb

import (
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://api.tidbcloud.com"
	defaultBillingURL = "https://billing.tidbapi.com"
)

// Client is a TiDB Cloud API client with Digest Authentication.
type Client struct {
	publicKey  string
	privateKey string
	baseURL    string
	billingURL string
	http       *http.Client
}

// NewClient creates a TiDB Cloud API client.
func NewClient(publicKey, privateKey string) *Client {
	return &Client{
		publicKey:  publicKey,
		privateKey: privateKey,
		baseURL:    defaultBaseURL,
		billingURL: defaultBillingURL,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Get performs an authenticated GET request to the TiDB Cloud API.
func (c *Client) Get(endpoint string) (*http.Response, error) {
	return c.get(c.baseURL + endpoint)
}

// getBilling performs an authenticated GET request to the TiDB Cloud billing API,
// which is served from a separate host from the main v1beta API.
func (c *Client) getBilling(endpoint string) (*http.Response, error) {
	return c.get(c.billingURL + endpoint)
}

func (c *Client) get(url string) (*http.Response, error) {
	// First request to get the WWW-Authenticate challenge.
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send initial request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		// Re-issue with no auth — unusual but handle gracefully.
		return c.doRequest(url, "")
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	authHeader, err := c.buildDigestHeader(http.MethodGet, url, challenge)
	if err != nil {
		return nil, fmt.Errorf("build digest header: %w", err)
	}
	return c.doRequest(url, authHeader)
}

func (c *Client) doRequest(url, authHeader string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return c.http.Do(req)
}

// buildDigestHeader constructs an RFC 2617 Digest Authorization header.
func (c *Client) buildDigestHeader(method, url, wwwAuth string) (string, error) {
	params := parseDigestChallenge(wwwAuth)
	realm := params["realm"]
	nonce := params["nonce"]
	qop := params["qop"]
	algorithm := params["algorithm"]

	// Extract URI path from URL.
	uri := url
	if idx := strings.Index(url, "://"); idx >= 0 {
		rest := url[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			uri = rest[slash:]
		}
	}

	cnonce := fmt.Sprintf("%x", rand.Int63())
	nc := "00000001"

	ha1 := md5hex(c.publicKey + ":" + realm + ":" + c.privateKey)
	if strings.ToLower(algorithm) == "md5-sess" {
		ha1 = md5hex(ha1 + ":" + nonce + ":" + cnonce)
	}
	ha2 := md5hex(method + ":" + uri)

	var response string
	if qop == "auth" || qop == "auth-int" {
		response = md5hex(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	} else {
		response = md5hex(ha1 + ":" + nonce + ":" + ha2)
	}

	header := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		c.publicKey, realm, nonce, uri, response,
	)
	if qop != "" {
		header += fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qop, nc, cnonce)
	}
	if algorithm != "" {
		header += fmt.Sprintf(`, algorithm=%s`, algorithm)
	}
	return header, nil
}

var digestParamRe = regexp.MustCompile(`(\w+)="?([^",]+)"?`)

func parseDigestChallenge(wwwAuth string) map[string]string {
	params := make(map[string]string)
	for _, m := range digestParamRe.FindAllStringSubmatch(wwwAuth, -1) {
		if len(m) == 3 {
			params[m[1]] = m[2]
		}
	}
	return params
}

func md5hex(s string) string {
	h := md5.New()
	io.WriteString(h, s)
	return fmt.Sprintf("%x", h.Sum(nil))
}
