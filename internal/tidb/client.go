package tidb

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
)

// DigestClient implements HTTP Digest Authentication for TiDB Cloud API.
// It manages authentication state and provides methods for making authenticated requests.
type DigestClient struct {
	publicKey  string
	privateKey string
	Client     *http.Client
	nc         int // nonce count used in digest authentication
}

// NewDigestClient creates a new client for digest authentication with the provided credentials.
// It initializes an HTTP client and sets the nonce count to 1.
func NewDigestClient(publicKey, privateKey string) *DigestClient {
	return &DigestClient{
		publicKey:  publicKey,
		privateKey: privateKey,
		Client:     &http.Client{},
		nc:         1,
	}
}

// generateCnonce creates a random client nonce for use in digest authentication.
// Returns a hex-encoded random string or an error if random generation fails.
func generateCnonce() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ParseDigestHeader extracts parameters from a WWW-Authenticate header.
// It returns a map of parameter names to values extracted from the header.
func ParseDigestHeader(header string) map[string]string {
	result := make(map[string]string)
	// Match both quoted values: key="value" and unquoted values: key=value
	re1 := regexp.MustCompile(`(\w+)="([^"]+)"`)
	re2 := regexp.MustCompile(`(\w+)=([^,\s"]+)`)

	// Extract quoted values
	matches := re1.FindAllStringSubmatch(header, -1)
	for _, match := range matches {
		result[match[1]] = match[2]
	}

	// Extract unquoted values
	matches2 := re2.FindAllStringSubmatch(header, -1)
	for _, match := range matches2 {
		// Only add if not already added from quoted values
		if _, exists := result[match[1]]; !exists {
			result[match[1]] = match[2]
		}
	}
	return result
}

// CreateDigestHeader generates an Authorization header for digest authentication.
// It calculates the response hash according to the HTTP Digest Authentication spec (RFC 2617).
// Returns the complete Authorization header value or an error if generation fails.
func (d *DigestClient) CreateDigestHeader(method, uri string, digestParams map[string]string) (string, error) {
	cnonce, err := generateCnonce()
	if err != nil {
		return "", fmt.Errorf("failed to generate cnonce: %v", err)
	}

	realm := digestParams["realm"]
	nonce := digestParams["nonce"]
	qop := digestParams["qop"]

	ha1 := md5.Sum([]byte(fmt.Sprintf("%s:%s:%s", d.publicKey, realm, d.privateKey)))
	ha2 := md5.Sum([]byte(fmt.Sprintf("%s:%s", method, uri)))

	ncStr := fmt.Sprintf("%08x", d.nc)
	response := md5.Sum([]byte(fmt.Sprintf(
		"%s:%s:%s:%s:%s:%s",
		hex.EncodeToString(ha1[:]),
		nonce,
		ncStr,
		cnonce,
		qop,
		hex.EncodeToString(ha2[:]),
	)))

	auth := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=MD5, qop=%s, nc=%s, cnonce="%s", response="%s"`,
		d.publicKey,
		realm,
		nonce,
		uri,
		qop,
		ncStr,
		cnonce,
		hex.EncodeToString(response[:]),
	)

	d.nc++
	return auth, nil
}

// Get performs an authenticated HTTP GET request to the specified endpoint.
// It handles the digest authentication challenge-response flow automatically.
// Returns the HTTP response or an error if the request fails.
func (d *DigestClient) Get(host, endpoint string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", host, endpoint)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader == "" {
		return nil, fmt.Errorf("WWW-Authenticate header not found")
	}

	digestParams := ParseDigestHeader(authHeader)

	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	authStr, err := d.CreateDigestHeader("GET", endpoint, digestParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth header: %v", err)
	}

	req.Header.Set("Authorization", authStr)
	req.Header.Set("Accept", "application/json")

	resp, err = d.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	return resp, nil
}
